package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type ledgerWrite struct {
	UserID     int64
	Currency   string
	At         time.Time
	Note       string
	CategoryID int64
	EntryType  string
	ReversalOf int64
	ImportID   int64
	Legs       []Leg
}

// appendLedger writes one balanced transaction and its outbox event. Callers
// resolve ownership/currency and hold LockUserLedger in their DB transaction.
func appendLedger(ctx context.Context, q *sqlc.Queries, in ledgerWrite) (TransactionDTO, error) {
	if len(in.Legs) < 2 {
		return TransactionDTO{}, ErrUnbalanced
	}
	if err := validateLegs(in.Legs, false); err != nil {
		return TransactionDTO{}, err
	}
	header, err := q.CreateTransactionHeader(ctx, sqlc.CreateTransactionHeaderParams{
		UserID: in.UserID, Currency: in.Currency, OccurredAt: in.At,
		Note: pgtype.Text{String: in.Note, Valid: in.Note != ""}, CategoryID: pgtype.Int8{Int64: in.CategoryID, Valid: in.CategoryID != 0},
		EntryType: in.EntryType, ReversalOf: pgtype.Int8{Int64: in.ReversalOf, Valid: in.ReversalOf != 0},
	})
	if err != nil {
		return TransactionDTO{}, err
	}
	dto := TransactionDTO{ID: header.ID, UserID: header.UserID, Currency: header.Currency, OccurredAt: header.OccurredAt, Note: in.Note, CategoryID: in.CategoryID, EntryType: headerEntryType(in.EntryType), CreatedAt: header.CreatedAt, ReversalOf: in.ReversalOf}
	for _, leg := range in.Legs {
		p, err := q.CreatePosting(ctx, sqlc.CreatePostingParams{TransactionID: header.ID, AccountID: leg.AccountID, AmountMinor: leg.AmountMinor})
		if err != nil {
			return TransactionDTO{}, err
		}
		dto.Postings = append(dto.Postings, PostingDTO{ID: p.ID, AccountID: p.AccountID, AmountMinor: p.AmountMinor})
	}
	payload := map[string]any{"transaction_id": dto.ID, "user_id": dto.UserID, "currency": dto.Currency, "occurred_at": dto.OccurredAt, "entry_type": dto.EntryType, "postings": dto.Postings}
	eventType := "TransactionCreated"
	if in.ReversalOf != 0 {
		payload["reversal_of"] = in.ReversalOf
		eventType = "TransactionReversed"
	}
	if in.ImportID != 0 {
		payload["import_id"] = in.ImportID
	}
	event, err := json.Marshal(payload)
	if err != nil {
		return TransactionDTO{}, err
	}
	if _, err := q.CreateOutboxEvent(ctx, sqlc.CreateOutboxEventParams{EventType: eventType, Payload: event}); err != nil {
		return TransactionDTO{}, err
	}
	return dto, nil
}

func appendReversal(ctx context.Context, q *sqlc.Queries, uid, id int64) (TransactionDTO, time.Time, []Leg, error) {
	orig, err := q.GetTransactionForUser(ctx, sqlc.GetTransactionForUserParams{ID: id, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return TransactionDTO{}, time.Time{}, nil, ErrTxNotFound
	}
	if err != nil {
		return TransactionDTO{}, time.Time{}, nil, err
	}
	if orig.ReversalOf.Valid {
		return TransactionDTO{}, time.Time{}, nil, ErrIsReversal
	}
	postings, err := q.ListPostingsByTransaction(ctx, orig.ID)
	if err != nil {
		return TransactionDTO{}, time.Time{}, nil, err
	}
	if len(postings) == 0 {
		return TransactionDTO{}, time.Time{}, nil, ErrTxNotFound
	}
	legs := make([]Leg, 0, len(postings))
	for _, p := range postings {
		legs = append(legs, Leg{AccountID: p.AccountID, AmountMinor: -p.AmountMinor})
	}
	dto, err := appendLedger(ctx, q, ledgerWrite{UserID: uid, Currency: orig.Currency, At: time.Now().UTC(), Note: fmt.Sprintf("reversal of transaction %d", orig.ID), EntryType: orig.EntryType, ReversalOf: orig.ID, Legs: legs})
	if isUniqueViolation(err) {
		return TransactionDTO{}, time.Time{}, nil, ErrAlreadyReversed
	}
	if err != nil {
		return TransactionDTO{}, time.Time{}, nil, err
	}
	if orig.EntryType == "opening_balance" {
		for _, p := range postings {
			account, err := q.GetAccount(ctx, p.AccountID)
			if err != nil {
				return TransactionDTO{}, time.Time{}, nil, err
			}
			if account.Kind == "normal" {
				if err := q.SetTrackingStart(ctx, sqlc.SetTrackingStartParams{AccountID: account.ID}); err != nil {
					return TransactionDTO{}, time.Time{}, nil, err
				}
			}
		}
	}
	return dto, orig.OccurredAt, legs, nil
}
