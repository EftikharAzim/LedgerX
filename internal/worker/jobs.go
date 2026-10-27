package worker

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	// TypeSnapshotAll identifies the account snapshot fan-out job.
	TypeSnapshotAll = "snapshot:all"
	// TypeSnapshotAccount identifies a snapshot job for one account and date.
	TypeSnapshotAccount = "snapshot:account"
)
const (
	// TypeExportCSV identifies a monthly CSV export job.
	TypeExportCSV = "export:csv"
)
const (
	// TypeCategorizeTx identifies an advisory transaction categorization job.
	TypeCategorizeTx = "categorize:transaction"
)

// SnapshotAllPayload selects the UTC date for a snapshot fan-out.
type SnapshotAllPayload struct {
	Date string `json:"date"` // YYYY-MM-DD UTC; empty = auto-yesterday
}

// SnapshotAccountPayload selects one account and UTC snapshot date.
type SnapshotAccountPayload struct {
	AccountID int64  `json:"account_id"`
	Date      string `json:"date"` // YYYY-MM-DD UTC
}

// ExportCSVPayload identifies a user's monthly export request.
type ExportCSVPayload struct {
	ExportID int64  `json:"export_id"`
	UserID   int64  `json:"user_id"`
	Month    string `json:"month"` // YYYY-MM-01
}

// CategorizeTxPayload identifies an owned transaction to classify.
type CategorizeTxPayload struct {
	TransactionID int64 `json:"transaction_id"`
	UserID        int64 `json:"user_id"`
}

// MustJSON encodes a task payload and panics if the value cannot be serialized.
func MustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Errorf("marshal payload: %w", err))
	}
	return b
}

// NewTaskSnapshotAll creates an account snapshot fan-out task.
func NewTaskSnapshotAll(p SnapshotAllPayload) *asynq.Task {
	return asynq.NewTask(TypeSnapshotAll, MustJSON(p))
}

// NewTaskSnapshotAccount creates a snapshot task for one account.
func NewTaskSnapshotAccount(p SnapshotAccountPayload) *asynq.Task {
	return asynq.NewTask(TypeSnapshotAccount, MustJSON(p))
}
