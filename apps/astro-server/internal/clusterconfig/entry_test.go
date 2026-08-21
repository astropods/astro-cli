package clusterconfig

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
)

// localDevConfig mirrors what scripts/dev.sh generates for a docker-desktop
// cluster: no CA, no VPCE IPs, and EKS coordinates that never get dialed
// because local mode builds its client from kubeconfig.
const localDevConfig = `[
  {
    "id": "docker-desktop",
    "region": "local",
    "eks_cluster_name": "docker-desktop",
    "eks_cluster_endpoint": "https://kubernetes.docker.internal:6443",
    "eks_cluster_ca": "",
    "agent_ingress_domain": "agents.localtest.me",
    "agent_public_ingress_domain": "agents.public.localtest.me",
    "ingestion_ingress_domain": "ingestion.localtest.me",
    "langfuse_base_url_ext": "https://langfuse.adhoc.dev.astropod.ai",
    "langfuse_vpce_ips": "",
    "pod_subnet_cidrs": "10.1.0.0/16",
    "pod_subnet_ipv6_cidrs": "",
    "loki_url": "https://loki.adhoc.dev.astropod.ai",
    "prometheus_url": "https://prometheus.adhoc.dev.astropod.ai",
    "tenant_router_internal_url": ""
  }
]`

// The generated entry has to clear UpsertFromConfig's validation, or boot sync
// skips it and local dev is back to a cluster with no row and no domains.
func TestLocalDevEntrySyncs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cluster-config.json")
	if err := os.WriteFile(path, []byte(localDevConfig), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	entries, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	entry, ok := Find(entries, "docker-desktop")
	if !ok {
		t.Fatal("Find(docker-desktop): not found")
	}

	row, err := entry.ToClusterRow()
	if err != nil {
		t.Fatalf("ToClusterRow: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectExec("INSERT INTO clusters").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := clusterstore.New(db).UpsertFromConfig(context.Background(), row, true); err != nil {
		t.Fatalf("UpsertFromConfig: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestLoadEmptyPath(t *testing.T) {
	entries, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\"): %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Load(\"\") returned %d entries, want 0", len(entries))
	}
}
