package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/EftikharAzim/ledgerx/internal/importing"
	"github.com/jackc/pgx/v5"
)

// ImportCopyRow is already normalized, ownership-checked, and user-reviewed.
type ImportCopyRow struct {
	Row        importing.Row
	CategoryID int64
}

// CopyImport writes a reviewed history through PostgreSQL COPY in the caller's
// transaction. Deferred balance checks, foreign keys, and rollback still apply.
// No optional AI tasks are enqueued, and future plans create no postings.
func (q *Queries) CopyImport(ctx context.Context, uid, account, external, importID int64, currency string, input []ImportCopyRow) ([]int64, []int64, error) {
	tx, ok := q.db.(pgx.Tx)
	if !ok {
		return nil, nil, fmt.Errorf("history import requires a database transaction")
	}
	posted, planned := []ImportCopyRow{}, []ImportCopyRow{}
	for _, row := range input {
		if row.Row.Planned {
			planned = append(planned, row)
		} else {
			posted = append(posted, row)
		}
	}
	allocate := func(query string, count int) ([][]int64, error) {
		if count == 0 {
			return nil, nil
		}
		rows, err := tx.Query(ctx, query, count)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, func(r pgx.CollectableRow) ([]int64, error) {
			values, err := r.Values()
			if err != nil {
				return nil, err
			}
			out := make([]int64, len(values))
			for i, v := range values {
				n, ok := v.(int64)
				if !ok {
					return nil, fmt.Errorf("invalid allocated ledger identity")
				}
				out[i] = n
			}
			return out, nil
		})
	}
	ids, err := allocate("SELECT nextval('transactions_id_seq'),nextval('postings_id_seq'),nextval('postings_id_seq') FROM generate_series(1,$1::integer)", len(posted))
	if err != nil {
		return nil, nil, err
	}
	planIDs, err := allocate("SELECT nextval('planned_transactions_id_seq') FROM generate_series(1,$1::integer)", len(planned))
	if err != nil {
		return nil, nil, err
	}
	copyRows := func(table string, columns []string, rows [][]any) error {
		if len(rows) == 0 {
			return nil
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{table}, columns, pgx.CopyFromRows(rows))
		if err == nil && n != int64(len(rows)) {
			return fmt.Errorf("incomplete history copy")
		}
		return err
	}
	nullID := func(id int64) any {
		if id == 0 {
			return nil
		}
		return id
	}
	headers, postings, origins, events, plans := [][]any{}, [][]any{}, [][]any{}, [][]any{}, [][]any{}
	transactionIDs, pendingIDs := []int64{}, []int64{}
	for i, in := range posted {
		r := in.Row
		id := ids[i][0]
		transactionIDs = append(transactionIDs, id)
		headers = append(headers, []any{id, uid, currency, nullID(in.CategoryID), r.OccurredAt, r.Note, r.EntryType, r.ExcludeReport})
		postings = append(postings, []any{ids[i][1], id, account, r.AmountMinor}, []any{ids[i][2], id, external, -r.AmountMinor})
		// Source line numbers are bounded by the 16 MiB parser, below int32.
		origins = append(origins, []any{uid, account, importID, id, int32(r.Number), r.Fingerprint, r.SourceRef, r.DateOnly}) // #nosec G115 -- bounded parser source line.
		payload, err := json.Marshal(map[string]any{"transaction_id": id, "user_id": uid, "currency": currency, "occurred_at": r.OccurredAt, "entry_type": r.EntryType, "exclude_from_reports": r.ExcludeReport, "import_id": importID, "postings": []map[string]any{{"id": ids[i][1], "account_id": account, "amount_minor": r.AmountMinor}, {"id": ids[i][2], "account_id": external, "amount_minor": -r.AmountMinor}}})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, []any{"TransactionCreated", payload})
	}
	for i, in := range planned {
		r := in.Row
		id := planIDs[i][0]
		pendingIDs = append(pendingIDs, id)
		plans = append(plans, []any{id, uid, account, currency, r.AmountMinor, nullID(in.CategoryID), r.OccurredAt, r.Note, r.EntryType, r.DateOnly, r.ExcludeReport, importID, int32(r.Number), r.Fingerprint, r.SourceRef}) // #nosec G115 -- bounded parser source line.
	}
	for _, part := range []struct {
		table   string
		columns []string
		rows    [][]any
	}{
		{"transactions", []string{"id", "user_id", "currency", "category_id", "occurred_at", "note", "entry_type", "exclude_from_reports"}, headers},
		{"postings", []string{"id", "transaction_id", "account_id", "amount_minor"}, postings},
		{"imported_transactions", []string{"user_id", "account_id", "import_id", "transaction_id", "row_number", "fingerprint", "source_ref", "date_only"}, origins},
		{"outbox", []string{"event_type", "payload"}, events},
		{"planned_transactions", []string{"id", "user_id", "account_id", "currency", "amount_minor", "category_id", "occurred_at", "note", "entry_type", "date_only", "exclude_from_reports", "import_id", "row_number", "fingerprint", "source_ref"}, plans},
	} {
		if err := copyRows(part.table, part.columns, part.rows); err != nil {
			return nil, nil, err
		}
	}
	return transactionIDs, pendingIDs, nil
}
