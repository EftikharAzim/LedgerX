package httptransport

import (
	"crypto/subtle"
	"net/http"
	"os"

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

// RunSnapshotAll requires ADMIN_TOKEN to be configured and presented via
// X-Admin-Token; with no token configured the endpoint is disabled.
func (a *AdminAPI) RunSnapshotAll(w http.ResponseWriter, r *http.Request) {
	want := os.Getenv("ADMIN_TOKEN")
	if want == "" {
		http.NotFound(w, r)
		return
	}
	got := r.Header.Get("X-Admin-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	payload := worker.SnapshotAllPayload{}
	if _, err := a.client.Enqueue(worker.NewTaskSnapshotAll(payload)); err != nil {
		http.Error(w, "enqueue failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("enqueued"))
}
