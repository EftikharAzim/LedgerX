package httptransport

import (
	"net/http"

	"github.com/EftikharAzim/ledgerx/internal/worker"
	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
)

type AdminAPI struct{ client *asynq.Client }

func NewAdminAPI(redisAddr string) *AdminAPI {
	return &AdminAPI{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}
func (a *AdminAPI) Routes(r chi.Router) {
	r.Post("/admin/snapshot/run", a.RunSnapshotAll)
}

func (a *AdminAPI) RunSnapshotAll(w http.ResponseWriter, r *http.Request) {
	// For demo: run yesterday
	payload := worker.SnapshotAllPayload{}
	_, _ = a.client.Enqueue(worker.NewTaskSnapshotAll(payload))
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("enqueued"))
}
