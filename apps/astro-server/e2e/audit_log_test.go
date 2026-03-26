//go:build integration

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	_ "github.com/lib/pq"
)

func TestAuditLog_InsertAndQuery(t *testing.T) {
	db := testDB(t)
	store := auditlog.NewStore(db)
	accountStore := account.NewAccountStore(db)

	acct := ensureDeleteTestAccount(t, accountStore, "audit-insert-"+deployid.New())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM audit_logs WHERE account_id = $1", acct.ID)
		_, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID)
	})

	ctx := context.Background()

	// Insert a single event
	err := store.Log(ctx, auditlog.Event{
		AccountID:    acct.ID,
		ActorID:      "user-abc",
		ActorType:    auditlog.ActorUser,
		Action:       auditlog.AccountCreate,
		ResourceType: "account",
		ResourceID:   acct.ID,
		ResourceName: acct.Name,
		Description:  "Created account " + acct.Name,
		Metadata:     map[string]any{"type": "personal"},
		IPAddress:    "127.0.0.1",
		UserAgent:    "test-agent",
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	// Query it back
	entries, err := store.Query(ctx, auditlog.QueryParams{
		AccountID: acct.ID,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.AccountID != acct.ID {
		t.Errorf("account_id = %q, want %q", e.AccountID, acct.ID)
	}
	if e.ActorID != "user-abc" {
		t.Errorf("actor_id = %q, want %q", e.ActorID, "user-abc")
	}
	if e.ActorType != auditlog.ActorUser {
		t.Errorf("actor_type = %q, want %q", e.ActorType, auditlog.ActorUser)
	}
	if e.Action != auditlog.AccountCreate {
		t.Errorf("action = %q, want %q", e.Action, auditlog.AccountCreate)
	}
	if e.ResourceType != "account" {
		t.Errorf("resource_type = %q, want %q", e.ResourceType, "account")
	}
	if e.ResourceID != acct.ID {
		t.Errorf("resource_id = %q, want %q", e.ResourceID, acct.ID)
	}
	if e.ResourceName != acct.Name {
		t.Errorf("resource_name = %q, want %q", e.ResourceName, acct.Name)
	}
	if e.Description != "Created account "+acct.Name {
		t.Errorf("description = %q, want %q", e.Description, "Created account "+acct.Name)
	}
	if e.IPAddress != "127.0.0.1" {
		t.Errorf("ip_address = %q, want %q", e.IPAddress, "127.0.0.1")
	}
	if e.UserAgent != "test-agent" {
		t.Errorf("user_agent = %q, want %q", e.UserAgent, "test-agent")
	}
	if e.Metadata == nil {
		t.Error("metadata should not be nil")
	}
}

func TestAuditLog_QueryFilters(t *testing.T) {
	db := testDB(t)
	store := auditlog.NewStore(db)
	accountStore := account.NewAccountStore(db)

	acct := ensureDeleteTestAccount(t, accountStore, "audit-filter-"+deployid.New())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM audit_logs WHERE account_id = $1", acct.ID)
		_, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID)
	})

	ctx := context.Background()

	// Insert events with different resource types, actions, and actors
	events := []auditlog.Event{
		{AccountID: acct.ID, ActorID: "user-1", ActorType: auditlog.ActorUser, Action: auditlog.AccountCreate, ResourceType: "account", ResourceID: "r1"},
		{AccountID: acct.ID, ActorID: "user-1", ActorType: auditlog.ActorUser, Action: auditlog.MemberAdd, ResourceType: "member", ResourceID: "r2"},
		{AccountID: acct.ID, ActorID: "user-2", ActorType: auditlog.ActorUser, Action: auditlog.DeploymentDeploy, ResourceType: "deployment", ResourceID: "r3"},
		{AccountID: acct.ID, ActorID: "admin:grpc", ActorType: auditlog.ActorAdmin, Action: auditlog.DeploymentStop, ResourceType: "deployment", ResourceID: "r4"},
	}
	for _, e := range events {
		if err := store.Log(ctx, e); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	// Filter by resource_type
	entries, err := store.Query(ctx, auditlog.QueryParams{
		AccountID:    acct.ID,
		ResourceType: "deployment",
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("Query resource_type: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("resource_type filter: expected 2 entries, got %d", len(entries))
	}

	// Filter by actor_id
	entries, err = store.Query(ctx, auditlog.QueryParams{
		AccountID: acct.ID,
		ActorID:   "user-1",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Query actor_id: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("actor_id filter: expected 2 entries, got %d", len(entries))
	}

	// Filter by action
	entries, err = store.Query(ctx, auditlog.QueryParams{
		AccountID: acct.ID,
		Action:    auditlog.DeploymentStop,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Query action: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("action filter: expected 1 entry, got %d", len(entries))
	}

	// Filter by resource_id
	entries, err = store.Query(ctx, auditlog.QueryParams{
		AccountID:  acct.ID,
		ResourceID: "r3",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("Query resource_id: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("resource_id filter: expected 1 entry, got %d", len(entries))
	}
}

func TestAuditLog_CursorPagination(t *testing.T) {
	db := testDB(t)
	store := auditlog.NewStore(db)
	accountStore := account.NewAccountStore(db)

	acct := ensureDeleteTestAccount(t, accountStore, "audit-page-"+deployid.New())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM audit_logs WHERE account_id = $1", acct.ID)
		_, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID)
	})

	ctx := context.Background()

	// Insert 5 events with small delays to ensure distinct timestamps
	for i := 0; i < 5; i++ {
		err := store.Log(ctx, auditlog.Event{
			AccountID:    acct.ID,
			ActorID:      "user-page",
			ActorType:    auditlog.ActorUser,
			Action:       auditlog.AgentRegister,
			ResourceType: "agent",
			ResourceID:   deployid.New(),
			Description:  "event",
		})
		if err != nil {
			t.Fatalf("Log %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Page 1: limit 2 (store fetches limit+1 internally)
	page1, err := store.Query(ctx, auditlog.QueryParams{
		AccountID: acct.ID,
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("Query page1: %v", err)
	}
	// Store returns limit+1 to detect has_more; caller trims
	if len(page1) != 3 {
		t.Fatalf("page1: expected 3 raw entries (limit+1), got %d", len(page1))
	}

	// Use the 2nd entry's created_at as cursor for next page
	cursor := page1[1].CreatedAt
	page2, err := store.Query(ctx, auditlog.QueryParams{
		AccountID: acct.ID,
		Before:    &cursor,
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("Query page2: %v", err)
	}
	if len(page2) != 3 {
		t.Fatalf("page2: expected 3 raw entries, got %d", len(page2))
	}

	// Page 3: should have 1 remaining entry
	cursor2 := page2[1].CreatedAt
	page3, err := store.Query(ctx, auditlog.QueryParams{
		AccountID: acct.ID,
		Before:    &cursor2,
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("Query page3: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("page3: expected 1 entry, got %d", len(page3))
	}

	// Verify ordering: entries should be newest first
	all, _ := store.Query(ctx, auditlog.QueryParams{AccountID: acct.ID, Limit: 10})
	for i := 1; i < len(all); i++ {
		if all[i].CreatedAt.After(all[i-1].CreatedAt) {
			t.Errorf("entries not in descending order: entry[%d].created_at=%v > entry[%d].created_at=%v",
				i, all[i].CreatedAt, i-1, all[i-1].CreatedAt)
		}
	}
}

func TestAuditLog_LogAsync(t *testing.T) {
	db := testDB(t)
	store := auditlog.NewStore(db)
	accountStore := account.NewAccountStore(db)
	log := logger.New("error", "json")

	acct := ensureDeleteTestAccount(t, accountStore, "audit-async-"+deployid.New())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM audit_logs WHERE account_id = $1", acct.ID)
		_, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID)
	})

	store.LogAsync(log, auditlog.Event{
		AccountID:    acct.ID,
		ActorID:      "user-async",
		ActorType:    auditlog.ActorUser,
		Action:       auditlog.AccountRename,
		ResourceType: "account",
		ResourceID:   acct.ID,
		Description:  "Async test event",
	})

	// Wait for the goroutine to complete
	time.Sleep(100 * time.Millisecond)

	entries, err := store.Query(context.Background(), auditlog.QueryParams{
		AccountID: acct.ID,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 async entry, got %d", len(entries))
	}
	if entries[0].Action != auditlog.AccountRename {
		t.Errorf("action = %q, want %q", entries[0].Action, auditlog.AccountRename)
	}
}

func TestAuditLog_NilStoreSafe(t *testing.T) {
	var store *auditlog.Store
	log := logger.New("error", "json")

	// Should not panic
	store.LogAsync(log, auditlog.Event{
		AccountID:    "test",
		ActorID:      "user-nil",
		ActorType:    auditlog.ActorUser,
		Action:       auditlog.AccountCreate,
		ResourceType: "account",
		ResourceID:   "test",
	})
}

func TestAuditLog_AccountIsolation(t *testing.T) {
	db := testDB(t)
	store := auditlog.NewStore(db)
	accountStore := account.NewAccountStore(db)

	acct1 := ensureDeleteTestAccount(t, accountStore, "audit-iso1-"+deployid.New())
	acct2 := ensureDeleteTestAccount(t, accountStore, "audit-iso2-"+deployid.New())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM audit_logs WHERE account_id IN ($1, $2)", acct1.ID, acct2.ID)
		_, _ = db.Exec("DELETE FROM accounts WHERE id IN ($1, $2)", acct1.ID, acct2.ID)
	})

	ctx := context.Background()

	// Insert events for each account
	_ = store.Log(ctx, auditlog.Event{AccountID: acct1.ID, ActorID: "u1", ActorType: auditlog.ActorUser, Action: auditlog.AccountCreate, ResourceType: "account", ResourceID: "r1"})
	_ = store.Log(ctx, auditlog.Event{AccountID: acct1.ID, ActorID: "u1", ActorType: auditlog.ActorUser, Action: auditlog.MemberAdd, ResourceType: "member", ResourceID: "r2"})
	_ = store.Log(ctx, auditlog.Event{AccountID: acct2.ID, ActorID: "u2", ActorType: auditlog.ActorUser, Action: auditlog.AgentRegister, ResourceType: "agent", ResourceID: "r3"})

	// Query account 1 — should see only its 2 entries
	entries1, err := store.Query(ctx, auditlog.QueryParams{AccountID: acct1.ID, Limit: 10})
	if err != nil {
		t.Fatalf("Query acct1: %v", err)
	}
	if len(entries1) != 2 {
		t.Errorf("acct1: expected 2 entries, got %d", len(entries1))
	}

	// Query account 2 — should see only its 1 entry
	entries2, err := store.Query(ctx, auditlog.QueryParams{AccountID: acct2.ID, Limit: 10})
	if err != nil {
		t.Fatalf("Query acct2: %v", err)
	}
	if len(entries2) != 1 {
		t.Errorf("acct2: expected 1 entry, got %d", len(entries2))
	}
}

func TestAuditLog_MetadataJsonb(t *testing.T) {
	db := testDB(t)
	store := auditlog.NewStore(db)
	accountStore := account.NewAccountStore(db)

	acct := ensureDeleteTestAccount(t, accountStore, "audit-meta-"+deployid.New())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM audit_logs WHERE account_id = $1", acct.ID)
		_, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID)
	})

	ctx := context.Background()

	// Insert with nested metadata (before/after pattern)
	err := store.Log(ctx, auditlog.Event{
		AccountID:    acct.ID,
		ActorID:      "user-meta",
		ActorType:    auditlog.ActorUser,
		Action:       auditlog.AccountRename,
		ResourceType: "account",
		ResourceID:   acct.ID,
		Metadata: map[string]any{
			"before": map[string]any{"name": "old-name"},
			"after":  map[string]any{"name": "new-name"},
		},
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, _ := store.Query(ctx, auditlog.QueryParams{AccountID: acct.ID, Limit: 1})
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Metadata == nil {
		t.Fatal("metadata should not be nil")
	}
	// Metadata is stored as json.RawMessage — verify it contains expected keys
	raw := string(entries[0].Metadata)
	if !containsSubstr(raw, "old-name") || !containsSubstr(raw, "new-name") {
		t.Errorf("metadata doesn't contain expected values: %s", raw)
	}
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstr(s, sub)
}

func searchSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
