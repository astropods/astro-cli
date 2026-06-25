package admingrpc

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

var deploymentFullColumns = []string{
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
	"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
}

func expectEnabledClusterGet(mock sqlmock.Sqlmock, clusterID string) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM clusters").
		WithArgs(clusterID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca", "enabled",
			"agent_ingress_domain", "agent_acm_certificate_arn", "agent_alb_group_name",
			"ingestion_ingress_domain", "ingestion_acm_certificate_arn", "ingestion_alb_group_name",
			"knowledge_domain",
			"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs",
			"created_at", "updated_at",
		}).AddRow(
			clusterID, "eu-west-1", "eks-"+clusterID, "https://eks.example", []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n"), true,
			"agents.example.com", "arn:acm:x", "astro",
			"ingestion.example.com", "arn:acm:y", "astro-ingest",
			"knowledge.example.com",
			"http://langfuse.example:3000", "10.0.1.10,10.0.2.10", "10.0.0.0/24,10.1.0.0/24",
			now, now,
		))
}

func expectAccountGetByID(mock sqlmock.Sqlmock, accountID string, clusterID string) {
	now := time.Now()
	mock.ExpectQuery("SELECT a.id, a.name, a.type").
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRowWithCluster(accountID, "test-org", "organization", nil, nil, now, now, clusterID)...))
}

func expectDeploymentsNeedingMigration(mock sqlmock.Sqlmock, accountID string, rows ...[]any) {
	result := sqlmock.NewRows(deploymentFullColumns)
	for _, row := range rows {
		vals := make([]driver.Value, len(row))
		for i, v := range row {
			vals[i] = v
		}
		result.AddRow(vals...)
	}
	mock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs(accountID, sqlmock.AnyArg()).
		WillReturnRows(result)
}

type mockAdminJobQueue struct {
	migrateCalls []migrateQueueCall
	failOnCall   int
	failErr      error
}

type migrateQueueCall struct {
	deploymentID    string
	targetClusterID string
	sourceClusterID string
}

func (m *mockAdminJobQueue) InsertUndeployJob(context.Context, string, string) error { return nil }
func (m *mockAdminJobQueue) InsertWakeUpJob(context.Context, string, string) error   { return nil }
func (m *mockAdminJobQueue) InsertDeployJob(context.Context, string, string) error   { return nil }
func (m *mockAdminJobQueue) InsertOpenMeterBackfillJob(context.Context) error        { return nil }
func (m *mockAdminJobQueue) TriggerJob(context.Context, string, json.RawMessage) (int64, error) {
	return 0, nil
}
func (m *mockAdminJobQueue) CancelJob(context.Context, int64) error        { return nil }
func (m *mockAdminJobQueue) RetryJob(context.Context, int64) (bool, error) { return true, nil }
func (m *mockAdminJobQueue) PauseQueue(context.Context, string) error      { return nil }
func (m *mockAdminJobQueue) ResumeQueue(context.Context, string) error     { return nil }

func (m *mockAdminJobQueue) InsertMigrateDeploymentClusterJob(_ context.Context, deploymentID, targetClusterID, sourceClusterID string) error {
	m.migrateCalls = append(m.migrateCalls, migrateQueueCall{deploymentID, targetClusterID, sourceClusterID})
	if m.failOnCall > 0 && len(m.migrateCalls) == m.failOnCall {
		if m.failErr != nil {
			return m.failErr
		}
		return errors.New("enqueue failed")
	}
	return nil
}

func TestSetAccountCluster_Clear(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		db:          db,
		deployStore: deploymentstore.NewStore(db),
		log:         logger.New("error", "json"),
	}

	expectAccountGetByID(mock, "acct-1", "eu")
	expectDeploymentsNeedingMigration(mock, "acct-1")

	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs(sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	resp, err := s.SetAccountCluster(context.Background(), &adminv1.SetAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "",
	})
	if err != nil {
		t.Fatalf("SetAccountCluster: %v", err)
	}
	if resp.MigrationsEnqueued != 0 {
		t.Fatalf("migrations_enqueued = %d, want 0", resp.MigrationsEnqueued)
	}
}

func TestSetAccountCluster_NoOpSameCluster(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db, log: logger.New("error", "json")}

	expectAccountGetByID(mock, "acct-1", "")
	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs(sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	resp, err := s.SetAccountCluster(context.Background(), &adminv1.SetAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "",
	})
	if err != nil {
		t.Fatalf("SetAccountCluster: %v", err)
	}
	if resp.MigrationsEnqueued != 0 {
		t.Fatalf("migrations_enqueued = %d, want 0 for unchanged cluster", resp.MigrationsEnqueued)
	}
}

func TestSetAccountCluster_DisabledCluster(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	cs := clusterstore.New(db)
	s := &Server{db: db, clusterStore: cs, log: logger.New("error", "json")}

	now := "2026-05-19T00:00:00Z"
	mock.ExpectQuery("SELECT .+ FROM clusters").
		WithArgs("staging-disabled").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "enabled", "created_at", "updated_at",
		}).AddRow("staging-disabled", "us-east-1", "eks", "https://eks.example", false, now, now))

	_, err = s.SetAccountCluster(context.Background(), &adminv1.SetAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "staging-disabled",
	})
	if err == nil {
		t.Fatal("expected error for disabled cluster")
	}
}

func TestSetAccountCluster_MigrationRequiresQueue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		db:           db,
		clusterStore: clusterstore.New(db),
		deployStore:  deploymentstore.NewStore(db),
		log:          logger.New("error", "json"),
	}

	expectEnabledClusterGet(mock, "eu")
	expectAccountGetByID(mock, "acct-1", "")
	expectDeploymentsNeedingMigration(mock, "acct-1",
		[]any{
			"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-dep1-0", "Agent A",
			`{"target":{"runtime":"kubernetes"}}`, nil, nil, nil,
			"active", nil, nil, time.Now(), 1,
			time.Now(), nil, nil, nil,
		},
	)

	_, err = s.SetAccountCluster(context.Background(), &adminv1.SetAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "eu",
	})
	if err == nil {
		t.Fatal("expected error when queue is nil but migrations are required")
	}
}

func TestSetAccountCluster_MigrationRequiresDeployStore(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db, log: logger.New("error", "json")}

	expectAccountGetByID(mock, "acct-1", "")

	_, err = s.SetAccountCluster(context.Background(), &adminv1.SetAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "eu",
	})
	if err == nil {
		t.Fatal("expected error when deployStore is nil but cluster is changing")
	}
}

func TestSetAccountCluster_EnqueuesBeforeAccountUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	q := &mockAdminJobQueue{}
	s := &Server{
		db:           db,
		clusterStore: clusterstore.New(db),
		deployStore:  deploymentstore.NewStore(db),
		queue:        q,
		log:          logger.New("error", "json"),
	}

	expectEnabledClusterGet(mock, "eu")
	expectAccountGetByID(mock, "acct-1", "")
	expectDeploymentsNeedingMigration(mock, "acct-1",
		[]any{
			"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-dep1-0", "Agent A",
			`{"target":{"runtime":"kubernetes"}}`, nil, nil, nil,
			"active", nil, nil, time.Now(), 1,
			time.Now(), nil, nil, nil,
		},
		[]any{
			"dep-2", "acct-1", nil, "agent-b", "build-2", "astro-dep2-0", "Agent B",
			`{"target":{"runtime":"kubernetes"}}`, nil, nil, nil,
			"failed", nil, nil, time.Now(), 1,
			time.Now(), nil, nil, nil,
		},
	)
	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs("eu", "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	resp, err := s.SetAccountCluster(context.Background(), &adminv1.SetAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "eu",
	})
	if err != nil {
		t.Fatalf("SetAccountCluster: %v", err)
	}
	if resp.MigrationsEnqueued != 2 {
		t.Fatalf("migrations_enqueued = %d, want 2", resp.MigrationsEnqueued)
	}
	if len(q.migrateCalls) != 2 {
		t.Fatalf("migrate calls = %d, want 2", len(q.migrateCalls))
	}
	if q.migrateCalls[0].deploymentID != "dep-1" || q.migrateCalls[0].targetClusterID != "eu" {
		t.Fatalf("unexpected first migrate call: %+v", q.migrateCalls[0])
	}
}

func TestSetAccountCluster_SetClusterIDFailureAfterEnqueue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	q := &mockAdminJobQueue{}
	s := &Server{
		db:           db,
		clusterStore: clusterstore.New(db),
		deployStore:  deploymentstore.NewStore(db),
		queue:        q,
		log:          logger.New("error", "json"),
	}

	expectEnabledClusterGet(mock, "eu")
	expectAccountGetByID(mock, "acct-1", "")
	expectDeploymentsNeedingMigration(mock, "acct-1",
		[]any{
			"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-dep1-0", "Agent A",
			`{"target":{"runtime":"kubernetes"}}`, nil, nil, nil,
			"active", nil, nil, time.Now(), 1,
			time.Now(), nil, nil, nil,
		},
	)
	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs("eu", "acct-1").
		WillReturnError(errors.New("connection reset"))

	_, err = s.SetAccountCluster(context.Background(), &adminv1.SetAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "eu",
	})
	if err == nil {
		t.Fatal("expected error when SetClusterID fails after enqueue")
	}
	errMsg := err.Error()
	for _, part := range []string{
		"enqueued 1 migration job",
		"retry SetAccountCluster",
		"avoid ReapplyDeployment",
		"connection reset",
	} {
		if !strings.Contains(errMsg, part) {
			t.Fatalf("error %q missing %q", errMsg, part)
		}
	}
	if len(q.migrateCalls) != 1 {
		t.Fatalf("migrate calls = %d, want 1", len(q.migrateCalls))
	}
}

func TestSetAccountCluster_EnqueueFailureLeavesAccountUnchanged(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	q := &mockAdminJobQueue{failOnCall: 2}
	s := &Server{
		db:           db,
		clusterStore: clusterstore.New(db),
		deployStore:  deploymentstore.NewStore(db),
		queue:        q,
		log:          logger.New("error", "json"),
	}

	expectEnabledClusterGet(mock, "eu")
	expectAccountGetByID(mock, "acct-1", "")
	expectDeploymentsNeedingMigration(mock, "acct-1",
		[]any{
			"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-dep1-0", "Agent A",
			`{"target":{"runtime":"kubernetes"}}`, nil, nil, nil,
			"active", nil, nil, time.Now(), 1,
			time.Now(), nil, nil, nil,
		},
		[]any{
			"dep-2", "acct-1", nil, "agent-b", "build-2", "astro-dep2-0", "Agent B",
			`{"target":{"runtime":"kubernetes"}}`, nil, nil, nil,
			"active", nil, nil, time.Now(), 1,
			time.Now(), nil, nil, nil,
		},
	)

	_, err = s.SetAccountCluster(context.Background(), &adminv1.SetAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "eu",
	})
	if err == nil {
		t.Fatal("expected error when second enqueue fails")
	}
	if len(q.migrateCalls) != 2 {
		t.Fatalf("migrate calls = %d, want 2 attempts", len(q.migrateCalls))
	}
}

func TestEnqueueAccountClusterMigrations(t *testing.T) {
	q := &mockAdminJobQueue{}
	deps := []*deploymentstore.Deployment{
		{ID: "dep-1", Namespace: "ns-1"},
	}
	ids, err := enqueueAccountClusterMigrations(context.Background(), q, "eu", deps)
	if err != nil {
		t.Fatalf("enqueueAccountClusterMigrations: %v", err)
	}
	if len(ids) != 1 || ids[0] != "dep-1" {
		t.Fatalf("ids = %v, want [dep-1]", ids)
	}
	if len(q.migrateCalls) != 1 || q.migrateCalls[0].targetClusterID != "eu" {
		t.Fatalf("unexpected calls: %+v", q.migrateCalls)
	}
}
