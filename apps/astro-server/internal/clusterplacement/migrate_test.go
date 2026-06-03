package clusterplacement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var deploymentFullColumns = []string{
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
	"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at", "avatar_colors",
}

func TestListDeploymentsNeedingMigration(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := deploymentstore.NewStore(db)

	eu := "eu"
	mock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("acct-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(deploymentFullColumns).
			AddRow(
				"dep-primary", "acct-1", nil, "agent-a", "build-1", "astro-dep1-0", "Agent A",
				`{"target":{"runtime":"kubernetes"}}`, nil, nil, nil,
				"active", nil, nil, time.Now(), 1,
				time.Now(), nil, nil,
			).
			AddRow(
				"dep-eu", "acct-1", nil, "agent-b", "build-2", "astro-dep2-0", "Agent B",
				`{"target":{"runtime":"kubernetes"}}`, nil, nil, eu,
				"active", nil, nil, time.Now(), 1,
				time.Now(), nil, nil,
			))

	got, err := ListDeploymentsNeedingMigration(store, "acct-1", "eu")
	if err != nil {
		t.Fatalf("ListDeploymentsNeedingMigration: %v", err)
	}
	if len(got) != 1 || got[0].ID != "dep-primary" {
		t.Fatalf("got %+v, want dep-primary only", got)
	}
}

func TestMigrateDeployment_ValidatesInput(t *testing.T) {
	m := &Migrator{Store: deploymentstore.NewStore(nil)}
	_, err := m.MigrateDeployment(context.Background(), MigrateInput{})
	if err == nil {
		t.Fatal("expected error for missing deployment_id")
	}
}

func TestMigrateDeployment_NilMigrator(t *testing.T) {
	var m *Migrator
	_, err := m.MigrateDeployment(context.Background(), MigrateInput{DeploymentID: "dep-1"})
	if err == nil {
		t.Fatal("expected error for nil migrator")
	}
}

func TestMigrateDeployment_RequiresQueueBeforeTeardown(t *testing.T) {
	store := deploymentstore.NewStore(nil)

	m := &Migrator{
		Store:    store,
		Deployer: &deployer.Deployer{Registry: k8s.NewRegistryWithPrimary(fakeClusterClient(t))},
	}
	_, err := m.MigrateDeployment(context.Background(), MigrateInput{
		DeploymentID:    "dep-1",
		TargetClusterID: "eu",
	})
	if err == nil || err.Error() != "clusterplacement: queue not configured" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type recordingDeployQueue struct {
	deploymentID string
	clusterID    string
}

func (r *recordingDeployQueue) InsertDeployJob(_ context.Context, deploymentID, clusterID string) error {
	r.deploymentID = deploymentID
	r.clusterID = clusterID
	return nil
}

func TestMigrateDeployment_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := deploymentstore.NewStore(db)
	teardownCalled := false

	client := fakeClusterClientWithHook(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			teardownCalled = true
		}
		w.WriteHeader(http.StatusOK)
	})

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows(deploymentFullColumns).
			AddRow(
				"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-ns-0", "Agent A",
				`{"target":{"runtime":"kubernetes"}}`, nil, nil, nil,
				"active", nil, nil, time.Now(), 1,
				time.Now(), nil, nil,
			))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE deployments").
		WithArgs("dep-1", "eu", sqlmock.AnyArg(), deploymentstore.StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").
		WithArgs("dep-1", deploymentstore.StatusActive, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE deployments").
		WithArgs("dep-1", deploymentstore.StatusPending, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").
		WithArgs("dep-1", deploymentstore.StatusPending, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	q := &recordingDeployQueue{}
	m := &Migrator{
		Store:    store,
		Deployer: &deployer.Deployer{Registry: k8s.NewRegistryWithPrimary(client)},
		Queue:    q,
	}
	skipped, err := m.MigrateDeployment(context.Background(), MigrateInput{
		DeploymentID:    "dep-1",
		TargetClusterID: "eu",
	})
	if err != nil {
		t.Fatalf("MigrateDeployment: %v", err)
	}
	if skipped {
		t.Fatal("expected migration to run")
	}
	if !teardownCalled {
		t.Fatal("expected teardown on source cluster before routing update")
	}
	if q.deploymentID != "dep-1" || q.clusterID != "eu" {
		t.Fatalf("deploy job = %+v, want dep-1/eu", q)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateDeployment_RecoveryWhenPendingAndAligned(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := deploymentstore.NewStore(db)
	eu := "eu"

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows(deploymentFullColumns).
			AddRow(
				"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-ns-0", "Agent A",
				`{"target":{"runtime":"kubernetes"}}`, nil, nil, eu,
				"pending", nil, nil, time.Now(), 1,
				time.Now(), nil, nil,
			))

	q := &recordingDeployQueue{}
	m := &Migrator{Store: store, Queue: q}
	skipped, err := m.MigrateDeployment(context.Background(), MigrateInput{
		DeploymentID:    "dep-1",
		TargetClusterID: "eu",
	})
	if err != nil {
		t.Fatalf("MigrateDeployment: %v", err)
	}
	if skipped {
		t.Fatal("expected deploy enqueue recovery, not skip")
	}
	if q.deploymentID != "dep-1" || q.clusterID != "eu" {
		t.Fatalf("deploy job = %+v, want dep-1/eu", q)
	}
}

func TestMigrateDeployment_ContinuesWhenSourceClusterUnavailable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := deploymentstore.NewStore(db)
	missing := "missing-cluster"

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows(deploymentFullColumns).
			AddRow(
				"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-ns-0", "Agent A",
				`{"target":{"runtime":"kubernetes"}}`, nil, nil, missing,
				"active", nil, nil, time.Now(), 1,
				time.Now(), nil, nil,
			))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE deployments").
		WithArgs("dep-1", sqlmock.AnyArg(), sqlmock.AnyArg(), deploymentstore.StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").
		WithArgs("dep-1", deploymentstore.StatusActive, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE deployments").
		WithArgs("dep-1", deploymentstore.StatusPending, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").
		WithArgs("dep-1", deploymentstore.StatusPending, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	q := &recordingDeployQueue{}
	m := &Migrator{
		Store:    store,
		Deployer: &deployer.Deployer{Registry: k8s.NewRegistryWithPrimary(fakeClusterClient(t))},
		Queue:    q,
	}
	skipped, err := m.MigrateDeployment(context.Background(), MigrateInput{
		DeploymentID:    "dep-1",
		TargetClusterID: "",
		SourceClusterID: missing,
	})
	if err != nil {
		t.Fatalf("MigrateDeployment: %v", err)
	}
	if skipped {
		t.Fatal("expected migration to continue after unavailable source cluster")
	}
	if q.deploymentID != "dep-1" {
		t.Fatalf("expected deploy enqueue, got %+v", q)
	}
}

func TestMigrateDeployment_SkipsWhenStatusChangedDuringMigration(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := deploymentstore.NewStore(db)
	teardownCalled := false

	client := fakeClusterClientWithHook(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			teardownCalled = true
		}
		w.WriteHeader(http.StatusOK)
	})

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows(deploymentFullColumns).
			AddRow(
				"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-ns-0", "Agent A",
				`{"target":{"runtime":"kubernetes"}}`, nil, nil, nil,
				"active", nil, nil, time.Now(), 1,
				time.Now(), nil, nil,
			))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE deployments").
		WithArgs("dep-1", "eu", sqlmock.AnyArg(), deploymentstore.StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	q := &recordingDeployQueue{}
	m := &Migrator{
		Store:    store,
		Deployer: &deployer.Deployer{Registry: k8s.NewRegistryWithPrimary(client)},
		Queue:    q,
	}
	skipped, err := m.MigrateDeployment(context.Background(), MigrateInput{
		DeploymentID:    "dep-1",
		TargetClusterID: "eu",
	})
	if err != nil {
		t.Fatalf("MigrateDeployment: %v", err)
	}
	if !skipped {
		t.Fatal("expected skip when status changed during migration")
	}
	if !teardownCalled {
		t.Fatal("expected teardown before guarded DB update")
	}
	if q.deploymentID != "" {
		t.Fatalf("expected no deploy enqueue, got %+v", q)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type stubClusterClient struct {
	clientset *kubernetes.Clientset
}

func (f *stubClusterClient) Clientset() *kubernetes.Clientset      { return f.clientset }
func (f *stubClusterClient) Config() *rest.Config                  { return nil }
func (f *stubClusterClient) CheckHealth() error                    { return nil }
func (f *stubClusterClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (f *stubClusterClient) DiagnoseConnection() map[string]string { return nil }

func fakeClusterClient(t *testing.T) k8s.ClusterClient {
	t.Helper()
	return fakeClusterClientWithHook(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func fakeClusterClientWithHook(t *testing.T, handler http.HandlerFunc) k8s.ClusterClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}
	return &stubClusterClient{clientset: cs}
}
