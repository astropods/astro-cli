package k8s

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComputeExpectedResourceNames_MinimalAgent(t *testing.T) {
	ds := minimalDeploymentSpec()
	expected := computeExpectedResourceNames(ds, "", "")

	agentName := deployment.GenerateAgentResourceName("my-agent", "agent")
	if !expected["Service"][agentName] {
		t.Errorf("expected agent service %s", agentName)
	}
	if !expected["Deployment"][agentName] {
		t.Errorf("expected agent deployment %s", agentName)
	}

	// No ingresses, statefulsets, cronjobs, jobs
	if len(expected["Ingress"]) != 0 {
		t.Errorf("expected no ingresses, got %v", expected["Ingress"])
	}
	if len(expected["StatefulSet"]) != 0 {
		t.Errorf("expected no statefulsets, got %v", expected["StatefulSet"])
	}
}

func TestComputeExpectedResourceNames_WithIntegrationsAndKnowledge(t *testing.T) {
	ds := minimalDeploymentSpec()
	ds.Integrations = map[string]spec.DeploymentIntegration{
		"search": {Image: "img", Endpoints: httpEp(3000), Replicas: 1, Update: spec.DefaultUpdateStrategy()},
	}
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"docs": {Image: "img", Endpoints: httpEp(6333), Replicas: 1, Persistent: true, Update: spec.DefaultUpdateStrategy()},
	}

	expected := computeExpectedResourceNames(ds, "", "")

	toolName := deployment.GenerateResourceName("my-agent", "integration", "search")
	if !expected["Service"][toolName] {
		t.Errorf("expected tool service %s", toolName)
	}
	if !expected["Deployment"][toolName] {
		t.Errorf("expected tool deployment %s", toolName)
	}

	kbName := deployment.GenerateResourceName("my-agent", "knowledge", "docs")
	if !expected["Service"][kbName] {
		t.Errorf("expected knowledge service %s", kbName)
	}
	if !expected["StatefulSet"][kbName] {
		t.Errorf("expected knowledge statefulset %s", kbName)
	}
	if expected["Deployment"][kbName] {
		t.Error("persistent knowledge should not produce a deployment")
	}
}

func TestComputeExpectedResourceNames_WithIngestion(t *testing.T) {
	ds := minimalDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"daily": {Image: "img", Trigger: spec.DeploymentTrigger{Type: "schedule", Schedule: "0 0 * * *"}},
		"init":  {Image: "img", Trigger: spec.DeploymentTrigger{Type: "startup"}},
		"hook":  {Image: "img", Endpoints: httpEp(9090), Trigger: spec.DeploymentTrigger{Type: "webhook"}},
	}

	expected := computeExpectedResourceNames(ds, "", "ingestion.example.com")

	cronName := deployment.GenerateResourceName("my-agent", "ingestion", "daily")
	if !expected["CronJob"][cronName] {
		t.Errorf("expected cronjob %s", cronName)
	}

	jobName := deployment.GenerateResourceName("my-agent", "ingestion", "init")
	if !expected["Job"][jobName] {
		t.Errorf("expected job %s", jobName)
	}

	webhookName := deployment.GenerateResourceName("my-agent", "ingestion", "hook")
	if !expected["Service"][webhookName] {
		t.Errorf("expected webhook service %s", webhookName)
	}
	if !expected["Deployment"][webhookName] {
		t.Errorf("expected webhook deployment %s", webhookName)
	}

	webhookIngressName := deployment.GenerateResourceName("my-agent", "ingress", "hook")
	if !expected["Ingress"][webhookIngressName] {
		t.Errorf("expected webhook ingress %s", webhookIngressName)
	}
}

func TestComputeExpectedResourceNames_WithObservability(t *testing.T) {
	ds := minimalDeploymentSpec()
	ds.Observability = spec.DeploymentObservability{Enabled: true}

	expected := computeExpectedResourceNames(ds, "", "")

	collectorName := deployment.GenerateAgentResourceName("my-agent", "collector")
	if !expected["Service"][collectorName] {
		t.Errorf("expected collector service %s", collectorName)
	}
	if !expected["Deployment"][collectorName] {
		t.Errorf("expected collector deployment %s", collectorName)
	}
}

func TestComputeExpectedResourceNames_WithMessagingAndIngress(t *testing.T) {
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http"},
		},
	}

	expected := computeExpectedResourceNames(ds, "example.com", "")

	msgName := deployment.GenerateAgentResourceName("my-agent", "messaging")
	if !expected["Service"][msgName] {
		t.Errorf("expected messaging service %s", msgName)
	}

	ingressName := deployment.GenerateAgentResourceName("my-agent", "ingress-messaging")
	if !expected["Ingress"][ingressName] {
		t.Errorf("expected messaging ingress %s", ingressName)
	}
}

func TestCleanupOrphanedResources_DeletesRemovedTool(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()

	// First deploy with an integration
	ds := minimalDeploymentSpec()
	ds.Integrations = map[string]spec.DeploymentIntegration{
		"search": {
			Image: "test-registry.example.com/search:latest", Endpoints: httpEp(3000),
			Replicas: 1, Update: spec.DefaultUpdateStrategy(),
		},
	}

	result1, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if len(result1.Errors) > 0 {
		t.Fatalf("first apply errors: %v", result1.Errors)
	}

	// Verify tool resources exist
	toolName := deployment.GenerateResourceName("my-agent", "integration", "search")
	_, err = a.clientset.CoreV1().Services(a.namespace).Get(ctx, toolName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("tool service should exist: %v", err)
	}
	_, err = a.clientset.AppsV1().Deployments(a.namespace).Get(ctx, toolName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("tool deployment should exist: %v", err)
	}

	// Redeploy without the integration
	ds2 := minimalDeploymentSpec()
	ds2.Integrations = nil

	result2, err := a.ApplyDeploymentSpec(ctx, ds2)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	// Orphan cleanup errors are non-fatal, but shouldn't happen here
	for _, e := range result2.Errors {
		if e.Kind == "Cleanup" {
			t.Errorf("unexpected cleanup error: %s", e.Error)
		}
	}

	// Tool resources should be deleted
	_, err = a.clientset.CoreV1().Services(a.namespace).Get(ctx, toolName, metav1.GetOptions{})
	if err == nil {
		t.Error("tool service should have been deleted")
	}
	_, err = a.clientset.AppsV1().Deployments(a.namespace).Get(ctx, toolName, metav1.GetOptions{})
	if err == nil {
		t.Error("tool deployment should have been deleted")
	}
}

func TestCleanupOrphanedResources_KeepsCurrentResources(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()

	ds := minimalDeploymentSpec()
	ds.Integrations = map[string]spec.DeploymentIntegration{
		"search": {
			Image: "test-registry.example.com/search:latest", Endpoints: httpEp(3000),
			Replicas: 1, Update: spec.DefaultUpdateStrategy(),
		},
	}

	// Apply twice — resources should survive
	_, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	_, err = a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}

	toolName := deployment.GenerateResourceName("my-agent", "integration", "search")
	_, err = a.clientset.CoreV1().Services(a.namespace).Get(ctx, toolName, metav1.GetOptions{})
	if err != nil {
		t.Error("tool service should still exist after reapply")
	}
	_, err = a.clientset.AppsV1().Deployments(a.namespace).Get(ctx, toolName, metav1.GetOptions{})
	if err != nil {
		t.Error("tool deployment should still exist after reapply")
	}
}

func TestCleanupOrphanedResources_DeletesOrphanedIngress(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	agentName := "my-agent"

	// Pre-create an orphaned ingress with the right labels
	labels := deployment.GenerateLabels("acme", agentName, "build-123", "tool-old")
	orphanIngress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-agent-ingress-old", Namespace: a.namespace, Labels: labels,
		},
	}
	_, err := a.clientset.NetworkingV1().Ingresses(a.namespace).Create(ctx, orphanIngress, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create orphan ingress: %v", err)
	}

	// Apply a minimal spec (no ingresses expected)
	ds := minimalDeploymentSpec()
	_, err = a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Orphaned ingress should be gone
	_, err = a.clientset.NetworkingV1().Ingresses(a.namespace).Get(ctx, "my-agent-ingress-old", metav1.GetOptions{})
	if err == nil {
		t.Error("orphaned ingress should have been deleted")
	}
}

func TestCleanupStaleBuildResources_SanitizedBuildID(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	agentName := "my-agent"

	// Create a resource with a sanitized dotted buildID (dots -> hyphens in labels)
	dottedBuildID := "1.0.0"
	sanitized := deployment.SanitizeName(dottedBuildID)
	labels := deployment.GenerateLabels("acme", agentName, dottedBuildID, "agent")

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-agent-agent", Namespace: a.namespace, Labels: labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 8080}},
		},
	}
	_, err := a.clientset.CoreV1().Services(a.namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	// The label should have the sanitized version
	if labels["app.kubernetes.io/version"] != sanitized {
		t.Fatalf("expected label version %q, got %q", sanitized, labels["app.kubernetes.io/version"])
	}

	// Cleanup with the same dotted buildID should NOT delete the resource
	errs := a.cleanupStaleBuildResources(ctx, "acme", agentName, dottedBuildID)
	if len(errs) > 0 {
		t.Fatalf("unexpected cleanup errors: %v", errs)
	}

	_, err = a.clientset.CoreV1().Services(a.namespace).Get(ctx, "my-agent-agent", metav1.GetOptions{})
	if err != nil {
		t.Error("service should NOT have been deleted when buildID matches (after sanitization)")
	}

	// Cleanup with a different buildID should delete it
	errs = a.cleanupStaleBuildResources(ctx, "acme", agentName, "2.0.0")
	if len(errs) > 0 {
		t.Fatalf("unexpected cleanup errors: %v", errs)
	}

	_, err = a.clientset.CoreV1().Services(a.namespace).Get(ctx, "my-agent-agent", metav1.GetOptions{})
	if err == nil {
		t.Error("service should have been deleted for different buildID")
	}
}

func TestCleanupOrphanedResources_DeletesOrphanedStatefulSet(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	agentName := "my-agent"

	// Pre-create an orphaned statefulset
	labels := deployment.GenerateLabels("acme", agentName, "build-123", "knowledge-old")
	orphanSS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-agent-knowledge-old", Namespace: a.namespace, Labels: labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
			},
		},
	}
	_, err := a.clientset.AppsV1().StatefulSets(a.namespace).Create(ctx, orphanSS, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create orphan statefulset: %v", err)
	}

	ds := minimalDeploymentSpec()
	_, err = a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	_, err = a.clientset.AppsV1().StatefulSets(a.namespace).Get(ctx, "my-agent-knowledge-old", metav1.GetOptions{})
	if err == nil {
		t.Error("orphaned statefulset should have been deleted")
	}
}

func TestCleanupOrphanedResources_DeletesOrphanedCronJob(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	agentName := "my-agent"

	// Pre-create an orphaned cronjob
	labels := deployment.GenerateLabels("acme", agentName, "build-123", "ingestion-old")
	orphanCJ := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-agent-ingestion-old", Namespace: a.namespace, Labels: labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 0 * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers:    []corev1.Container{{Name: "c", Image: "img"}},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			},
		},
	}
	_, err := a.clientset.BatchV1().CronJobs(a.namespace).Create(ctx, orphanCJ, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create orphan cronjob: %v", err)
	}

	ds := minimalDeploymentSpec()
	_, err = a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	_, err = a.clientset.BatchV1().CronJobs(a.namespace).Get(ctx, "my-agent-ingestion-old", metav1.GetOptions{})
	if err == nil {
		t.Error("orphaned cronjob should have been deleted")
	}
}
