//go:build k8s

// Package e2e contains integration tests that run against a real Kubernetes
// cluster (vcluster in CI, or any kubeconfig-accessible cluster locally).
// These tests verify that the K8s applier correctly creates, updates, and
// cleans up real resources — catching issues that the fake clientset misses
// (immutable field conflicts, resource version handling, label selectors).
//
// Run: go test -tags k8s -race ./e2e/...   (CI adds -race on main pushes only)
// CI job: `K8s integration tests (vcluster + Postgres)` in .github/workflows/test.yml.
// Postgres-only e2e tests use //go:build integration and run in a separate job.
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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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

// slackSpecJSON focuses on messaging-sidecar env wiring for Slack.
const slackSpecJSON = `{
  "spec": "deployment/v1",
  "source": {"account": "test-account", "name": "k8s-slack-e2e", "build": "build001", "registry": "test-registry.example.com"},
  "target": {"runtime": "kubernetes", "account": "test-account", "display_name": "K8s Slack E2E"},
  "agent": {
    "image": "gcr.io/google-containers/pause:3.2",
    "endpoints": {"http": {"port": 8080, "protocol": "http"}},
    "replicas": 1,
    "resources": {"cpu": "50m", "memory": "64Mi", "cpu_limit": "100m", "memory_limit": "128Mi"},
    "environment": {"AGENT_PORT": "8080"},
    "update": {"strategy": "rolling"}
  },
  "interfaces": {
    "adapters": ["slack"],
    "image": "gcr.io/google-containers/pause:3.2",
    "endpoints": {"grpc": {"port": 9090, "protocol": "grpc"}},
    "resources": {"cpu": "50m", "memory": "64Mi", "cpu_limit": "100m", "memory_limit": "128Mi"},
    "environment": {
      "SLACK_CONFIG": "${variables.SLACK_CONFIG}",
      "SLACK_BOT_TOKEN": "${variables.SLACK_BOT_TOKEN}",
      "SLACK_APP_TOKEN": "${variables.SLACK_APP_TOKEN}"
    }
  },
  "variables": {
    "SLACK_CONFIG": {"value": "{\"actionable_reactions\":[\"ticket\",\"bug\"],\"allowed_channel_ids\":[\"C123\",\"C999\"],\"allowed_user_ids\":[\"U123\",\"U999\"]}", "secret": false, "targets": ["interface.slack"]},
    "SLACK_BOT_TOKEN": {"value": "xoxb-test", "secret": true, "targets": ["interface.slack"]},
    "SLACK_APP_TOKEN": {"value": "xapp-test", "secret": true, "targets": ["interface.slack"]}
  },
  "observability": {"enabled": false}
}`

func parseMinimalSpec(t *testing.T) *deployment.AstroDeploymentSpec {
	t.Helper()
	var s deployment.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(minimalSpecJSON), &s); err != nil {
		t.Fatalf("parse minimal spec: %v", err)
	}
	return &s
}

func parseSlackSpec(t *testing.T) *deployment.AstroDeploymentSpec {
	t.Helper()
	var s deployment.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(slackSpecJSON), &s); err != nil {
		t.Fatalf("parse slack spec: %v", err)
	}
	return &s
}

func copySpec(t *testing.T, s *deployment.AstroDeploymentSpec) *deployment.AstroDeploymentSpec {
	t.Helper()
	data, _ := json.Marshal(s)
	var out deployment.AstroDeploymentSpec
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

	// If a namespace from a previous run is stuck in Terminating, delete it
	// and wait for it to be fully gone before proceeding. Creating resources
	// in a Terminating namespace silently discards them.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	existing, err := clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil && existing.Status.Phase == corev1.NamespaceTerminating {
		t.Logf("namespace %s is Terminating from previous run, waiting for deletion", ns)
		_ = wait.PollUntilContextCancel(ctx, 1*time.Second, true, func(ctx context.Context) (bool, error) {
			_, err := clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, nil
		})
	} else if err == nil {
		// Namespace exists and is active — delete it and wait
		_ = clientset.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
		_ = wait.PollUntilContextCancel(ctx, 1*time.Second, true, func(ctx context.Context) (bool, error) {
			_, err := clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, nil
		})
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = clientset.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
	})
}

// waitForStatefulSets polls until exactly count managed StatefulSets exist in ns.
// Needed because vcluster controller sync can lag behind the API server on slow CI.
func waitForStatefulSets(t *testing.T, clientset kubernetes.Interface, ns string, count int) []appsv1.StatefulSet {
	t.Helper()
	var result []appsv1.StatefulSet
	var lastSeen int
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := wait.PollUntilContextCancel(ctx, 500*time.Millisecond, true, func(ctx context.Context) (bool, error) {
		ssets, err := clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=astro-server",
		})
		if err != nil {
			return false, nil
		}
		lastSeen = len(ssets.Items)
		if lastSeen == count {
			result = ssets.Items
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		names := listResourceNames(clientset, ns, "statefulsets")
		t.Fatalf("timed out waiting for %d statefulsets in %s (saw %d): %v\n  found: %v", count, ns, lastSeen, err, names)
	}
	return result
}

// listResourceNames returns the names of managed resources in a namespace for diagnostics.
func listResourceNames(clientset kubernetes.Interface, ns, kind string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var names []string
	switch kind {
	case "deployments":
		list, err := clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, d := range list.Items {
				names = append(names, d.Name)
			}
		}
	case "statefulsets":
		list, err := clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, s := range list.Items {
				names = append(names, s.Name)
			}
		}
	}
	return names
}

func fillSecrets(s *deployment.AstroDeploymentSpec) {
	for k, v := range s.Variables {
		if v.Secret && v.Value == "" {
			v.Value = "test-value-" + k
			s.Variables[k] = v
		}
	}
}

func applySpec(t *testing.T, client k8s.ClusterClient, ns string, s *deployment.AstroDeploymentSpec, extraCfg *k8s.ApplierConfig) *k8s.ApplyResult {
	t.Helper()

	cfg := k8s.ApplierConfig{
		Namespace:       ns,
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

func applySlackSpec(t *testing.T, client k8s.ClusterClient, ns string) *k8s.ApplyResult {
	t.Helper()
	s := parseSlackSpec(t)
	fillSecrets(s)
	return applySpec(t, client, ns, s, nil)
}

func applySlackSpecWithEmptyAllowlist(t *testing.T, client k8s.ClusterClient, ns string) *k8s.ApplyResult {
	t.Helper()
	s := parseSlackSpec(t)
	if v, ok := s.Variables["SLACK_CONFIG"]; ok {
		v.Value = `{"actionable_reactions":["ticket","bug"]}`
		s.Variables["SLACK_CONFIG"] = v
	}
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

	// Poll until the vcluster controller syncs the workloads. The agent now runs
	// as a StatefulSet (every agent gets a persistent disk), alongside the
	// qdrant knowledge StatefulSet — redis knowledge stays a Deployment.
	waitForStatefulSets(t, client.Clientset(), ns, 2)

	ctx := context.Background()

	// Verify agent StatefulSet
	agentDeplName := deployment.GenerateAgentResourceName("k8s-e2e", "agent")
	agentDepl, err := client.Clientset().AppsV1().StatefulSets(ns).Get(ctx, agentDeplName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("agent statefulset %q not found: %v", agentDeplName, err)
	}
	if *agentDepl.Spec.Replicas != 1 {
		t.Errorf("agent replicas: got %d, want 1", *agentDepl.Spec.Replicas)
	}
	if agentDepl.Spec.Template.Spec.Containers[0].ImagePullPolicy != corev1.PullNever {
		t.Errorf("agent image pull policy: got %s, want Never", agentDepl.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	}
}

func TestK8s_SlackReactionsEnvOnMessagingSidecar(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applySlackSpec(t, client, ns)

	waitForStatefulSets(t, client.Clientset(), ns, 1)

	ctx := context.Background()
	agentDeplName := deployment.GenerateAgentResourceName("k8s-slack-e2e", "agent")
	agentDepl, err := client.Clientset().AppsV1().StatefulSets(ns).Get(ctx, agentDeplName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("agent statefulset %q not found: %v", agentDeplName, err)
	}

	var messaging *corev1.Container
	for i := range agentDepl.Spec.Template.Spec.InitContainers {
		if agentDepl.Spec.Template.Spec.InitContainers[i].Name == "messaging" {
			messaging = &agentDepl.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if messaging == nil {
		t.Fatal("messaging sidecar not found on agent deployment")
	}

	envMap := make(map[string]string)
	for _, e := range messaging.Env {
		envMap[e.Name] = e.Value
	}
	wantCfg := `{"actionable_reactions":["ticket","bug"],"allowed_channel_ids":["C123","C999"],"allowed_user_ids":["U123","U999"]}`
	if envMap["SLACK_CONFIG"] != wantCfg {
		t.Errorf("SLACK_CONFIG = %q, want %q", envMap["SLACK_CONFIG"], wantCfg)
	}
}

func TestK8s_SlackAllowlistEmptyDefaultsOnMessagingSidecar(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applySlackSpecWithEmptyAllowlist(t, client, ns)

	waitForStatefulSets(t, client.Clientset(), ns, 1)

	ctx := context.Background()
	agentDeplName := deployment.GenerateAgentResourceName("k8s-slack-e2e", "agent")
	agentDepl, err := client.Clientset().AppsV1().StatefulSets(ns).Get(ctx, agentDeplName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("agent statefulset %q not found: %v", agentDeplName, err)
	}

	var messaging *corev1.Container
	for i := range agentDepl.Spec.Template.Spec.InitContainers {
		if agentDepl.Spec.Template.Spec.InitContainers[i].Name == "messaging" {
			messaging = &agentDepl.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if messaging == nil {
		t.Fatal("messaging sidecar not found on agent deployment")
	}

	envMap := make(map[string]string)
	for _, e := range messaging.Env {
		envMap[e.Name] = e.Value
	}

	wantCfg := `{"actionable_reactions":["ticket","bug"]}`
	if envMap["SLACK_CONFIG"] != wantCfg {
		t.Errorf("SLACK_CONFIG = %q, want %q", envMap["SLACK_CONFIG"], wantCfg)
	}
}

func TestK8s_SlackSecretsStayInSecretRef(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applySlackSpec(t, client, ns)

	waitForStatefulSets(t, client.Clientset(), ns, 1)

	ctx := context.Background()
	agentDeplName := deployment.GenerateAgentResourceName("k8s-slack-e2e", "agent")
	agentDepl, err := client.Clientset().AppsV1().StatefulSets(ns).Get(ctx, agentDeplName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("agent statefulset %q not found: %v", agentDeplName, err)
	}

	var messaging *corev1.Container
	for i := range agentDepl.Spec.Template.Spec.InitContainers {
		if agentDepl.Spec.Template.Spec.InitContainers[i].Name == "messaging" {
			messaging = &agentDepl.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if messaging == nil {
		t.Fatal("messaging sidecar not found on agent deployment")
	}

	for _, e := range messaging.Env {
		if e.Name == "SLACK_BOT_TOKEN" || e.Name == "SLACK_APP_TOKEN" {
			t.Errorf("%s should not be present as plaintext env var on messaging container", e.Name)
		}
	}

	messagingSecretName := deployment.GenerateMessagingSecretName("k8s-slack-e2e", "build001")
	hasSecretEnvFrom := false
	for _, from := range messaging.EnvFrom {
		if from.SecretRef != nil && from.SecretRef.Name == messagingSecretName {
			hasSecretEnvFrom = true
			break
		}
	}
	if !hasSecretEnvFrom {
		t.Fatalf("messaging sidecar should source credentials from Secret %q via envFrom", messagingSecretName)
	}

	agentSecretName := deployment.GenerateSecretName("k8s-slack-e2e", "build001")
	for _, from := range messaging.EnvFrom {
		if from.SecretRef != nil && from.SecretRef.Name == agentSecretName {
			t.Errorf("messaging sidecar should not envFrom the agent's full credentials Secret %q", agentSecretName)
		}
	}

	secret, err := client.Clientset().CoreV1().Secrets(ns).Get(ctx, messagingSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("messaging credentials secret %q not found: %v", messagingSecretName, err)
	}
	if _, ok := secret.Data["SLACK_BOT_TOKEN"]; !ok {
		t.Error("messaging credentials secret missing SLACK_BOT_TOKEN")
	}

	configMapName := deployment.GenerateConfigMapName("k8s-slack-e2e", "build001")
	cm, err := client.Clientset().CoreV1().ConfigMaps(ns).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("configmap %q not found: %v", configMapName, err)
	}
	if _, ok := cm.Data["SLACK_BOT_TOKEN"]; ok {
		t.Error("SLACK_BOT_TOKEN should not be present in ConfigMap data")
	}
	if _, ok := cm.Data["SLACK_APP_TOKEN"]; ok {
		t.Error("SLACK_APP_TOKEN should not be present in ConfigMap data")
	}
}

func TestK8s_ApplyCreatesStatefulSets(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applyMinimalSpec(t, client, ns)

	// Poll until the vcluster controller syncs the StatefulSets: the qdrant
	// knowledge store plus the agent (every agent gets a persistent disk).
	items := waitForStatefulSets(t, client.Clientset(), ns, 2)

	// Assert the qdrant knowledge store's volume is sized per its spec (1Gi).
	knowledgeName := deployment.GenerateAgentResourceName("k8s-e2e", "knowledge-docs")
	var knowledge *appsv1.StatefulSet
	for i := range items {
		if items[i].Name == knowledgeName {
			knowledge = &items[i]
			break
		}
	}
	if knowledge == nil {
		t.Fatalf("knowledge StatefulSet %q not found among synced sets", knowledgeName)
	}
	if len(knowledge.Spec.VolumeClaimTemplates) == 0 {
		t.Error("statefulset missing volume claim templates")
	} else {
		storage := knowledge.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
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

	// Resources should be updated, not duplicated. The minimal spec yields one
	// Deployment (redis knowledge) and two StatefulSets (the agent's default
	// disk + the qdrant knowledge store).
	ctx := context.Background()
	depls, _ := client.Clientset().AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if len(depls.Items) != 1 {
		t.Errorf("expected 1 deployment after reapply, got %d", len(depls.Items))
	}
	stss, _ := client.Clientset().AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if len(stss.Items) != 2 {
		t.Errorf("expected 2 statefulsets after reapply, got %d", len(stss.Items))
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

	// Policies astro-server applies as part of the per-deploy NP bundle.
	// (allow-apiserver-proxy is conditional on cpSubnetCIDRs, not set here.)
	want := []string{"default-deny-all", "allow-namespace-traffic", "allow-from-tenant-router"}

	if len(nps.Items) != len(want) {
		names := make([]string, len(nps.Items))
		for i, np := range nps.Items {
			names[i] = np.Name
		}
		t.Fatalf("expected %d network policies, got %d: %v", len(want), len(nps.Items), names)
	}

	npNames := map[string]bool{}
	for _, np := range nps.Items {
		npNames[np.Name] = true
	}
	for _, name := range want {
		if !npNames[name] {
			t.Errorf("missing %s network policy", name)
		}
	}
}

func TestK8s_LabelsAndSelectors(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	cleanupNamespace(t, client.Clientset(), ns)

	applyMinimalSpec(t, client, ns)

	ctx := context.Background()

	// All resources should be findable by agent label (account.agent format)
	agentLabel := deployment.LabelKeyAgent + "=" + deployment.AgentLabelValue("test-account", "k8s-e2e")
	depls, _ := client.Clientset().AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{
		LabelSelector: agentLabel,
	})
	if len(depls.Items) == 0 {
		t.Error("no deployments found with " + agentLabel + " label")
	}

	svcs, _ := client.Clientset().CoreV1().Services(ns).List(ctx, metav1.ListOptions{
		LabelSelector: agentLabel,
	})
	if len(svcs.Items) == 0 {
		t.Error("no services found with " + agentLabel + " label")
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

// ingestionScheduleSpecJSON includes a schedule ingestion trigger with a daily
// cron expression, used to verify that ApplyDeploymentSpec creates a CronJob.
const ingestionScheduleSpecJSON = `{
  "spec": "deployment/v1",
  "source": {"account": "test-account", "name": "k8s-ingest-e2e", "build": "build001", "registry": "test-registry.example.com"},
  "target": {"runtime": "kubernetes", "account": "test-account", "display_name": "K8s Ingestion E2E"},
  "agent": {
    "image": "gcr.io/google-containers/pause:3.2",
    "endpoints": {"http": {"port": 8080, "protocol": "http"}},
    "replicas": 1,
    "resources": {"cpu": "50m", "memory": "64Mi", "cpu_limit": "100m", "memory_limit": "128Mi"},
    "environment": {"AGENT_PORT": "8080"},
    "update": {"strategy": "rolling"}
  },
  "ingestion": {
    "daily": {
      "image": "gcr.io/google-containers/pause:3.2",
      "trigger": {"type": "schedule", "schedule": "0 0 * * *"},
      "resources": {"cpu": "50m", "memory": "64Mi", "cpu_limit": "100m", "memory_limit": "128Mi"}
    }
  },
  "observability": {"enabled": false}
}`

func parseIngestionScheduleSpec(t *testing.T) *deployment.AstroDeploymentSpec {
	t.Helper()
	var s deployment.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(ingestionScheduleSpecJSON), &s); err != nil {
		t.Fatalf("parse ingestion schedule spec: %v", err)
	}
	return &s
}

func applyIngestionScheduleSpec(t *testing.T, client k8s.ClusterClient, ns string) *k8s.ApplyResult {
	t.Helper()
	s := parseIngestionScheduleSpec(t)
	fillSecrets(s)
	return applySpec(t, client, ns, s, nil)
}

// Applies a spec with a schedule ingestion trigger and verifies that a CronJob
// is created in the namespace with the expected cron schedule.
func TestK8s_ScheduleIngestionCreatesCronJob(t *testing.T) {
	client := clusterClient(t)
	ns := uniqueNS(t)
	clientset := client.Clientset()
	cleanupNamespace(t, clientset, ns)

	applyIngestionScheduleSpec(t, client, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var cronJobNames []string
	err := wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		cronJobs, listErr := clientset.BatchV1().CronJobs(ns).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=astro-server",
		})
		if listErr != nil {
			return false, nil
		}
		if len(cronJobs.Items) == 0 {
			return false, nil
		}
		cronJobNames = make([]string, len(cronJobs.Items))
		for i, cj := range cronJobs.Items {
			cronJobNames[i] = cj.Name
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("timed out waiting for CronJob: %v", err)
	}

	if len(cronJobNames) != 1 {
		t.Fatalf("expected 1 CronJob, got %d: %v", len(cronJobNames), cronJobNames)
	}

	cj, err := clientset.BatchV1().CronJobs(ns).Get(ctx, cronJobNames[0], metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get CronJob: %v", err)
	}

	if cj.Spec.Schedule != "0 0 * * *" {
		t.Errorf("CronJob schedule = %q, want %q", cj.Spec.Schedule, "0 0 * * *")
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
