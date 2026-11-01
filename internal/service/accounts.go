package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/currency"
)

// Account creation errors are safe to show to the caller.
var (
	ErrAccountNameTaken = errors.New("an account with that name already exists")
	ErrInvalidAccount   = errors.New("provide an account name of 1–80 characters and a valid currency")
	ErrOpeningBalance   = errors.New("provide a valid starting balance and a date no later than now")
)

// AccountService creates an account and its starting ledger entry atomically.
type AccountService struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewAccountService uses the ledger database for account creation.
func NewAccountService(pool *pgxpool.Pool, q *sqlc.Queries) *AccountService {
	return &AccountService{pool: pool, q: q}
}

// AccountInput includes optional starting money and a retry identity.
type AccountInput struct {
	UserID         int64
	Name, Currency string
	OpeningMinor   int64
	OpeningAt      time.Time
	IdempotencyKey string
}

// Create commits the account, opening postings, outbox, and retry identity together.
func (s *AccountService) Create(ctx context.Context, in AccountInput) (sqlc.CreateAccountWithIdentityRow, error) {
	var empty sqlc.CreateAccountWithIdentityRow
	in.Name, in.Currency = strings.TrimSpace(in.Name), strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = "USD"
	}
	if _, err := currency.ParseISO(in.Currency); err != nil || len(in.Currency) != 3 || len([]rune(in.Name)) < 1 || len([]rune(in.Name)) > 80 || len(in.IdempotencyKey) > 128 {
		return empty, ErrInvalidAccount
	}
	if in.OpeningMinor > 9007199254740991 || in.OpeningMinor < -9007199254740991 ||
		(in.OpeningMinor != 0 && (in.OpeningAt.IsZero() || in.OpeningAt.Year() < 1 || in.OpeningAt.Year() > 9999 || in.OpeningAt.After(time.Now().Add(time.Minute)))) {
		return empty, ErrOpeningBalance
	}
	in.OpeningAt = in.OpeningAt.UTC()
	if in.OpeningMinor == 0 {
		in.OpeningAt = time.Time{}
	}
	payload, err := json.Marshal(struct {
		Name, Currency string
		OpeningMinor   int64
		OpeningAt      time.Time
	}{in.Name, in.Currency, in.OpeningMinor, in.OpeningAt})
	if err != nil {
		return empty, err
	}
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	if err := qtx.LockUserLedger(ctx, in.UserID); err != nil {
		return empty, err
	}
	identity := pgtype.Text{String: in.IdempotencyKey, Valid: in.IdempotencyKey != ""}
	if identity.Valid {
		if err := qtx.LockAccountCreation(ctx, fmt.Sprintf("account-create:%d:%s", in.UserID, in.IdempotencyKey)); err != nil {
			return empty, err
		}
	}
	account, err := qtx.CreateAccountWithIdentity(ctx, sqlc.CreateAccountWithIdentityParams{
		UserID: in.UserID, Name: in.Name, Currency: in.Currency,
		CreationKey: identity, CreationHash: pgtype.Text{String: hash, Valid: identity.Valid},
	})
	if errors.Is(err, pgx.ErrNoRows) && identity.Valid {
		previous, readErr := qtx.GetAccountByCreationKey(ctx, sqlc.GetAccountByCreationKeyParams{UserID: in.UserID, CreationKey: identity})
		if readErr != nil {
			return empty, readErr
		}
		if previous.CreationHash.String != hash {
			return empty, ErrIdempotencyConflict
		}
		return sqlc.CreateAccountWithIdentityRow{ID: previous.ID, UserID: previous.UserID, Name: previous.Name, Currency: previous.Currency, IsActive: previous.IsActive, Kind: previous.Kind, CreatedAt: previous.CreatedAt}, nil
	}
	if err != nil {
		if isUniqueViolation(err) {
			return empty, ErrAccountNameTaken
		}
		return empty, err
	}
	if in.OpeningMinor != 0 {
		external, err := qtx.EnsureExternalAccount(ctx, sqlc.EnsureExternalAccountParams{UserID: in.UserID, Currency: in.Currency})
		if err != nil {
			return empty, err
		}
		if _, err := appendLedger(ctx, qtx, ledgerWrite{UserID: in.UserID, Currency: in.Currency, At: in.OpeningAt, Note: "Starting balance", EntryType: "opening_balance", Legs: []Leg{{AccountID: account.ID, AmountMinor: in.OpeningMinor}, {AccountID: external, AmountMinor: -in.OpeningMinor}}}); err != nil {
			return empty, err
		}
		if err := qtx.SetTrackingStart(ctx, sqlc.SetTrackingStartParams{AccountID: account.ID, TrackingStartAt: pgtype.Timestamptz{Time: in.OpeningAt, Valid: true}}); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return account, nil
}
