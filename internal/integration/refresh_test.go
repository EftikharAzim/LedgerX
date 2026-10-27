package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	httptransport "github.com/EftikharAzim/ledgerx/internal/transport/http"
)

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func postJSON(t *testing.T, h http.HandlerFunc, body any) (*httptest.ResponseRecorder, tokenResp) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	h(rec, req)
	var tr tokenResp
	_ = json.Unmarshal(rec.Body.Bytes(), &tr)
	return rec, tr
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	ctx := context.Background()
	api := httptransport.NewAuthAPI(pool, time.Hour)

	// A user with a known password (login goes through the real handler).
	userSeq++
	email := fmt.Sprintf("refresh-%d-%d@example.com", os.Getpid(), userSeq)
	hash, err := service.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateUser(ctx, sqlc.CreateUserParams{Email: email, PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}

	// Login -> first token pair.
	rec, login := postJSON(t, api.Login, map[string]string{"email": email, "password": "password123"})
	if rec.Code != http.StatusOK || login.RefreshToken == "" {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}

	// Refresh -> rotates: new pair, old refresh now revoked.
	rec, rotated := postJSON(t, api.Refresh, map[string]string{"refresh_token": login.RefreshToken})
	if rec.Code != http.StatusOK || rotated.RefreshToken == "" {
		t.Fatalf("refresh failed: %d %s", rec.Code, rec.Body.String())
	}
	if rotated.RefreshToken == login.RefreshToken {
		t.Fatal("refresh must rotate the token")
	}

	// Reusing the OLD (now revoked) refresh token = theft signal -> 401.
	rec, _ = postJSON(t, api.Refresh, map[string]string{"refresh_token": login.RefreshToken})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reused token should be 401, got %d", rec.Code)
	}

	// Reuse detection must have revoked the whole chain, so the freshly
	// rotated token is now dead too.
	rec, _ = postJSON(t, api.Refresh, map[string]string{"refresh_token": rotated.RefreshToken})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("rotated token should be revoked after reuse detection, got %d", rec.Code)
	}
}

func TestLogoutRevokesRefreshToken(t *testing.T) {
	ctx := context.Background()
	api := httptransport.NewAuthAPI(pool, time.Hour)

	userSeq++
	email := fmt.Sprintf("logout-%d-%d@example.com", os.Getpid(), userSeq)
	hash, _ := service.HashPassword("password123")
	if _, err := q.CreateUser(ctx, sqlc.CreateUserParams{Email: email, PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}

	_, login := postJSON(t, api.Login, map[string]string{"email": email, "password": "password123"})

	rec, _ := postJSON(t, api.Logout, map[string]string{"refresh_token": login.RefreshToken})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout should be 204, got %d", rec.Code)
	}
	// Logged-out token can no longer refresh.
	rec, _ = postJSON(t, api.Refresh, map[string]string{"refresh_token": login.RefreshToken})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout should be 401, got %d", rec.Code)
	}
	// Logout is idempotent.
	rec, _ = postJSON(t, api.Logout, map[string]string{"refresh_token": login.RefreshToken})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("repeat logout should still be 204, got %d", rec.Code)
	}
}
