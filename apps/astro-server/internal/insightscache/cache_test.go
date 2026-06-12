package insightscache

import (
	"context"
	"testing"
	"time"
)

type testCache struct {
	data map[string][]byte
	ttls map[string]time.Duration
}

func newTestCache() *testCache {
	return &testCache{
		data: map[string][]byte{},
		ttls: map[string]time.Duration{},
	}
}

func (c *testCache) Get(_ context.Context, key string) ([]byte, bool) {
	v, ok := c.data[key]
	return v, ok
}

func (c *testCache) Set(_ context.Context, key string, data []byte, ttl time.Duration) error {
	c.data[key] = data
	c.ttls[key] = ttl
	return nil
}

func (c *testCache) Invalidate(_ context.Context, key string) error {
	delete(c.data, key)
	delete(c.ttls, key)
	return nil
}

func TestInvalidateAccountRemovesWarmedKeys(t *testing.T) {
	cache := newTestCache()
	ctx := t.Context()

	for _, variant := range WarmedVariants {
		if err := Put(ctx, cache, "acct-1", variant.Endpoint, variant.Params, []byte(`{"cached":true}`)); err != nil {
			t.Fatalf("seed warmed variant %s: %v", variant.Endpoint, err)
		}
	}

	InvalidateAccount(ctx, cache, "acct-1")

	for _, variant := range WarmedVariants {
		if _, ok := cache.Get(ctx, Key("acct-1", variant.Endpoint, variant.Params)); ok {
			t.Fatalf("expected warmed variant %s to be invalidated", variant.Endpoint)
		}
	}
}
