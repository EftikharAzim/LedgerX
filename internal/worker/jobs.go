package worker

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	TypeSnapshotAll     = "snapshot:all"
	TypeSnapshotAccount = "snapshot:account"
)
const (
	TypeExportCSV = "export:csv"
)

type SnapshotAllPayload struct {
	Date string `json:"date"` // YYYY-MM-DD UTC; empty = auto-yesterday
}
type SnapshotAccountPayload struct {
	AccountID int64  `json:"account_id"`
	Date      string `json:"date"` // YYYY-MM-DD UTC
}
type ExportCSVPayload struct {
	ExportID int64  `json:"export_id"`
	UserID   int64  `json:"user_id"`
	Month    string `json:"month"` // YYYY-MM-01
}

func MustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Errorf("marshal payload: %w", err))
	}
	return b
}

func NewTaskSnapshotAll(p SnapshotAllPayload) *asynq.Task {
	return asynq.NewTask(TypeSnapshotAll, MustJSON(p))
}
func NewTaskSnapshotAccount(p SnapshotAccountPayload) *asynq.Task {
	return asynq.NewTask(TypeSnapshotAccount, MustJSON(p))
}
