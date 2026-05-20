package riverqueue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

type noopClusterClient struct {
	cs *kubernetes.Clientset
}

func (n *noopClusterClient) Clientset() *kubernetes.Clientset      { return n.cs }
func (n *noopClusterClient) Config() *rest.Config                  { return nil }
func (n *noopClusterClient) CheckHealth() error                    { return nil }
func (n *noopClusterClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (n *noopClusterClient) DiagnoseConnection() map[string]string { return nil }

func TestUndeployWorker_SkipsK8sWhenClusterClientUnavailable(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now()
	clusterID := "eu-west-1-secondary"
	depID := "dep-test-1"

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors",
		}).AddRow(
			depID, "acct-1", nil, "test-agent", "build-1", "astro-test-0",
			"test-agent", `{}`, []byte(nil), (*string)(nil), clusterID,
			deploymentstore.StatusUndeploying, (*string)(nil), json.RawMessage(nil), now, 1,
			now, nil, nil,
		))

	mock.ExpectExec(`DELETE FROM scaled_namespaces`).
		WithArgs("astro-test-0").
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE deployments`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	mock.ExpectExec(`UPDATE deployments`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}

	d := &deployer.Deployer{Registry: k8s.NewRegistryWithPrimary(&noopClusterClient{cs: cs})}
	w := &UndeployWorker{
		deployer: d,
		store:    deploymentstore.NewStore(db),
		log:      logger.New("error", "json"),
		cache:    k8scache.NoopCache{},
	}

	jobErr := w.Work(context.Background(), &river.Job[UndeployArgs]{
		Args: UndeployArgs{DeploymentID: depID, ClusterID: clusterID},
	})
	if jobErr != nil {
		t.Fatalf("Work() = %v, want nil", jobErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
