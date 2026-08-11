package admingrpc

import (
	"context"
	"database/sql/driver"
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
	"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca", "enabled",
	"agent_ingress_domain", "ingestion_ingress_domain",
	"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs",
	"pull_credential", "pull_key_hash",
	"created_at", "updated_at",
}

// fakeCA returns a deterministic PEM blob for cluster fixtures.
func fakeCA() []byte {
	return []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n")
}

// clusterRow fills clusterColumns for tests that don't care about the ingress
// values — they get populated so the row is valid.
func clusterRow(id, region, eksName, eksEndpoint string, enabled bool, now time.Time) []driver.Value {
	return []driver.Value{
		id, region, eksName, eksEndpoint, fakeCA(), enabled,
		"agents.example.com", "ingestion.example.com",
		"http://langfuse.platform.astroids.ai:3000", "10.0.1.10,10.0.2.10", "10.0.0.0/24,10.1.0.0/24",
		nil, nil,
		now, now,
	}
}

// fullRegisterRequest is the minimum valid request for RegisterCluster — every
// required field is populated. Tests override only the field under test.
func fullRegisterRequest(id string) *adminv1.RegisterClusterRequest {
	return &adminv1.RegisterClusterRequest{
		ID:                     id,
		Region:                 "eu-west-1",
		EKSClusterName:         "eks-eu",
		EKSClusterEndpoint:     "https://eu.example",
		EKSClusterCA:           fakeCA(),
		AgentIngressDomain:     "agents.example.com",
		IngestionIngressDomain: "ingestion.example.com",
		LangfuseBaseURLExt:     "http://langfuse.platform.astroids.ai:3000",
		LangfuseVPCEIPs:        "10.0.1.10,10.0.2.10",
		PodSubnetCIDRs:         "10.0.0.0/24,10.1.0.0/24",
	}
}

// fullUpdateRequest mirrors fullRegisterRequest for UpdateCluster.
func fullUpdateRequest(id string) *adminv1.UpdateClusterRequest {
	return &adminv1.UpdateClusterRequest{
		ID:                     id,
		Region:                 "eu-central-1",
		EKSClusterName:         "eks-eu-new",
		EKSClusterEndpoint:     "https://eu-new.example",
		EKSClusterCA:           fakeCA(),
		AgentIngressDomain:     "agents.example.com",
		IngestionIngressDomain: "ingestion.example.com",
		LangfuseBaseURLExt:     "http://langfuse.platform.astroids.ai:3000",
		LangfuseVPCEIPs:        "10.0.1.10,10.0.2.10",
		PodSubnetCIDRs:         "10.0.0.0/24,10.1.0.0/24",
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
	})
	srv := New(
		logger.New("error", "json"),
		nil,
		reg.Default(),
		nil,
		db,
		"",
		nil,
		"",
		"",
		nil,
		store,
		reg,
		nil,
	)
	return srv, mock
}

func TestRegisterCluster_Success(t *testing.T) {
	srv, mock := newClusterTestServer(t)

	mock.ExpectExec("INSERT INTO clusters").
		WillReturnResult(sqlmock.NewResult(0, 1))

	now := time.Now()
	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WithArgs("eu-west-1").
		WillReturnRows(sqlmock.NewRows(clusterColumns).
			AddRow(clusterRow("eu-west-1", "eu-west-1", "eks-eu", "https://eu.example", true, now)...))

	resp, err := srv.RegisterCluster(context.Background(), fullRegisterRequest("eu-west-1"))
	if err != nil {
		t.Fatalf("RegisterCluster: %v", err)
	}
	if resp.Cluster == nil || resp.Cluster.ID != "eu-west-1" {
		t.Fatalf("cluster: %+v", resp.Cluster)
	}
	if resp.Cluster.IsPrimary {
		t.Fatal("expected additional cluster, not primary")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterCluster_Duplicate(t *testing.T) {
	srv, mock := newClusterTestServer(t)

	mock.ExpectExec("INSERT INTO clusters").
		WillReturnError(&pq.Error{Code: "23505"})

	resp, err := srv.RegisterCluster(context.Background(), fullRegisterRequest("eu-west-1"))
	if err == nil {
		t.Fatal("expected error")
	}
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("code = %v, want AlreadyExists", status.Code(err))
	}
	_ = resp
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterCluster_PrimaryRejected(t *testing.T) {
	srv, _ := newClusterTestServer(t)
	_, err := srv.RegisterCluster(context.Background(), &adminv1.RegisterClusterRequest{
		ID: k8s.PrimaryClusterID,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestListClusters_IncludesPrimary(t *testing.T) {
	srv, mock := newClusterTestServer(t)

	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WillReturnRows(sqlmock.NewRows(clusterColumns))

	resp, err := srv.ListClusters(context.Background(), &adminv1.ListClustersRequest{})
	if err != nil {
		t.Fatalf("ListClusters: %v", err)
	}
	if len(resp.Clusters) != 1 || !resp.Clusters[0].IsPrimary {
		t.Fatalf("clusters: %+v", resp.Clusters)
	}
	if resp.Clusters[0].ID != k8s.PrimaryClusterID {
		t.Fatalf("primary id = %q", resp.Clusters[0].ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDisableCluster_RefreshEvictsCache(t *testing.T) {
	srv, mock := newClusterTestServer(t)
	now := time.Now()

	mock.ExpectExec("UPDATE clusters SET enabled").
		WithArgs(false, "eu-west-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WithArgs("eu-west-1").
		WillReturnRows(sqlmock.NewRows(clusterColumns).
			AddRow(clusterRow("eu-west-1", "eu-west-1", "eks-eu", "https://eu.example", false, now)...))

	resp, err := srv.DisableCluster(context.Background(), &adminv1.DisableClusterRequest{ID: "eu-west-1"})
	if err != nil {
		t.Fatalf("DisableCluster: %v", err)
	}
	if resp.Cluster == nil || resp.Cluster.Enabled {
		t.Fatalf("cluster: %+v", resp.Cluster)
	}
	if resp.Cluster.Healthy {
		t.Fatal("disabled cluster should not be healthy")
	}
	if resp.Cluster.HealthError != "cluster disabled" {
		t.Fatalf("health_error = %q", resp.Cluster.HealthError)
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

func TestEnableCluster_PrimaryRejected(t *testing.T) {
	srv, _ := newClusterTestServer(t)
	_, err := srv.EnableCluster(context.Background(), &adminv1.EnableClusterRequest{ID: k8s.PrimaryClusterID})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestUpdateCluster_Success(t *testing.T) {
	srv, mock := newClusterTestServer(t)
	now := time.Now()

	mock.ExpectExec("UPDATE clusters SET region").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE clusters SET pull_credential").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "eu-west-1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WithArgs("eu-west-1").
		WillReturnRows(sqlmock.NewRows(clusterColumns).
			AddRow(clusterRow("eu-west-1", "eu-central-1", "eks-eu-new", "https://eu-new.example", true, now)...))

	resp, err := srv.UpdateCluster(context.Background(), fullUpdateRequest("eu-west-1"))
	if err != nil {
		t.Fatalf("UpdateCluster: %v", err)
	}
	if resp.Cluster == nil || resp.Cluster.Region != "eu-central-1" {
		t.Fatalf("cluster: %+v", resp.Cluster)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateCluster_PrimaryRejected(t *testing.T) {
	srv, _ := newClusterTestServer(t)
	_, err := srv.UpdateCluster(context.Background(), &adminv1.UpdateClusterRequest{
		ID:                 k8s.PrimaryClusterID,
		Region:             "us-east-1",
		EKSClusterName:     "primary",
		EKSClusterEndpoint: "https://primary.example",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestCheckClusterHealth_Primary(t *testing.T) {
	srv, mock := newClusterTestServer(t)

	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WillReturnRows(sqlmock.NewRows(clusterColumns))

	resp, err := srv.CheckClusterHealth(context.Background(), &adminv1.CheckClusterHealthRequest{
		ID: k8s.PrimaryClusterID,
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
