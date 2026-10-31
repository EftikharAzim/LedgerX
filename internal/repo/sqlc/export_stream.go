// Package repo contains typed database queries and streaming export support.
package repo

import "context"

// StreamExportPostings uses the generated query and row type, but consumes rows
// one at a time. A single SELECT gives every report one consistent DB snapshot.
func (q *Queries) StreamExportPostings(ctx context.Context, arg ListPostingsForExportParams, consume func(ListPostingsForExportRow) error) error {
	rows, err := q.db.Query(ctx, listPostingsForExport, arg.UserID, arg.StartDate, arg.EndDate)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row ListPostingsForExportRow
		if err := rows.Scan(&row.TransactionID, &row.AccountID, &row.AccountName, &row.CategoryID, &row.AmountMinor, &row.Currency, &row.OccurredAt, &row.Note); err != nil {
			return err
		}
		if err := consume(row); err != nil {
			return err
		}
	}
	return rows.Err()
}
