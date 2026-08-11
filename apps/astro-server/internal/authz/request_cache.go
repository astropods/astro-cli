package authz

import "context"

type requestCacheContextKey struct{}
type authorizationRouteContextKey struct{}

type requestCache struct {
	accounts map[ResourceRef]accountCacheEntry
}

type accountCacheEntry struct {
	account resourceAccount
	err     error
}

// WithRequestCache deduplicates resource resolution within one request.
func WithRequestCache(ctx context.Context) context.Context {
	if requestCacheFromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, requestCacheContextKey{}, &requestCache{
		accounts: make(map[ResourceRef]accountCacheEntry),
	})
}

// WithAuthorizationRoute attaches the matched HTTP route to shadow-decision logs.
func WithAuthorizationRoute(ctx context.Context, route string) context.Context {
	return context.WithValue(ctx, authorizationRouteContextKey{}, route)
}

func authorizationRoute(ctx context.Context) string {
	route, _ := ctx.Value(authorizationRouteContextKey{}).(string)
	return route
}

func requestCacheFromContext(ctx context.Context) *requestCache {
	cache, _ := ctx.Value(requestCacheContextKey{}).(*requestCache)
	return cache
}

func cachedAccount(ctx context.Context, resource ResourceRef, load func() (resourceAccount, error)) (resourceAccount, error) {
	cache := requestCacheFromContext(ctx)
	if cache == nil {
		return load()
	}

	if entry, ok := cache.accounts[resource]; ok {
		return entry.account, entry.err
	}

	account, err := load()
	cache.accounts[resource] = accountCacheEntry{account: account, err: err}
	return account, err
}
