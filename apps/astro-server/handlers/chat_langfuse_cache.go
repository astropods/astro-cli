package handlers

import (
	"sync"
	"time"
)

const langfuseHydrationCacheTTL = 30 * time.Second

type langfuseHydrationCache struct {
	mu      sync.RWMutex
	entries map[string]langfuseCacheEntry
}

type langfuseCacheEntry struct {
	messages  []ChatMessageResponse
	truncated bool
	expiresAt time.Time
}

func (c *langfuseHydrationCache) get(key string) ([]ChatMessageResponse, bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false, false
	}
	out := make([]ChatMessageResponse, len(entry.messages))
	copy(out, entry.messages)
	return out, entry.truncated, true
}

func (c *langfuseHydrationCache) set(key string, messages []ChatMessageResponse, truncated bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]langfuseCacheEntry)
	}
	stored := make([]ChatMessageResponse, len(messages))
	copy(stored, messages)
	c.entries[key] = langfuseCacheEntry{
		messages:  stored,
		truncated: truncated,
		expiresAt: time.Now().Add(langfuseHydrationCacheTTL),
	}
}

var chatLangfuseHydrationCache langfuseHydrationCache
