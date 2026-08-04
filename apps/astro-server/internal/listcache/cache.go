// Package listcache provides a small two-tier response cache for authenticated
// list pages. L1 keeps local and Redis-free environments fast; L2 shares hot
// pages across production replicas. Callers remain responsible for including
// authorization scope and invalidation generations in keys.
package listcache

import (
	"bytes"
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"golang.org/x/sync/singleflight"
)

const defaultLoadTimeout = 15 * time.Second

type entry struct {
	response  Response
	expiresAt time.Time
}

// Response is the cached HTTP body plus the small telemetry values list
// handlers need on every hit. Keeping these values beside the body avoids
// decoding a potentially large items array just to populate request logs.
type Response struct {
	Data              []byte
	ResultCount       int
	NextCursorPresent bool
}

// LoadResult lets callers keep a degraded but usable response in the short L1
// cache without sharing it across replicas through L2.
type LoadResult struct {
	Response
	RemoteCacheable bool
}

var remoteEnvelopeMagic = []byte("listcache:v1\x00")

// Cache is a bounded process-local cache backed by the shared server cache.
type Cache struct {
	remote      k8scache.Cache
	prefix      string
	localTTL    time.Duration
	remoteTTL   time.Duration
	loadTimeout time.Duration
	maxItems    int

	mu      sync.Mutex
	entries map[string]entry
	group   singleflight.Group
}

// New creates a response cache. maxItems <= 0 disables L1 storage while the
// remote tier remains available.
func New(
	remote k8scache.Cache,
	prefix string,
	localTTL time.Duration,
	remoteTTL time.Duration,
	maxItems int,
) *Cache {
	return &Cache{
		remote:      remote,
		prefix:      prefix,
		localTTL:    localTTL,
		remoteTTL:   remoteTTL,
		loadTimeout: defaultLoadTimeout,
		maxItems:    maxItems,
		entries:     make(map[string]entry),
	}
}

// GetOrLoad returns a response and its source: l1, l2, or miss.
// Concurrent misses for the same key share one load. Successful loads always
// enter the short L1; LoadResult.RemoteCacheable controls whether they enter L2.
func (c *Cache) GetOrLoad(
	ctx context.Context,
	key string,
	load func(context.Context) (LoadResult, error),
) (Response, string, error) {
	fullKey := c.prefix + key
	if response, ok := c.getLocal(fullKey); ok {
		return response, "l1", nil
	}
	if c.remote != nil {
		if data, ok := c.remote.Get(ctx, fullKey); ok {
			if response, valid := decodeRemoteResponse(data); valid {
				c.putLocal(fullKey, response)
				return response, "l2", nil
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return Response{}, "miss", err
	}

	resultCh := c.group.DoChan(fullKey, func() (any, error) {
		// The shared load must not inherit the first caller's cancellation: that
		// would let one disconnected request fail every healthy waiter. Keep the
		// request values, detach cancellation, and apply a hard upper bound so an
		// abandoned load cannot occupy a goroutine or DB connection indefinitely.
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.loadTimeout)
		defer cancel()
		if response, ok := c.getLocal(fullKey); ok {
			return cacheValue{response: response, source: "l1"}, nil
		}
		if c.remote != nil {
			if data, ok := c.remote.Get(loadCtx, fullKey); ok {
				if response, valid := decodeRemoteResponse(data); valid {
					c.putLocal(fullKey, response)
					return cacheValue{response: response, source: "l2"}, nil
				}
			}
		}
		loaded, err := load(loadCtx)
		if err != nil {
			return nil, err
		}
		response := cloneResponse(loaded.Response)
		c.putLocal(fullKey, response)
		if loaded.RemoteCacheable && c.remote != nil {
			_ = c.remote.Set(loadCtx, fullKey, encodeRemoteResponse(response), c.remoteTTL)
		}
		return cacheValue{response: response, source: "miss"}, nil
	})

	select {
	case <-ctx.Done():
		return Response{}, "miss", ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return Response{}, "miss", result.Err
		}
		value := result.Val.(cacheValue)
		return cloneResponse(value.response), value.source, nil
	}
}

type cacheValue struct {
	response Response
	source   string
}

func cloneResponse(response Response) Response {
	response.Data = append([]byte(nil), response.Data...)
	return response
}

func encodeRemoteResponse(response Response) []byte {
	encoded := make([]byte, 0, len(remoteEnvelopeMagic)+24+len(response.Data))
	encoded = append(encoded, remoteEnvelopeMagic...)
	encoded = strconv.AppendInt(encoded, int64(response.ResultCount), 10)
	encoded = append(encoded, '\n', 0)
	if response.NextCursorPresent {
		encoded[len(encoded)-1] = 1
	}
	encoded = append(encoded, response.Data...)
	return encoded
}

func decodeRemoteResponse(encoded []byte) (Response, bool) {
	if len(encoded) <= len(remoteEnvelopeMagic) || !bytes.Equal(encoded[:len(remoteEnvelopeMagic)], remoteEnvelopeMagic) {
		return Response{}, false
	}
	payload := encoded[len(remoteEnvelopeMagic):]
	countEnd := bytes.IndexByte(payload, '\n')
	if countEnd < 1 || len(payload) <= countEnd+1 || payload[countEnd+1] > 1 {
		return Response{}, false
	}
	resultCount, err := strconv.Atoi(string(payload[:countEnd]))
	if err != nil || resultCount < 0 {
		return Response{}, false
	}
	return Response{
		Data:              append([]byte(nil), payload[countEnd+2:]...),
		ResultCount:       resultCount,
		NextCursorPresent: payload[countEnd+1] == 1,
	}, true
}

func (c *Cache) getLocal(key string) (Response, bool) {
	if c.maxItems <= 0 || c.localTTL <= 0 {
		return Response{}, false
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.entries[key]
	if !ok {
		return Response{}, false
	}
	if !now.Before(value.expiresAt) {
		delete(c.entries, key)
		return Response{}, false
	}
	return cloneResponse(value.response), true
}

func (c *Cache) putLocal(key string, response Response) {
	if c.maxItems <= 0 || c.localTTL <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxItems {
		var oldestKey string
		var oldest time.Time
		for candidate, value := range c.entries {
			if oldestKey == "" || value.expiresAt.Before(oldest) {
				oldestKey = candidate
				oldest = value.expiresAt
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[key] = entry{
		response:  cloneResponse(response),
		expiresAt: time.Now().Add(c.localTTL),
	}
}
