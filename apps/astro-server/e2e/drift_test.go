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
	"strings"
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

	// Resolve env the same way ApplyDeploymentSpec will, so the DB's
	// resolved keys match what K8s will actually contain.
	rctx := deployment.ResolveContext{
		Namespace:  ns,
		AgentName:  "drift-agent",
		BuildID:    "dbuild01",
		SecretName: deployment.GenerateSecretName("drift-agent", "dbuild01"),
	}
	resolved := deployment.ResolveDeploymentSpecEnv(&specObj, rctx)

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

func (e *driftTestEnv) liveSecretData() map[string][]byte {
	e.t.Helper()
	secretName := deployment.GenerateSecretName("drift-agent", "dbuild01")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	secret, err := e.client.Clientset().CoreV1().Secrets(e.ns).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	return secret.Data
}

func (e *driftTestEnv) buildReport() *ds.DriftReport {
	e.t.Helper()
	workloads, err := e.store.GetWorkloads(e.depID)
	if err != nil {
		e.t.Fatalf("GetWorkloads: %v", err)
	}
	services, _ := e.store.GetServices(e.depID)
	ingresses, _ := e.store.GetIngresses(e.depID)
	variables, _ := e.store.GetDeploymentVariables(e.depID)
	resolvedKeys, _ := e.store.GetResolvedKeys(e.depID)

	svcNameByID := map[int]string{}
	for _, svc := range services {
		svcNameByID[svc.ID] = svc.WorkloadName
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return riverqueue.BuildDriftReport(ctx, e.client.Clientset(), e.ns, "drift-agent", "dbuild01", workloads, services, ingresses, svcNameByID, variables, resolvedKeys)
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
	}, env.liveSecretData())
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

// TestDrift_ResolvedKeysStoredAtDeploy verifies that deployment_resolved_keys
// are populated during SaveNormalizedSpec and used by BuildDriftReport.
func TestDrift_ResolvedKeysStoredAtDeploy(t *testing.T) {
	env := setupDriftEnv(t)

	rk, err := env.store.GetResolvedKeys(env.depID)
	if err != nil {
		t.Fatalf("GetResolvedKeys: %v", err)
	}
	if rk == nil {
		t.Fatal("expected resolved keys to be stored, got nil")
	}

	// setupDriftEnv now calls ResolveDeploymentSpecEnv, so the resolved keys
	// include both user env (AGENT_PORT) and platform vars (ASTRO_AGENT_NAME, etc).
	for _, want := range []string{"AGENT_PORT", "ASTRO_AGENT_NAME", "AGENT_URL"} {
		found := false
		for _, k := range rk.ConfigMapKeys {
			if k == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %s in configmap_keys, got %v", want, rk.ConfigMapKeys)
		}
	}

	// SECRET_KEY should be in secret keys
	foundSecret := false
	for _, k := range rk.SecretKeys {
		if k == "SECRET_KEY" {
			foundSecret = true
		}
	}
	if !foundSecret {
		t.Errorf("expected SECRET_KEY in secret_keys, got %v", rk.SecretKeys)
	}

	// Hashes should be populated
	if len(rk.ConfigMapHashes) == 0 {
		t.Error("expected configmap_hashes to be non-empty")
	}
	if len(rk.SecretHashes) == 0 {
		t.Error("expected secret_hashes to be non-empty")
	}
}

// TestDrift_ConfigMapValueChanged verifies that editing a ConfigMap value
// (without adding/removing keys) is detected as drift via hash comparison.
func TestDrift_ConfigMapValueChanged(t *testing.T) {
	env := setupDriftEnv(t)

	// Baseline: no drift
	report := env.buildReport()
	if report.Summary.Drift != 0 {
		t.Fatalf("expected 0 drift at baseline, got %d", report.Summary.Drift)
	}

	// Mutate a ConfigMap value in K8s
	configMapName := deployment.GenerateConfigMapName("drift-agent", "dbuild01")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cm, err := env.client.Clientset().CoreV1().ConfigMaps(env.ns).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}

	// Find a key to mutate
	var mutatedKey string
	for k := range cm.Data {
		mutatedKey = k
		break
	}
	if mutatedKey == "" {
		t.Fatal("configmap has no keys to mutate")
	}

	cm.Data[mutatedKey] = "tampered-value"
	if _, err := env.client.Clientset().CoreV1().ConfigMaps(env.ns).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update configmap: %v", err)
	}

	// Drift should now be detected
	report = env.buildReport()
	if report.Summary.Drift == 0 {
		t.Error("expected drift after configmap value change, got 0")
	}

	// The "Changed" field should name the mutated key
	found := false
	for _, item := range report.EnvVars {
		if item.Status == "drift" {
			if changed, ok := item.Expected["Changed"]; ok {
				for _, k := range splitCSV(changed) {
					if k == mutatedKey {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("expected Changed to include %q, got report: %+v", mutatedKey, report.EnvVars)
	}
}

// TestDrift_SecretValueChanged verifies that editing a Secret value is detected
// as drift via hash comparison.
func TestDrift_SecretValueChanged(t *testing.T) {
	env := setupDriftEnv(t)

	// Baseline: no drift
	report := env.buildReport()
	if report.Summary.Drift != 0 {
		t.Fatalf("expected 0 drift at baseline, got %d", report.Summary.Drift)
	}

	// Mutate secret value in K8s
	secretName := deployment.GenerateSecretName("drift-agent", "dbuild01")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	secret, err := env.client.Clientset().CoreV1().Secrets(env.ns).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}

	secret.Data["SECRET_KEY"] = []byte("tampered-secret")
	if _, err := env.client.Clientset().CoreV1().Secrets(env.ns).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update secret: %v", err)
	}

	report = env.buildReport()
	if report.Summary.Drift == 0 {
		t.Error("expected drift after secret value change, got 0")
	}

	found := false
	for _, item := range report.Secrets {
		if item.Status == "drift" {
			if changed, ok := item.Expected["Changed"]; ok {
				for _, k := range splitCSV(changed) {
					if k == "SECRET_KEY" {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("expected Changed to include SECRET_KEY, got report: %+v", report.Secrets)
	}
}

// TestDrift_RepairRegeneratesResolvedKeys verifies that RepairNormalizedSpec
// re-populates the resolved keys table.
func TestDrift_RepairRegeneratesResolvedKeys(t *testing.T) {
	env := setupDriftEnv(t)

	// Wipe resolved keys
	if _, err := env.db.Exec("DELETE FROM deployment_resolved_keys WHERE deployment_id = $1", env.depID); err != nil {
		t.Fatalf("delete resolved keys: %v", err)
	}

	// Verify they're gone
	rk, _ := env.store.GetResolvedKeys(env.depID)
	if rk != nil {
		t.Fatal("expected nil resolved keys after delete")
	}

	// Repair
	_, _, _, err := env.store.RepairNormalizedSpec(env.depID, &ds.NormalizedSpecConfig{
		Namespace: env.ns,
	}, env.liveSecretData())
	if err != nil {
		t.Fatalf("RepairNormalizedSpec: %v", err)
	}

	// Resolved keys should be back
	rk, err = env.store.GetResolvedKeys(env.depID)
	if err != nil {
		t.Fatalf("GetResolvedKeys after repair: %v", err)
	}
	if rk == nil {
		t.Fatal("expected resolved keys after repair, got nil")
	}
	if len(rk.ConfigMapKeys) == 0 {
		t.Error("expected non-empty configmap_keys after repair")
	}
	if len(rk.ConfigMapHashes) == 0 {
		t.Error("expected non-empty configmap_hashes after repair")
	}

	// Drift report should match
	report := env.buildReport()
	if report.Summary.Missing > 0 || report.Summary.Drift > 0 {
		t.Errorf("expected all match after repair, got missing=%d drift=%d",
			report.Summary.Missing, report.Summary.Drift)
	}
}

// TestDrift_EmptySecretNotInResolvedKeys verifies that a secret variable with
// an empty value is excluded from both the resolved keys table and K8s, so it
// does not cause false drift.
func TestDrift_EmptySecretNotInResolvedKeys(t *testing.T) {
	env := setupDriftEnv(t)

	// Add an empty secret variable to the spec and re-save normalized data.
	// This simulates a deploy where a secret is declared but not yet configured.
	var specObj spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(driftSpecJSON), &specObj); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	// Fill the existing secret so it's in K8s
	for k, v := range specObj.Variables {
		if v.Secret && v.Value == "" {
			v.Value = "test-" + k
			specObj.Variables[k] = v
		}
	}
	// Add an empty secret — should NOT appear in resolved keys
	specObj.Variables["EMPTY_SECRET"] = spec.Variable{Secret: true, Value: ""}

	rctx := deployment.ResolveContext{
		Namespace:  env.ns,
		AgentName:  "drift-agent",
		BuildID:    "dbuild01",
		SecretName: deployment.GenerateSecretName("drift-agent", "dbuild01"),
	}
	resolved := deployment.ResolveDeploymentSpecEnv(&specObj, rctx)

	// Verify EMPTY_SECRET is not in resolved SecretData
	if _, ok := resolved.SecretData["EMPTY_SECRET"]; ok {
		t.Error("expected EMPTY_SECRET to be excluded from resolved SecretData")
	}

	// Re-save normalized spec with the new variable set.
	// Delete existing workloads/sidecars first to avoid duplicate key errors.
	tx, err := env.db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec("DELETE FROM deployment_workloads WHERE deployment_id = $1", env.depID); err != nil {
		t.Fatalf("delete workloads: %v", err)
	}
	if _, err := tx.Exec("DELETE FROM deployment_sidecars WHERE deployment_id = $1", env.depID); err != nil {
		t.Fatalf("delete sidecars: %v", err)
	}
	if _, err := tx.Exec("DELETE FROM deployment_variables WHERE deployment_id = $1", env.depID); err != nil {
		t.Fatalf("delete variables: %v", err)
	}
	if err := ds.SaveNormalizedSpec(tx, env.depID, &specObj, resolved, nil, &ds.NormalizedSpecConfig{
		Namespace: env.ns,
	}); err != nil {
		t.Fatalf("SaveNormalizedSpec: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Verify resolved keys: EMPTY_SECRET should NOT be in secret_keys
	rk, err := env.store.GetResolvedKeys(env.depID)
	if err != nil {
		t.Fatalf("GetResolvedKeys: %v", err)
	}
	if rk == nil {
		t.Fatal("expected resolved keys, got nil")
	}
	for _, k := range rk.SecretKeys {
		if k == "EMPTY_SECRET" {
			t.Error("EMPTY_SECRET should not be in resolved secret_keys")
		}
	}

	// SECRET_KEY (non-empty) should still be present
	foundSecret := false
	for _, k := range rk.SecretKeys {
		if k == "SECRET_KEY" {
			foundSecret = true
		}
	}
	if !foundSecret {
		t.Errorf("expected SECRET_KEY in secret_keys, got %v", rk.SecretKeys)
	}

	// Drift report should show no drift (EMPTY_SECRET isn't expected in K8s)
	report := env.buildReport()
	if report.Summary.Drift > 0 || report.Summary.Missing > 0 {
		t.Errorf("expected no drift/missing, got drift=%d missing=%d",
			report.Summary.Drift, report.Summary.Missing)
		for _, item := range report.Secrets {
			t.Logf("  secret %s: %s expected=%v actual=%v", item.Name, item.Status, item.Expected, item.Actual)
		}
	}
}

// TestDrift_RepairWithLiveSecrets verifies that RepairNormalizedSpec reads
// live K8s secret values to store correct hashes, enabling full value drift
// detection after repair.
func TestDrift_RepairWithLiveSecrets(t *testing.T) {
	env := setupDriftEnv(t)

	// Run repair with live secret data from K8s
	_, _, _, err := env.store.RepairNormalizedSpec(env.depID, &ds.NormalizedSpecConfig{
		Namespace: env.ns,
	}, env.liveSecretData())
	if err != nil {
		t.Fatalf("RepairNormalizedSpec: %v", err)
	}

	rk, err := env.store.GetResolvedKeys(env.depID)
	if err != nil {
		t.Fatalf("GetResolvedKeys: %v", err)
	}
	if rk == nil {
		t.Fatal("expected resolved keys after repair, got nil")
	}

	// Secret key should be tracked
	foundSecret := false
	for _, k := range rk.SecretKeys {
		if k == "SECRET_KEY" {
			foundSecret = true
		}
	}
	if !foundSecret {
		t.Errorf("expected SECRET_KEY in secret_keys after repair, got %v", rk.SecretKeys)
	}

	// Live secret values should produce hashes
	if _, ok := rk.SecretHashes["SECRET_KEY"]; !ok {
		t.Error("expected SECRET_KEY hash after repair with live data")
	}

	// Drift should be fully clean — keys and values match
	report := env.buildReport()
	if report.Summary.Missing > 0 || report.Summary.Drift > 0 {
		t.Errorf("expected all match after repair, got missing=%d drift=%d",
			report.Summary.Missing, report.Summary.Drift)
		for _, item := range report.Secrets {
			t.Logf("  secret %s: %s expected=%v actual=%v", item.Name, item.Status, item.Expected, item.Actual)
		}
	}
}

// splitCSV splits a ", "-separated string into trimmed parts.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
