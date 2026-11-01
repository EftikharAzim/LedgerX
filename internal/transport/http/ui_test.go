package httptransport

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltUIServesDeepLinksWithoutMaskingMissingAssetsOrAPI(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{"index.html": "<html>LedgerX</html>", "assets/main-123.js": "console.log('app')", ".env": "PRIVATE"} {
		filename := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("OUTSIDE SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escaped.txt")); err != nil {
		t.Fatal(err)
	}
	r := NewRouter()
	r.Get("/v1/existing", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	closeUI, err := MountUI(r, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeUI() })
	for _, tc := range []struct {
		path string
		code int
		body string
	}{
		{"/", 200, "LedgerX"}, {"/accounts/123", 200, "LedgerX"},
		{"/assets/main-123.js", 200, "console.log"}, {"/assets/missing.js", 404, ""},
		{"/v1/missing", 404, ""}, {"/auth/missing", 404, ""}, {"/v1/existing", 401, ""},
		{"/.env", 404, ""}, {"/escaped.txt", 404, ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Accept", "text/html")
			r.ServeHTTP(rec, req)
			if rec.Code != tc.code || (tc.body != "" && !strings.Contains(rec.Body.String(), tc.body)) || strings.Contains(rec.Body.String(), "SECRET") {
				t.Fatalf("%s: status=%d body=%q", tc.path, rec.Code, rec.Body.String())
			}
			if strings.HasPrefix(tc.path, "/assets/") && tc.code == 200 && !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
				t.Fatal("built assets should be cached")
			}
		})
	}
}

func TestMountUIRequiresABuild(t *testing.T) {
	if _, err := MountUI(NewRouter(), t.TempDir()); err == nil {
		t.Fatal("missing build must fail startup")
	}
}
