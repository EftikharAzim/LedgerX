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

// Storage saves and reads export objects by key.
type Storage interface {
	// Save stores the object under key, overwriting any existing object.
	Save(ctx context.Context, key string, body io.Reader) error
	// Open returns a reader for the object; the caller must close it.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
}

// Local stores objects as files under a directory. Dev/demo only: files
// don't survive container replacement and aren't shared across instances.
type Local struct{ dir string }

// NewLocal creates a local export store rooted at dir.
func NewLocal(dir string) (*Local, error) {
	// #nosec G703 -- dir is trusted startup configuration, never an export key; key-based I/O is confined by os.Root.
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	return &Local{dir: dir}, nil
}

func (l *Local) rootFor(key string) (*os.Root, error) {
	// Keys are server-generated, but defend against traversal anyway.
	if !filepath.IsLocal(key) {
		return nil, fmt.Errorf("invalid export key %q", key)
	}
	// Root confines operations even when a key encounters a symlink.
	return os.OpenRoot(l.dir)
}

// Save stores an export under key, overwriting any existing object.
func (l *Local) Save(_ context.Context, key string, body io.Reader) error {
	root, err := l.rootFor(key)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := root.MkdirAll(filepath.Dir(key), 0o750); err != nil {
		return err
	}
	f, err := root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	// Overwriting an older export must also remove its previous public mode.
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := io.Copy(f, body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Open returns an export reader that the caller must close.
func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	root, err := l.rootFor(key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.Open(key)
}
