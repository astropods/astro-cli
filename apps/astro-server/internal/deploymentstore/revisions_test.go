package deploymentstore

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGetCurrentRevision(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "rev-current",
		DisplayName: "RevCurrent", BuildID: "build-1", Namespace: "ns-rev-cur",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	rev, err := store.GetCurrentRevision(d.ID)
	if err != nil {
		t.Fatalf("GetCurrentRevision failed: %v", err)
	}
	if rev == nil {
		t.Fatal("expected revision, got nil")
	}
	if rev.Revision != 1 {
		t.Errorf("expected revision 1, got %d", rev.Revision)
	}
	if rev.DeploymentID != d.ID {
		t.Errorf("deployment_id mismatch: got %q, want %q", rev.DeploymentID, d.ID)
	}
	if rev.BuildID != "build-1" {
		t.Errorf("build_id mismatch: got %q, want 'build-1'", rev.BuildID)
	}
	// jsonb reformats on the way out, so compare the decoded value.
	var gotSpec map[string]any
	if err := json.Unmarshal(rev.SpecJSON, &gotSpec); err != nil {
		t.Fatalf("spec_json is not valid JSON: %v", err)
	}
	if gotSpec["spec"] != "v1" {
		t.Errorf("spec_json mismatch: got %s", rev.SpecJSON)
	}
}

func TestGetCurrentRevision_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	rev, err := store.GetCurrentRevision("nonexistent-deploy-id")
	if err != nil {
		t.Fatalf("GetCurrentRevision failed: %v", err)
	}
	if rev != nil {
		t.Errorf("expected nil for nonexistent deployment, got %+v", rev)
	}
}

func TestGetRevisions(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Create initial deployment (revision 1)
	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "rev-list",
		DisplayName: "RevList", BuildID: "build-1", Namespace: "ns-rev-list",
		SpecJSON: `{"v":1}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// Mark active so UpdateDeploymentPending can redeploy
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d.ID)

	// Redeploy (revision 2)
	_, err = store.UpdateDeploymentPending(SaveDeploymentParams{
		ID: d.ID, AccountID: accountID, AgentName: "rev-list",
		DisplayName: "RevList", BuildID: "build-2", Namespace: "ns-rev-list",
		SpecJSON: `{"v":2}`,
	}, nil)
	if err != nil {
		t.Fatalf("UpdateDeploymentPending failed: %v", err)
	}

	// Mark active again, redeploy (revision 3)
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d.ID)
	_, err = store.UpdateDeploymentPending(SaveDeploymentParams{
		ID: d.ID, AccountID: accountID, AgentName: "rev-list",
		DisplayName: "RevList", BuildID: "build-3", Namespace: "ns-rev-list",
		SpecJSON: `{"v":3}`,
	}, nil)
	if err != nil {
		t.Fatalf("UpdateDeploymentPending (rev 3) failed: %v", err)
	}

	revisions, err := store.GetRevisions(d.ID)
	if err != nil {
		t.Fatalf("GetRevisions failed: %v", err)
	}
	if len(revisions) != 3 {
		t.Fatalf("expected 3 revisions, got %d", len(revisions))
	}

	// Newest first (revision DESC)
	if revisions[0].Revision != 3 {
		t.Errorf("expected newest revision 3, got %d", revisions[0].Revision)
	}
	if revisions[0].BuildID != "build-3" {
		t.Errorf("expected build-3, got %q", revisions[0].BuildID)
	}
	if revisions[1].Revision != 2 {
		t.Errorf("expected revision 2, got %d", revisions[1].Revision)
	}
	if revisions[2].Revision != 1 {
		t.Errorf("expected oldest revision 1, got %d", revisions[2].Revision)
	}
	if revisions[2].BuildID != "build-1" {
		t.Errorf("expected build-1, got %q", revisions[2].BuildID)
	}
}

func TestGetRevisions_Empty(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	revisions, err := store.GetRevisions("nonexistent-deploy-id")
	if err != nil {
		t.Fatalf("GetRevisions failed: %v", err)
	}
	if len(revisions) != 0 {
		t.Errorf("expected 0 revisions for nonexistent deployment, got %d", len(revisions))
	}
}

func TestGetDeploymentHistoryByRevisions_DeploymentActors(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)
	agentName := "history-actors-" + newID()

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: agentName,
		DisplayName: "History Actors", BuildID: "build-1", Namespace: "ns-history-actors",
		SpecJSON: `{"v":1}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM audit_logs WHERE resource_id = $1", d.ID)
	})

	insertDeployAudit := func(actorID, buildID string) {
		t.Helper()
		_, err := db.Exec(`
			INSERT INTO audit_logs (
				account_id, actor_id, actor_type, action, resource_type, resource_id, metadata
			)
			VALUES ($1, $2, 'user', 'deployment.deploy', 'deployment', $3, jsonb_build_object('build_id', $4::text))
		`, accountID, actorID, d.ID, buildID)
		if err != nil {
			t.Fatalf("failed to insert deploy audit event: %v", err)
		}
	}

	insertDeployAudit("user-first", "build-1")

	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d.ID)
	_, err = store.UpdateDeploymentPending(SaveDeploymentParams{
		ID: d.ID, AccountID: accountID, AgentName: agentName,
		DisplayName: "History Actors", BuildID: "build-2", Namespace: "ns-history-actors",
		SpecJSON: `{"v":2}`,
	}, nil)
	if err != nil {
		t.Fatalf("UpdateDeploymentPending failed: %v", err)
	}
	insertDeployAudit("user-second", "build-2")

	history, err := store.GetDeploymentHistoryByRevisions(accountID, agentName)
	if err != nil {
		t.Fatalf("GetDeploymentHistoryByRevisions failed: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history records, got %d", len(history))
	}
	if history[0].Revision != 2 || history[0].DeployedBy != "user-second" {
		t.Errorf("revision 2 actor = %q, want user-second", history[0].DeployedBy)
	}
	if history[1].Revision != 1 || history[1].DeployedBy != "user-first" {
		t.Errorf("revision 1 actor = %q, want user-first", history[1].DeployedBy)
	}
}

func TestSetCurrentRevision(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Create deployment with revision 1
	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "rev-set",
		DisplayName: "RevSet", BuildID: "build-1", Namespace: "ns-rev-set",
		SpecJSON: `{"v":1}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// Mark active, create revision 2
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d.ID)
	_, err = store.UpdateDeploymentPending(SaveDeploymentParams{
		ID: d.ID, AccountID: accountID, AgentName: "rev-set",
		DisplayName: "RevSet", BuildID: "build-2", Namespace: "ns-rev-set",
		SpecJSON: `{"v":2}`,
	}, nil)
	if err != nil {
		t.Fatalf("UpdateDeploymentPending failed: %v", err)
	}

	// Mark active, then roll back to revision 1
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d.ID)
	err = store.SetCurrentRevision(d.ID, 1, nil)
	if err != nil {
		t.Fatalf("SetCurrentRevision failed: %v", err)
	}

	// Verify deployment is now pending with current_revision=1
	dep, err := store.GetDeploymentByID(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID failed: %v", err)
	}
	if dep.Status != StatusPending {
		t.Errorf("expected status 'pending' after rollback, got %q", dep.Status)
	}
	if dep.CurrentRevision == nil || *dep.CurrentRevision != 1 {
		t.Errorf("expected current_revision=1, got %v", dep.CurrentRevision)
	}

	// Verify an event was recorded for the rollback
	events, err := store.GetDeploymentEvents(d.ID, 100)
	if err != nil {
		t.Fatalf("GetDeploymentEvents failed: %v", err)
	}
	// Find the rollback event (should be newest)
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	newest := events[0]
	if newest.Status != StatusPending {
		t.Errorf("expected rollback event status 'pending', got %q", newest.Status)
	}
	if !strings.Contains(newest.Message, "Rollback to revision 1") {
		t.Errorf("expected rollback message, got %q", newest.Message)
	}
}

func TestSetCurrentRevision_NonexistentRevision(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "rev-noexist",
		DisplayName: "RevNoExist", BuildID: "build-1", Namespace: "ns-rev-noexist",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// Try to set to revision 99 which does not exist
	err = store.SetCurrentRevision(d.ID, 99, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent revision, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}
