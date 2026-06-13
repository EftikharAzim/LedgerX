package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
)

// Sentinel errors the transport layer maps to HTTP status codes.
var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrForbidden           = errors.New("account does not belong to user")
	ErrAccountInactive     = errors.New("account is inactive")
	ErrCurrencyMismatch    = errors.New("transaction currency does not match account currency")
	ErrUnbalanced          = errors.New("postings must sum to zero")
	ErrZeroAmount          = errors.New("posting amount must be non-zero")
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different payload")
	ErrIdempotencyInFlight = errors.New("a request with this idempotency key is already in progress")
	ErrTxNotFound          = errors.New("transaction not found")
	ErrAlreadyReversed     = errors.New("transaction has already been reversed")
	ErrIsReversal          = errors.New("a reversal cannot itself be reversed")
)

type TransactionService struct {
	pool       *pgxpool.Pool
	q          *sqlc.Queries
	summarySvc *SummaryService
}

func NewTransactionService(pool *pgxpool.Pool, q *sqlc.Queries, summarySvc *SummaryService) *TransactionService {
	return &TransactionService{pool: pool, q: q, summarySvc: summarySvc}
}

// Leg is one side of a balanced transaction.
type Leg struct {
	AccountID   int64 `json:"account_id"`
	AmountMinor int64 `json:"amount_minor"`
}

type PostingDTO struct {
	ID          int64 `json:"id"`
	AccountID   int64 `json:"account_id"`
	AmountMinor int64 `json:"amount_minor"`
}

type TransactionDTO struct {
	ID         int64        `json:"id"`
	UserID     int64        `json:"user_id"`
	Currency   string       `json:"currency"`
	OccurredAt time.Time    `json:"occurred_at"`
	Note       string       `json:"note,omitempty"`
	ReversalOf int64        `json:"reversal_of,omitempty"`
	Postings   []PostingDTO `json:"postings"`
	CreatedAt  time.Time    `json:"created_at"`
}

type CreateInput struct {
	UserID         int64
	AccountID      int64
	AmountMinor    int64 // positive = income into the account, negative = expense
	Currency       string
	OccurredAt     time.Time
	Note           string
	IdempotencyKey string
}

type TransferInput struct {
	UserID         int64
	FromAccountID  int64
	ToAccountID    int64
	AmountMinor    int64 // must be positive
	Currency       string
	OccurredAt     time.Time
	Note           string
	IdempotencyKey string
}

// Create records an income/expense against one account. The offsetting leg
// goes to the user's external account (the outside world), keeping the
// ledger balanced.
func (s *TransactionService) Create(ctx context.Context, in CreateInput) (TransactionDTO, error) {
	if in.AmountMinor == 0 {
		return TransactionDTO{}, ErrZeroAmount
	}
	legs := []Leg{{AccountID: in.AccountID, AmountMinor: in.AmountMinor}}
	// externalLegAmount: the external account absorbs the opposite of the sum
	// of the explicit legs; resolved inside the DB transaction.
	return s.createBalanced(ctx, balancedInput{
		UserID:         in.UserID,
		Legs:           legs,
		UseExternalLeg: true,
		Currency:       in.Currency,
		OccurredAt:     in.OccurredAt,
		Note:           in.Note,
		IdempotencyKey: in.IdempotencyKey,
	})
}

// Transfer moves money between two of the user's accounts.
func (s *TransactionService) Transfer(ctx context.Context, in TransferInput) (TransactionDTO, error) {
	if in.AmountMinor <= 0 {
		return TransactionDTO{}, fmt.Errorf("%w: transfer amount must be positive", ErrZeroAmount)
	}
	if in.FromAccountID == in.ToAccountID {
		return TransactionDTO{}, fmt.Errorf("%w: cannot transfer to the same account", ErrUnbalanced)
	}
	legs := []Leg{
		{AccountID: in.FromAccountID, AmountMinor: -in.AmountMinor},
		{AccountID: in.ToAccountID, AmountMinor: in.AmountMinor},
	}
	return s.createBalanced(ctx, balancedInput{
		UserID:         in.UserID,
		Legs:           legs,
		Currency:       in.Currency,
		OccurredAt:     in.OccurredAt,
		Note:           in.Note,
		IdempotencyKey: in.IdempotencyKey,
	})
}

type balancedInput struct {
	UserID         int64
	Legs           []Leg
	UseExternalLeg bool
	Currency       string
	OccurredAt     time.Time
	Note           string
	IdempotencyKey string
}

// validateLegs enforces the double-entry invariant before touching the DB.
// The DB-level deferred trigger is the backstop; this gives clean errors.
func validateLegs(legs []Leg, useExternalLeg bool) error {
	var sum int64
	for _, l := range legs {
		if l.AmountMinor == 0 {
			return ErrZeroAmount
		}
		sum += l.AmountMinor
	}
	if !useExternalLeg && sum != 0 {
		return ErrUnbalanced
	}
	return nil
}

// requestHash canonicalizes the request for idempotency comparison. Struct
// field order makes the JSON deterministic.
func requestHash(in balancedInput) string {
	payload := struct {
		UserID     int64     `json:"user_id"`
		Legs       []Leg     `json:"legs"`
		External   bool      `json:"external"`
		Currency   string    `json:"currency"`
		OccurredAt time.Time `json:"occurred_at"`
		Note       string    `json:"note"`
	}{in.UserID, in.Legs, in.UseExternalLeg, in.Currency, in.OccurredAt.UTC(), in.Note}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *TransactionService) createBalanced(ctx context.Context, in balancedInput) (TransactionDTO, error) {
	if err := validateLegs(in.Legs, in.UseExternalLeg); err != nil {
		return TransactionDTO{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TransactionDTO{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	// 1) Claim the idempotency key inside the transaction: a crash before
	// commit rolls the claim back, so retries can never observe a claimed
	// key without its committed transaction.
	hash := requestHash(in)
	cached, err := claimIdempotency(ctx, qtx, in.UserID, in.IdempotencyKey, hash)
	if err != nil {
		return TransactionDTO{}, err
	}
	if cached != nil {
		return *cached, nil
	}

	// 2) Resolve the external leg if requested.
	legs := in.Legs
	if in.UseExternalLeg {
		var sum int64
		for _, l := range legs {
			sum += l.AmountMinor
		}
		var extID int64
		ext, err := qtx.GetExternalAccount(ctx, in.UserID)
		switch {
		case err == nil:
			extID = ext.ID
		case errors.Is(err, pgx.ErrNoRows):
			// Users created before the double-entry migration get one lazily.
			created, err := qtx.CreateExternalAccount(ctx, sqlc.CreateExternalAccountParams{
				UserID: in.UserID, Currency: in.Currency,
			})
			if err != nil {
				return TransactionDTO{}, err
			}
			extID = created.ID
		default:
			return TransactionDTO{}, err
		}
		legs = append(legs, Leg{AccountID: extID, AmountMinor: -sum})
	}

	// 3) Ownership / state / currency checks on every leg.
	for _, l := range legs {
		acc, err := qtx.GetAccount(ctx, l.AccountID)
		if errors.Is(err, pgx.ErrNoRows) {
			return TransactionDTO{}, ErrAccountNotFound
		}
		if err != nil {
			return TransactionDTO{}, err
		}
		if acc.UserID != in.UserID {
			return TransactionDTO{}, ErrForbidden
		}
		if !acc.IsActive {
			return TransactionDTO{}, ErrAccountInactive
		}
		// The external account is currency-agnostic by design; real accounts
		// must match the transaction currency.
		if acc.Kind != "external" && acc.Currency != in.Currency {
			return TransactionDTO{}, ErrCurrencyMismatch
		}
	}

	// 4) Header + postings.
	header, err := qtx.CreateTransactionHeader(ctx, sqlc.CreateTransactionHeaderParams{
		UserID:     in.UserID,
		Currency:   in.Currency,
		OccurredAt: in.OccurredAt,
		Note:       pgtype.Text{String: in.Note, Valid: in.Note != ""},
	})
	if err != nil {
		return TransactionDTO{}, err
	}
	dto := TransactionDTO{
		ID:         header.ID,
		UserID:     header.UserID,
		Currency:   header.Currency,
		OccurredAt: header.OccurredAt,
		Note:       in.Note,
		CreatedAt:  header.CreatedAt,
	}
	for _, l := range legs {
		p, err := qtx.CreatePosting(ctx, sqlc.CreatePostingParams{
			TransactionID: header.ID, AccountID: l.AccountID, AmountMinor: l.AmountMinor,
		})
		if err != nil {
			return TransactionDTO{}, err
		}
		dto.Postings = append(dto.Postings, PostingDTO{ID: p.ID, AccountID: p.AccountID, AmountMinor: p.AmountMinor})
	}

	// 5) Outbox event in the same transaction — atomic with the ledger write.
	event, err := json.Marshal(map[string]any{
		"transaction_id": header.ID,
		"user_id":        header.UserID,
		"currency":       header.Currency,
		"occurred_at":    header.OccurredAt,
		"postings":       dto.Postings,
	})
	if err != nil {
		return TransactionDTO{}, err
	}
	if _, err := qtx.CreateOutboxEvent(ctx, sqlc.CreateOutboxEventParams{
		EventType: "TransactionCreated", Payload: event,
	}); err != nil {
		return TransactionDTO{}, err
	}

	// 6) Cache the response for idempotent replays, then commit.
	if in.IdempotencyKey != "" {
		resp, err := json.Marshal(dto)
		if err != nil {
			return TransactionDTO{}, err
		}
		if err := qtx.MarkIdempotencySuccess(ctx, sqlc.MarkIdempotencySuccessParams{
			UserID: in.UserID, Key: in.IdempotencyKey, RequestHash: hash,
			ResponseJson: pgtype.Text{String: string(resp), Valid: true},
		}); err != nil {
			return TransactionDTO{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return TransactionDTO{}, err
	}

	// 7) Best-effort cache invalidation after commit.
	month := time.Date(in.OccurredAt.Year(), in.OccurredAt.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, l := range legs {
		s.summarySvc.Invalidate(ctx, l.AccountID, month)
	}
	return dto, nil
}

// claimIdempotency claims key inside the caller's DB transaction. Returns a
// non-nil DTO when this is a replay of an already-committed request; the
// caller should return it as-is. A nil key is a no-op.
func claimIdempotency(ctx context.Context, qtx *sqlc.Queries, userID int64, key, hash string) (*TransactionDTO, error) {
	if key == "" {
		return nil, nil
	}
	_, err := qtx.InsertIdempotencyKey(ctx, sqlc.InsertIdempotencyKeyParams{
		UserID: userID, Key: key, RequestHash: hash,
	})
	if err == nil {
		return nil, nil // freshly claimed
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	// Key exists: replay, conflict, or in flight.
	rec, err := qtx.GetIdempotency(ctx, sqlc.GetIdempotencyParams{UserID: userID, Key: key})
	if err != nil {
		return nil, err
	}
	if rec.RequestHash != hash {
		return nil, ErrIdempotencyConflict
	}
	if rec.Status == "succeeded" && rec.ResponseJson.Valid {
		var dto TransactionDTO
		if err := json.Unmarshal([]byte(rec.ResponseJson.String), &dto); err != nil {
			return nil, err
		}
		return &dto, nil
	}
	return nil, ErrIdempotencyInFlight
}

type ReverseInput struct {
	UserID         int64
	TransactionID  int64
	IdempotencyKey string
}

// Reverse corrects a transaction the append-only way: a new transaction
// whose postings negate the original's, linked via reversal_of. The UNIQUE
// constraint on reversal_of makes double-reversal impossible at the DB level.
func (s *TransactionService) Reverse(ctx context.Context, in ReverseInput) (TransactionDTO, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TransactionDTO{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	hash := reverseHash(in)
	cached, err := claimIdempotency(ctx, qtx, in.UserID, in.IdempotencyKey, hash)
	if err != nil {
		return TransactionDTO{}, err
	}
	if cached != nil {
		return *cached, nil
	}

	orig, err := qtx.GetTransactionForUser(ctx, sqlc.GetTransactionForUserParams{
		ID: in.TransactionID, UserID: in.UserID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return TransactionDTO{}, ErrTxNotFound
	}
	if err != nil {
		return TransactionDTO{}, err
	}
	if orig.ReversalOf.Valid {
		return TransactionDTO{}, ErrIsReversal
	}

	origPostings, err := qtx.ListPostingsByTransaction(ctx, orig.ID)
	if err != nil {
		return TransactionDTO{}, err
	}
	if len(origPostings) == 0 {
		// Legacy zero-amount headers carry no postings; nothing to reverse.
		return TransactionDTO{}, ErrTxNotFound
	}

	now := time.Now().UTC()
	note := fmt.Sprintf("reversal of transaction %d", orig.ID)
	header, err := qtx.CreateTransactionHeader(ctx, sqlc.CreateTransactionHeaderParams{
		UserID:     in.UserID,
		Currency:   orig.Currency,
		OccurredAt: now,
		Note:       pgtype.Text{String: note, Valid: true},
		ReversalOf: pgtype.Int8{Int64: orig.ID, Valid: true},
	})
	if err != nil {
		if isUniqueViolation(err) {
			return TransactionDTO{}, ErrAlreadyReversed
		}
		return TransactionDTO{}, err
	}

	dto := TransactionDTO{
		ID:         header.ID,
		UserID:     header.UserID,
		Currency:   header.Currency,
		OccurredAt: header.OccurredAt,
		Note:       note,
		ReversalOf: orig.ID,
		CreatedAt:  header.CreatedAt,
	}
	for _, p := range origPostings {
		np, err := qtx.CreatePosting(ctx, sqlc.CreatePostingParams{
			TransactionID: header.ID, AccountID: p.AccountID, AmountMinor: -p.AmountMinor,
		})
		if err != nil {
			return TransactionDTO{}, err
		}
		dto.Postings = append(dto.Postings, PostingDTO{ID: np.ID, AccountID: np.AccountID, AmountMinor: np.AmountMinor})
	}

	event, err := json.Marshal(map[string]any{
		"transaction_id": header.ID,
		"reversal_of":    orig.ID,
		"user_id":        header.UserID,
		"currency":       header.Currency,
		"occurred_at":    header.OccurredAt,
		"postings":       dto.Postings,
	})
	if err != nil {
		return TransactionDTO{}, err
	}
	if _, err := qtx.CreateOutboxEvent(ctx, sqlc.CreateOutboxEventParams{
		EventType: "TransactionReversed", Payload: event,
	}); err != nil {
		return TransactionDTO{}, err
	}

	if in.IdempotencyKey != "" {
		resp, err := json.Marshal(dto)
		if err != nil {
			return TransactionDTO{}, err
		}
		if err := qtx.MarkIdempotencySuccess(ctx, sqlc.MarkIdempotencySuccessParams{
			UserID: in.UserID, Key: in.IdempotencyKey, RequestHash: hash,
			ResponseJson: pgtype.Text{String: string(resp), Valid: true},
		}); err != nil {
			return TransactionDTO{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return TransactionDTO{}, err
	}

	// Invalidate both affected months (original's and the reversal's).
	for _, p := range origPostings {
		for _, at := range []time.Time{orig.OccurredAt, now} {
			month := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
			s.summarySvc.Invalidate(ctx, p.AccountID, month)
		}
	}
	return dto, nil
}

func reverseHash(in ReverseInput) string {
	payload := struct {
		UserID        int64  `json:"user_id"`
		Op            string `json:"op"`
		TransactionID int64  `json:"transaction_id"`
	}{in.UserID, "reverse", in.TransactionID}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
