//go:build k8s

// Drift detection integration tests — requires both Postgres (DATABASE_URL)
// and a real K8s cluster (KUBECONFIG). These tests verify the full loop:
// save deployment to DB → apply to K8s → run BuildDriftReport → assert.
//
// Run: go test -tags k8s -race -timeout 5m ./e2e/...
// Requires: KUBECONFIG + DATABASE_URL
package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	spec "github.com/astropods/astro/packages/astro-spec"
	_ "github.com/lib/pq"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// driftSpecJSON is a minimal spec with an agent + one non-persistent knowledge store.
const driftSpecJSON = `{
  "spec": "deployment/v1",
  "source": {"account": "drift-test", "name": "drift-agent", "build": "dbuild01", "registry": "test-registry.example.com"},
  "target": {"runtime": "kubernetes", "account": "drift-test", "display_name": "Drift Test Agent"},
  "agent": {
    "image": "gcr.io/google-containers/pause:3.2",
    "endpoints": {"http": {"port": 8080, "protocol": "http"}},
    "replicas": 1,
    "resources": {"cpu": "50m", "memory": "64Mi", "cpu_limit": "100m", "memory_limit": "128Mi"},
    "environment": {"AGENT_PORT": "8080"},
    "update": {"strategy": "rolling"}
  },
  "knowledge": {
    "cache": {
      "image": "gcr.io/google-containers/pause:3.2",
      "endpoints": {"http": {"port": 6379, "protocol": "http"}},
      "replicas": 1,
      "persistent": false,
      "resources": {"cpu": "50m", "memory": "64Mi", "cpu_limit": "100m", "memory_limit": "128Mi"},
      "provider": "redis",
      "update": {"strategy": "rolling"}
    }
  },
  "variables": {
    "SECRET_KEY": {"secret": true, "targets": ["agent"]}
  },
  "observability": {"enabled": false}
}`

// driftTestEnv bundles everything the drift tests need.
type driftTestEnv struct {
	t      *testing.T
	db     *sql.DB
	store  *ds.Store
	client k8s.ClusterClient
	ns     string
	depID  string
}

func setupDriftEnv(t *testing.T) *driftTestEnv {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping drift integration test")
	}
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("KUBECONFIG not set — skipping drift integration test")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	client, err := k8s.NewClusterClient(context.Background(), k8s.ClusterClientConfig{
		Mode:           k8s.ClientModeLocal,
		KubeconfigPath: kubeconfig,
	})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}

	store := ds.NewStore(db)
	ns := "e2e-drift-" + sanitize(t.Name())
	if len(ns) > 40 {
		ns = ns[:40]
	}

	// Clean up namespace after test
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = client.Clientset().CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
	})

	// Ensure test account
	var accountID string
	err = db.QueryRow(`
		INSERT INTO accounts (name, type) VALUES ('drift-e2e', 'personal')
		ON CONFLICT DO NOTHING RETURNING id
	`).Scan(&accountID)
	if err != nil {
		err = db.QueryRow(`SELECT id FROM accounts WHERE name = 'drift-e2e'`).Scan(&accountID)
		if err != nil {
			t.Fatalf("get test account: %v", err)
		}
	}

	// Parse spec and fill secrets
	var specObj spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(driftSpecJSON), &specObj); err != nil {
		t.Fatalf("parse drift spec: %v", err)
	}
	for k, v := range specObj.Variables {
		if v.Secret && v.Value == "" {
			v.Value = "test-" + k
			specObj.Variables[k] = v
		}
	}

	resolved := &deployment.ResolvedEnv{
		ConfigMapData: map[string]string{"AGENT_PORT": "8080"},
		SecretData:    map[string]string{"SECRET_KEY": "test-SECRET_KEY"},
	}

	// Save deployment to DB with normalized spec
	depID := fmt.Sprintf("drf%08d", time.Now().UnixMilli()%100000000)
	dep, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "drift-agent",
		DisplayName: t.Name(), BuildID: "dbuild01", Namespace: ns,
		SpecJSON: driftSpecJSON,
	}, func(tx *sql.Tx, deploymentID string) error {
		return ds.SaveNormalizedSpec(tx, deploymentID, &specObj, resolved, nil, &ds.NormalizedSpecConfig{
			Namespace: ns,
		})
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	if err := store.UpdateStatus(dep.ID, ds.StatusActive, "", nil); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM deployments WHERE id = $1", dep.ID)
	})

	// Apply spec to K8s
	applier := k8s.NewApplier(client, k8s.ApplierConfig{
		Namespace:       ns,
		RegistryURL:     "test-registry.example.com",
		ImagePullPolicy: corev1.PullNever,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := applier.ApplyDeploymentSpec(ctx, &specObj)
	if err != nil {
		t.Fatalf("ApplyDeploymentSpec: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("apply errors: %v", result.Errors)
	}

	return &driftTestEnv{
		t: t, db: db, store: store, client: client, ns: ns, depID: dep.ID,
	}
}

func (e *driftTestEnv) buildReport() *ds.DriftReport {
	e.t.Helper()
	workloads, err := e.store.GetWorkloads(e.depID)
	if err != nil {
		e.t.Fatalf("GetWorkloads: %v", err)
	}
	services, _ := e.store.GetServices(e.depID)
	ingresses, _ := e.store.GetIngresses(e.depID)

	svcNameByID := map[int]string{}
	for _, svc := range services {
		svcNameByID[svc.ID] = svc.WorkloadName
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return riverqueue.BuildDriftReport(ctx, e.client.Clientset(), e.ns, workloads, services, ingresses, svcNameByID)
}

// --- Tests ---

func TestDrift_AllMatch(t *testing.T) {
	env := setupDriftEnv(t)
	report := env.buildReport()

	if report.Summary.Missing != 0 {
		t.Errorf("expected 0 missing, got %d", report.Summary.Missing)
	}
	if report.Summary.Drift != 0 {
		t.Errorf("expected 0 drift, got %d", report.Summary.Drift)
	}
	if report.Summary.Match == 0 {
		t.Error("expected at least 1 match, got 0")
	}
	// agent + cache = 2 workloads, 2 services = at least 4 items
	if report.Summary.Total < 4 {
		t.Errorf("expected at least 4 total items, got %d", report.Summary.Total)
	}

	for _, wl := range report.Workloads {
		if wl.Status != "match" {
			t.Errorf("workload %s: expected match, got %s", wl.Name, wl.Status)
		}
	}
	for _, svc := range report.Services {
		if svc.Status != "match" {
			t.Errorf("service %s: expected match, got %s", svc.Name, svc.Status)
		}
	}
}

func TestDrift_MissingDeployment(t *testing.T) {
	env := setupDriftEnv(t)

	// Delete the cache deployment from K8s
	cacheName := deployment.GenerateResourceName("drift-agent", "knowledge", "cache")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := env.client.Clientset().AppsV1().Deployments(env.ns).Delete(ctx, cacheName, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete cache deployment: %v", err)
	}

	report := env.buildReport()

	if report.Summary.Missing != 1 {
		t.Errorf("expected 1 missing, got %d", report.Summary.Missing)
		for _, wl := range report.Workloads {
			t.Logf("  workload %s: %s", wl.Name, wl.Status)
		}
	}

	found := false
	for _, wl := range report.Workloads {
		if wl.Name == cacheName && wl.Status == "missing" {
			found = true
		}
	}
	if !found {
		t.Errorf("workload %s not marked as missing", cacheName)
	}
}

func TestDrift_ExtraDeployment(t *testing.T) {
	env := setupDriftEnv(t)

	// Create an extra deployment with astro-server managed-by labels
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	labels := deployment.GenerateLabels("drift-agent", "dbuild01", "extra-thing")
	replicas := int32(1)
	_, err := env.client.Clientset().AppsV1().Deployments(env.ns).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "drift-agent-extra",
			Namespace: env.ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "extra"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "extra"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "busybox", Image: "gcr.io/google-containers/pause:3.2",
						Command: []string{"sleep", "3600"},
					}},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create extra deployment: %v", err)
	}

	report := env.buildReport()

	if report.Summary.Extra < 1 {
		t.Errorf("expected at least 1 extra, got %d", report.Summary.Extra)
		for _, wl := range report.Workloads {
			t.Logf("  workload %s: %s", wl.Name, wl.Status)
		}
	}
}

func TestDrift_ScaledReplicas(t *testing.T) {
	env := setupDriftEnv(t)

	// Scale agent deployment to 2 replicas (DB expects 1)
	agentName := deployment.GenerateAgentResourceName("drift-agent", "agent")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	agentDepl, err := env.client.Clientset().AppsV1().Deployments(env.ns).Get(ctx, agentName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get agent deployment: %v", err)
	}
	two := int32(2)
	agentDepl.Spec.Replicas = &two
	if _, err = env.client.Clientset().AppsV1().Deployments(env.ns).Update(ctx, agentDepl, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("scale agent deployment: %v", err)
	}

	report := env.buildReport()

	if report.Summary.Drift != 1 {
		t.Errorf("expected 1 drifted, got %d", report.Summary.Drift)
		for _, wl := range report.Workloads {
			t.Logf("  workload %s: %s (expected=%v actual=%v)", wl.Name, wl.Status, wl.Expected, wl.Actual)
		}
	}

	for _, wl := range report.Workloads {
		if wl.Name == agentName && wl.Status != "drift" {
			t.Errorf("agent workload: expected drift, got %s", wl.Status)
		}
	}
}

func TestDrift_MissingService(t *testing.T) {
	env := setupDriftEnv(t)

	// Delete the agent service
	agentSvcName := deployment.GenerateAgentResourceName("drift-agent", "agent")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := env.client.Clientset().CoreV1().Services(env.ns).Delete(ctx, agentSvcName, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete agent service: %v", err)
	}

	report := env.buildReport()

	missing := 0
	for _, svc := range report.Services {
		if svc.Status == "missing" {
			missing++
		}
	}
	if missing != 1 {
		t.Errorf("expected 1 missing service, got %d", missing)
		for _, svc := range report.Services {
			t.Logf("  service %s: %s", svc.Name, svc.Status)
		}
	}
}

func TestDrift_RepairRestoresDBState(t *testing.T) {
	env := setupDriftEnv(t)

	// Wipe normalized data (simulates the old repair bug)
	if _, err := env.db.Exec("DELETE FROM deployment_workloads WHERE deployment_id = $1", env.depID); err != nil {
		t.Fatalf("delete workloads: %v", err)
	}
	if _, err := env.db.Exec("DELETE FROM deployment_sidecars WHERE deployment_id = $1", env.depID); err != nil {
		t.Fatalf("delete sidecars: %v", err)
	}

	// Verify cascaded deletion
	ingresses, _ := env.store.GetIngresses(env.depID)
	if len(ingresses) != 0 {
		t.Fatalf("expected 0 ingresses after wipe, got %d", len(ingresses))
	}

	// Repair with config
	workloads, services, _, err := env.store.RepairNormalizedSpec(env.depID, &ds.NormalizedSpecConfig{
		Namespace: env.ns,
	})
	if err != nil {
		t.Fatalf("RepairNormalizedSpec: %v", err)
	}
	if workloads == 0 {
		t.Error("repair produced 0 workloads")
	}
	if services == 0 {
		t.Error("repair produced 0 services")
	}

	// Drift report should show all K8s resources as match
	report := env.buildReport()
	if report.Summary.Missing > 0 {
		t.Errorf("expected 0 missing after repair, got %d", report.Summary.Missing)
		for _, wl := range report.Workloads {
			if wl.Status == "missing" {
				t.Logf("  missing: %s (%s)", wl.Name, wl.Type)
			}
		}
	}
	if report.Summary.Match < 4 {
		t.Errorf("expected at least 4 matching after repair, got %d", report.Summary.Match)
	}
}
