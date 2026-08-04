package blueprintcache

import (
	"context"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/listcache"
)

const SafetyTTL = time.Hour

var generations = listcache.NewGenerations("blueprint:generation:", SafetyTTL, 4096)

func Invalidate(ctx context.Context, cache k8scache.Cache, accountID string) error {
	return generations.Invalidate(ctx, cache, accountID)
}

func Generations(ctx context.Context, cache k8scache.Cache, accountIDs []string) []string {
	return generations.Values(ctx, cache, accountIDs)
}
