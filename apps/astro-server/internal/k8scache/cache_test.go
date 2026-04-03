package k8scache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedisCache(t *testing.T) (*RedisCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &RedisCache{rdb: client}, mr
}

// --- NoopCache ---

func TestNoopCache_AlwaysMisses(t *testing.T) {
	ctx := context.Background()
	c := NoopCache{}
	if _, ok := c.Get(ctx, "any-key"); ok {
		t.Error("NoopCache.Get should always miss")
	}
}

func TestNoopCache_SetReturnsNil(t *testing.T) {
	c := NoopCache{}
	if err := c.Set(context.Background(), "k", []byte("v"), time.Second); err != nil {
		t.Errorf("NoopCache.Set should return nil, got %v", err)
	}
}

func TestNoopCache_InvalidateReturnsNil(t *testing.T) {
	c := NoopCache{}
	if err := c.Invalidate(context.Background(), "k"); err != nil {
		t.Errorf("NoopCache.Invalidate should return nil, got %v", err)
	}
}

// --- New ---

func TestNew_NilClientReturnsNoopCache(t *testing.T) {
	c := New(nil)
	if _, ok := c.(NoopCache); !ok {
		t.Error("New(nil) should return NoopCache")
	}
}

func TestNew_NonNilClientReturnsRedisCache(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	if _, ok := New(client).(*RedisCache); !ok {
		t.Error("New(client) should return *RedisCache")
	}
}

// --- RedisCache ---

func TestRedisCache_SetAndGet(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestRedisCache(t)

	if err := c.Set(ctx, "mykey", []byte("myvalue"), 30*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	data, ok := c.Get(ctx, "mykey")
	if !ok {
		t.Fatal("Get should hit after Set")
	}
	if string(data) != "myvalue" {
		t.Errorf("expected %q, got %q", "myvalue", string(data))
	}
}

func TestRedisCache_GetMissOnUnknownKey(t *testing.T) {
	c, _ := newTestRedisCache(t)
	if _, ok := c.Get(context.Background(), "nonexistent"); ok {
		t.Error("Get should miss for unknown key")
	}
}

func TestRedisCache_Invalidate(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestRedisCache(t)

	_ = c.Set(ctx, "mykey", []byte("v"), 30*time.Second)
	if err := c.Invalidate(ctx, "mykey"); err != nil {
		t.Fatalf("Invalidate failed: %v", err)
	}
	if _, ok := c.Get(ctx, "mykey"); ok {
		t.Error("Get should miss after Invalidate")
	}
}

func TestRedisCache_TTLExpiry(t *testing.T) {
	ctx := context.Background()
	c, mr := newTestRedisCache(t)

	_ = c.Set(ctx, "mykey", []byte("v"), 5*time.Second)
	mr.FastForward(6 * time.Second)

	if _, ok := c.Get(ctx, "mykey"); ok {
		t.Error("Get should miss after TTL expiry")
	}
}

// --- InvalidateNamespace ---

func TestInvalidateNamespace_ClearsBothKeys(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestRedisCache(t)

	_ = c.Set(ctx, DetailKeyPrefix+"ns1", []byte("detail"), 30*time.Second)
	_ = c.Set(ctx, ListKeyPrefix+"ns1", []byte("list"), 30*time.Second)

	InvalidateNamespace(ctx, c, "ns1")

	if _, ok := c.Get(ctx, DetailKeyPrefix+"ns1"); ok {
		t.Error("detail key should be cleared after InvalidateNamespace")
	}
	if _, ok := c.Get(ctx, ListKeyPrefix+"ns1"); ok {
		t.Error("list key should be cleared after InvalidateNamespace")
	}
}

func TestInvalidateNamespace_DoesNotClearOtherNamespaces(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestRedisCache(t)

	_ = c.Set(ctx, ListKeyPrefix+"ns1", []byte("ns1"), 30*time.Second)
	_ = c.Set(ctx, ListKeyPrefix+"ns2", []byte("ns2"), 30*time.Second)

	InvalidateNamespace(ctx, c, "ns1")

	if _, ok := c.Get(ctx, ListKeyPrefix+"ns2"); !ok {
		t.Error("ns2 should not be cleared when invalidating ns1")
	}
}
