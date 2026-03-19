package riverqueue

import (
	"context"
	"database/sql"
	"strings"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

// MessageCountSyncArgs are the job arguments for the message count sync worker.
type MessageCountSyncArgs struct{}

func (MessageCountSyncArgs) Kind() string { return "metrics.message_count_sync" }

// MessageCountSyncWorker periodically queries Prometheus for agent message counters
// and accumulates lifetime totals in Postgres, surviving counter resets.
type MessageCountSyncWorker struct {
	river.WorkerDefaults[MessageCountSyncArgs]
	promClient   *promquery.Client
	accountStore *account.AccountStore
	db           *sql.DB
	log          *logger.Logger
}

func (w *MessageCountSyncWorker) Work(ctx context.Context, _ *river.Job[MessageCountSyncArgs]) error {
	if w.promClient == nil {
		w.log.Debug("Message count sync skipped: no Prometheus client configured")
		return nil
	}

	samples, err := w.promClient.Query(ctx, `sum by (agent) (messaging_messages_forwarded_total)`)
	if err != nil {
		w.log.Error("Message count sync: failed to query Prometheus", "error", err)
		return nil // Don't retry — transient Prometheus issues shouldn't wedge the queue
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
