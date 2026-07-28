package metering

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro-spec"
)

const heartbeatInterval = 5 * time.Minute

// Heartbeat emits periodic metering events for active deployments and agent counts.
type Heartbeat struct {
	provider billing.BillingProvider
	db       *sql.DB
	log      *logger.Logger
	billing  *BillingStateManager
}

// NewHeartbeat creates a new metering heartbeat.
func NewHeartbeat(provider billing.BillingProvider, db *sql.DB, log *logger.Logger, billing *BillingStateManager) *Heartbeat {
	return &Heartbeat{
		provider: provider,
		db:       db,
		log:      log,
		billing:  billing,
	}
}

// Tick runs a single heartbeat iteration. It emits metered-consumption usage
// only. Resource counts are served from the quota DB and no longer metered.
// Knowledge compute/storage metering is disabled for now — only deployment_compute_usage
// is emitted; emitKnowledgeStorage/emitKnowledgeCompute remain dormant.
func (h *Heartbeat) Tick(ctx context.Context) {
	h.log.Debug("metering: tick starting")
	h.emitComputeUsage(ctx)
	h.log.Debug("metering: tick complete")
}

// activeDeploymentRow represents a row from the active deployments query.
type activeDeploymentRow struct {
	AccountID    string
	AgentName    string
	DeploymentID string
	SpecJSON     string
}

func (h *Heartbeat) getActiveDeployments(ctx context.Context) ([]activeDeploymentRow, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT account_id, agent_name, id, deployment_spec_json
		FROM deployments
		WHERE status = 'active'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []activeDeploymentRow
	for rows.Next() {
		var r activeDeploymentRow
		if err := rows.Scan(&r.AccountID, &r.AgentName, &r.DeploymentID, &r.SpecJSON); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// containerUsage holds the compute unit calculation for a single container.
type containerUsage struct {
	Component string  // e.g. "agent", "model/llm", "knowledge/docs", "integration/search", "interfaces", "observability"
	CU        float64 // compute units for this container
	CPU       string  // original CPU request string
	Memory    string  // original memory request string
	Replicas  int
}

// emitComputeUsage calculates CU-hours per container for each active deployment and emits deployment_compute_usage events.
// When a BillingStateManager is attached, delegates to delta-based reconciliation. Otherwise falls back
// to the legacy full-interval approach (reads from normalized deployment_workloads table or JSON parsing).
func (h *Heartbeat) emitComputeUsage(ctx context.Context) {
	if h.billing != nil {
		h.billing.RunBillingCycle(ctx)
		return
	}
	intervalHours := heartbeatInterval.Hours()
	var events []billing.UsageEvent

	// Try normalized workloads table first
	workloads, err := h.getActiveWorkloads(ctx)
	if err != nil {
		h.log.Warn("metering: normalized workloads query failed, falling back to JSON", "error", err)
		workloads = nil
	}

	if len(workloads) > 0 {
		for _, w := range workloads {
			component := w.ComponentKind
			if w.ComponentKey != "" {
				component += "/" + w.ComponentKey
			}
			replicas := w.Replicas
			if replicas <= 0 {
				replicas = 1
			}
			cu := containerCU(spec.DeploymentResources{CPU: w.CPURequest, Memory: w.MemoryRequest}, replicas)
			if cu <= 0 {
				continue
			}
			events = append(events, usageEvent("deployment_compute_usage", w.AccountID, map[string]any{
				"cu_hours":      cu * intervalHours,
				"agent_name":    w.AgentName,
				"deployment_id": w.DeploymentID,
				"component":     component,
				"cpu":           w.CPURequest,
				"memory":        w.MemoryRequest,
				"replicas":      replicas,
			}))
		}
	} else {
		// Fallback: parse JSON for deployments without normalized data
		deployments, err := h.getActiveDeployments(ctx)
		if err != nil {
			h.log.Error("metering: failed to query active deployments", "error", err)
			return
		}
		for _, d := range deployments {
			var depSpec spec.AstroDeploymentSpec
			if err := json.Unmarshal([]byte(d.SpecJSON), &depSpec); err != nil {
				h.log.Error("metering: failed to parse deployment spec", "error", err, "account_id", d.AccountID, "agent", d.AgentName)
				continue
			}
			containers := containerBreakdown(&depSpec)
			for _, c := range containers {
				if c.CU <= 0 {
					continue
				}
				events = append(events, usageEvent("deployment_compute_usage", d.AccountID, map[string]any{
					"cu_hours":      c.CU * intervalHours,
					"agent_name":    d.AgentName,
					"deployment_id": d.DeploymentID,
					"component":     c.Component,
					"cpu":           c.CPU,
					"memory":        c.Memory,
					"replicas":      c.Replicas,
				}))
			}
		}
	}

	if len(events) > 0 {
		if err := h.provider.IngestUsage(ctx, events); err != nil {
			h.log.Error("metering: failed to emit deployment_compute_usage events", "error", err)
		} else {
			h.log.Info("metering: emitted deployment_compute_usage", "events", len(events), "sample_subject", events[0].AccountID, "sample_type", events[0].Type)
		}
	}
}

// activeWorkloadRow holds workload data from the normalized tables.
type activeWorkloadRow struct {
	AccountID     string
	AgentName     string
	DeploymentID  string
	ComponentKind string
	ComponentKey  string
	Replicas      int
	CPURequest    string
	MemoryRequest string
}

func (h *Heartbeat) getActiveWorkloads(ctx context.Context) ([]activeWorkloadRow, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT d.account_id, d.agent_name, d.id,
			w.component_kind, w.component_key, w.replicas, w.cpu_request, w.memory_request
		FROM deployments d
		JOIN deployment_workloads w ON w.deployment_id = d.id
		WHERE d.status = 'active'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []activeWorkloadRow
	for rows.Next() {
		var r activeWorkloadRow
		if err := rows.Scan(&r.AccountID, &r.AgentName, &r.DeploymentID,
			&r.ComponentKind, &r.ComponentKey, &r.Replicas, &r.CPURequest, &r.MemoryRequest); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// containerBreakdown returns per-container compute unit calculations for a deployment spec.
func containerBreakdown(s *spec.AstroDeploymentSpec) []containerUsage {
	var result []containerUsage

	result = append(result, makeContainerUsage("agent", s.Agent.Resources, s.Agent.Replicas))

	for name, m := range s.Models {
		result = append(result, makeContainerUsage("model/"+name, m.Resources, m.Replicas))
	}
	for name, k := range s.Knowledge {
		result = append(result, makeContainerUsage("knowledge/"+name, k.Resources, k.Replicas))
	}
	for name, t := range s.Integrations {
		result = append(result, makeContainerUsage("integration/"+name, t.Resources, t.Replicas))
	}
	if s.Interfaces != nil {
		result = append(result, makeContainerUsage("interfaces", s.Interfaces.Resources, 1))
	}
	if s.Observability.Enabled {
		result = append(result, makeContainerUsage("observability", s.Observability.Resources, 1))
	}

	return result
}

func makeContainerUsage(component string, r spec.DeploymentResources, replicas int) containerUsage {
	if replicas <= 0 {
		replicas = 1
	}
	return containerUsage{
		Component: component,
		CU:        containerCU(r, replicas),
		CPU:       r.CPU,
		Memory:    r.Memory,
		Replicas:  replicas,
	}
}

// containerCU calculates compute units for a single container type.
func containerCU(r spec.DeploymentResources, replicas int) float64 {
	if replicas <= 0 {
		replicas = 1
	}
	cpuCores := parseCPU(r.CPU)
	memGB := parseMemory(r.Memory)
	cu := math.Max(cpuCores, memGB/2)
	return cu * float64(replicas)
}

// parseCPU parses a K8s CPU string (e.g. "100m", "2", "1.5") to cores.
func parseCPU(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if rest, ok := strings.CutSuffix(s, "m"); ok {
		v, err := strconv.ParseFloat(rest, 64)
		if err != nil {
			return 0
		}
		return v / 1000
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseMemory parses a K8s memory string (e.g. "256Mi", "1Gi", "512M") to GB.
func parseMemory(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	suffixes := []struct {
		suffix string
		mult   float64
	}{
		{"Ti", 1024},
		{"Gi", 1},
		{"Mi", 1.0 / 1024},
		{"Ki", 1.0 / (1024 * 1024)},
		{"T", 1000},
		{"G", 1},
		{"M", 1.0 / 1000},
		{"K", 1.0 / (1000 * 1000)},
	}

	for _, sf := range suffixes {
		if rest, ok := strings.CutSuffix(s, sf.suffix); ok {
			v, err := strconv.ParseFloat(rest, 64)
			if err != nil {
				return 0
			}
			return v * sf.mult
		}
	}

	// Plain bytes
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v / (1024 * 1024 * 1024)
}

// emitKnowledgeStorage emits knowledge_storage_provisioned events per managed store,
// with provisioned storage parsed from K8s quantity to GB.
func (h *Heartbeat) emitKnowledgeStorage(ctx context.Context) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT account_id, name, provider, storage
		FROM knowledge_stores
		WHERE mode = 'managed' AND status != 'error'
	`)
	if err != nil {
		h.log.Error("metering: failed to query knowledge storage", "error", err)
		return
	}
	defer rows.Close() //nolint:errcheck

	var events []billing.UsageEvent
	for rows.Next() {
		var accountID, name, provider, storage string
		if err := rows.Scan(&accountID, &name, &provider, &storage); err != nil {
			h.log.Error("metering: failed to scan knowledge storage row", "error", err)
			continue
		}
		gb := parseMemory(storage)
		if gb <= 0 {
			continue
		}
		events = append(events, usageEvent("knowledge_storage_provisioned", accountID, map[string]any{
			"storage_gb": gb,
			"store_name": name,
			"provider":   provider,
		}))
	}

	if len(events) > 0 {
		if err := h.provider.IngestUsage(ctx, events); err != nil {
			h.log.Error("metering: failed to emit knowledge_storage_provisioned events", "error", err)
		} else {
			h.log.Info("metering: emitted knowledge_storage_provisioned", "events", len(events))
		}
	}
}

// emitKnowledgeCompute emits knowledge_compute_usage events per managed+ready store.
// When a BillingStateManager is attached, delegates to delta-based reconciliation. Otherwise
// uses the legacy full-interval approach with per-provider default resource requests.
func (h *Heartbeat) emitKnowledgeCompute(ctx context.Context) {
	if h.billing != nil {
		h.billing.RunKnowledgeBillingCycle(ctx)
		return
	}
	intervalHours := heartbeatInterval.Hours()
	rows, err := h.db.QueryContext(ctx, `
		SELECT account_id, name, provider
		FROM knowledge_stores
		WHERE mode = 'managed' AND status = 'ready'
	`)
	if err != nil {
		h.log.Error("metering: failed to query knowledge stores for compute", "error", err)
		return
	}
	defer rows.Close() //nolint:errcheck

	var events []billing.UsageEvent
	for rows.Next() {
		var accountID, name, provider string
		if err := rows.Scan(&accountID, &name, &provider); err != nil {
			h.log.Error("metering: failed to scan knowledge compute row", "error", err)
			continue
		}
		cu := knowledgeCU(provider)
		if cu <= 0 {
			continue
		}
		res := knowledgeProviderResourceStrings(provider)
		events = append(events, usageEvent("knowledge_compute_usage", accountID, map[string]any{
			"cu_hours": cu * intervalHours,
			"store_name":         name,
			"provider":           provider,
			"cpu":                res.cpu,
			"memory":             res.memory,
		}))
	}

	if len(events) > 0 {
		if err := h.provider.IngestUsage(ctx, events); err != nil {
			h.log.Error("metering: failed to emit knowledge_compute_usage events", "error", err)
		} else {
			h.log.Info("metering: emitted knowledge_compute_usage", "events", len(events))
		}
	}
}

// knowledgeProviderResourceStrings returns the default CPU and memory request strings
// for a knowledge store provider. Used to populate event payloads.
type providerResources struct{ cpu, memory string }

func knowledgeProviderResourceStrings(provider string) providerResources {
	defaults := map[string]providerResources{
		"postgres": {"250m", "256Mi"},
		"redis":    {"50m", "64Mi"},
		"qdrant":   {"250m", "512Mi"},
		"neo4j":    {"500m", "512Mi"},
	}
	r, ok := defaults[provider]
	if !ok {
		return providerResources{"100m", "128Mi"}
	}
	return r
}
