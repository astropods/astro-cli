// Package obssummary defines the cache contract between the periodic
// refresher (writes) and the GetLangfuseSummaries handler (reads). The
// page-load path never calls Langfuse — it only reads from this cache.
//
// The cache holds the "summary" data the agents page needs to render
// sparklines, usage totals, and last-active timestamps for every deployment in
// the account.
// Worker refreshes happen every RefreshInterval; entries expire at CacheTTL
// (one refresh window plus a buffer) so a missed tick still serves
// stale-but-recent data instead of dropping the value off the page.
package obssummary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
)

// RefreshInterval is how often the worker re-fetches Langfuse for every
// active deployment and writes the cache. The handler never waits this
// long — it just reads whatever the most recent worker run left behind.
const RefreshInterval = 10 * time.Minute

// CacheTTL must exceed RefreshInterval so a worker run that's a little
// late doesn't cause the page to suddenly lose data. 50% buffer.
const CacheTTL = 15 * time.Minute

// DaysOfHistory is the length of the request_series, token_series, and
// cost_series arrays the worker writes and the handler returns. Bumping this
// changes the sparkline window everywhere it's rendered.
const DaysOfHistory = 30

// keyPrefix scopes the cache so other features can't accidentally collide.
const keyPrefix = "obs:summary:"

// Entry is the value the worker writes and the handler reads. Field names
// match the JSON the handler used to compute inline so the over-the-wire
// shape stays the same from the frontend's perspective.
type Entry struct {
	TotalTraces   int       `json:"total_traces"`
	LastTraceAt   string    `json:"last_trace_at"`
	CostUSD       float64   `json:"cost_usd"`
	RequestSeries []int     `json:"request_series"`
	TokenSeries   []int     `json:"token_series"`
	CostSeries    []float64 `json:"cost_series"`
	RefreshedAt   time.Time `json:"refreshed_at"`
}

// KeyFor returns the cache key for a deployment.
func KeyFor(deploymentID string) string {
	return keyPrefix + deploymentID
}

// Get reads the cached entry for a deployment. Returns (nil, false, nil) when
// no entry exists; (nil, false, err) only on unmarshal failure (Redis read
// errors are swallowed by k8scache).
func Get(ctx context.Context, cache k8scache.Cache, deploymentID string) (*Entry, bool, error) {
	if cache == nil {
		return nil, false, nil
	}
	data, ok := cache.Get(ctx, KeyFor(deploymentID))
	if !ok {
		return nil, false, nil
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, false, fmt.Errorf("obssummary: unmarshal: %w", err)
	}
	return &e, true, nil
}

// GetMany reads a bounded visible-card set in one Redis MGET when supported.
// Malformed entries are omitted and returned as a joined error so callers can
// serve the remaining summaries while retaining observability.
func GetMany(ctx context.Context, cache k8scache.Cache, deploymentIDs []string) (map[string]*Entry, error) {
	entries := make(map[string]*Entry, len(deploymentIDs))
	if cache == nil || len(deploymentIDs) == 0 {
		return entries, nil
	}
	keys := make([]string, len(deploymentIDs))
	for i, id := range deploymentIDs {
		keys[i] = KeyFor(id)
	}
	values := k8scache.GetMany(ctx, cache, keys)
	var decodeErrors []error
	for i, id := range deploymentIDs {
		data, ok := values[keys[i]]
		if !ok {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			decodeErrors = append(decodeErrors, fmt.Errorf("%s: %w", id, err))
			continue
		}
		entries[id] = &entry
	}
	return entries, errors.Join(decodeErrors...)
}

// Put writes an entry for a deployment with the configured TTL.
func Put(ctx context.Context, cache k8scache.Cache, deploymentID string, e *Entry) error {
	if cache == nil {
		return nil
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("obssummary: marshal: %w", err)
	}
	return cache.Set(ctx, KeyFor(deploymentID), data, CacheTTL)
}

// Delete invalidates the entry for a deployment. Called when a deployment
// is undeployed so the page doesn't keep showing data for a gone agent
// until TTL.
func Delete(ctx context.Context, cache k8scache.Cache, deploymentID string) error {
	if cache == nil {
		return nil
	}
	return cache.Invalidate(ctx, KeyFor(deploymentID))
}
