package admingrpc

import (
	"context"
	"database/sql/driver"
	"net"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type stubClusterClient struct {
	checkHealthErr error
}

func (s *stubClusterClient) Clientset() *kubernetes.Clientset      { return nil }
func (s *stubClusterClient) Config() *rest.Config                  { return nil }
func (s *stubClusterClient) CheckHealth() error                    { return s.checkHealthErr }
func (s *stubClusterClient) GetServerVersion() (string, error)     { return "v1.0", nil }
func (s *stubClusterClient) DiagnoseConnection() map[string]string { return nil }

// clusterColumns is the full clusters table projection used by clusterstore.baseSelect.
var clusterColumns = []string{
	"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca",
	"agent_ingress_domain", "agent_public_ingress_domain", "ingestion_ingress_domain",
	"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs", "pod_subnet_ipv6_cidrs",
	"loki_url", "prometheus_url", "tenant_router_internal_url",
	"pull_credential", "pull_key_hash",
	"created_at", "updated_at",
}

// fakeCA returns a deterministic PEM blob for cluster fixtures.
func fakeCA() []byte {
	return []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n")
}

// clusterRow fills clusterColumns for tests that don't care about the ingress
// values — they get populated so the row is valid.
func clusterRow(id, region, eksName, eksEndpoint string, now time.Time) []driver.Value {
	return []driver.Value{
		id, region, eksName, eksEndpoint, fakeCA(),
		"agents.example.com", "agents.public.example.com", "ingestion.example.com",
		"http://langfuse.platform.astroids.ai:3000", "10.0.1.10,10.0.2.10", "10.0.0.0/24,10.1.0.0/24", "",
		"", "", "",
		nil, nil,
		now, now,
	}
}

func newClusterTestServer(t *testing.T) (*Server, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := clusterstore.New(db)
	reg := k8s.NewRegistryForTest(&stubClusterClient{}, store, k8s.RegistryConfig{
		Region:           "us-east-1",
		EKSBootstrapName: "primary-eks",
		EKSBootstrapURL:  "https://primary.example",
		DefaultClusterID: "primary-eks",
	})
	srv := New(
		logger.New("error", "json"),
		nil,
		reg.Default(),
		nil,
		db,
		"",
		nil,
		nil,
		store,
		reg,
		nil,
	)
	return srv, mock
}

func TestListClusters_IncludesDefault(t *testing.T) {
	srv, mock := newClusterTestServer(t)
	now := time.Now()

	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WillReturnRows(sqlmock.NewRows(clusterColumns).
			AddRow(clusterRow("primary-eks", "us-east-1", "primary-eks", "https://primary.example", now)...))

	resp, err := srv.ListClusters(context.Background(), &adminv1.ListClustersRequest{})
	if err != nil {
		t.Fatalf("ListClusters: %v", err)
	}
	if len(resp.Clusters) != 1 || !resp.Clusters[0].IsPrimary {
		t.Fatalf("clusters: %+v", resp.Clusters)
	}
	if resp.Clusters[0].ID != "primary-eks" {
		t.Fatalf("default id = %q", resp.Clusters[0].ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestListClusters_DefaultMissingRowIsOmitted covers the bug the removed
// synthesized-entry fallback caused: a default cluster whose boot sync
// failed (or hasn't run yet) must not show up as a fake healthy entry.
func TestListClusters_DefaultMissingRowIsOmitted(t *testing.T) {
	srv, mock := newClusterTestServer(t)

	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WillReturnRows(sqlmock.NewRows(clusterColumns))

	resp, err := srv.ListClusters(context.Background(), &adminv1.ListClustersRequest{})
	if err != nil {
		t.Fatalf("ListClusters: %v", err)
	}
	if len(resp.Clusters) != 0 {
		t.Fatalf("clusters: %+v, want none — the default cluster has no row", resp.Clusters)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeregisterCluster_InUse(t *testing.T) {
	srv, mock := newClusterTestServer(t)

	mock.ExpectExec("DELETE FROM clusters").
		WithArgs("eu-west-1").
		WillReturnError(&pq.Error{Code: "23503"})

	_, err := srv.DeregisterCluster(context.Background(), &adminv1.DeregisterClusterRequest{ID: "eu-west-1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition", status.Code(err))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCheckClusterHealth_Default(t *testing.T) {
	srv, mock := newClusterTestServer(t)
	now := time.Now()

	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WithArgs("primary-eks").
		WillReturnRows(sqlmock.NewRows(clusterColumns).
			AddRow(clusterRow("primary-eks", "us-east-1", "primary-eks", "https://primary.example", now)...))

	resp, err := srv.CheckClusterHealth(context.Background(), &adminv1.CheckClusterHealthRequest{
		ID: "primary-eks",
	})
	if err != nil {
		t.Fatalf("CheckClusterHealth: %v", err)
	}
	if resp.Cluster == nil || !resp.Cluster.IsPrimary || !resp.Cluster.Healthy {
		t.Fatalf("cluster: %+v", resp.Cluster)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCheckClusterHealth_DefaultMissingRowErrors covers the case that used to
// silently synthesize a healthy default entry: with no clusters row, the
// health check must fail loudly instead of reporting fabricated health.
func TestCheckClusterHealth_DefaultMissingRowErrors(t *testing.T) {
	srv, mock := newClusterTestServer(t)

	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WithArgs("primary-eks").
		WillReturnRows(sqlmock.NewRows(clusterColumns))

	_, err := srv.CheckClusterHealth(context.Background(), &adminv1.CheckClusterHealthRequest{
		ID: "primary-eks",
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCheckURLReachability(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	reachable, errMsg := checkURLReachability(context.Background(), "http://"+ln.Addr().String())
	if !reachable || errMsg != "" {
		t.Fatalf("reachable = %v, err = %q, want true, \"\"", reachable, errMsg)
	}

	reachable, errMsg = checkURLReachability(context.Background(), "127.0.0.1:1")
	if reachable || errMsg == "" {
		t.Fatalf("reachable = %v, err = %q, want false, non-empty", reachable, errMsg)
	}
}

func TestCheckClusterURLs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	entry := k8s.ClusterEntry{
		LangfuseBaseURLExt:      "http://" + ln.Addr().String(),
		TenantRouterInternalURL: "127.0.0.1:1",
		// LokiURL and PrometheusURL left empty — should be skipped entirely.
	}
	results := checkClusterURLs(context.Background(), entry)
	if len(results) != 2 {
		t.Fatalf("results = %+v, want 2 entries (empty fields skipped)", results)
	}

	byLabel := make(map[string]adminv1.UrlReachability, len(results))
	for _, r := range results {
		byLabel[r.Label] = r
	}
	if r, ok := byLabel["langfuse_base_url_ext"]; !ok || !r.Reachable {
		t.Fatalf("langfuse_base_url_ext = %+v, want reachable", r)
	}
	if r, ok := byLabel["tenant_router_internal_url"]; !ok || r.Reachable || r.Error == "" {
		t.Fatalf("tenant_router_internal_url = %+v, want unreachable with error", r)
	}
}
