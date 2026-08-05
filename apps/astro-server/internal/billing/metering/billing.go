package metering

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// BillingStateManager handles event-driven compute metering by tracking billing
// state across deployment lifecycle transitions. It emits
// precise CU-hour events at each state change and provides a reconciliation
// method for the heartbeat to fill in gaps.
type BillingStateManager struct {
	provider billing.BillingProvider
	db       *sql.DB
	log      *logger.Logger
}

// NewBillingStateManager creates a new BillingStateManager. Returns nil if the
// billing provider is nil (no metering backend configured).
func NewBillingStateManager(provider billing.BillingProvider, db *sql.DB, log *logger.Logger) *BillingStateManager {
	if provider == nil {
		return nil
	}
	return &BillingStateManager{provider: provider, db: db, log: log}
}

// WorkloadInfo describes a single billable workload component within a deployment.
type WorkloadInfo struct {
	Component     string // e.g. "agent", "model/llm"
	CPURequest    string
	MemoryRequest string
	Replicas      int
}

// StartBilling records that a deployment's workloads are now consuming compute.
// Called when a deployment transitions to active. No CU-hours are emitted — the
// anchor timestamp is set so the next stop or heartbeat event can calculate the delta.
func (m *BillingStateManager) StartBilling(ctx context.Context, deploymentID string, workloads []WorkloadInfo) {
	if m == nil {
		return
	}
	now := time.Now().UTC()
	for _, w := range workloads {
		replicas := w.Replicas
		if replicas <= 0 {
			replicas = 1
		}
		_, err := m.db.ExecContext(ctx, `
			INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas)
			VALUES ($1, $2, true, $3, $4, $5, $6)
			ON CONFLICT (deployment_id, component) DO UPDATE
			SET billing_active = true, last_emitted_at = $3, cpu_request = $4, memory_request = $5, replicas = $6, stopped_at = NULL
		`, deploymentID, w.Component, now, w.CPURequest, w.MemoryRequest, replicas)
		if err != nil {
			m.log.Error("Failed to start billing state", "error", err, "deployment_id", deploymentID, "component", w.Component)
		}
	}
}

// StopBilling atomically records the stop time for a deployment's billing rows.
// No events are emitted here — the heartbeat picks up stopped_at and emits the
// final CU-hours on its next tick.
func (m *BillingStateManager) StopBilling(ctx context.Context, deploymentID string, stoppedAt time.Time) {
	if m == nil {
		return
	}
	_, err := m.db.ExecContext(ctx, `
		UPDATE deployment_billing_state
		SET billing_active = false, stopped_at = $2
		WHERE deployment_id = $1 AND billing_active = true
	`, deploymentID, stoppedAt)
	if err != nil {
		m.log.Error("billing: failed to record deployment stop", "error", err, "deployment_id", deploymentID)
	}
}

// RunBillingCycle is called by the heartbeat. For each active billing row, it emits
// CU-hours for the delta since last_emitted_at and advances the anchor. It also
// catches stale rows where the deployment is no longer active (crash recovery).
func (m *BillingStateManager) RunBillingCycle(ctx context.Context) {
	if m == nil {
		return
	}
	now := time.Now().UTC()
	m.healMissingBillingRows(ctx, now)
	m.emitActiveBilling(ctx, now)
	m.reconcileStale(ctx)
	m.reconcileStopped(ctx)
}

// emitActiveBilling emits delta CU-hours for non-stale active billing rows and
// advances their last_emitted_at anchor. Stale rows are excluded so reconcileStale
// can bill them accurately up to status_changed_at.
func (m *BillingStateManager) emitActiveBilling(ctx context.Context, now time.Time) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT bs.deployment_id, bs.component, bs.last_emitted_at,
		       bs.cpu_request, bs.memory_request, bs.replicas,
		       d.account_id, d.agent_name
		FROM deployment_billing_state bs
		JOIN deployments d ON d.id = bs.deployment_id
		WHERE bs.billing_active = true AND d.status IN ('active', 'provisioning')
	`)
	if err != nil {
		m.log.Error("metering: failed to query active billing states", "error", err)
		return
	}
	defer rows.Close() //nolint:errcheck

	type rowKey struct{ deploymentID, component string }
	var events []billing.UsageEvent
	var keys []rowKey
	for rows.Next() {
		var deploymentID, component, cpuReq, memReq, accountID, agentName string
		var lastEmitted time.Time
		var replicas int
		if err := rows.Scan(&deploymentID, &component, &lastEmitted, &cpuReq, &memReq, &replicas, &accountID, &agentName); err != nil {
			m.log.Error("metering: failed to scan active billing row", "error", err)
			continue
		}
		keys = append(keys, rowKey{deploymentID, component})
		elapsed := now.Sub(lastEmitted).Hours()
		if elapsed <= 0 {
			continue
		}
		cu := rawCU(cpuReq, memReq, replicas)
		if cu <= 0 {
			continue
		}
		events = append(events, computeUsageEvent(accountID, agentName, deploymentID, component, cpuReq, memReq, replicas, cu*elapsed))
	}
	if err := rows.Err(); err != nil {
		m.log.Error("metering: failed to iterate active billing rows", "error", err)
	}

	if len(events) > 0 {
		if err := m.provider.IngestUsage(ctx, events); err != nil {
			m.log.Error("metering: failed to emit reconcile compute events", "error", err)
			return // don't advance timestamps if emission failed
		}
		m.log.Info("metering: reconciled active compute", "events", len(events))
	}

	if len(keys) == 0 {
		return
	}

	placeholders := make([]string, len(keys))
	params := make([]any, 0, len(keys)*2+1)
	params = append(params, now)
	for i, k := range keys {
		placeholders[i] = fmt.Sprintf("($%d, $%d)", 2*i+2, 2*i+3)
		params = append(params, k.deploymentID, k.component)
	}
	query := fmt.Sprintf( //nolint:gosec // placeholders are $N positional params, not user input
		"UPDATE deployment_billing_state SET last_emitted_at = $1 WHERE (deployment_id, component) IN (%s)",
		strings.Join(placeholders, ", "),
	)
	if _, err := m.db.ExecContext(ctx, query, params...); err != nil {
		m.log.Error("metering: failed to advance billing timestamps", "error", err)
	}
}

// reconcileStale catches billing rows left active due to crashes or missed stop events.
func (m *BillingStateManager) reconcileStale(ctx context.Context) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT bs.deployment_id, bs.component, bs.last_emitted_at,
		       bs.cpu_request, bs.memory_request, bs.replicas,
		       d.account_id, d.agent_name, d.status_changed_at
		FROM deployment_billing_state bs
		JOIN deployments d ON d.id = bs.deployment_id
		WHERE bs.billing_active = true AND d.status NOT IN ('active', 'provisioning')
	`)
	if err != nil {
		m.log.Error("metering: failed to query stale billing states", "error", err)
		return
	}
	defer rows.Close() //nolint:errcheck

	var events []billing.UsageEvent
	for rows.Next() {
		var deploymentID, component, cpuReq, memReq, accountID, agentName string
		var lastEmitted, statusChangedAt time.Time
		var replicas int
		if err := rows.Scan(&deploymentID, &component, &lastEmitted, &cpuReq, &memReq, &replicas,
			&accountID, &agentName, &statusChangedAt); err != nil {
			m.log.Error("metering: failed to scan stale billing row", "error", err)
			continue
		}

		// Bill up to the time the deployment actually stopped
		elapsed := statusChangedAt.Sub(lastEmitted).Hours()
		cu := rawCU(cpuReq, memReq, replicas)
		if elapsed > 0 && cu > 0 {
			events = append(events, computeUsageEvent(accountID, agentName, deploymentID, component, cpuReq, memReq, replicas, cu*elapsed))
		}
	}
	if err := rows.Err(); err != nil {
		m.log.Error("metering: failed to iterate stale billing rows", "error", err)
	}

	if len(events) > 0 {
		if err := m.provider.IngestUsage(ctx, events); err != nil {
			m.log.Error("metering: failed to emit stale billing events", "error", err)
			return
		}
		m.log.Info("metering: reconciled stale compute", "events", len(events))
	}

	if _, err := m.db.ExecContext(ctx, `
		UPDATE deployment_billing_state SET billing_active = false
		WHERE billing_active = true
		  AND deployment_id IN (
		    SELECT id FROM deployments WHERE status NOT IN ('active', 'provisioning')
		  )
	`); err != nil {
		m.log.Error("metering: failed to deactivate stale billing rows", "error", err)
	}
}

// reconcileStopped emits the final CU-hours for rows where StopBilling recorded a
// stopped_at but the heartbeat hasn't processed them yet. Clears stopped_at after
// successful emission so the rows are not reprocessed.
func (m *BillingStateManager) reconcileStopped(ctx context.Context) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT bs.deployment_id, bs.component, bs.last_emitted_at, bs.stopped_at,
		       bs.cpu_request, bs.memory_request, bs.replicas,
		       d.account_id, d.agent_name
		FROM deployment_billing_state bs
		JOIN deployments d ON d.id = bs.deployment_id
		WHERE bs.billing_active = false AND bs.stopped_at IS NOT NULL
		  AND bs.last_emitted_at < bs.stopped_at
	`)
	if err != nil {
		m.log.Error("metering: failed to query stopped billing rows", "error", err)
		return
	}
	defer rows.Close() //nolint:errcheck

	type rowKey struct{ deploymentID, component string }
	var events []billing.UsageEvent
	var keys []rowKey
	for rows.Next() {
		var deploymentID, component, cpuReq, memReq, accountID, agentName string
		var lastEmitted, stoppedAt time.Time
		var replicas int
		if err := rows.Scan(&deploymentID, &component, &lastEmitted, &stoppedAt,
			&cpuReq, &memReq, &replicas, &accountID, &agentName); err != nil {
			m.log.Error("metering: failed to scan stopped billing row", "error", err)
			continue
		}
		keys = append(keys, rowKey{deploymentID, component})
		elapsed := stoppedAt.Sub(lastEmitted).Hours()
		if elapsed <= 0 {
			continue
		}
		cu := rawCU(cpuReq, memReq, replicas)
		if cu <= 0 {
			continue
		}
		events = append(events, computeUsageEvent(accountID, agentName, deploymentID, component, cpuReq, memReq, replicas, cu*elapsed))
	}
	if err := rows.Err(); err != nil {
		m.log.Error("metering: failed to iterate stopped billing rows", "error", err)
	}

	if len(keys) == 0 {
		return
	}

	if len(events) > 0 {
		if err := m.provider.IngestUsage(ctx, events); err != nil {
			m.log.Error("metering: failed to emit stopped billing events", "error", err)
			return
		}
		m.log.Info("metering: emitted final period for stopped deployments", "events", len(events))
	}

	placeholders := make([]string, len(keys))
	params := make([]any, 0, len(keys)*2)
	for i, k := range keys {
		placeholders[i] = fmt.Sprintf("($%d, $%d)", 2*i+1, 2*i+2)
		params = append(params, k.deploymentID, k.component)
	}
	query := fmt.Sprintf( //nolint:gosec // placeholders are $N positional params, not user input
		"UPDATE deployment_billing_state SET stopped_at = NULL WHERE (deployment_id, component) IN (%s) AND billing_active = false AND stopped_at IS NOT NULL",
		strings.Join(placeholders, ", "),
	)
	if _, err := m.db.ExecContext(ctx, query, params...); err != nil {
		m.log.Error("metering: failed to clear stopped_at on billing rows", "error", err)
	}
}

// healMissingBillingRows inserts billing rows for active deployments that have none.
// ON CONFLICT DO NOTHING makes this idempotent — existing rows are untouched.
// Logs any rows that were actually inserted so gaps are visible in observability.
func (m *BillingStateManager) healMissingBillingRows(ctx context.Context, now time.Time) {
	rows, err := m.db.QueryContext(ctx, `
		INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas)
		SELECT dw.deployment_id,
		       CASE WHEN dw.component_key = '' THEN dw.component_kind
		            ELSE dw.component_kind || '/' || dw.component_key END,
		       true, $1, dw.cpu_request, dw.memory_request, dw.replicas
		FROM deployment_workloads dw
		JOIN deployments d ON d.id = dw.deployment_id
		WHERE d.status IN ('active', 'provisioning')
		ON CONFLICT (deployment_id, component) DO NOTHING
		RETURNING deployment_id, component
	`, now)
	if err != nil {
		m.log.Error("metering: failed to heal missing billing rows", "error", err)
		return
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var deploymentID, component string
		if err := rows.Scan(&deploymentID, &component); err != nil {
			m.log.Error("metering: failed to scan healed billing row", "error", err)
			continue
		}
		m.log.Info("metering: healed missing billing row", "deployment_id", deploymentID, "component", component)
	}
}

func computeUsageEvent(accountID, agentName, deploymentID, component, cpuReq, memReq string, replicas int, cuHours float64) billing.UsageEvent {
	return usageEvent("deployment_compute_usage", accountID, map[string]any{
		"cu_hours":      cuHours,
		"agent_name":    agentName,
		"deployment_id": deploymentID,
		"component":     component,
		"cpu":           cpuReq,
		"memory":        memReq,
		"replicas":      replicas,
	})
}

// rawCU calculates compute units from raw CPU/memory strings and replica count.
func rawCU(cpuReq, memReq string, replicas int) float64 {
	if replicas <= 0 {
		replicas = 1
	}
	cpuCores := parseCPU(cpuReq)
	memGB := parseMemory(memReq)
	cu := math.Max(cpuCores, memGB/2)
	return cu * float64(replicas)
}
