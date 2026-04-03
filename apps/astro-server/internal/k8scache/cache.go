package k8scache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Key prefixes distinguish full vs lightweight payloads and scope keys to this
// application, avoiding collisions on shared Redis instances.
const (
	DetailKeyPrefix = "astro:k8s:detail:" // full listAstroDeployments result (GetDeployment)
	ListKeyPrefix   = "astro:k8s:list:"   // light listAstroDeploymentsLight result (ListDeployments)
)

// Default TTLs per cache type.
const (
	ListTTL   = 15 * time.Second
	DetailTTL = 15 * time.Second
)

// Cache abstracts K8s namespace state caching for deployment query results.
// Get/Set/Invalidate accept the full cache key (including prefix); callers are
// responsible for constructing keys using DetailKeyPrefix or ListKeyPrefix.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, data []byte, ttl time.Duration) error
	Invalidate(ctx context.Context, key string) error
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

// InvalidateNamespace clears both the detail and list cache entries for a namespace.
func InvalidateNamespace(ctx context.Context, cache Cache, namespace string) {
	_ = cache.Invalidate(ctx, DetailKeyPrefix+namespace)
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
