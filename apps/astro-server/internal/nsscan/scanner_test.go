package nsscan

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

func TestScan_TracksActiveDeployments(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	scanner := New(db, nil, log) // no K8s client

	// Active deployments query
	mock.ExpectQuery("SELECT .+ FROM deployments WHERE status = 'active'").
		WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "agent_name", "namespace"}).
			AddRow("dep-1", "acct-1", "agent-a", "astro-ns1").
			AddRow("dep-2", "acct-1", "agent-b", "astro-ns2"))

	// Two upserts into namespace_ownership
	mock.ExpectExec("INSERT INTO namespace_ownership").
		WithArgs("astro-ns1", "acct-1", "agent-a", "dep-1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO namespace_ownership").
		WithArgs("astro-ns2", "acct-1", "agent-b", "dep-2", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Drifted rows query
	mock.ExpectQuery("SELECT namespace FROM namespace_ownership WHERE scanned_at <").
		WillReturnRows(sqlmock.NewRows([]string{"namespace"}))

	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if result.Tracked != 2 {
		t.Errorf("expected 2 tracked, got %d", result.Tracked)
	}
	if len(result.Orphaned) != 0 {
		t.Errorf("expected 0 orphaned without K8s, got %d", len(result.Orphaned))
	}
	if len(result.Stale) != 0 {
		t.Errorf("expected 0 stale without K8s, got %d", len(result.Stale))
	}
}

func TestScan_DetectsDriftedRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	scanner := New(db, nil, log)

	// No active deployments
	mock.ExpectQuery("SELECT .+ FROM deployments WHERE status = 'active'").
		WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "agent_name", "namespace"}))

	// One drifted row (old scanned_at, no longer in deployments)
	mock.ExpectQuery("SELECT namespace FROM namespace_ownership WHERE scanned_at <").
		WillReturnRows(sqlmock.NewRows([]string{"namespace"}).AddRow("astro-old-ns"))

	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if result.Tracked != 0 {
		t.Errorf("expected 0 tracked, got %d", result.Tracked)
	}
	if len(result.Drifted) != 1 || result.Drifted[0] != "astro-old-ns" {
		t.Errorf("expected 1 drifted (astro-old-ns), got %v", result.Drifted)
	}
}

func TestScan_RunsHooks(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	scanner := New(db, nil, log)

	hookCalled := false
	scanner.AddHook(func(ctx context.Context, result *ScanResult) error {
		hookCalled = true
		return nil
	})

	mock.ExpectQuery("SELECT .+ FROM deployments WHERE status = 'active'").
		WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "agent_name", "namespace"}))
	mock.ExpectQuery("SELECT namespace FROM namespace_ownership WHERE scanned_at <").
		WillReturnRows(sqlmock.NewRows([]string{"namespace"}))

	_, err = scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if !hookCalled {
		t.Error("expected hook to be called")
	}
}

func TestScan_NoActiveDeployments(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := logger.New("error", "json")
	scanner := New(db, nil, log)

	mock.ExpectQuery("SELECT .+ FROM deployments WHERE status = 'active'").
		WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "agent_name", "namespace"}))
	mock.ExpectQuery("SELECT namespace FROM namespace_ownership WHERE scanned_at <").
		WillReturnRows(sqlmock.NewRows([]string{"namespace"}))

	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if result.Tracked != 0 {
		t.Errorf("expected 0 tracked, got %d", result.Tracked)
	}
}
