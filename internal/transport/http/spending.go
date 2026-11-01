package httptransport

import (
	"net/http"
	"strconv"
	"strings"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/reporting"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/text/currency"
)

// SpendingAPI reads corrected spending and unified transaction activity.
type SpendingAPI struct{ q *sqlc.Queries }

// NewSpendingAPI uses ownership-scoped queries for every read.
func NewSpendingAPI(q *sqlc.Queries) *SpendingAPI { return &SpendingAPI{q: q} }

// Routes registers workspace spending and activity endpoints.
func (a *SpendingAPI) Routes(r chi.Router) {
	r.Get("/spending", a.Spending)
	r.Get("/activity", a.Activity)
}

type spendingCategoryDTO struct {
	CategoryID    int64  `json:"category_id"`
	Name          string `json:"name"`
	SpendingMinor int64  `json:"spending_minor"`
	Count         int64  `json:"count"`
}

// Spending summarizes recorded income and net spending in one currency.
func (a *SpendingAPI) Spending(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(UserIDKey).(int64)
	if uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	v := r.URL.Query()
	code := v.Get("currency")
	if _, err := currency.ParseISO(code); err != nil || len(code) != 3 || code != strings.ToUpper(code) {
		http.Error(w, "choose a valid currency", http.StatusBadRequest)
		return
	}
	period, err := reporting.Parse(reporting.Selection{Type: v.Get("type"), Month: v.Get("month"), WeekOf: v.Get("week_of"), Year: v.Get("year"), StartDate: v.Get("start_date"), EndDate: v.Get("end_date")})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rows, err := a.q.GetSpendingByCategory(r.Context(), sqlc.GetSpendingByCategoryParams{
		UserID: uid, Currency: code, StartDate: pgtype.Date{Time: period.Start, Valid: period.Type != "all"}, EndDate: pgtype.Date{Time: period.End, Valid: period.Type != "all"},
	})
	if err != nil {
		http.Error(w, "could not load spending", http.StatusInternalServerError)
		return
	}
	var income, spending, count int64
	categories := make([]spendingCategoryDTO, 0, len(rows))
	for _, row := range rows {
		income += row.IncomeMinor
		spending += row.SpendingMinor
		count += row.SpendingCount
		if row.SpendingCount > 0 {
			categories = append(categories, spendingCategoryDTO{CategoryID: row.CategoryID.Int64, Name: row.CategoryName, SpendingMinor: row.SpendingMinor, Count: row.SpendingCount})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"currency": code, "income_minor": income, "spending_minor": spending, "net_minor": income - spending,
		"spending_count": count, "categories": categories, "time_zone": "UTC", "correction_policy": "exclude_reversed",
	})
}

type activityDTO struct {
	entryDTO
	AccountName             string `json:"account_name"`
	CounterpartyAccountID   int64  `json:"counterparty_account_id,omitempty"`
	CounterpartyAccountName string `json:"counterparty_account_name,omitempty"`
	ActivityType            string `json:"activity_type"`
}

// Activity filters the entire user's history before keyset pagination.
func (a *SpendingAPI) Activity(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(UserIDKey).(int64)
	if uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	v := r.URL.Query()
	params := sqlc.ListWorkspaceActivityParams{UserID: uid, CategoryFilter: -1, ActivityType: v.Get("type"), Currency: v.Get("currency"), Search: strings.TrimSpace(v.Get("search")), PageLimit: 31}
	invalid := func(message string) { http.Error(w, message, http.StatusBadRequest) }
	if len([]rune(params.Search)) > 120 {
		invalid("search must be at most 120 characters")
		return
	}
	if params.Currency != "" {
		if _, err := currency.ParseISO(params.Currency); err != nil || len(params.Currency) != 3 || params.Currency != strings.ToUpper(params.Currency) {
			invalid("choose a valid currency")
			return
		}
	}
	switch params.ActivityType {
	case "", "income", "expense", "refund", "transfer", "opening_balance", "reversal":
	default:
		invalid("unknown transaction type")
		return
	}
	if value := v.Get("account_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			invalid("invalid account")
			return
		}
		if _, err := ownedAccount(r.Context(), a.q, uid, id); err != nil {
			writeServiceError(w, err)
			return
		}
		params.AccountID = id
	}
	if value := v.Get("category_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id < 0 {
			invalid("invalid category")
			return
		}
		params.CategoryFilter = id
	}
	if value := v.Get("transaction_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			invalid("invalid transaction")
			return
		}
		if _, err := a.q.GetTransactionForUser(r.Context(), sqlc.GetTransactionForUserParams{ID: id, UserID: uid}); err != nil {
			http.Error(w, "transaction not found", http.StatusNotFound)
			return
		}
		params.TransactionFilter = id
	}
	if value := v.Get("import_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			invalid("invalid import")
			return
		}
		if _, err := a.q.GetImportSession(r.Context(), sqlc.GetImportSessionParams{ID: id, UserID: uid}); err != nil {
			http.Error(w, "import not found", http.StatusNotFound)
			return
		}
		params.ImportFilter = id
	}
	if v.Get("start_date") != "" || v.Get("end_date") != "" {
		period, err := reporting.Parse(reporting.Selection{Type: "custom", StartDate: v.Get("start_date"), EndDate: v.Get("end_date")})
		if err != nil {
			invalid(err.Error())
			return
		}
		params.StartDate, params.EndDate = pgtype.Date{Time: period.Start, Valid: true}, pgtype.Date{Time: period.End, Valid: true}
	}
	limit := 30
	if value := v.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			invalid("limit must be from 1 to 100")
			return
		}
		limit = parsed
	}
	params.PageLimit = int32(limit + 1)
	var err error
	params.CursorOccurredAt, params.CursorTransactionID, err = decodeCursor(v.Get("cursor"))
	if err != nil || params.CursorTransactionID <= 0 {
		invalid("invalid cursor")
		return
	}
	rows, err := a.q.ListWorkspaceActivity(r.Context(), params)
	if err != nil {
		http.Error(w, "could not load activity", http.StatusInternalServerError)
		return
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = encodeCursor(last.OccurredAt, last.TransactionID)
	}
	entries := make([]activityDTO, 0, len(rows))
	for _, row := range rows {
		e := entryDTO{PostingID: row.PostingID, TransactionID: row.TransactionID, AccountID: row.AccountID, AmountMinor: row.AmountMinor, Currency: row.Currency,
			OccurredAt: row.OccurredAt, Note: row.Note.String, IsTransfer: row.IsTransfer, EntryType: row.EntryType, ImportDateOnly: row.ImportDateOnly, ExcludeReport: row.ExcludeFromReports}
		if row.CategoryID.Valid {
			e.CategoryID = &row.CategoryID.Int64
		}
		if row.CorrectionOf.Valid {
			e.CorrectionOf = &row.CorrectionOf.Int64
		}
		if row.CorrectedBy.Valid {
			e.CorrectedBy = &row.CorrectedBy.Int64
		}
		if row.ImportID.Valid {
			e.ImportID = &row.ImportID.Int64
		}
		if row.SuggestedCategoryID.Valid {
			e.SuggestedCategoryID = &row.SuggestedCategoryID.Int64
		}
		if row.SuggestionScore.Valid {
			e.SuggestionScore = &row.SuggestionScore.Float64
		}
		if row.ReversalOf.Valid {
			e.ReversalOf = &row.ReversalOf.Int64
		}
		if row.ReversedBy.Valid {
			e.ReversedBy = &row.ReversedBy.Int64
		}
		entries = append(entries, activityDTO{entryDTO: e, AccountName: row.AccountName, CounterpartyAccountID: row.CounterpartyAccountID, CounterpartyAccountName: row.CounterpartyAccountName, ActivityType: row.ActivityType})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "next_cursor": next})
}
