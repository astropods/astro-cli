// Package insightscache holds pre-computed Insights endpoint responses in
// Redis so the page-load path never has to fan out to Langfuse synchronously.
//
// A periodic River worker (InsightsRefreshWorker) writes entries every
// RefreshInterval; the three Insights handlers read first and only fall
// through to a live Langfuse query on a miss. Entries hold raw JSON bytes
// so the Go-struct response shape can evolve without invalidating the
// cache — fields that are present at write time will be returned verbatim
// at read time; new fields land as the cron catches up.
package insightscache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
)

// ErrAllLangfuseCallsFailed signals that every Langfuse sub-query inside
// a Compute* function failed. Lives here (vs in the handlers package)
// because both the handler and the periodic refresh worker need to
// distinguish it: handlers translate it into the metrics_unavailable
// banner; the worker treats it as a "preserve cache, don't retry" signal
// (an outage that River would retry against will keep failing — wait for
// the next periodic tick instead).
var ErrAllLangfuseCallsFailed = errors.New("all langfuse calls failed")

// RefreshInterval is how often the worker recomputes the cached responses.
// Bump cautiously — every account's three endpoints fan out to Langfuse on
// every refresh, so this directly throttles our load on the upstream.
const RefreshInterval = 6 * time.Hour

// CacheTTL deliberately exceeds several refresh windows so a multi-hour
// Langfuse outage doesn't drop the last-known-good value off the page. The
// worker skips writes when Langfuse is unhealthy, which means a cached
// entry can outlive a missed refresh.
const CacheTTL = 7 * 24 * time.Hour

const keyPrefix = "astro:insights:"

// Endpoint is the discriminator inside a cache key.
type Endpoint string

const (
	EndpointSummary            Endpoint = "summary"
	EndpointDeploymentsSummary Endpoint = "deployments-summary"
	EndpointUsersSummary       Endpoint = "users-summary"
)

// Params is the small subset of request parameters that varies between
// cached entries for the same (account, endpoint) pair. Only the canonical
// param sets the worker pre-warms are cached; anything else flows through
// live and skips the cache entirely.
type Params struct {
	GroupBy         string // "" or "user" (summary endpoint only)
	IncludeArchived bool
}

// Variant is one (endpoint, params) tuple the periodic worker pre-warms.
// Lives here so the writer (worker) and any reader/invalidator stay in
// lockstep — adding a new variant means appending to WarmedVariants and
// the worker picks it up automatically.
type Variant struct {
	Endpoint Endpoint
	Params   Params
}

// WarmedVariants is the canonical list of (endpoint, params) tuples the
// InsightsRefreshAccountWorker writes per account. Single source of truth
// for both the worker (which uses it to choose what to compute + write)
// and InvalidateAccount (which uses it to choose what to delete).
var WarmedVariants = []Variant{
	{Endpoint: EndpointSummary, Params: Params{GroupBy: "user", IncludeArchived: false}},
	{Endpoint: EndpointDeploymentsSummary, Params: Params{IncludeArchived: false}},
	{Endpoint: EndpointUsersSummary, Params: Params{}},
}

// Key returns the Redis key for an (account, endpoint, params) tuple. Param
// values are inlined; the surface is tiny (single string, single bool) and
// hashing earns nothing here.
func Key(accountID string, endpoint Endpoint, p Params) string {
	return keyPrefix + string(endpoint) + ":" + accountID +
		":group_by=" + p.GroupBy +
		":include_archived=" + strconv.FormatBool(p.IncludeArchived)
}

// Get returns the cached JSON bytes for (account, endpoint, params), or
// (nil, false) on miss. Caller pipes the bytes straight into the gin
// response — we deliberately don't unmarshal here so the on-wire shape can
// add fields without invalidating older cache entries.
func Get(ctx context.Context, cache k8scache.Cache, accountID string, endpoint Endpoint, p Params) ([]byte, bool) {
	if cache == nil {
		return nil, false
	}
	return cache.Get(ctx, Key(accountID, endpoint, p))
}

// Put writes the JSON bytes for (account, endpoint, params) with CacheTTL.
func Put(ctx context.Context, cache k8scache.Cache, accountID string, endpoint Endpoint, p Params, data []byte) error {
	if cache == nil {
		return nil
	}
	if err := cache.Set(ctx, Key(accountID, endpoint, p), data, CacheTTL); err != nil {
		return fmt.Errorf("insightscache: set: %w", err)
	}
	return nil
}

// InvalidateAccount removes every cache entry for an account that the
// refresh worker writes. Called by the admin grpc cache-invalidation RPC
// so an operator can force a fresh fetch without waiting on the 6h
// refresh cycle. Iterates WarmedVariants so the bust set tracks the
// populate set automatically.
func InvalidateAccount(ctx context.Context, cache k8scache.Cache, accountID string) {
	if cache == nil {
		return
	}
	for _, v := range WarmedVariants {
		_ = cache.Invalidate(ctx, Key(accountID, v.Endpoint, v.Params))
	}
}
