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
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "enabled", "created_at", "updated_at",
		}).AddRow("cl-1", "eu-west-1", "eks-name", "https://endpoint", false, now, now))

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
