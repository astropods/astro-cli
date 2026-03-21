package avatar

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// LocalBackend stores avatar images on the local filesystem.
// Used for local development instead of S3.
type LocalBackend struct {
	// root is the base directory, e.g. "../../assets" (repo root assets/ dir).
	// Files are stored at root/{key}, mirroring the S3 key layout.
	root string
}

// NewLocalBackend creates a new filesystem-backed storage backend.
func NewLocalBackend(root string) *LocalBackend {
	return &LocalBackend{root: root}
}

func (b *LocalBackend) path(key string) string {
	return filepath.Join(b.root, filepath.Clean("/"+key))
}

func (b *LocalBackend) Read(_ context.Context, key string) ([]byte, error) {
	data, err := os.ReadFile(b.path(key))
	if err != nil {
		return nil, fmt.Errorf("local read %s: %w", key, err)
	}
	return data, nil
}

func (b *LocalBackend) Write(_ context.Context, key string, data []byte, _ string) error {
	p := b.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("local mkdir %s: %w", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("local write %s: %w", key, err)
	}
	return nil
}

func (b *LocalBackend) Copy(_ context.Context, src, dst string) error {
	data, err := os.ReadFile(b.path(src))
	if err != nil {
		return fmt.Errorf("local copy read %s: %w", src, err)
	}
	dstPath := filepath.Clean(b.path(dst))
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o750); err != nil {
		return fmt.Errorf("local copy mkdir %s: %w", filepath.Dir(dstPath), err)
	}
	if err := os.WriteFile(dstPath, data, 0o600); err != nil {
		return fmt.Errorf("local copy write %s: %w", dst, err)
	}
	return nil
}

func (b *LocalBackend) Delete(_ context.Context, key string) error {
	if err := os.Remove(b.path(key)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("local delete %s: %w", key, err)
	}
	return nil
}

func (b *LocalBackend) Exists(_ context.Context, key string) (bool, error) {
	_, err := os.Stat(b.path(key))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("local stat %s: %w", key, err)
	}
	return true, nil
}
