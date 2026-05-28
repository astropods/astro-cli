package deploycache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
)

// mapCache is a tiny in-memory k8scache.Cache for these unit tests.
type mapCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMapCache() *mapCache { return &mapCache{data: make(map[string][]byte)} }

func (c *mapCache) Get(_ context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.data[key]
	return v, ok
}

func (c *mapCache) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = data
	return nil
}

func (c *mapCache) Invalidate(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	return nil
}

var _ k8scache.Cache = (*mapCache)(nil)

func TestKeyFor_Prefix(t *testing.T) {
	if got := KeyFor("acct-123"); got != "dep:agent:acct-123" {
		t.Errorf("KeyFor() = %q, want dep:agent:acct-123", got)
	}
}

func TestPutGet_Roundtrip(t *testing.T) {
	cache := newMapCache()
	ctx := context.Background()
	payload := []byte(`{"deployments":[],"count":0}`)

	if err := Put(ctx, cache, "acct-1", payload); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := Get(ctx, cache, "acct-1")
	if !ok {
		t.Fatal("Get: cache miss after Put")
	}
	if string(got) != string(payload) {
		t.Errorf("Get returned %q, want %q", string(got), string(payload))
	}
}

func TestGet_MissReturnsFalse(t *testing.T) {
	if _, ok := Get(context.Background(), newMapCache(), "nobody"); ok {
		t.Error("Get on empty cache should return ok=false")
	}
}

func TestInvalidate_ClearsEntry(t *testing.T) {
	cache := newMapCache()
	ctx := context.Background()
	_ = Put(ctx, cache, "acct-1", []byte("x"))
	if err := Invalidate(ctx, cache, "acct-1"); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	if _, ok := Get(ctx, cache, "acct-1"); ok {
		t.Error("entry still present after Invalidate")
	}
}

func TestInvalidate_IdempotentOnMiss(t *testing.T) {
	// Calling Invalidate when there's no entry must not error — write paths
	// shouldn't have to check existence first.
	if err := Invalidate(context.Background(), newMapCache(), "ghost"); err != nil {
		t.Errorf("Invalidate on missing key returned error: %v", err)
	}
}

func TestNilCache_Safe(t *testing.T) {
	// All entry points must tolerate a nil cache so dev environments without
	// Redis don't NPE — they just operate as a pass-through (cache miss).
	ctx := context.Background()
	if _, ok := Get(ctx, nil, "a"); ok {
		t.Error("Get(nil) should return ok=false")
	}
	if err := Put(ctx, nil, "a", []byte("x")); err != nil {
		t.Errorf("Put(nil) returned error: %v", err)
	}
	if err := Invalidate(ctx, nil, "a"); err != nil {
		t.Errorf("Invalidate(nil) returned error: %v", err)
	}
}

func TestSafetyTTL_Sane(t *testing.T) {
	// Sanity: the safety TTL is long enough to cover normal worker cadences
	// but short enough that a missed bust path doesn't strand the page
	// indefinitely. Anything in [10m, 24h] is reasonable.
	if SafetyTTL < 10*time.Minute || SafetyTTL > 24*time.Hour {
		t.Errorf("SafetyTTL = %s — outside the [10m, 24h] sanity band", SafetyTTL)
	}
}
