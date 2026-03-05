package openmeter

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/postman/astro/apps/astro-server/internal/logger"
	spec "github.com/postman/astro/packages/astro-spec"
)

const heartbeatInterval = 5 * time.Minute

// Heartbeat emits periodic metering events for active deployments and agent counts.
type Heartbeat struct {
	client *Client
	db     *sql.DB
	log    *logger.Logger
}

// NewHeartbeat creates a new metering heartbeat.
func NewHeartbeat(client *Client, db *sql.DB, log *logger.Logger) *Heartbeat {
	return &Heartbeat{
		client: client,
		db:     db,
		log:    log,
	}
}

// Start runs the heartbeat loop until the context is cancelled.
func (h *Heartbeat) Start(ctx context.Context) {
	h.log.Info("OpenMeter heartbeat started", "interval", heartbeatInterval.String())

	// Run immediately, then on interval
	h.tick(ctx)

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.log.Info("OpenMeter heartbeat stopping")
			return
		case <-ticker.C:
			h.tick(ctx)
		}
	}
}

func (h *Heartbeat) tick(ctx context.Context) {
	h.emitComputeUsage(ctx)
	h.emitActiveDeployments(ctx)
	h.emitActiveAgents(ctx)
}

// activeDeploymentRow represents a row from the active deployments query.
type activeDeploymentRow struct {
	AccountID string
	AgentName string
	Namespace string
	SpecJSON  string
}

func (h *Heartbeat) getActiveDeployments(ctx context.Context) ([]activeDeploymentRow, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT account_id, agent_name, namespace, deployment_spec_json
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
		if err := rows.Scan(&r.AccountID, &r.AgentName, &r.Namespace, &r.SpecJSON); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// containerUsage holds the compute unit calculation for a single container.
type containerUsage struct {
	Component string  // e.g. "agent", "model/llm", "knowledge/docs", "tool/search", "interfaces", "observability"
	CU        float64 // compute units for this container
	CPU       string  // original CPU request string
	Memory    string  // original memory request string
	Replicas  int
}

// emitComputeUsage calculates CU-hours per container for each active deployment and emits compute_usage events.
func (h *Heartbeat) emitComputeUsage(ctx context.Context) {
	deployments, err := h.getActiveDeployments(ctx)
	if err != nil {
		h.log.Error("Heartbeat: failed to query active deployments", "error", err)
		return
	}

	if len(deployments) == 0 {
		return
	}

	intervalHours := heartbeatInterval.Hours()
	var events []CloudEvent

	for _, d := range deployments {
		var depSpec spec.AstroDeploymentSpec
		if err := json.Unmarshal([]byte(d.SpecJSON), &depSpec); err != nil {
			h.log.Error("Heartbeat: failed to parse deployment spec", "error", err, "account_id", d.AccountID, "agent", d.AgentName)
			continue
		}

		containers := containerBreakdown(&depSpec)
		for _, c := range containers {
			if c.CU <= 0 {
				continue
			}
			events = append(events, NewCloudEvent("compute_usage", d.AccountID, map[string]any{
				"compute_unit_hours": c.CU * intervalHours,
				"agent_name":         d.AgentName,
				"namespace":          d.Namespace,
				"component":          c.Component,
				"cpu":                c.CPU,
				"memory":             c.Memory,
				"replicas":           c.Replicas,
			}))
		}
	}

	if len(events) > 0 {
		if err := h.client.IngestEvents(ctx, events); err != nil {
			h.log.Error("Heartbeat: failed to emit compute_usage events", "error", err)
		}
	}
}

// emitActiveDeployments counts active deployments per account and emits active_deployments events.
func (h *Heartbeat) emitActiveDeployments(ctx context.Context) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT account_id, COUNT(*) AS cnt
		FROM deployments
		WHERE status = 'active'
		GROUP BY account_id
	`)
	if err != nil {
		h.log.Error("Heartbeat: failed to query deployment counts", "error", err)
		return
	}
	defer rows.Close() //nolint:errcheck

	var events []CloudEvent
	for rows.Next() {
		var accountID string
		var count int
		if err := rows.Scan(&accountID, &count); err != nil {
			h.log.Error("Heartbeat: failed to scan deployment count", "error", err)
			continue
		}
		events = append(events, NewCloudEvent("active_deployments", accountID, map[string]any{
			"count": count,
		}))
	}

	if len(events) > 0 {
		if err := h.client.IngestEvents(ctx, events); err != nil {
			h.log.Error("Heartbeat: failed to emit active_deployments events", "error", err)
		}
	}
}

// emitActiveAgents counts distinct agents per account and emits active_agents events.
func (h *Heartbeat) emitActiveAgents(ctx context.Context) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT account_id, COUNT(*) AS cnt
		FROM agents
		GROUP BY account_id
	`)
	if err != nil {
		h.log.Error("Heartbeat: failed to query agent counts", "error", err)
		return
	}
	defer rows.Close() //nolint:errcheck

	var events []CloudEvent
	for rows.Next() {
		var accountID string
		var count int
		if err := rows.Scan(&accountID, &count); err != nil {
			h.log.Error("Heartbeat: failed to scan agent count", "error", err)
			continue
		}
		events = append(events, NewCloudEvent("active_agents", accountID, map[string]any{
			"count": count,
		}))
	}

	if len(events) > 0 {
		if err := h.client.IngestEvents(ctx, events); err != nil {
			h.log.Error("Heartbeat: failed to emit active_agents events", "error", err)
		}
	}
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
	for name, t := range s.Tools {
		result = append(result, makeContainerUsage("tool/"+name, t.Resources, t.Replicas))
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
	if strings.HasSuffix(s, "m") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
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
		if strings.HasSuffix(s, sf.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSuffix(s, sf.suffix), 64)
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
