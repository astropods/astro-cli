//go:build integration

package deploymentstore

import (
	"testing"
)

func TestUpdateStatusIfCurrent(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	dep, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "cas-agent",
		DisplayName: "CAS", BuildID: "build-1", Namespace: "ns-cas",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}
	if err := store.UpdateStatus(dep.ID, StatusUpdate{Status: StatusDeploying}); err != nil {
		t.Fatalf("UpdateStatus deploying: %v", err)
	}

	// deploying → active applies (current is in allowed set).
	applied, err := store.UpdateStatusIfCurrent(dep.ID, StatusUpdate{Status: StatusActive}, StatusDeploying)
	if err != nil || !applied {
		t.Fatalf("expected deploying→active to apply, applied=%v err=%v", applied, err)
	}

	// Simulate a concurrent stop: status is now active, not deploying.
	// A controller trying to fail it "from deploying" must be a no-op.
	applied, err = store.UpdateStatusIfCurrent(dep.ID, StatusUpdate{Status: StatusFailed}, StatusDeploying)
	if err != nil {
		t.Fatalf("CAS err: %v", err)
	}
	if applied {
		t.Error("expected no-op when current status not in allowedCurrent")
	}
	cur, _ := store.GetDeploymentByID(dep.ID)
	if cur.Status != StatusActive {
		t.Errorf("status should remain active, got %q", cur.Status)
	}

	// active → failed applies when active is allowed (post-deploy regression).
	applied, err = store.UpdateStatusIfCurrent(dep.ID, StatusUpdate{Status: StatusFailed, ErrorMsg: "regressed"}, StatusDeploying, StatusActive)
	if err != nil || !applied {
		t.Fatalf("expected active→failed to apply, applied=%v err=%v", applied, err)
	}
	cur, _ = store.GetDeploymentByID(dep.ID)
	if cur.Status != StatusFailed {
		t.Errorf("status should be failed, got %q", cur.Status)
	}
}

func TestReplaceAndGetWorkloadStatuses(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	dep, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "ws-agent",
		DisplayName: "WS", BuildID: "build-1", Namespace: "ns-ws",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	// Empty on a fresh deployment.
	got, err := store.GetWorkloadStatuses(dep.ID)
	if err != nil {
		t.Fatalf("GetWorkloadStatuses (empty): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 statuses initially, got %d", len(got))
	}

	// Replace with two workloads.
	err = store.ReplaceWorkloadStatuses(dep.ID, []WorkloadStatus{
		{WorkloadName: "ws-agent", WorkloadType: "deployment", Phase: WorkloadPhaseReady,
			ObservedReady: 2, ObservedDesired: 2, ObservedGeneration: 3},
		{WorkloadName: "ws-cache", WorkloadType: "statefulset", Phase: WorkloadPhaseProgressing,
			Reason: "ContainerCreating", Message: "0/1 ready", ObservedReady: 0, ObservedDesired: 1},
	})
	if err != nil {
		t.Fatalf("ReplaceWorkloadStatuses: %v", err)
	}

	got, err = store.GetWorkloadStatuses(dep.ID)
	if err != nil {
		t.Fatalf("GetWorkloadStatuses: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(got))
	}
	// Ordered by workload_name: ws-agent, ws-cache.
	if got[0].WorkloadName != "ws-agent" || got[0].Phase != WorkloadPhaseReady || got[0].ObservedReady != 2 {
		t.Errorf("unexpected first row: %+v", got[0])
	}
	if got[1].WorkloadName != "ws-cache" || got[1].Phase != WorkloadPhaseProgressing || got[1].Reason != "ContainerCreating" {
		t.Errorf("unexpected second row: %+v", got[1])
	}
	if got[0].ObservedAt.IsZero() {
		t.Error("observed_at should be stamped by DB default")
	}

	// Replace prunes stale rows: down to a single workload.
	err = store.ReplaceWorkloadStatuses(dep.ID, []WorkloadStatus{
		{WorkloadName: "ws-agent", WorkloadType: "deployment", Phase: WorkloadPhaseFailed,
			Reason: "ImagePullBackOff", ObservedReady: 0, ObservedDesired: 2},
	})
	if err != nil {
		t.Fatalf("ReplaceWorkloadStatuses (prune): %v", err)
	}
	got, err = store.GetWorkloadStatuses(dep.ID)
	if err != nil {
		t.Fatalf("GetWorkloadStatuses (after prune): %v", err)
	}
	if len(got) != 1 || got[0].WorkloadName != "ws-agent" || got[0].Phase != WorkloadPhaseFailed {
		t.Fatalf("expected 1 failed ws-agent row after prune, got %+v", got)
	}

	// Empty slice clears all rows.
	if err := store.ReplaceWorkloadStatuses(dep.ID, nil); err != nil {
		t.Fatalf("ReplaceWorkloadStatuses (clear): %v", err)
	}
	got, err = store.GetWorkloadStatuses(dep.ID)
	if err != nil {
		t.Fatalf("GetWorkloadStatuses (after clear): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 statuses after clear, got %d", len(got))
	}
}
