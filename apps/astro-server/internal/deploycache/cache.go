// Package deploycache stores the per-account "agents page view" payload
// (the JSON the listDeployments handler returns) so reads are Redis-only.
//
// The contract is intentionally schemaless: the cache stores raw JSON bytes
// keyed by account_id, and the handler owns the marshal/unmarshal. That keeps
// this package out of the handlers package's dependency graph (avoiding the
// import cycle) and lets the response shape evolve without touching this file.
//
// Lifecycle: writes happen at deploy-success / undeploy / reconcile status
// changes / display-name updates / avatar updates / publish-time
// latest_build_id shifts. Each of those call sites invokes Invalidate before
// the next read repopulates. A SafetyTTL bounds worst-case staleness if a
// future write path forgets to bust.
package deploycache

import (
	"context"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
)

// SafetyTTL is the upper-bound TTL on a cache entry. Primary invalidation is
// explicit; this is a safety net against missed bust paths (most notably the
// publish flow that affects downstream accounts' `latest_build_id`).
const SafetyTTL = 1 * time.Hour

// keyPrefix scopes the cache so other features can't accidentally collide.
const keyPrefix = "dep:agent:"

// KeyFor returns the cache key for an account.
func KeyFor(accountID string) string {
	return keyPrefix + accountID
}

// Get reads the cached payload for an account. Returns (nil, false) when no
// entry exists or the underlying cache is nil (Redis disabled in dev).
func Get(ctx context.Context, cache k8scache.Cache, accountID string) ([]byte, bool) {
	if cache == nil {
		return nil, false
	}
	return cache.Get(ctx, KeyFor(accountID))
}

// Put writes a payload for an account with SafetyTTL.
func Put(ctx context.Context, cache k8scache.Cache, accountID string, data []byte) error {
	if cache == nil {
		return nil
	}
	return cache.Set(ctx, KeyFor(accountID), data, SafetyTTL)
}

// Invalidate clears the cache for an account. Idempotent. Safe to call when
// the underlying cache is nil (no-op).
func Invalidate(ctx context.Context, cache k8scache.Cache, accountID string) error {
	if cache == nil {
		return nil
	}
	return cache.Invalidate(ctx, KeyFor(accountID))
}

// LineageLookup is the minimal subset of deploymentstore.Store that
// InvalidateForLineage needs. Defined as an interface so this package
// stays independent of the store and tests can fake the lookup.
type LineageLookup interface {
	ListAccountIDsWithLineageAgent(accountID, agentName string) ([]string, error)
}

// InvalidateForLineage busts the cache for every account that has a
// deployment whose lineage points at the given (accountID, agentName) tuple.
// Used by publish paths so downstream consumers' `latest_build_id` pills
// update immediately instead of waiting for SafetyTTL.
//
// Returns the list of accounts that were invalidated (for logging).
// Lookup failures are swallowed — SafetyTTL is the backstop.
func InvalidateForLineage(
	ctx context.Context,
	cache k8scache.Cache,
	store LineageLookup,
	accountID, agentName string,
) []string {
	if cache == nil || store == nil {
		return nil
	}
	accts, err := store.ListAccountIDsWithLineageAgent(accountID, agentName)
	if err != nil || len(accts) == 0 {
		return nil
	}
	for _, id := range accts {
		_ = Invalidate(ctx, cache, id)
	}
	return accts
}
