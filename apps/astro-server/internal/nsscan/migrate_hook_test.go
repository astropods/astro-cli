package nsscan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// legacyNamespace reproduces the old SHA256-based namespace derivation that
// was used before multi-deployment support.
func legacyNamespace(accountID, sourceAccount, agentName string) string {
	h := sha256.Sum256([]byte(accountID + ":" + sourceAccount + ":" + agentName))
	return "astro-" + hex.EncodeToString(h[:])[:20]
}

// TestMigrationHook_AdoptsLegacySHA256Namespaces verifies that the hook matches
// orphaned K8s namespaces (using the old SHA256 format) to stale DB deployments
// and updates the DB namespace to the actual K8s namespace.
func TestMigrationHook_AdoptsLegacySHA256Namespaces(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	hook := MigrationHook(db, log)

	acctID := "acct-uuid-1"
	agentName := "my-agent"
	legacyNS := legacyNamespace(acctID, acctID, agentName) // old SHA256 namespace in K8s
	newNS := "astro-abcdefghi-0"                           // new format stored in DB after schema change

	result := &ScanResult{
		Orphaned: []OrphanedNamespace{
			{Name: legacyNS, AccountID: acctID, AgentName: agentName},
		},
		Stale: []string{newNS},
		StaleDeployments: []staleDeployment{
			{ID: "abc-def-ghi", AccountID: acctID, AgentName: agentName, Namespace: newNS},
		},
	}

	// Expect the deployment namespace to be updated to the legacy K8s namespace
	mock.ExpectExec("UPDATE deployments SET namespace").
		WithArgs(legacyNS, "abc-def-ghi").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect the old namespace_ownership row to be deleted
	mock.ExpectExec("DELETE FROM namespace_ownership WHERE namespace").
		WithArgs(newNS).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect the legacy count query
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err = hook(context.Background(), result)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestMigrationHook_MultipleAgentsSameAccount verifies adoption works when
// multiple agents under the same account all have legacy namespaces.
func TestMigrationHook_MultipleAgentsSameAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	hook := MigrationHook(db, log)

	acctID := "acct-uuid-1"

	agent1 := "agent-alpha"
	agent2 := "agent-beta"
	legacy1 := legacyNamespace(acctID, acctID, agent1)
	legacy2 := legacyNamespace(acctID, acctID, agent2)

	result := &ScanResult{
		Orphaned: []OrphanedNamespace{
			{Name: legacy1, AccountID: acctID, AgentName: agent1},
			{Name: legacy2, AccountID: acctID, AgentName: agent2},
		},
		Stale: []string{"astro-aaaaaaaaa-0", "astro-bbbbbbbbb-0"},
		StaleDeployments: []staleDeployment{
			{ID: "aaa-aaa-aaa", AccountID: acctID, AgentName: agent1, Namespace: "astro-aaaaaaaaa-0"},
			{ID: "bbb-bbb-bbb", AccountID: acctID, AgentName: agent2, Namespace: "astro-bbbbbbbbb-0"},
		},
	}

	// Agent 1 adoption
	mock.ExpectExec("UPDATE deployments SET namespace").
		WithArgs(legacy1, "aaa-aaa-aaa").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM namespace_ownership WHERE namespace").
		WithArgs("astro-aaaaaaaaa-0").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Agent 2 adoption
	mock.ExpectExec("UPDATE deployments SET namespace").
		WithArgs(legacy2, "bbb-bbb-bbb").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM namespace_ownership WHERE namespace").
		WithArgs("astro-bbbbbbbbb-0").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Legacy count
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err = hook(context.Background(), result)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestMigrationHook_NoOpWhenNoOrphans verifies the hook exits early
// when there are no orphaned namespaces.
func TestMigrationHook_NoOpWhenNoOrphans(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	hook := MigrationHook(db, log)

	result := &ScanResult{
		Tracked:  5,
		Orphaned: nil,
		Stale:    []string{"astro-something-0"},
		StaleDeployments: []staleDeployment{
			{ID: "xxx-xxx-xxx", AccountID: "acct-1", AgentName: "agent-a", Namespace: "astro-something-0"},
		},
	}

	// No DB calls expected — early return
	err = hook(context.Background(), result)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestMigrationHook_NoOpWhenNoStale verifies the hook exits early
// when there are no stale deployments.
func TestMigrationHook_NoOpWhenNoStale(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	hook := MigrationHook(db, log)

	result := &ScanResult{
		Orphaned: []OrphanedNamespace{
			{Name: "astro-abc123", AccountID: "acct-1", AgentName: "agent-a"},
		},
		StaleDeployments: nil,
	}

	// No DB calls expected — early return
	err = hook(context.Background(), result)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestMigrationHook_SkipsOrphansWithoutLabels verifies that orphaned namespaces
// missing the account-id or agent labels are ignored.
func TestMigrationHook_SkipsOrphansWithoutLabels(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	hook := MigrationHook(db, log)

	result := &ScanResult{
		Orphaned: []OrphanedNamespace{
			{Name: "astro-unlabeled-ns", AccountID: "", AgentName: ""},     // no labels
			{Name: "astro-partial-ns", AccountID: "acct-1", AgentName: ""}, // missing agent
		},
		Stale: []string{"astro-something-0"},
		StaleDeployments: []staleDeployment{
			{ID: "xxx-xxx-xxx", AccountID: "acct-1", AgentName: "agent-a", Namespace: "astro-something-0"},
		},
	}

	// No adoption expected — orphans don't have sufficient labels to match.
	// Only the legacy count query runs.
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err = hook(context.Background(), result)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestMigrationHook_NoMatchBetweenOrphanAndStale verifies that when orphaned
// namespaces and stale deployments exist but don't match on (account_id, agent_name),
// no adoption occurs.
func TestMigrationHook_NoMatchBetweenOrphanAndStale(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	hook := MigrationHook(db, log)

	result := &ScanResult{
		Orphaned: []OrphanedNamespace{
			{Name: legacyNamespace("acct-1", "acct-1", "agent-x"), AccountID: "acct-1", AgentName: "agent-x"},
		},
		Stale: []string{"astro-something-0"},
		StaleDeployments: []staleDeployment{
			{ID: "xxx-xxx-xxx", AccountID: "acct-2", AgentName: "agent-y", Namespace: "astro-something-0"},
		},
	}

	// No adoption — different account/agent. Only legacy count query.
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err = hook(context.Background(), result)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestMigrationHook_CrossAccountDeploy verifies adoption works when the
// source account differs from the target account (the legacy namespace was
// derived from both).
func TestMigrationHook_CrossAccountDeploy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	hook := MigrationHook(db, log)

	targetAcctID := "target-acct-uuid"
	sourceAcctName := "source-org"
	agentName := "shared-agent"

	// The legacy namespace was derived using sourceAccount in the hash,
	// but the K8s label stores the target account ID.
	legacyNS := legacyNamespace(targetAcctID, sourceAcctName, agentName)

	result := &ScanResult{
		Orphaned: []OrphanedNamespace{
			{Name: legacyNS, AccountID: targetAcctID, AgentName: agentName},
		},
		Stale: []string{"astro-ccccccccc-0"},
		StaleDeployments: []staleDeployment{
			{ID: "ccc-ccc-ccc", AccountID: targetAcctID, AgentName: agentName, Namespace: "astro-ccccccccc-0"},
		},
	}

	mock.ExpectExec("UPDATE deployments SET namespace").
		WithArgs(legacyNS, "ccc-ccc-ccc").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM namespace_ownership WHERE namespace").
		WithArgs("astro-ccccccccc-0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err = hook(context.Background(), result)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
