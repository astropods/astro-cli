package k8scache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// ListKeyPrefix scopes list cache keys to this application, avoiding collisions
// on shared Redis instances.
const ListKeyPrefix = "astro:k8s:list:" // light listAstroDeploymentsLight result (ListDeployments)

// ListTTL is the TTL for list cache entries.
const ListTTL = 15 * time.Second

// Cache abstracts K8s namespace state caching for deployment query results.
// Get/Set/Invalidate accept the full cache key (including prefix); callers are
// responsible for constructing keys using DetailKeyPrefix or ListKeyPrefix.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, data []byte, ttl time.Duration) error
	Invalidate(ctx context.Context, key string) error
}

type multiGetter interface {
	GetMany(ctx context.Context, keys []string) map[string][]byte
}

// GetMany fetches several keys in one Redis round trip when the cache supports
// it. Test and custom caches that only implement Cache retain a correct
// sequential fallback.
func GetMany(ctx context.Context, cache Cache, keys []string) map[string][]byte {
	result := make(map[string][]byte, len(keys))
	if cache == nil || len(keys) == 0 {
		return result
	}
	if cache, ok := cache.(multiGetter); ok {
		return cache.GetMany(ctx, keys)
	}
	for _, key := range keys {
		if value, ok := cache.Get(ctx, key); ok {
			result[key] = value
		}
	}
	return result
}

// New returns a RedisCache backed by client, or a NoopCache if client is nil.
// Callers should initialize a single *redis.Client in main and pass it here,
// rather than constructing one per feature.
func New(client *redis.Client) Cache {
	if client == nil {
		return NoopCache{}
	}
	return &RedisCache{rdb: client}
}

// InvalidateNamespace clears the list cache entry for a namespace.
func InvalidateNamespace(ctx context.Context, cache Cache, namespace string) {
	_ = cache.Invalidate(ctx, ListKeyPrefix+namespace)
}

// RedisCache is a Cache backed by Redis with a 15-second TTL.
type RedisCache struct {
	rdb *redis.Client
}

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool) {
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return data, true
}

func (c *RedisCache) GetMany(ctx context.Context, keys []string) map[string][]byte {
	result := make(map[string][]byte, len(keys))
	if len(keys) == 0 {
		return result
	}
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return result
	}
	for i, value := range values {
		switch value := value.(type) {
		case string:
			result[keys[i]] = []byte(value)
		case []byte:
			result[keys[i]] = value
		}
	}
	return result
}

func (c *RedisCache) Set(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, data, ttl).Err()
}

func (c *RedisCache) Invalidate(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// NoopCache always misses; used when Redis is not configured.
type NoopCache struct{}

func (NoopCache) Get(_ context.Context, _ string) ([]byte, bool)                   { return nil, false }
func (NoopCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }
func (NoopCache) Invalidate(_ context.Context, _ string) error                     { return nil }
func (NoopCache) GetMany(_ context.Context, _ []string) map[string][]byte          { return map[string][]byte{} }
