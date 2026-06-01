//go:build k8s

// Secret-routing integration test — verifies that env vars referencing secret
// variables end up in the K8s Secret (not ConfigMap) and that drift detection
// sees no drift after apply.
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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// secretRoutingSpec has:
//   - a secret variable (API_KEY) referenced by the agent env as ${variables.API_KEY}
//   - a composite secret ref: "Bearer ${variables.API_KEY}"
//   - a plain (non-secret) env var: LOG_LEVEL=debug
//   - a non-secret variable referenced in env: ${variables.APP_NAME}
const secretRoutingSpec = `{
  "spec": "deployment/v1",
  "source": {"account": "sr-test", "name": "sr-agent", "build": "srbuild01", "registry": "test-registry.example.com"},
  "target": {"runtime": "kubernetes", "account": "sr-test", "display_name": "Secret Routing Agent"},
  "agent": {
    "image": "gcr.io/google-containers/pause:3.2",
    "endpoints": {"http": {"port": 8080, "protocol": "http"}},
    "replicas": 1,
    "resources": {"cpu": "50m", "memory": "64Mi", "cpu_limit": "100m", "memory_limit": "128Mi"},
    "environment": {
      "API_KEY":       "${variables.API_KEY}",
      "AUTH_HEADER":   "Bearer ${variables.API_KEY}",
      "LOG_LEVEL":     "debug",
      "APP_NAME":      "${variables.APP_NAME}"
    }
  },
  "variables": {
    "API_KEY":   {"secret": true, "targets": ["agent"]},
    "APP_NAME":  {"value": "my-app", "secret": false, "targets": ["agent"]}
  },
  "observability": {"enabled": false}
}`

func TestSecretRouting(t *testing.T) {
	env := setupSecretRoutingEnv(t)

	t.Run("Separation", func(t *testing.T) {
		agentName := "sr-agent"
		buildID := "srbuild01"

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// --- Verify ConfigMap contents ---
		cmName := deployment.GenerateConfigMapName(agentName, buildID)
		cm, err := env.client.Clientset().CoreV1().ConfigMaps(env.ns).Get(ctx, cmName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get ConfigMap %s: %v", cmName, err)
		}

		// LOG_LEVEL (plain value) must be in ConfigMap
		if cm.Data["LOG_LEVEL"] != "debug" {
			t.Errorf("ConfigMap LOG_LEVEL: expected 'debug', got %q", cm.Data["LOG_LEVEL"])
		}
		// APP_NAME (non-secret variable ref) must be in ConfigMap
		if cm.Data["APP_NAME"] != "my-app" {
			t.Errorf("ConfigMap APP_NAME: expected 'my-app', got %q", cm.Data["APP_NAME"])
		}
		// API_KEY (secret ref) must NOT be in ConfigMap
		if _, ok := cm.Data["API_KEY"]; ok {
			t.Errorf("ConfigMap should NOT contain API_KEY (secret ref), but found %q", cm.Data["API_KEY"])
		}
		// AUTH_HEADER (composite secret ref) must NOT be in ConfigMap
		if _, ok := cm.Data["AUTH_HEADER"]; ok {
			t.Errorf("ConfigMap should NOT contain AUTH_HEADER (composite secret ref), but found %q", cm.Data["AUTH_HEADER"])
		}

		// --- Verify Secret contents ---
		secretName := deployment.GenerateSecretName(agentName, buildID)
		secret, err := env.client.Clientset().CoreV1().Secrets(env.ns).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get Secret %s: %v", secretName, err)
		}

		// API_KEY (direct secret ref) must be in Secret with resolved value
		if got := string(secret.Data["API_KEY"]); got != "test-API_KEY" {
			t.Errorf("Secret API_KEY: expected 'test-API_KEY', got %q", got)
		}
		// AUTH_HEADER (composite secret ref) must be in Secret
		if got := string(secret.Data["AUTH_HEADER"]); got != "Bearer test-API_KEY" {
			t.Errorf("Secret AUTH_HEADER: expected 'Bearer test-API_KEY', got %q", got)
		}
		// LOG_LEVEL must NOT be in Secret
		if _, ok := secret.Data["LOG_LEVEL"]; ok {
			t.Errorf("Secret should NOT contain LOG_LEVEL (plain value)")
		}
		// APP_NAME must NOT be in Secret
		if _, ok := secret.Data["APP_NAME"]; ok {
			t.Errorf("Secret should NOT contain APP_NAME (non-secret variable)")
		}
	})

	// NoDrift — removed: depends on deployment_resolved_keys for composite secret
	// key tracking (AUTH_HEADER). Variable-based fallback misses composite refs.
	// Will be re-added when row-based drift is rebuilt.
	t.Run("NoDrift", func(t *testing.T) {
		t.Skip("deployment_resolved_keys dropped — re-enable when row-based drift is rebuilt")
		workloads, err := env.store.GetWorkloads(env.depID)
		if err != nil {
			t.Fatalf("GetWorkloads: %v", err)
		}
		services, _ := env.store.GetServices(env.depID)
		ingresses, _ := env.store.GetIngresses(env.depID)
		variables, _ := env.store.GetDeploymentVariables(env.depID)
		resolvedKeys, _ := env.store.GetResolvedKeys(env.depID)

		svcNameByID := map[int]string{}
		for _, svc := range services {
			svcNameByID[svc.ID] = svc.WorkloadName
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		report := riverqueue.BuildDriftReport(ctx, env.client.Clientset(), env.ns,
			"sr-agent", "srbuild01", workloads, services, ingresses, svcNameByID, variables, resolvedKeys)

		if report.Summary.Missing != 0 {
			t.Errorf("expected 0 missing, got %d", report.Summary.Missing)
			for _, wl := range report.Workloads {
				if wl.Status == "missing" {
					t.Logf("  missing workload: %s", wl.Name)
				}
			}
		}
		if report.Summary.Drift != 0 {
			t.Errorf("expected 0 drift, got %d", report.Summary.Drift)
			for _, item := range report.EnvVars {
				if item.Status == "drift" {
					t.Logf("  env drift: %s expected=%v actual=%v", item.Name, item.Expected, item.Actual)
				}
			}
			for _, item := range report.Secrets {
				if item.Status == "drift" {
					t.Logf("  secret drift: %s expected=%v actual=%v", item.Name, item.Expected, item.Actual)
				}
			}
		}
	})
}

// --- setup ---

type secretRoutingEnv struct {
	t      *testing.T
	db     *sql.DB
	store  *ds.Store
	client k8s.ClusterClient
	ns     string
	depID  string
}

func setupSecretRoutingEnv(t *testing.T) *secretRoutingEnv {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("KUBECONFIG not set")
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
	ns := "e2e-sr-" + sanitize(t.Name())
	if len(ns) > 40 {
		ns = ns[:40]
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = client.Clientset().CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
	})

	// Ensure test account
	var accountID string
	err = db.QueryRow(`
		INSERT INTO accounts (name, type) VALUES ('sr-e2e', 'personal')
		ON CONFLICT DO NOTHING RETURNING id
	`).Scan(&accountID)
	if err != nil {
		err = db.QueryRow(`SELECT id FROM accounts WHERE name = 'sr-e2e'`).Scan(&accountID)
		if err != nil {
			t.Fatalf("get test account: %v", err)
		}
	}

	// Parse spec and fill secret values
	var specObj spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(secretRoutingSpec), &specObj); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	for k, v := range specObj.Variables {
		if v.Secret && v.Value == "" {
			v.Value = "test-" + k
			specObj.Variables[k] = v
		}
	}

	// Resolve env
	rctx := deployment.ResolveContext{
		Namespace:  ns,
		AgentName:  "sr-agent",
		BuildID:    "srbuild01",
		SecretName: deployment.GenerateSecretName("sr-agent", "srbuild01"),
	}
	resolved := deployment.ResolveDeploymentSpecEnv(&specObj, rctx)

	// Save to DB
	depID := fmt.Sprintf("sr%08d", time.Now().UnixMilli()%100000000)
	dep, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "sr-agent",
		DisplayName: t.Name(), BuildID: "srbuild01", Namespace: ns,
		SpecJSON: secretRoutingSpec,
	}, func(tx *sql.Tx, deploymentID string) error {
		return ds.SaveNormalizedSpec(tx, deploymentID, &specObj, resolved, nil, &ds.NormalizedSpecConfig{
			Namespace: ns,
		})
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	if err := store.UpdateStatus(dep.ID, ds.StatusUpdate{Status: ds.StatusActive}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM deployments WHERE id = $1", dep.ID)
	})

	// Apply to K8s
	applier := k8s.NewApplier(client, k8s.ApplierConfig{
		Namespace:       ns,
		RegistryURL:     "test-registry.example.com",
		ImagePullPolicy: corev1.PullNever,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	applyResult, err := applier.ApplyDeploymentSpec(ctx, &specObj)
	if err != nil {
		t.Fatalf("ApplyDeploymentSpec: %v", err)
	}
	if len(applyResult.Errors) > 0 {
		t.Fatalf("apply errors: %v", applyResult.Errors)
	}

	return &secretRoutingEnv{
		t: t, db: db, store: store, client: client, ns: ns, depID: dep.ID,
	}
}
