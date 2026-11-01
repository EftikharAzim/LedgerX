package httptransport

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
)

// MountUI serves an optional built SPA. os.Root confines every read, including
// symlinks; missing assets and API routes never fall back to an HTML success.
func MountUI(r chi.Router, directory string) (func() error, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open UI: %w", err)
	}
	if info, err := root.Stat("index.html"); err != nil || !info.Mode().IsRegular() {
		_ = root.Close()
		return nil, fmt.Errorf("UI_DIR must contain a built index.html")
	}
	files := http.FileServerFS(root.FS())
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		for _, prefix := range []string{"/auth", "/v1", "/admin", "/healthz", "/readyz", "/metrics"} {
			if req.URL.Path == prefix || strings.HasPrefix(req.URL.Path, prefix+"/") {
				http.NotFound(w, req)
				return
			}
		}
		name := strings.TrimPrefix(path.Clean("/"+req.URL.Path), "/")
		// No directory listings or dotfiles. Browser routes receive only index.
		for _, part := range strings.Split(name, "/") {
			if strings.HasPrefix(part, ".") {
				http.NotFound(w, req)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		if info, err := root.Stat(name); name != "" && err == nil && info.Mode().IsRegular() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, req)
			return
		}
		if path.Ext(name) != "" || (req.Header.Get("Accept") != "" && !strings.Contains(req.Header.Get("Accept"), "text/html")) {
			http.NotFound(w, req)
			return
		}
		clone := req.Clone(req.Context())
		clone.URL.Path = "/"
		files.ServeHTTP(w, clone)
	})
	r.Get("/*", handler.ServeHTTP)
	r.Head("/*", handler.ServeHTTP)
	return root.Close, nil
}
