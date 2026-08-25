package riverqueue

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// primaryClient stands in for the primary cluster. Only identity matters here;
// the resolver never calls through it.
type primaryClient struct{}

func (primaryClient) Clientset() *kubernetes.Clientset      { return nil }
func (primaryClient) Config() *rest.Config                  { return nil }
func (primaryClient) CheckHealth() error                    { return nil }
func (primaryClient) GetServerVersion() (string, error)     { return "", nil }
func (primaryClient) DiagnoseConnection() map[string]string { return nil }

// A deployment row with no cluster_id lives on the primary cluster. Registry.Get
// rejects an empty id, so without the fallback suspension silently no-ops with
// "registry.Get: empty cluster id" and the account keeps running unpaid. That
// is what preview did, where 23 of 24 active rows carry no cluster_id.
func TestSuspendClusterClient_DefaultsWhenRowHasNoCluster(t *testing.T) {
	want := primaryClient{}
	reg := k8s.NewRegistryWithPrimary(want)

	got, err := suspendClusterClient(context.Background(), reg, &deploymentstore.Deployment{ID: "dep-1"})
	if err != nil {
		t.Fatalf("suspendClusterClient: %v", err)
	}
	if got != k8s.ClusterClient(want) {
		t.Errorf("client = %v, want the primary cluster", got)
	}
}

// No primary and no cluster_id is a real misconfiguration, not something to
// resolve to nil and dereference later.
func TestSuspendClusterClient_ErrorsWithoutPrimary(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)

	if _, err := suspendClusterClient(context.Background(), reg, &deploymentstore.Deployment{ID: "dep-1"}); err == nil {
		t.Fatal("err is nil; a missing primary must not resolve to a nil client")
	}
}

// Scaling deployments to zero leaves the keys that answer from outside the
// cluster. A suspended account that keeps its dev key keeps spending, which is
// the gap between a gating decision and the money actually stopping.
func TestBillingSuspend_RevokesTheKeysThatOutliveTheWorkloads(t *testing.T) {
	var deleted []string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "wrong method", http.StatusBadRequest)
			return
		}
		deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/api/governance/virtual-keys/"))
		w.WriteHeader(http.StatusOK)
	}))
	defer gw.Close()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM account_ai_gateway_dev_keys").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("vk-dev"))
	mock.ExpectExec("DELETE FROM account_ai_gateway_dev_keys").WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM account_llm_judge_keys").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("vk-judge"))
	mock.ExpectExec("DELETE FROM account_llm_judge_keys").WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := &BillingSuspendWorker{
		log:             logger.New("error", "json"),
		aigwProvisioner: aigateway.NewProvisioner(aigateway.NewClient("https://aig.example", gw.URL, ""), nil, nil),
		aigwDevStore:    aigateway.NewDevStore(db),
		aigwJudgeStore:  aigateway.NewJudgeStore(db),
	}
	w.revokeGatewayKeys(context.Background(), "acct-1")

	if len(deleted) != 2 || deleted[0] != "vk-dev" || deleted[1] != "vk-judge" {
		t.Errorf("revoked upstream keys = %v, want [vk-dev vk-judge]", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("local rows: %v", err)
	}
}

// A gateway that is down must not fail a suspension that already stopped the
// workloads; the purge sweep retries the revoke.
func TestBillingSuspend_GatewayFailureDoesNotUndoTheSuspension(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer gw.Close()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM account_ai_gateway_dev_keys").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("vk-dev"))
	mock.ExpectExec("DELETE FROM account_ai_gateway_dev_keys").WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM account_llm_judge_keys").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}))

	w := &BillingSuspendWorker{
		log:             logger.New("error", "json"),
		aigwProvisioner: aigateway.NewProvisioner(aigateway.NewClient("https://aig.example", gw.URL, ""), nil, nil),
		aigwDevStore:    aigateway.NewDevStore(db),
		aigwJudgeStore:  aigateway.NewJudgeStore(db),
	}
	w.revokeGatewayKeys(context.Background(), "acct-1")
}
