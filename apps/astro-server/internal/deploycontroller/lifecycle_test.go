package deploycontroller

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing/metering"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// fakeBiller records deployment ids passed to StartBilling on a channel so tests
// can assert whether billing was (or wasn't) kicked off — startBilling detaches
// the call to a goroutine, so the channel is the synchronization point.
type fakeBiller struct {
	started chan string
}

func (b *fakeBiller) StartBilling(_ context.Context, deploymentID string, _ []metering.WorkloadInfo) {
	b.started <- deploymentID
}

// lifecycleStore is a minimal Store that records the transition driveLifecycle
// attempts. It only implements the methods driveLifecycle touches.
type lifecycleStore struct {
	workloads []*deploymentstore.Workload

	lastUpdate   *deploymentstore.StatusUpdate
	allowed      []string
	applied      bool
	updateCalled bool
}

func (s *lifecycleStore) GetWorkloads(string) ([]*deploymentstore.Workload, error) {
	return s.workloads, nil
}

func (s *lifecycleStore) UpdateStatusIfCurrent(_ string, u deploymentstore.StatusUpdate, allowedCurrent ...string) (bool, error) {
	s.updateCalled = true
	s.lastUpdate = &u
	s.allowed = allowedCurrent
	return s.applied, nil
}

// Unused Store methods for these tests.
func (s *lifecycleStore) GetLatestDeploymentByNamespace(string) (*deploymentstore.Deployment, error) {
	return nil, nil
}
func (s *lifecycleStore) ReplaceWorkloadStatuses(string, []deploymentstore.WorkloadStatus) error {
	return nil
}
func (s *lifecycleStore) UpsertRuntimeSnapshot(string, deploymentstore.RuntimeSnapshot) error {
	return nil
}
func (s *lifecycleStore) DeleteRuntimeSnapshot(string) error { return nil }

func TestDriveLifecycle(t *testing.T) {
	expected := []*deploymentstore.Workload{{Name: "agent"}}
	ready := []deploymentstore.WorkloadStatus{{WorkloadName: "agent", Phase: deploymentstore.WorkloadPhaseReady}}
	failed := []deploymentstore.WorkloadStatus{{WorkloadName: "agent", Phase: deploymentstore.WorkloadPhaseFailed, Reason: "CrashLoopBackOff"}}
	progressing := []deploymentstore.WorkloadStatus{{WorkloadName: "agent", Phase: deploymentstore.WorkloadPhaseProgressing}}

	tests := []struct {
		name       string
		status     string
		observed   []deploymentstore.WorkloadStatus
		wantUpdate bool
		wantStatus string
	}{
		{"deploying + ready → active", deploymentstore.StatusDeploying, ready, true, deploymentstore.StatusActive},
		{"failed + ready → active (recovery)", deploymentstore.StatusFailed, ready, true, deploymentstore.StatusActive},
		{"failed + still failed → no transition", deploymentstore.StatusFailed, failed, false, ""},
		{"failed + progressing → no transition (wait for ready)", deploymentstore.StatusFailed, progressing, false, ""},
		{"active + failed → failed (regression)", deploymentstore.StatusActive, failed, true, deploymentstore.StatusFailed},
		{"deploying + failed → failed", deploymentstore.StatusDeploying, failed, true, deploymentstore.StatusFailed},
		{"stopped → hands-off", deploymentstore.StatusStopped, ready, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &lifecycleStore{workloads: expected, applied: true}
			c := &Controller{store: store, log: logger.New("error", "text")}
			dep := &deploymentstore.Deployment{ID: "dep-1", Status: tt.status}

			if err := c.driveLifecycle(dep, tt.observed); err != nil {
				t.Fatalf("driveLifecycle: %v", err)
			}
			if store.updateCalled != tt.wantUpdate {
				t.Fatalf("update called = %v, want %v", store.updateCalled, tt.wantUpdate)
			}
			if tt.wantUpdate && store.lastUpdate.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", store.lastUpdate.Status, tt.wantStatus)
			}
		})
	}
}

// TestDriveLifecycle_RecoveryClearsError locks that recovering failed → active
// writes an empty error message so the stale failure reason is cleared.
func TestDriveLifecycle_RecoveryClearsError(t *testing.T) {
	store := &lifecycleStore{workloads: []*deploymentstore.Workload{{Name: "agent"}}, applied: true}
	c := &Controller{store: store, log: logger.New("error", "text")}
	dep := &deploymentstore.Deployment{ID: "dep-1", Status: deploymentstore.StatusFailed}

	ready := []deploymentstore.WorkloadStatus{{WorkloadName: "agent", Phase: deploymentstore.WorkloadPhaseReady}}
	if err := c.driveLifecycle(dep, ready); err != nil {
		t.Fatalf("driveLifecycle: %v", err)
	}
	if store.lastUpdate == nil || store.lastUpdate.ErrorMsg != "" {
		t.Errorf("expected cleared error message on recovery, got %+v", store.lastUpdate)
	}
	if !slices.Contains(store.allowed, deploymentstore.StatusFailed) {
		t.Errorf("failed must be an allowed current state for recovery, got %v", store.allowed)
	}
}

// TestDriveLifecycle_Billing locks that billing starts on every transition to
// active — the first deploy and a failed → active recovery — and never starts
// when the transition is a no-op.
func TestDriveLifecycle_Billing(t *testing.T) {
	ready := []deploymentstore.WorkloadStatus{{WorkloadName: "agent", Phase: deploymentstore.WorkloadPhaseReady}}
	failed := []deploymentstore.WorkloadStatus{{WorkloadName: "agent", Phase: deploymentstore.WorkloadPhaseFailed, Reason: "CrashLoopBackOff"}}

	tests := []struct {
		name        string
		status      string
		observed    []deploymentstore.WorkloadStatus
		applied     bool
		wantBilling bool
	}{
		{"first deploy → active starts billing", deploymentstore.StatusDeploying, ready, true, true},
		{"recovery failed → active starts billing", deploymentstore.StatusFailed, ready, true, true},
		{"regression active → failed does not bill", deploymentstore.StatusActive, failed, true, false},
		{"lost CAS race (concurrent stop) does not bill", deploymentstore.StatusFailed, ready, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &lifecycleStore{workloads: []*deploymentstore.Workload{{Name: "agent"}}, applied: tt.applied}
			biller := &fakeBiller{started: make(chan string, 1)}
			c := &Controller{store: store, billing: biller, log: logger.New("error", "text")}
			dep := &deploymentstore.Deployment{ID: "dep-1", Status: tt.status}

			if err := c.driveLifecycle(dep, tt.observed); err != nil {
				t.Fatalf("driveLifecycle: %v", err)
			}

			if tt.wantBilling {
				select {
				case id := <-biller.started:
					if id != "dep-1" {
						t.Errorf("billed deployment %q, want dep-1", id)
					}
				case <-time.After(time.Second):
					t.Fatal("expected billing to start")
				}
				return
			}
			select {
			case <-biller.started:
				t.Fatal("billing must not start for this transition")
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}

// TestDriveLifecycle_MultiWorkloadRecovery covers the realistic shape from the
// bug report: an agent StatefulSet plus a cache workload. Once the cache stops
// crash-looping and both read ready, the whole deployment recovers to active.
func TestDriveLifecycle_MultiWorkloadRecovery(t *testing.T) {
	expected := []*deploymentstore.Workload{{Name: "agent"}, {Name: "cache"}}

	stillCrashing := []deploymentstore.WorkloadStatus{
		{WorkloadName: "agent", Phase: deploymentstore.WorkloadPhaseReady},
		{WorkloadName: "cache", Phase: deploymentstore.WorkloadPhaseFailed, Reason: "CrashLoopBackOff"},
	}
	recovered := []deploymentstore.WorkloadStatus{
		{WorkloadName: "agent", Phase: deploymentstore.WorkloadPhaseReady},
		{WorkloadName: "cache", Phase: deploymentstore.WorkloadPhaseReady},
	}

	store := &lifecycleStore{workloads: expected, applied: true}
	c := &Controller{store: store, log: logger.New("error", "text")}
	dep := &deploymentstore.Deployment{ID: "dep-1", Status: deploymentstore.StatusFailed}

	// While the cache is still crash-looping, a failed deployment must stay failed.
	if err := c.driveLifecycle(dep, stillCrashing); err != nil {
		t.Fatalf("driveLifecycle: %v", err)
	}
	if store.updateCalled {
		t.Fatal("must not transition while a workload is still failed")
	}

	// Both ready → recover to active.
	if err := c.driveLifecycle(dep, recovered); err != nil {
		t.Fatalf("driveLifecycle: %v", err)
	}
	if !store.updateCalled || store.lastUpdate.Status != deploymentstore.StatusActive {
		t.Errorf("expected recovery to active, got %+v", store.lastUpdate)
	}
}
