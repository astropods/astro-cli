package listcache

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/google/uuid"
)

type generationEntry struct {
	value     string
	expiresAt time.Time
}

// Generations tracks bounded, per-resource invalidation tokens. Redis gives
// every replica the same token while the local registry keeps Redis-free
// environments correct after a mutation.
type Generations struct {
	prefix   string
	ttl      time.Duration
	maxItems int

	mu      sync.Mutex
	entries map[string]generationEntry
}

// NewGenerations creates a bounded generation registry.
func NewGenerations(prefix string, ttl time.Duration, maxItems int) *Generations {
	return &Generations{
		prefix:   prefix,
		ttl:      ttl,
		maxItems: maxItems,
		entries:  make(map[string]generationEntry),
	}
}

// KeyFor returns the remote cache key for id.
func (g *Generations) KeyFor(id string) string {
	return g.prefix + id
}

func (g *Generations) remember(id, generation string) {
	if g.maxItems <= 0 || g.ttl <= 0 {
		return
	}
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.entries[id]; !exists && g.maxItems > 0 && len(g.entries) >= g.maxItems {
		var earliestID string
		var earliestExpiry time.Time
		for candidate, entry := range g.entries {
			if earliestID == "" || entry.expiresAt.Before(earliestExpiry) {
				earliestID = candidate
				earliestExpiry = entry.expiresAt
			}
		}
		delete(g.entries, earliestID)
	}
	g.entries[id] = generationEntry{value: generation, expiresAt: now.Add(g.ttl)}
}

func (g *Generations) loadLocal(id string) (string, bool) {
	if g.maxItems <= 0 || g.ttl <= 0 {
		return "", false
	}
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, ok := g.entries[id]
	if !ok {
		return "", false
	}
	if !now.Before(entry.expiresAt) {
		delete(g.entries, id)
		return "", false
	}
	return entry.value, true
}

// Invalidate assigns a new token to id. The local token is written first so a
// transient Redis failure cannot leave this process serving its stale page.
func (g *Generations) Invalidate(ctx context.Context, cache k8scache.Cache, id string) error {
	generation := uuid.NewString()
	g.remember(id, generation)
	if cache == nil {
		return nil
	}
	return cache.Set(ctx, g.KeyFor(id), []byte(generation), g.ttl)
}

// Values returns a deterministic id:generation vector. Redis-backed caches use
// one MGET for the whole selected scope.
func (g *Generations) Values(ctx context.Context, cache k8scache.Cache, ids []string) []string {
	sortedIDs := append([]string(nil), ids...)
	sort.Strings(sortedIDs)
	keys := make([]string, len(sortedIDs))
	for i, id := range sortedIDs {
		keys[i] = g.KeyFor(id)
	}
	remote := k8scache.GetMany(ctx, cache, keys)

	result := make([]string, 0, len(sortedIDs))
	for i, id := range sortedIDs {
		generation := "0"
		if value, ok := remote[keys[i]]; ok && len(value) > 0 {
			generation = string(value)
			g.remember(id, generation)
		} else if value, ok := g.loadLocal(id); ok {
			generation = value
		}
		result = append(result, id+":"+generation)
	}
	return result
}

// InvalidateWithLegacyKey updates the generation and removes an older payload
// key in one helper. It exists for deploycache's pre-generation cache entry.
func (g *Generations) InvalidateWithLegacyKey(ctx context.Context, cache k8scache.Cache, id, legacyKey string) error {
	if cache == nil {
		return g.Invalidate(ctx, nil, id)
	}
	return errors.Join(cache.Invalidate(ctx, legacyKey), g.Invalidate(ctx, cache, id))
}
