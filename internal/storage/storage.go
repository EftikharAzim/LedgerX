// Package storage abstracts where export files live so the API can run
// with local disk in dev and object storage (S3/MinIO) in real deployments,
// where instances are replaceable and share nothing.
package storage

import (
	"context"
	"crypto/rand"
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
	// Delete is idempotent when the object is already absent.
	Delete(ctx context.Context, key string) error
}

// Local stores objects under a directory. Mount durable storage to survive
// container replacement; separate local disks are not shared across instances.
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
	if info, err := root.Lstat(key); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("export key cannot be a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	// Stage alongside the destination so rename is atomic on the same volume.
	// A failed write must not truncate an already downloadable report.
	temporary := filepath.Join(filepath.Dir(key), ".ledgerx-export-"+rand.Text())
	f, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = root.Remove(temporary) }()
	if _, err := io.Copy(f, body); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return root.Rename(temporary, key)
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

// Delete removes an export without allowing traversal or symlink escape.
func (l *Local) Delete(_ context.Context, key string) error {
	root, err := l.rootFor(key)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := root.Remove(key); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
