package deploymentstore

import (
	"encoding/json"
	"testing"
)

func TestGetDeploymentEvents_AfterStatusUpdates(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "events-agent",
		DisplayName: "Events", BuildID: "build-1", Namespace: "ns-events",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// SaveDeploymentPending creates an initial "pending" event.
	// Add more status transitions.
	if err := store.UpdateStatus(d.ID, StatusUpdate{Status: StatusProvisioning}); err != nil {
		t.Fatalf("UpdateStatus to provisioning: %v", err)
	}
	if err := store.UpdateStatus(d.ID, StatusUpdate{Status: StatusActive}); err != nil {
		t.Fatalf("UpdateStatus to active: %v", err)
	}

	events, err := store.GetDeploymentEvents(d.ID, 100)
	if err != nil {
		t.Fatalf("GetDeploymentEvents failed: %v", err)
	}

	// Expect 3 events: pending (from save), provisioning, active
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Newest first
	if events[0].Status != StatusActive {
		t.Errorf("expected newest event status 'active', got %q", events[0].Status)
	}
	if events[1].Status != StatusProvisioning {
		t.Errorf("expected second event status 'provisioning', got %q", events[1].Status)
	}
	if events[2].Status != StatusPending {
		t.Errorf("expected oldest event status 'pending', got %q", events[2].Status)
	}

	// All events should reference the correct deployment
	for _, e := range events {
		if e.DeploymentID != d.ID {
			t.Errorf("event deployment_id mismatch: got %q, want %q", e.DeploymentID, d.ID)
		}
	}
}

func TestGetDeploymentEvents_Limit(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "events-limit",
		DisplayName: "Limit", BuildID: "build-1", Namespace: "ns-limit",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// Add several status transitions
	if err := store.UpdateStatus(d.ID, StatusUpdate{Status: StatusProvisioning}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := store.UpdateStatus(d.ID, StatusUpdate{Status: StatusActive}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := store.UpdateStatus(d.ID, StatusUpdate{Status: StatusFailed, ErrorMsg: "something broke", ErrorDetails: json.RawMessage(`{"code":"ERR"}`)}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	// Request only 2 most recent
	events, err := store.GetDeploymentEvents(d.ID, 2)
	if err != nil {
		t.Fatalf("GetDeploymentEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events with limit=2, got %d", len(events))
	}
	if events[0].Status != StatusFailed {
		t.Errorf("expected newest event 'failed', got %q", events[0].Status)
	}
	if events[0].Message != "something broke" {
		t.Errorf("expected error message 'something broke', got %q", events[0].Message)
	}
}

func TestGetDeploymentEvents_EmptyForNonexistent(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	events, err := store.GetDeploymentEvents("nonexistent-deploy-id", 100)
	if err != nil {
		t.Fatalf("GetDeploymentEvents failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events for nonexistent deployment, got %d", len(events))
	}
}
