//go:build integration

package deploymentstore

import (
	"testing"
	"time"
)

func TestRuntimeSnapshotRoundTrip(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	dep, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "rt-agent",
		DisplayName: "RT", BuildID: "build-1", Namespace: "ns-rt",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	// No row yet → (nil, zero, nil): the read path renders an empty runtime.
	got, _, err := store.GetRuntimeSnapshot(dep.ID)
	if err != nil {
		t.Fatalf("GetRuntimeSnapshot (empty): %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil snapshot before any write, got %+v", got)
	}

	snap := RuntimeSnapshot{
		Ready:    1,
		Replicas: 2,
		Services: []RuntimeService{{Name: "rt-agent-messaging", Type: "ClusterIP"}},
		Workloads: []RuntimeWorkload{{
			Name:      "rt-agent-agent",
			Kind:      "Deployment",
			CreatedAt: time.Now().UTC().Truncate(time.Second),
			Pods: []RuntimePod{{
				Name:    "rt-agent-agent-0",
				Phase:   "Running",
				BuildID: "build-1",
				Containers: []RuntimeContainer{
					{Name: "app", State: "Running", Ready: true, RestartCount: 3},
				},
			}},
		}},
	}
	if err := store.UpsertRuntimeSnapshot(dep.ID, snap); err != nil {
		t.Fatalf("UpsertRuntimeSnapshot: %v", err)
	}

	got, observedAt, err := store.GetRuntimeSnapshot(dep.ID)
	if err != nil {
		t.Fatalf("GetRuntimeSnapshot: %v", err)
	}
	if got == nil {
		t.Fatal("expected a snapshot after upsert")
	}
	if got.Ready != 1 || got.Replicas != 2 {
		t.Errorf("ready/replicas: got %d/%d, want 1/2", got.Ready, got.Replicas)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "rt-agent-messaging" {
		t.Errorf("services round-trip mismatch: %+v", got.Services)
	}
	if len(got.Workloads) != 1 || len(got.Workloads[0].Pods) != 1 ||
		len(got.Workloads[0].Pods[0].Containers) != 1 ||
		got.Workloads[0].Pods[0].Containers[0].RestartCount != 3 {
		t.Errorf("workloads round-trip mismatch: %+v", got.Workloads)
	}
	if observedAt.IsZero() {
		t.Error("expected non-zero observed_at")
	}

	// Upsert replaces on the primary key.
	snap.Ready = 5
	if err := store.UpsertRuntimeSnapshot(dep.ID, snap); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got, _, _ = store.GetRuntimeSnapshot(dep.ID)
	if got.Ready != 5 {
		t.Errorf("expected replace to update ready to 5, got %d", got.Ready)
	}

	// Delete clears it.
	if err := store.DeleteRuntimeSnapshot(dep.ID); err != nil {
		t.Fatalf("DeleteRuntimeSnapshot: %v", err)
	}
	got, _, err = store.GetRuntimeSnapshot(dep.ID)
	if err != nil {
		t.Fatalf("GetRuntimeSnapshot after delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil snapshot after delete")
	}
}
