package k8s

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const testNS = "test-ns"
const testAgent = "my-agent"
const testBuild = "build-123"

// seedResources creates a full set of labeled resources in the fake cluster
// that mimic what ApplyDeploymentSpec would produce.
func seedResources(t *testing.T, client *fake.Clientset) {
	t.Helper()
	ctx := context.Background()
	labels := deployment.GenerateLabels(testAgent, testBuild, "agent")

	// Namespace
	_, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNS},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	// Service
	_, err = client.CoreV1().Services(testNS).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent-agent", Namespace: testNS, Labels: labels},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	// Deployment
	_, err = client.AppsV1().Deployments(testNS).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent-agent", Namespace: testNS, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}

	// Ingress
	_, err = client.NetworkingV1().Ingresses(testNS).Create(ctx, &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent-ingress-agent", Namespace: testNS, Labels: labels},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create ingress: %v", err)
	}

	// StatefulSet
	ssLabels := deployment.GenerateLabels(testAgent, testBuild, "knowledge-docs")
	_, err = client.AppsV1().StatefulSets(testNS).Create(ctx, &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent-knowledge-docs", Namespace: testNS, Labels: ssLabels},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create statefulset: %v", err)
	}

	// CronJob
	cjLabels := deployment.GenerateLabels(testAgent, testBuild, "ingestion-daily")
	_, err = client.BatchV1().CronJobs(testNS).Create(ctx, &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent-ingestion-daily", Namespace: testNS, Labels: cjLabels},
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
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create cronjob: %v", err)
	}

	// Job
	jobLabels := deployment.GenerateLabels(testAgent, testBuild, "ingestion-init")
	_, err = client.BatchV1().Jobs(testNS).Create(ctx, &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent-ingestion-init", Namespace: testNS, Labels: jobLabels},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers:    []corev1.Container{{Name: "c", Image: "img"}},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	// ConfigMap
	cmName := deployment.GenerateConfigMapName(testAgent, deployment.SanitizeName(testBuild))
	_, err = client.CoreV1().ConfigMaps(testNS).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: testNS, Labels: labels},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create configmap: %v", err)
	}

	// Secret
	secName := deployment.GenerateSecretName(testAgent, deployment.SanitizeName(testBuild))
	_, err = client.CoreV1().Secrets(testNS).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secName, Namespace: testNS, Labels: labels},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create secret: %v", err)
	}

	// PVC
	_, err = client.CoreV1().PersistentVolumeClaims(testNS).Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data-my-agent-knowledge-docs-0", Namespace: testNS, Labels: ssLabels},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create pvc: %v", err)
	}
}

func TestDeleter_DeleteAll(t *testing.T) {
	client := fake.NewClientset()
	seedResources(t, client)
	ctx := context.Background()

	deleter := NewDeleter(client, testNS)
	result, err := deleter.Delete(ctx, testAgent, testBuild)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify all resource kinds are present in the result
	kindCounts := map[string]int{}
	for _, r := range result.Resources {
		if r.Status != "deleted" {
			t.Errorf("resource %s/%s: expected status 'deleted', got %q", r.Kind, r.Name, r.Status)
		}
		kindCounts[r.Kind]++
	}

	expectedKinds := []string{"Service", "Deployment", "Ingress", "StatefulSet", "CronJob", "Job", "ConfigMap", "Secret", "PersistentVolumeClaim", "Namespace"}
	for _, kind := range expectedKinds {
		if kindCounts[kind] == 0 {
			t.Errorf("expected at least one deleted %s resource", kind)
		}
	}

	// Verify resources are actually gone
	svcs, _ := client.CoreV1().Services(testNS).List(ctx, metav1.ListOptions{})
	if len(svcs.Items) != 0 {
		t.Errorf("expected 0 services, got %d", len(svcs.Items))
	}
	deps, _ := client.AppsV1().Deployments(testNS).List(ctx, metav1.ListOptions{})
	if len(deps.Items) != 0 {
		t.Errorf("expected 0 deployments, got %d", len(deps.Items))
	}
	ings, _ := client.NetworkingV1().Ingresses(testNS).List(ctx, metav1.ListOptions{})
	if len(ings.Items) != 0 {
		t.Errorf("expected 0 ingresses, got %d", len(ings.Items))
	}
	ssets, _ := client.AppsV1().StatefulSets(testNS).List(ctx, metav1.ListOptions{})
	if len(ssets.Items) != 0 {
		t.Errorf("expected 0 statefulsets, got %d", len(ssets.Items))
	}
	cjs, _ := client.BatchV1().CronJobs(testNS).List(ctx, metav1.ListOptions{})
	if len(cjs.Items) != 0 {
		t.Errorf("expected 0 cronjobs, got %d", len(cjs.Items))
	}
	jobs, _ := client.BatchV1().Jobs(testNS).List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs.Items))
	}

	// Non-fatal errors for missing configmap/secret on empty buildID are expected
	// but no hard errors
	if len(result.Errors) > 0 {
		// ConfigMap and Secret might fail with "not found" when buildID is empty
		// but with the seeded resources they should be deleted
		for _, e := range result.Errors {
			t.Logf("error (may be expected): %s/%s: %s", e.Kind, e.Resource, e.Error)
		}
	}
}

func TestDeleter_DeleteIngresses(t *testing.T) {
	client := fake.NewClientset()
	ctx := context.Background()
	labels := deployment.GenerateLabels(testAgent, testBuild, "agent")

	// Create multiple ingresses
	for _, name := range []string{"my-agent-ingress-agent", "my-agent-ingress-messaging"} {
		_, err := client.NetworkingV1().Ingresses(testNS).Create(ctx, &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS, Labels: labels},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create ingress %s: %v", name, err)
		}
	}

	// Create namespace so namespace deletion doesn't fail
	_, _ = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNS},
	}, metav1.CreateOptions{})

	deleter := NewDeleter(client, testNS)
	result, err := deleter.Delete(ctx, testAgent, testBuild)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Both ingresses should be deleted
	ingressCount := 0
	for _, r := range result.Resources {
		if r.Kind == "Ingress" && r.Status == "deleted" {
			ingressCount++
		}
	}
	if ingressCount != 2 {
		t.Errorf("expected 2 deleted ingresses, got %d", ingressCount)
	}

	// Verify they're gone
	ings, _ := client.NetworkingV1().Ingresses(testNS).List(ctx, metav1.ListOptions{})
	if len(ings.Items) != 0 {
		t.Errorf("expected 0 ingresses remaining, got %d", len(ings.Items))
	}
}

func TestDeleter_EmptyNamespace(t *testing.T) {
	client := fake.NewClientset()
	ctx := context.Background()

	// Create namespace only
	_, _ = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNS},
	}, metav1.CreateOptions{})

	deleter := NewDeleter(client, testNS)
	result, err := deleter.Delete(ctx, testAgent, "")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Should have namespace deletion + configmap/secret errors (not found)
	nsDeleted := false
	for _, r := range result.Resources {
		if r.Kind == "Namespace" && r.Status == "deleted" {
			nsDeleted = true
		}
	}
	if !nsDeleted {
		t.Error("expected namespace to be deleted")
	}
}
