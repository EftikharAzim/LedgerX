package httptransport

import (
	"net/http"
	"strings"

	"github.com/EftikharAzim/ledgerx/internal/reporting"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
	"golang.org/x/text/currency"
)

// MoneyManagementAPI exposes budgets and interactive financial reports.
type MoneyManagementAPI struct {
	budgets *service.BudgetService
	reports *service.FinancialReportService
}

// NewMoneyManagementAPI shares the existing financial rules.
func NewMoneyManagementAPI(b *service.BudgetService, r *service.FinancialReportService) *MoneyManagementAPI {
	return &MoneyManagementAPI{b, r}
}

// Routes registers owner-scoped report and budget workflows.
func (a *MoneyManagementAPI) Routes(r chi.Router) {
	r.Get("/reports/summary", a.Report)
	r.Get("/budgets", a.List)
	r.Put("/budgets", a.Save)
	r.Delete("/budgets/{id}", a.Delete)
	r.Post("/budgets/copy", a.Copy)
}

// Report returns a coherent single-currency period snapshot.
func (a *MoneyManagementAPI) Report(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	code := v.Get("currency")
	if _, err := currency.ParseISO(code); err != nil || len(code) != 3 || code != strings.ToUpper(code) {
		http.Error(w, "choose a valid currency", 400)
		return
	}
	period, err := reporting.Parse(reporting.Selection{Type: v.Get("type"), Month: v.Get("month"), WeekOf: v.Get("week_of"), Year: v.Get("year"), StartDate: v.Get("start_date"), EndDate: v.Get("end_date")})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out, err := a.reports.Get(r.Context(), uid, code, period)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

// List returns the requested month's limits and actual usage.
func (a *MoneyManagementAPI) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	out, err := a.budgets.List(r.Context(), uid, r.URL.Query().Get("month"), r.URL.Query().Get("currency"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

// Save creates or updates a monthly budget without changing transactions.
func (a *MoneyManagementAPI) Save(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	var in service.BudgetInput
	if err := readJSON(w, r, 4096, &in); err != nil {
		http.Error(w, "invalid budget", 400)
		return
	}
	id, err := a.budgets.Save(r.Context(), uid, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, map[string]int64{"id": id})
}

// Delete removes a budget, leaving all transactions unchanged.
func (a *MoneyManagementAPI) Delete(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	if err := a.budgets.Delete(r.Context(), uid, categoryID(r)); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"deleted": true})
}

// Copy reuses limits in another month, preserving existing limits.
func (a *MoneyManagementAPI) Copy(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	var in struct {
		From     string `json:"from_month"`
		To       string `json:"to_month"`
		Currency string `json:"currency"`
	}
	if err := readJSON(w, r, 4096, &in); err != nil {
		http.Error(w, "invalid copy", 400)
		return
	}
	n, err := a.budgets.Copy(r.Context(), uid, in.From, in.To, in.Currency)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, map[string]int64{"added": n})
}
