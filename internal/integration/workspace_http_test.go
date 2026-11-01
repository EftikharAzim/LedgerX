package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/service"
	httptransport "github.com/EftikharAzim/ledgerx/internal/transport/http"
	"github.com/EftikharAzim/ledgerx/internal/workspace"
	"github.com/go-chi/chi/v5"
)

func workspaceRouter(uid int64) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), httptransport.UserIDKey, uid)))
		})
	})
	httptransport.NewWorkspaceAPI(q, service.NewWorkspaceService(pool, q)).Routes(r)
	httptransport.NewCorrectionsAPI(service.NewCorrectionService(pool, q, sumSvc)).Routes(r)
	httptransport.NewSpendingAPI(q).Routes(r)
	return r
}

func workspaceRequest(t *testing.T, uid int64, method, path, body, key string, status int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	workspaceRouter(uid).ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s: %d %s; want %d", method, path, w.Code, w.Body.String(), status)
	}
	if uid != 0 && w.Header().Get("Cache-Control") != "no-store" && !strings.HasPrefix(path, "/activity") {
		t.Fatal("private recovery response is cacheable")
	}
	return w
}

func TestWorkspaceHTTPAuthIsolationAndExpiry(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	other, _ := newUserWithAccount(t, "USD")
	original, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: 123, Currency: "USD", OccurredAt: time.Now().UTC(), Note: "Only in source workspace"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/workspace", "/workspace/backup", fmt.Sprintf("/transactions/%d/edit-context", original.ID)} {
		workspaceRequest(t, 0, http.MethodGet, path, "", "", http.StatusUnauthorized)
	}
	workspaceRequest(t, other, http.MethodGet, fmt.Sprintf("/transactions/%d/edit-context", original.ID), "", "", http.StatusNotFound)
	workspaceRequest(t, other, http.MethodGet, fmt.Sprintf("/activity?transaction_id=%d", original.ID), "", "", http.StatusNotFound)
	exported := workspaceRequest(t, uid, http.MethodGet, "/workspace/backup", "", "", http.StatusOK)
	if !strings.Contains(exported.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("backup is not a download")
	}
	var backup workspace.Backup
	if err := json.Unmarshal(exported.Body.Bytes(), &backup); err != nil {
		t.Fatal(err)
	}
	if _, err := backup.Validate(); err != nil {
		t.Fatal(err)
	}
	foreign := workspaceRequest(t, other, http.MethodGet, "/workspace/backup", "", "", http.StatusOK)
	if strings.Contains(foreign.Body.String(), "Only in source workspace") {
		t.Fatal("foreign ledger leaked")
	}
	preview := workspaceRequest(t, uid, http.MethodPost, "/workspace/restores/preview?filename=test.json", exported.Body.String(), "", http.StatusCreated)
	var view service.RestoreView
	if err := json.Unmarshal(preview.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(preview.Body.String(), "Only in source workspace") || strings.Contains(preview.Body.String(), `"payload"`) {
		t.Fatal("restore review exposed full financial payload")
	}
	workspaceRequest(t, other, http.MethodGet, fmt.Sprintf("/workspace/restores/%d", view.ID), "", "", http.StatusNotFound)
	workspaceRequest(t, uid, http.MethodPost, fmt.Sprintf("/workspace/restores/%d/commit", view.ID), fmt.Sprintf(`{"checksum":%q}`, view.Checksum), "", http.StatusConflict)
	if _, err := pool.Exec(ctx, `UPDATE workspace_restores SET expires_at=now()-interval '1 second' WHERE id=$1`, view.ID); err != nil {
		t.Fatal(err)
	}
	workspaceRequest(t, uid, http.MethodGet, fmt.Sprintf("/workspace/restores/%d", view.ID), "", "", http.StatusGone)
	workspaceRequest(t, uid, http.MethodPost, fmt.Sprintf("/workspace/restores/%d/commit", view.ID), fmt.Sprintf(`{"checksum":%q}`, view.Checksum), "", http.StatusGone)
	workspaceRequest(t, uid, http.MethodPost, "/workspace/restores/preview", `{"format":"unknown"}`, "", http.StatusUnprocessableEntity)
}

func TestCorrectionHTTPPreviewCommitLinksAndResultPrivacy(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	other, _ := newUserWithAccount(t, "USD")
	original, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: -100, Currency: "USD", OccurredAt: time.Now().UTC(), Note: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	contextResponse := workspaceRequest(t, uid, http.MethodGet, fmt.Sprintf("/transactions/%d/edit-context", original.ID), "", "", http.StatusOK)
	var edit service.CorrectionContext
	if err := json.Unmarshal(contextResponse.Body.Bytes(), &edit); err != nil {
		t.Fatal(err)
	}
	edit.Draft.AmountMinor = -200
	input := service.CorrectionInput{Draft: edit.Draft, SourceToken: edit.SourceToken}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	preview := workspaceRequest(t, uid, http.MethodPost, fmt.Sprintf("/transactions/%d/correction-preview", original.ID), string(raw), "", http.StatusOK)
	var plan service.CorrectionPlan
	if err := json.Unmarshal(preview.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	input.PreviewToken = plan.PreviewToken
	raw, err = json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	workspaceRequest(t, uid, http.MethodPost, fmt.Sprintf("/transactions/%d/corrections", original.ID), string(raw), "", http.StatusUnprocessableEntity)
	saved := workspaceRequest(t, uid, http.MethodPost, fmt.Sprintf("/transactions/%d/corrections", original.ID), string(raw), "http-edit", http.StatusCreated)
	result := workspaceRequest(t, uid, http.MethodGet, fmt.Sprintf("/transactions/%d/corrections/result", original.ID), "", "http-edit", http.StatusOK)
	if result.Body.String() != saved.Body.String() {
		t.Fatal("lost-response recovery receipt changed")
	}
	workspaceRequest(t, other, http.MethodGet, fmt.Sprintf("/transactions/%d/corrections/result", original.ID), "", "http-edit", http.StatusNotFound)
	workspaceRequest(t, uid, http.MethodGet, fmt.Sprintf("/transactions/%d/corrections/result", original.ID+999999), "", "http-edit", http.StatusNotFound)
	var receipt service.CorrectionReceipt
	if err := json.Unmarshal(saved.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	var activity struct {
		Entries []struct {
			CorrectionOf int64 `json:"correction_of"`
			CorrectedBy  int64 `json:"corrected_by"`
		} `json:"entries"`
	}
	getWorkspace(t, workspaceRouter(uid), fmt.Sprintf("/activity?transaction_id=%d", receipt.ReplacementID), &activity)
	if len(activity.Entries) != 1 || activity.Entries[0].CorrectionOf != original.ID {
		t.Fatal(activity)
	}
	getWorkspace(t, workspaceRouter(uid), fmt.Sprintf("/activity?transaction_id=%d", original.ID), &activity)
	if len(activity.Entries) != 1 || activity.Entries[0].CorrectedBy != receipt.ReplacementID {
		t.Fatal(activity)
	}
}
