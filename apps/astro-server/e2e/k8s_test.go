//go:build k8s

// Package e2e contains integration tests that run against a real Kubernetes
// cluster (vcluster in CI, or any kubeconfig-accessible cluster locally).
// These tests verify that the K8s applier correctly creates, updates, and
// cleans up real resources — catching issues that the fake clientset misses
// (immutable field conflicts, resource version handling, label selectors).
//
// Run: go test -tags k8s -race ./e2e/...
// Requires: KUBECONFIG pointing at an accessible cluster.
package e2e

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	spec "github.com/astropods/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// minimalSpecJSON is a small deployment spec for testing the applier against a
// real cluster. Uses busybox to avoid pulling real images (PullNever policy).
const minimalSpecJSON = `{
  "spec": "deployment/v1",
  "source": {"account": "test-account", "name": "k8s-e2e", "build": "build001", "registry": "test-registry.example.com"},
  "target": {"runtime": "kubernetes", "account": "test-account", "display_name": "K8s E2E Test Agent"},
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
    },
    "docs": {
      "image": "gcr.io/google-containers/pause:3.2",
      "endpoints": {"http": {"port": 6333, "protocol": "http"}},
      "replicas": 1,
      "persistent": true,
      "storage": {"size": "1Gi", "access_mode": "ReadWriteOnce"},
      "resources": {"cpu": "50m", "memory": "64Mi", "cpu_limit": "100m", "memory_limit": "128Mi"},
      "provider": "qdrant",
      "update": {"strategy": "recreate"}
    }
  },
  "variables": {
    "SECRET_KEY": {"secret": true, "targets": ["agent"]}
  },
  "observability": {"enabled": false}
}`

func parseMinimalSpec(t *testing.T) *spec.AstroDeploymentSpec {
	t.Helper()
	var s spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(minimalSpecJSON), &s); err != nil {
		t.Fatalf("parse minimal spec: %v", err)
	}
	return &s
}

func copySpec(t *testing.T, s *spec.AstroDeploymentSpec) *spec.AstroDeploymentSpec {
	t.Helper()
	data, _ := json.Marshal(s)
	var out spec.AstroDeploymentSpec
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("copy spec: %v", err)
	}
	return &out
}

func clusterClient(t *testing.T) k8s.ClusterClient {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("KUBECONFIG not set — skipping k8s integration test")
	}
	client, err := k8s.NewClusterClient(context.Background(), k8s.ClusterClientConfig{
		Mode:           k8s.ClientModeLocal,
		KubeconfigPath: kubeconfig,
	})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	if err := client.CheckHealth(); err != nil {
		t.Fatalf("cluster health check failed: %v", err)
	}
	return client
}

func uniqueNS(t *testing.T) string {
	// Use a short hash of the test name to keep namespace names short and valid
	name := t.Name()
	if len(name) > 20 {
		name = name[:20]
	}
	return "e2e-" + sanitize(name)
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			out = append(out, c)
		} else if c >= 'A' && c <= 'Z' {
			out = append(out, c+32) // lowercase
		} else {
			out = append(out, '-')
		}
	}
	return string(out)
}

func cleanupNamespace(t *testing.T, clientset kubernetes.Interface, ns string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = clientset.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
	})
}

func fillSecrets(s *spec.AstroDeploymentSpec) {
	for k, v := range s.Variables {
		if v.Secret && v.Value == "" {
			v.Value = "test-value-" + k
			s.Variables[k] = v
		}
	}
}

func applySpec(t *testing.T, client k8s.ClusterClient, ns string, s *spec.AstroDeploymentSpec, extraCfg *k8s.ApplierConfig) *k8s.ApplyResult {
	t.Helper()

	cfg := k8s.ApplierConfig{
		Namespace:       ns,
		RegistryURL:     "test-registry.example.com",
		ImagePullPolicy: corev1.PullNever,
	}
	if extraCfg != nil {
		if extraCfg.PodSubnetCIDRs != nil {
			cfg.PodSubnetCIDRs = extraCfg.PodSubnetCIDRs
		}
	}

	applier := k8s.NewApplier(client, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := applier.ApplyDeploymentSpec(ctx, s)
	if err != nil {
		t.Fatalf("ApplyDeploymentSpec: %v", err)
	}
	return result
}

func applyMinimalSpec(t *testing.T, client k8s.ClusterClient, ns string) *k8s.ApplyResult {
	t.Helper()
	s := parseMinimalSpec(t)
	fillSecrets(s)
	return applySpec(t, client, ns, s, nil)
}

// --- Tests ---

func TestK8s_ApplyCreatesNamespace(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applyMinimalSpec(t, client, ns)

	ctx := context.Background()
	namespace, err := client.Clientset().CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("namespace %q not found: %v", ns, err)
	}
	if namespace.Labels["app.kubernetes.io/managed-by"] != "astro-server" {
		t.Errorf("namespace missing managed-by label: %v", namespace.Labels)
	}
}

func TestK8s_ApplyCreatesSecretAndConfigMap(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applyMinimalSpec(t, client, ns)

	ctx := context.Background()

	// Secret should exist with the credential
	secretName := deployment.GenerateSecretName("k8s-e2e", "build001")
	secret, err := client.Clientset().CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("secret %q not found: %v", secretName, err)
	}
	if _, ok := secret.Data["SECRET_KEY"]; !ok {
		t.Errorf("secret missing SECRET_KEY, keys: %v", secretKeys(secret))
	}

	// ConfigMap should exist with resolved env
	cmName := deployment.GenerateConfigMapName("k8s-e2e", "build001")
	cm, err := client.Clientset().CoreV1().ConfigMaps(ns).Get(ctx, cmName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("configmap %q not found: %v", cmName, err)
	}
	if len(cm.Data) == 0 {
		t.Error("configmap has no data entries")
	}
}

func TestK8s_ApplyCreatesServices(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applyMinimalSpec(t, client, ns)

	ctx := context.Background()
	svcs, err := client.Clientset().CoreV1().Services(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if err != nil {
		t.Fatalf("list services: %v", err)
	}

	// Expect: agent + cache + docs = 3 services
	if len(svcs.Items) < 3 {
		names := make([]string, len(svcs.Items))
		for i, s := range svcs.Items {
			names[i] = s.Name
		}
		t.Fatalf("expected at least 3 services, got %d: %v", len(svcs.Items), names)
	}

	// Verify agent service has correct port
	agentSvcName := deployment.GenerateAgentResourceName("k8s-e2e", "agent")
	agentSvc, err := client.Clientset().CoreV1().Services(ns).Get(ctx, agentSvcName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("agent service %q not found: %v", agentSvcName, err)
	}
	if agentSvc.Spec.Ports[0].Port != 8080 {
		t.Errorf("agent service port: got %d, want 8080", agentSvc.Spec.Ports[0].Port)
	}
	if agentSvc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("agent service type: got %s, want ClusterIP", agentSvc.Spec.Type)
	}
}

func TestK8s_ApplyCreatesDeployments(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applyMinimalSpec(t, client, ns)

	ctx := context.Background()
	depls, err := client.Clientset().AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}

	// Expect: agent + cache (non-persistent knowledge) = 2
	if len(depls.Items) != 2 {
		names := make([]string, len(depls.Items))
		for i, d := range depls.Items {
			names[i] = d.Name
		}
		t.Fatalf("expected 2 deployments, got %d: %v", len(depls.Items), names)
	}

	// Verify agent deployment
	agentDeplName := deployment.GenerateAgentResourceName("k8s-e2e", "agent")
	agentDepl, err := client.Clientset().AppsV1().Deployments(ns).Get(ctx, agentDeplName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("agent deployment %q not found: %v", agentDeplName, err)
	}
	if *agentDepl.Spec.Replicas != 1 {
		t.Errorf("agent replicas: got %d, want 1", *agentDepl.Spec.Replicas)
	}
	if agentDepl.Spec.Template.Spec.Containers[0].ImagePullPolicy != corev1.PullNever {
		t.Errorf("agent image pull policy: got %s, want Never", agentDepl.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	}
}

func TestK8s_ApplyCreatesStatefulSets(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applyMinimalSpec(t, client, ns)

	ctx := context.Background()
	ssets, err := client.Clientset().AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if err != nil {
		t.Fatalf("list statefulsets: %v", err)
	}

	// Expect: docs (persistent knowledge) = 1
	if len(ssets.Items) != 1 {
		names := make([]string, len(ssets.Items))
		for i, s := range ssets.Items {
			names[i] = s.Name
		}
		t.Fatalf("expected 1 statefulset, got %d: %v", len(ssets.Items), names)
	}

	ss := ssets.Items[0]
	if len(ss.Spec.VolumeClaimTemplates) == 0 {
		t.Error("statefulset missing volume claim templates")
	} else {
		storage := ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
		if storage.String() != "1Gi" {
			t.Errorf("storage size: got %s, want 1Gi", storage.String())
		}
	}
}

func TestK8s_ReapplyIsIdempotent(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	// First apply
	result1 := applyMinimalSpec(t, client, ns)
	if len(result1.Errors) > 0 {
		t.Fatalf("first apply had errors: %v", result1.Errors)
	}

	// Second apply — should update, not fail
	result2 := applyMinimalSpec(t, client, ns)
	if len(result2.Errors) > 0 {
		t.Fatalf("second apply had errors: %v", result2.Errors)
	}

	// Resources should be updated, not duplicated
	ctx := context.Background()
	depls, _ := client.Clientset().AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if len(depls.Items) != 2 {
		t.Errorf("expected 2 deployments after reapply, got %d", len(depls.Items))
	}
}

func TestK8s_CleanupRemovesStaleResources(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	// Apply with build001
	applyMinimalSpec(t, client, ns)

	// Apply with build002 — stale resources from build001 should be cleaned
	s := parseMinimalSpec(t)
	s.Source.Build = "build002"
	fillSecrets(s)

	applySpec(t, client, ns, s, nil)

	ctx := context.Background()

	// Old secret from build001 should be gone
	oldSecretName := deployment.GenerateSecretName("k8s-e2e", "build001")
	_, err := client.Clientset().CoreV1().Secrets(ns).Get(ctx, oldSecretName, metav1.GetOptions{})
	if err == nil {
		t.Errorf("stale secret %q still exists after build002 apply", oldSecretName)
	} else if !errors.IsNotFound(err) {
		t.Errorf("unexpected error checking stale secret: %v", err)
	}
}

func TestK8s_NetworkPoliciesApplied(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	s := parseMinimalSpec(t)
	fillSecrets(s)

	applySpec(t, client, ns, s, &k8s.ApplierConfig{
		PodSubnetCIDRs: []string{"10.0.0.0/16"},
	})

	ctx := context.Background()
	nps, err := client.Clientset().NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list network policies: %v", err)
	}
	if len(nps.Items) != 2 {
		names := make([]string, len(nps.Items))
		for i, np := range nps.Items {
			names[i] = np.Name
		}
		t.Fatalf("expected 2 network policies, got %d: %v", len(nps.Items), names)
	}

	npNames := map[string]bool{}
	for _, np := range nps.Items {
		npNames[np.Name] = true
	}
	if !npNames["default-deny-all"] {
		t.Error("missing default-deny-all network policy")
	}
	if !npNames["allow-namespace-traffic"] {
		t.Error("missing allow-namespace-traffic network policy")
	}
}

func TestK8s_LabelsAndSelectors(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applyMinimalSpec(t, client, ns)

	ctx := context.Background()

	// All resources should be findable by agent label
	depls, _ := client.Clientset().AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "astro.dev/agent=k8s-e2e",
	})
	if len(depls.Items) == 0 {
		t.Error("no deployments found with astro.dev/agent=k8s-e2e label")
	}

	svcs, _ := client.Clientset().CoreV1().Services(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "astro.dev/agent=k8s-e2e",
	})
	if len(svcs.Items) == 0 {
		t.Error("no services found with astro.dev/agent=k8s-e2e label")
	}

	// Verify deployment selector matches pod template labels
	for _, d := range depls.Items {
		selectorLabels := d.Spec.Selector.MatchLabels
		podLabels := d.Spec.Template.Labels
		for k, v := range selectorLabels {
			if podLabels[k] != v {
				t.Errorf("deployment %s: selector %s=%s not in pod template labels", d.Name, k, v)
			}
		}
	}
}

// --- helpers ---

func secretKeys(s *corev1.Secret) []string {
	keys := make([]string, 0, len(s.Data))
	for k := range s.Data {
		keys = append(keys, k)
	}
	return keys
}
