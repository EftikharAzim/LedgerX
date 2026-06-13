package httptransport

import (
	"context"
	"errors"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/jackc/pgx/v5"
)

// ownedAccount loads an account and verifies it belongs to uid. Returns
// service sentinel errors so callers can reuse writeServiceError.
func ownedAccount(ctx context.Context, q *sqlc.Queries, uid, accountID int64) (sqlc.GetAccountRow, error) {
	acc, err := q.GetAccount(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.GetAccountRow{}, service.ErrAccountNotFound
	}
	if err != nil {
		return sqlc.GetAccountRow{}, err
	}
	if acc.UserID != uid {
		// Hide other users' account existence: report not-found, not forbidden.
		return sqlc.GetAccountRow{}, service.ErrAccountNotFound
	}
	return acc, nil
}
