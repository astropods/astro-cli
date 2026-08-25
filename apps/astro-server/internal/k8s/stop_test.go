package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func managed() map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": "astro-server"}
}

func replicas(n int32) *int32 { return &n }

func newDeploy(name string, labels map[string]string, want int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "tenant", Labels: labels},
		Spec:       appsv1.DeploymentSpec{Replicas: replicas(want)},
	}
}

func newStatefulSet(name string, labels map[string]string, want int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "tenant", Labels: labels},
		Spec:       appsv1.StatefulSetSpec{Replicas: replicas(want)},
	}
}

func newCronJob(name string, labels map[string]string, suspended bool) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "tenant", Labels: labels},
		Spec:       batchv1.CronJobSpec{Suspend: &suspended},
	}
}

func deploymentReplicas(t *testing.T, cs kubernetes.Interface, name string) int32 {
	t.Helper()
	got, err := cs.AppsV1().Deployments("tenant").Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment %s: %v", name, err)
	}
	return *got.Spec.Replicas
}

// A workload kind that never reaches zero keeps the account spending after the
// gating decision went against it.
func TestStopNamespaceWorkloads_ScalesEveryManagedWorkloadToZero(t *testing.T) {
	cs := fake.NewClientset(
		newDeploy("api", managed(), 3),
		newStatefulSet("db", managed(), 2),
		newCronJob("nightly", managed(), false),
	)

	if err := StopNamespaceWorkloads(context.Background(), cs, "tenant"); err != nil {
		t.Fatalf("StopNamespaceWorkloads: %v", err)
	}

	if got := deploymentReplicas(t, cs, "api"); got != 0 {
		t.Errorf("deployment replicas = %d, want 0", got)
	}
	ss, err := cs.AppsV1().StatefulSets("tenant").Get(context.Background(), "db", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	if *ss.Spec.Replicas != 0 {
		t.Errorf("statefulset replicas = %d, want 0", *ss.Spec.Replicas)
	}
	cj, err := cs.BatchV1().CronJobs("tenant").Get(context.Background(), "nightly", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get cronjob: %v", err)
	}
	if !*cj.Spec.Suspend {
		t.Error("cronjob suspend = false, want true")
	}
}

// Scaling an unlabelled workload makes a billing action reach past its own
// blast radius.
func TestStopNamespaceWorkloads_LeavesUnmanagedWorkloadsRunning(t *testing.T) {
	cs := fake.NewClientset(
		newDeploy("ours", managed(), 3),
		newDeploy("theirs", map[string]string{"app": "sidecar"}, 4),
	)

	if err := StopNamespaceWorkloads(context.Background(), cs, "tenant"); err != nil {
		t.Fatalf("StopNamespaceWorkloads: %v", err)
	}

	if got := deploymentReplicas(t, cs, "ours"); got != 0 {
		t.Errorf("managed replicas = %d, want 0", got)
	}
	if got := deploymentReplicas(t, cs, "theirs"); got != 4 {
		t.Errorf("unmanaged replicas = %d, want 4 left untouched", got)
	}
}

// River retries this job and the purge sweep re-runs it, so a second pass that
// errored would fail forever.
func TestStopNamespaceWorkloads_SecondPassIsANoop(t *testing.T) {
	cs := fake.NewClientset(
		newDeploy("api", managed(), 3),
		newCronJob("nightly", managed(), true),
	)

	for pass := 1; pass <= 2; pass++ {
		if err := StopNamespaceWorkloads(context.Background(), cs, "tenant"); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
	}

	if got := deploymentReplicas(t, cs, "api"); got != 0 {
		t.Errorf("replicas = %d, want 0", got)
	}
}

func TestStopNamespaceWorkloads_EmptyNamespaceSucceeds(t *testing.T) {
	if err := StopNamespaceWorkloads(context.Background(), fake.NewClientset(), "tenant"); err != nil {
		t.Fatalf("StopNamespaceWorkloads: %v", err)
	}
}
