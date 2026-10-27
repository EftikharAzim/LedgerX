package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalSaveReadAndOverwrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "exports")
	store, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	key := "nested/export_1.csv"
	for _, contents := range []string{"first export", "replacement"} {
		if err := store.Save(context.Background(), key, strings.NewReader(contents)); err != nil {
			t.Fatal(err)
		}
		reader, err := store.Open(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("read export: %v; close: %v", err, closeErr)
		}
		if string(got) != contents {
			t.Fatalf("export = %q, want %q", got, contents)
		}
	}
	for _, path := range []string{dir, filepath.Join(dir, "nested")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o027 != 0 {
			t.Fatalf("export directory permissions are too broad: %o", info.Mode().Perm())
		}
	}
	info, err := os.Stat(filepath.Join(dir, key))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("export permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestLocalOverwriteRestrictsExistingFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.csv")
	// #nosec G306 -- recreate an older world-readable export to test permission repair.
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), "legacy.csv", strings.NewReader("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("overwritten export permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestLocalRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocal(filepath.Join(dir, "exports"))
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside.csv")
	if err := os.WriteFile(outside, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../outside.csv", "nested/../../outside.csv", outside} {
		t.Run(key, func(t *testing.T) {
			if err := store.Save(context.Background(), key, strings.NewReader("overwritten")); err == nil {
				t.Fatal("Save accepted a key outside the export root")
			}
			if reader, err := store.Open(context.Background(), key); err == nil {
				_ = reader.Close()
				t.Fatal("Open accepted a key outside the export root")
			}
		})
	}
	contents, err := os.ReadFile(outside) // #nosec G304 -- outside is a sentinel file under t.TempDir, never external input.
	if err != nil || string(contents) != "private" {
		t.Fatalf("outside contents = %q, error = %v", contents, err)
	}
}

func TestLocalRejectsSymlinkEscapes(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	store, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "private.csv")
	if err := os.WriteFile(outsideFile, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "directory-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(dir, "file-link.csv")); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"directory-link/private.csv", "file-link.csv"} {
		t.Run(key, func(t *testing.T) {
			if err := store.Save(context.Background(), key, strings.NewReader("overwritten")); err == nil {
				t.Fatal("Save followed a symlink outside the export root")
			}
			if reader, err := store.Open(context.Background(), key); err == nil {
				_ = reader.Close()
				t.Fatal("Open followed a symlink outside the export root")
			}
		})
	}
	contents, err := os.ReadFile(outsideFile) // #nosec G304 -- outsideFile is a sentinel under t.TempDir, never external input.
	if err != nil || string(contents) != "private" {
		t.Fatalf("outside contents = %q, error = %v", contents, err)
	}
}
