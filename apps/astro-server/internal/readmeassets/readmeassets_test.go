package readmeassets

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// fakeBackend is an in-memory Backend for tests.
type fakeBackend struct {
	mu     sync.Mutex
	writes map[string][]byte
	types  map[string]string
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{writes: map[string][]byte{}, types: map[string]string{}}
}

func (b *fakeBackend) Write(_ context.Context, key string, data []byte, contentType string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.writes[key] = data
	b.types[key] = contentType
	return nil
}

func (b *fakeBackend) Exists(_ context.Context, key string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.writes[key]
	return ok, nil
}

var (
	pngBytes = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	gifBytes = []byte("GIF89a\x01\x00\x01\x00")
	svgBytes = []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
)

func TestUpload(t *testing.T) {
	t.Run("png stored with content-addressed key", func(t *testing.T) {
		b := newFakeBackend()
		s := NewStore(b, "https://assets.example")
		url, err := s.Upload(context.Background(), "acc", "agent", pngBytes)
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if !strings.HasPrefix(url, "https://assets.example/readme-assets/acc/agent/") || !strings.HasSuffix(url, ".png") {
			t.Errorf("unexpected url %q", url)
		}
		if len(b.writes) != 1 {
			t.Errorf("expected 1 write, got %d", len(b.writes))
		}
		for k, ct := range b.types {
			if ct != "image/png" {
				t.Errorf("content type for %s = %q, want image/png", k, ct)
			}
		}
	})

	t.Run("identical bytes not re-written", func(t *testing.T) {
		b := newFakeBackend()
		s := NewStore(b, "https://assets.example")
		u1, _ := s.Upload(context.Background(), "acc", "agent", pngBytes)
		u2, err := s.Upload(context.Background(), "acc", "agent", pngBytes)
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if u1 != u2 {
			t.Errorf("expected stable url, got %q then %q", u1, u2)
		}
		if len(b.writes) != 1 {
			t.Errorf("expected dedup to 1 write, got %d", len(b.writes))
		}
	})

	t.Run("svg stored as svg+xml", func(t *testing.T) {
		b := newFakeBackend()
		s := NewStore(b, "https://assets.example")
		url, err := s.Upload(context.Background(), "acc", "agent", svgBytes)
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if !strings.HasSuffix(url, ".svg") {
			t.Errorf("expected .svg url, got %q", url)
		}
	})

	t.Run("unsupported type rejected", func(t *testing.T) {
		b := newFakeBackend()
		s := NewStore(b, "https://assets.example")
		if _, err := s.Upload(context.Background(), "acc", "agent", []byte("just some text, not an image")); err == nil {
			t.Error("expected error for unsupported type")
		}
	})

	t.Run("oversized rejected", func(t *testing.T) {
		b := newFakeBackend()
		s := NewStore(b, "https://assets.example")
		big := make([]byte, MaxAssetSize+1)
		copy(big, pngBytes)
		if _, err := s.Upload(context.Background(), "acc", "agent", big); err == nil {
			t.Error("expected error for oversized image")
		}
	})
}

func TestProcessMarkdown(t *testing.T) {
	b := newFakeBackend()
	s := NewStore(b, "https://assets.example")

	readme := strings.Join([]string{
		"# Title",
		"![arch](./docs/arch.png)",
		"![logo](images/logo.gif)",
		"![remote](https://x.com/a.png)",
		"![missing](docs/missing.png)",
	}, "\n")

	fetch := func(relPath string) ([]byte, error) {
		switch relPath {
		case "docs/arch.png":
			return pngBytes, nil
		case "images/logo.gif":
			return gifBytes, nil
		case "docs/missing.png":
			return nil, nil // not found
		default:
			return nil, fmt.Errorf("unexpected fetch %q", relPath)
		}
	}

	out, warnings := s.ProcessMarkdown(context.Background(), "acc", "agent", readme, fetch)

	if strings.Contains(out, "./docs/arch.png") || strings.Contains(out, "images/logo.gif") {
		t.Errorf("local images not rewritten:\n%s", out)
	}
	if !strings.Contains(out, "https://assets.example/readme-assets/acc/agent/") {
		t.Errorf("expected CDN urls in output:\n%s", out)
	}
	if !strings.Contains(out, "https://x.com/a.png") {
		t.Error("remote image should be left untouched")
	}
	if !strings.Contains(out, "docs/missing.png") {
		t.Error("missing image should be left as original reference")
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning for missing image, got %d: %v", len(warnings), warnings)
	}
}
