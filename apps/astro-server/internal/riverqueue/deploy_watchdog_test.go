package riverqueue

import (
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// TestStaleStatusRules_OnlyInProgress guards the single most dangerous mistake:
// the watchdog flips its target statuses to failed, so it must only ever target
// non-terminal in-progress statuses the deploy pipeline owns. Sweeping active
// would fail healthy deployments; sweeping a terminal status is pointless.
func TestStaleStatusRules_OnlyInProgress(t *testing.T) {
	allowed := map[string]bool{
		deploymentstore.StatusPending:      true,
		deploymentstore.StatusProvisioning: true,
		deploymentstore.StatusDeploying:    true,
	}
	seen := map[string]bool{}
	for _, r := range staleStatusRules {
		if !allowed[r.status] {
			t.Errorf("watchdog rule targets non-in-progress status %q — it would fail healthy or terminal deployments", r.status)
		}
		if seen[r.status] {
			t.Errorf("duplicate watchdog rule for status %q", r.status)
		}
		seen[r.status] = true
		if r.deadline <= 0 {
			t.Errorf("watchdog rule for %q has non-positive deadline %s", r.status, r.deadline)
		}
		if r.errMsg == "" {
			t.Errorf("watchdog rule for %q has empty error message", r.status)
		}
	}
	// deploying must be at least as generous as the K8s progress deadline it backstops.
	for _, r := range staleStatusRules {
		if r.status == deploymentstore.StatusDeploying && r.deadline < 3*time.Minute {
			t.Errorf("deploying deadline %s is tighter than the K8s progressDeadline (180s)", r.deadline)
		}
	}
}
