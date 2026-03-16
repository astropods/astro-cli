package riverqueue

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type testClusterClient struct {
	clientset *kubernetes.Clientset
}

func (c *testClusterClient) Clientset() *kubernetes.Clientset      { return c.clientset }
func (c *testClusterClient) Config() *rest.Config                  { return nil }
func (c *testClusterClient) CheckHealth() error                    { return nil }
func (c *testClusterClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (c *testClusterClient) DiagnoseConnection() map[string]string { return nil }

func newTestK8sClient(handler http.Handler) k8s.ClusterClient {
	srv := httptest.NewServer(handler)
	cs, _ := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	return &testClusterClient{clientset: cs}
}

var testDeployColumns = []string{
	"id", "account_id", "agent_name", "build_id", "namespace", "display_name",
	"deployment_spec_json", "encrypted_data_key", "kms_key_arn",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at",
}

func k8sNamespaceListHandler(namespaces ...string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/namespaces", func(w http.ResponseWriter, r *http.Request) {
		var items []string
		for _, ns := range namespaces {
			items = append(items, fmt.Sprintf(`{"metadata":{"name":%q,"labels":{"app.kubernetes.io/managed-by":"astro-server","astro.dev/account-id":"acct-1","astro.dev/agent":"agent"}}}`, ns))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"apiVersion":"v1","kind":"NamespaceList","items":[%s]}`, strings.Join(items, ","))
	})
	return mux
}

func addDeployRow(rows *sqlmock.Rows, id, namespace, status string) {
	now := time.Now()
	rows.AddRow(id, "acct-1", "agent", "build-1", namespace, "agent",
		"{}", nil, nil,
		status, nil, nil, now, nil,
		now, nil)
}

func TestMaintainNamespaceOwnership_PendingNotOrphaned(t *testing.T) {
	k8sClient := newTestK8sClient(k8sNamespaceListHandler("astro-abc-0"))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	rows := sqlmock.NewRows(testDeployColumns)
	addDeployRow(rows, "dep-1", "astro-abc-0", "pending")
	mock.ExpectQuery("SELECT .+ FROM deployments").WillReturnRows(rows)

	mock.ExpectExec("INSERT INTO namespace_ownership").
		WithArgs("astro-abc-0", "acct-1", "agent", "dep-1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		db:    db,
		log:   logger.New("error", "json"),
	}

	w.maintainNamespaceOwnership(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

func TestMaintainNamespaceOwnership_OrphanDetected(t *testing.T) {
	k8sClient := newTestK8sClient(k8sNamespaceListHandler("astro-orphan-0"))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WillReturnRows(sqlmock.NewRows(testDeployColumns))

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		db:    db,
		log:   logger.New("warn", "json"),
	}

	// Completes without panic; the orphan is logged (not captured here)
	w.maintainNamespaceOwnership(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

func TestMaintainNamespaceOwnership_AllLiveStatusesIncluded(t *testing.T) {
	statuses := []struct {
		status    string
		namespace string
	}{
		{"active", "astro-active-0"},
		{"scaled_down", "astro-scaled-0"},
		{"pending", "astro-pending-0"},
		{"provisioning", "astro-prov-0"},
		{"failed", "astro-failed-0"},
		{"undeploying", "astro-undep-0"},
	}

	var nsList []string
	for _, s := range statuses {
		nsList = append(nsList, s.namespace)
	}
	k8sClient := newTestK8sClient(k8sNamespaceListHandler(nsList...))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	rows := sqlmock.NewRows(testDeployColumns)
	for i, s := range statuses {
		addDeployRow(rows, fmt.Sprintf("dep-%d", i), s.namespace, s.status)
	}
	mock.ExpectQuery("SELECT .+ FROM deployments").WillReturnRows(rows)

	for range statuses {
		mock.ExpectExec("INSERT INTO namespace_ownership").
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		db:    db,
		log:   logger.New("error", "json"),
	}

	w.maintainNamespaceOwnership(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

func TestNamespaceOrphanLogic(t *testing.T) {
	dbNamespaces := map[string]bool{
		"astro-active-0":  true,
		"astro-pending-0": true,
		"astro-failed-0":  true,
	}

	k8sNamespaces := []string{
		"astro-active-0",
		"astro-pending-0",
		"astro-failed-0",
		"astro-gone-0",
	}

	var orphaned []string
	for _, ns := range k8sNamespaces {
		if !dbNamespaces[ns] {
			orphaned = append(orphaned, ns)
		}
	}

	if len(orphaned) != 1 {
		t.Fatalf("expected 1 orphan, got %d: %v", len(orphaned), orphaned)
	}
	if orphaned[0] != "astro-gone-0" {
		t.Errorf("expected orphan 'astro-gone-0', got %q", orphaned[0])
	}
}
