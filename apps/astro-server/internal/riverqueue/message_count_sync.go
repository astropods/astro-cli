package riverqueue

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

// MessageCountSyncArgs are the job arguments for the message count sync worker.
type MessageCountSyncArgs struct{}

func (MessageCountSyncArgs) Kind() string { return "metering.message_count_sync" }

func (MessageCountSyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMetering}
}

func init() {
	registerJobKind[MessageCountSyncArgs]()
}

// MessageCountSyncWorker periodically queries Prometheus for agent message counters
// and accumulates lifetime totals in Postgres, surviving counter resets.
type MessageCountSyncWorker struct {
	river.WorkerDefaults[MessageCountSyncArgs]
	promClient   *promquery.Client
	registry     *k8s.Registry
	accountStore *account.AccountStore
	db           *sql.DB
	log          *logger.Logger
}

func (w *MessageCountSyncWorker) Work(ctx context.Context, _ *river.Job[MessageCountSyncArgs]) error {
	if w.promClient == nil {
		w.log.Debug("Message count sync skipped: no Prometheus client configured")
		return nil
	}

	entries := []k8s.ClusterEntry{{ID: "", IsDefault: true}}
	if w.registry != nil {
		listed, err := w.registry.List(ctx)
		if err != nil {
			w.log.Error("Message count sync: failed to list clusters, falling back to primary only", "error", err)
		} else {
			entries = listed
			hasDefault := false
			for _, e := range entries {
				if e.IsDefault {
					hasDefault = true
					break
				}
			}
			if !hasDefault {
				// The default cluster's own row may be missing (e.g. boot sync
				// hasn't run or failed) — query it anyway via the bare sentinel;
				// clusterLabel below falls back to the client's own baked-in name.
				entries = append([]k8s.ClusterEntry{{ID: "", IsDefault: true}}, entries...)
			}
		}
	}

	var samples []promquery.Sample
	for _, entry := range entries {
		// ForEntry, not the ID-based PrometheusClientFor: entries already came
		// from List, so re-resolving by id would cost a redundant clusterstore
		// round trip per cluster.
		client := w.registry.PrometheusClientForEntry(entry, w.promClient)
		if client == nil {
			continue
		}

		// Prefer the registry's name for this cluster; fall back to the
		// client's own baked-in label when there's no registry (e.g. tests
		// constructing a client directly via promquery.NewClient).
		clusterLabel := entry.EKSClusterName
		if clusterLabel == "" {
			clusterLabel = client.Cluster()
		}
		query := `sum by (agent) (messaging_messages_forwarded_total)`
		if clusterLabel != "" {
			query = fmt.Sprintf(`sum by (agent) (messaging_messages_forwarded_total{cluster=%q})`, clusterLabel)
		}

		clusterSamples, err := client.Query(ctx, query)
		if err != nil {
			w.log.Error("Message count sync: failed to query Prometheus", "cluster", entry.ID, "error", err)
			continue // Don't retry — transient Prometheus issues shouldn't wedge the queue
		}
		samples = append(samples, clusterSamples...)
	}

	if len(samples) == 0 {
		return nil
	}

	for _, s := range samples {
		// The "agent" label is set by Alloy relabeling from the astro.dev/agent
		// pod label, which uses "account_name.agent_name" format (see deployment/naming.go).
		agentLabel := s.Labels["agent"]
		accountName, agentName, ok := strings.Cut(agentLabel, ".")
		if !ok || accountName == "" || agentName == "" {
			continue
		}
		acct, err := w.accountStore.GetByName(accountName)
		if err != nil {
			w.log.Error("Message count sync: account lookup failed",
				"account_name", accountName,
				"error", err,
			)
			continue
		}
		if err := upsertMessageCount(ctx, w.db, acct.ID, agentName, s.Value); err != nil {
			w.log.Error("Message count sync: upsert failed",
				"account_id", acct.ID,
				"agent_name", agentName,
				"error", err,
			)
		}
	}

	w.log.Info("Message count sync completed", "agents", len(samples))
	return nil
}

// upsertMessageCount applies delta-based accumulation with counter-reset detection.
// If current >= last_prom_value: delta = current - last_prom_value (normal increment).
// If current < last_prom_value: delta = current (counter reset — pod restarted).
func upsertMessageCount(ctx context.Context, db *sql.DB, accountID, agentName string, currentValue float64) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO agent_message_counts (account_id, agent_name, lifetime_total, last_prom_value, updated_at)
		VALUES ($1, $2, $3::bigint, $3::double precision, now())
		ON CONFLICT (account_id, agent_name) DO UPDATE SET
			lifetime_total = agent_message_counts.lifetime_total + CASE
				WHEN $3::double precision >= agent_message_counts.last_prom_value
				THEN ($3::double precision - agent_message_counts.last_prom_value)::bigint
				ELSE $3::bigint
			END,
			last_prom_value = $3::double precision,
			updated_at = now()
	`, accountID, agentName, currentValue)
	return err
}
