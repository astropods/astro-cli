package clusterconfig

import (
	"context"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func Sync(ctx context.Context, store *clusterstore.Store, entries []Entry, defaultClusterID string, log *logger.Logger) {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		row, err := e.ToClusterRow()
		if err != nil {
			log.Warn("cluster config: skipping invalid entry", "id", e.ID, "error", err)
			continue
		}
		if err := store.UpsertFromConfig(ctx, row, e.ID == defaultClusterID); err != nil {
			log.Warn("cluster config: sync failed for entry", "id", e.ID, "error", err)
			continue
		}
		log.Info("cluster config: synced entry", "id", e.ID)
		ids = append(ids, e.ID)
	}

	deleted, blocked, err := store.DeleteRemoved(ctx, ids)
	if err != nil {
		log.Warn("cluster config: delete-removed pass failed", "error", err)
		return
	}
	for _, id := range deleted {
		log.Info("cluster config: deleted cluster no longer present in config", "id", id)
	}
	for _, id := range blocked {
		log.Warn("cluster config: cluster no longer present in config, but still referenced — not deleted", "id", id)
	}
}
