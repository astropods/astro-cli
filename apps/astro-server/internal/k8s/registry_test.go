package k8s

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// fakeClient is a stand-in for ClusterClient that lets tests assert the
// registry handed back a constructed client without dialing a real cluster.
type fakeClient struct{ id string }

func (f *fakeClient) Clientset() *kubernetes.Clientset      { return nil }
func (f *fakeClient) Config() *rest.Config                  { return nil }
func (f *fakeClient) CheckHealth() error                    { return nil }
func (f *fakeClient) GetServerVersion() (string, error)     { return "fake-v0.0.0", nil }
func (f *fakeClient) DiagnoseConnection() map[string]string { return map[string]string{"id": f.id} }

// newRegistryDirect mirrors NewRegistry but injects a stub primary and an
// empty cache for tests that do not dial Kubernetes.
func newRegistryDirect(primary ClusterClient) *Registry {
	return &Registry{
		primary: primary,
		cache:   make(map[string]ClusterClient),
	}
}

func TestNewRegistry_PropagatesClientConstructionError(t *testing.T) {
	log := logger.New("error", "json")
	// EKS with empty cluster name — NewClusterClient fails fast.
	_, err := NewRegistry(context.Background(), nil, RegistryConfig{
		Mode:             ClientModeEKS,
		EKSBootstrapName: "",
	}, log)
	if err == nil {
		t.Fatal("expected NewRegistry to fail when EKS config is empty")
	}
}

func TestRegistry_Default_ReturnsPrimary(t *testing.T) {
	primary := &fakeClient{id: "primary"}
	r := newRegistryDirect(primary)

	got := r.Default()
	if got != primary {
		t.Fatalf("Default() = %p, want %p", got, primary)
	}
}

func TestRegistry_Default_NeverReturnsNil(t *testing.T) {
	primary := &fakeClient{id: "primary"}
	r := newRegistryDirect(primary)
	if r.Default() == nil {
		t.Fatal("Default() returned nil")
	}
}

// NewRegistry's errors should be wrapped with "registry:" so operators
// reading logs can attribute boot failures to the registry layer (not the
// raw EKS / kubeconfig error from one layer deeper).
func TestNewRegistry_ErrorIsWrapped(t *testing.T) {
	log := logger.New("error", "json")
	_, err := NewRegistry(context.Background(), nil, RegistryConfig{
		Mode: ClientMode("not-a-real-mode"),
	}, log)
	if err == nil {
		t.Fatal("expected NewRegistry to fail on unknown mode")
	}
	if !strings.Contains(err.Error(), "registry:") {
		t.Fatalf("expected error to be wrapped with 'registry:', got %q", err.Error())
	}
}

func TestRegistry_Get_EmptyID(t *testing.T) {
	r := newRegistryDirect(&fakeClient{id: "p"})
	_, err := r.Get(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "empty cluster id") {
		t.Fatalf("expected empty-id error, got %v", err)
	}
}

func TestRegistry_Get_NoClusterStore(t *testing.T) {
	r := newRegistryDirect(&fakeClient{id: "p"})
	_, err := r.Get(context.Background(), "any-id")
	if !errors.Is(err, ErrClusterNotFound) {
		t.Fatalf("want ErrClusterNotFound, got %v", err)
	}
}

func TestRegistry_Get_NotFoundRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	r := &Registry{
		primary:      &fakeClient{id: "p"},
		clusterStore: clusterstore.New(db),
		cache:        make(map[string]ClusterClient),
		log:          logger.New("error", "json"),
	}
	_, err = r.Get(context.Background(), "missing")
	if !errors.Is(err, ErrClusterNotFound) {
		t.Fatalf("want ErrClusterNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_Get_Disabled(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WithArgs("cl-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca", "enabled",
			"agent_ingress_domain", "ingestion_ingress_domain",
			"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs", "pod_subnet_ipv6_cidrs",
			"pull_credential", "pull_key_hash",
			"created_at", "updated_at",
		}).AddRow("cl-1", "eu-west-1", "eks-name", "https://endpoint", []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n"), false,
			"agents.example.com", "ingestion.example.com",
			"http://langfuse.platform.astroids.ai:3000", "10.0.1.10", "10.0.0.0/24", "",
			nil, nil,
			now, now))

	r := &Registry{
		primary:      &fakeClient{id: "p"},
		clusterStore: clusterstore.New(db),
		cache:        make(map[string]ClusterClient),
		log:          logger.New("error", "json"),
	}
	_, err = r.Get(context.Background(), "cl-1")
	if !errors.Is(err, ErrClusterDisabled) {
		t.Fatalf("want ErrClusterDisabled, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRegistry_GetEntry_IncludesPullCredential guards against the field
// being dropped in this row->entry mapping, distinct from the one in
// admingrpc.rowToEntry — clustercfg.Resolve depends on this one.
func TestRegistry_GetEntry_IncludesPullCredential(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WithArgs("eu-west-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca", "enabled",
			"agent_ingress_domain", "ingestion_ingress_domain",
			"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs", "pod_subnet_ipv6_cidrs",
			"pull_credential", "pull_key_hash",
			"created_at", "updated_at",
		}).AddRow("eu-west-1", "eu-west-1", "eks-eu", "https://eu.example", []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n"), true,
			"agents.example.com", "ingestion.example.com",
			"http://langfuse.platform.astroids.ai:3000", "10.0.1.10", "10.0.0.0/24", "2a05:d018:51b:d540::/64",
			"astrocp_eu-west-1_secret", []byte("hash"),
			now, now))

	r := &Registry{
		primary:      &fakeClient{id: "p"},
		clusterStore: clusterstore.New(db),
		cache:        make(map[string]ClusterClient),
		entryCache:   make(map[string]ClusterEntry),
		log:          logger.New("error", "json"),
	}
	entry, err := r.GetEntry(context.Background(), "eu-west-1")
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if entry.PullCredential != "astrocp_eu-west-1_secret" {
		t.Fatalf("PullCredential = %q, want astrocp_eu-west-1_secret", entry.PullCredential)
	}
	if entry.PodSubnetIPv6CIDRs != "2a05:d018:51b:d540::/64" {
		t.Fatalf("PodSubnetIPv6CIDRs = %q, want 2a05:d018:51b:d540::/64", entry.PodSubnetIPv6CIDRs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_List_IncludesPrimaryAndRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca", "enabled",
			"agent_ingress_domain", "ingestion_ingress_domain",
			"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs", "pod_subnet_ipv6_cidrs",
			"pull_credential", "pull_key_hash",
			"created_at", "updated_at",
		}).AddRow("eu-west-1", "eu-west-1", "eks-eu", "https://eu.example", []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n"), true,
			"agents.example.com", "ingestion.example.com",
			"http://langfuse.platform.astroids.ai:3000", "10.0.1.10", "10.0.0.0/24", "2a05:d018:51b:d540::/64",
			"astrocp_eu-west-1_secret", nil,
			now, now))

	r := &Registry{
		primary:      &fakeClient{id: "primary"},
		clusterStore: clusterstore.New(db),
		regCfg: RegistryConfig{
			Region:           "us-east-1",
			EKSBootstrapName: "primary-eks",
			EKSBootstrapURL:  "https://primary.example",
		},
		cache: make(map[string]ClusterClient),
	}

	entries, err := r.List(context.Background(), false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if !entries[0].IsPrimary || entries[0].ID != PrimaryClusterID {
		t.Fatalf("entries[0] = %+v, want synthesized primary", entries[0])
	}
	if entries[0].Region != "us-east-1" || entries[0].EKSClusterName != "primary-eks" {
		t.Fatalf("primary coords: %+v", entries[0])
	}
	if entries[1].ID != "eu-west-1" || entries[1].IsPrimary {
		t.Fatalf("entries[1] = %+v", entries[1])
	}
	if entries[1].PodSubnetIPv6CIDRs != "2a05:d018:51b:d540::/64" {
		t.Fatalf("entries[1].PodSubnetIPv6CIDRs = %q, want 2a05:d018:51b:d540::/64", entries[1].PodSubnetIPv6CIDRs)
	}
	if entries[1].PullCredential != "astrocp_eu-west-1_secret" {
		t.Fatalf("entries[1].PullCredential = %q, want astrocp_eu-west-1_secret", entries[1].PullCredential)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_Refresh_EvictsCache(t *testing.T) {
	r := &Registry{
		primary: &fakeClient{id: "primary"},
		cache:   map[string]ClusterClient{"cl-1": &fakeClient{id: "cl-1"}},
	}
	if err := r.Refresh(context.Background(), "cl-1"); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	r.mu.RLock()
	_, ok := r.cache["cl-1"]
	r.mu.RUnlock()
	if ok {
		t.Fatal("expected cache entry to be evicted")
	}
}

func TestRegistry_Refresh_PrimaryNoOp(t *testing.T) {
	r := &Registry{
		primary: &fakeClient{id: "primary"},
		cache:   map[string]ClusterClient{"cl-1": &fakeClient{id: "cl-1"}},
	}
	if err := r.Refresh(context.Background(), PrimaryClusterID); err != nil {
		t.Fatalf("Refresh primary: %v", err)
	}
	r.mu.RLock()
	_, ok := r.cache["cl-1"]
	r.mu.RUnlock()
	if !ok {
		t.Fatal("Refresh(primary) should not evict other cache entries")
	}
}
