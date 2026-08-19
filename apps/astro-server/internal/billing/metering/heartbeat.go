package metering

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

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
// only: deployment compute. Resource counts are served from the quota DB and
// are no longer metered.
func (h *Heartbeat) Tick(ctx context.Context) {
	h.log.Debug("metering: tick starting")
	h.emitComputeUsage(ctx)
	h.log.Debug("metering: tick complete")
}

// activeDeploymentRow represents a row from the active deployments query.
// emitComputeUsage delegates to the delta-free window billing in
// BillingStateManager. A nil manager means no provider is configured, so there
// is nothing to meter into.
func (h *Heartbeat) emitComputeUsage(ctx context.Context) {
	if h.billing == nil {
		return
	}
	h.billing.RunBillingCycle(ctx)
}

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
