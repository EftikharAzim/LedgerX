// Package storage abstracts where export files live so the API can run
// with local disk in dev and object storage (S3/MinIO) in real deployments,
// where instances are replaceable and share nothing.
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Storage interface {
	// Save stores the object under key, overwriting any existing object.
	Save(ctx context.Context, key string, body io.Reader) error
	// Open returns a reader for the object; the caller must close it.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
}

// Local stores objects as files under a directory. Dev/demo only: files
// don't survive container replacement and aren't shared across instances.
type Local struct{ dir string }

func NewLocal(dir string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	return &Local{dir: dir}, nil
}

func (l *Local) path(key string) (string, error) {
	// Keys are server-generated, but defend against traversal anyway.
	p := filepath.Join(l.dir, filepath.Clean("/"+key))
	return p, nil
}

func (l *Local) Save(_ context.Context, key string, body io.Reader) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}
