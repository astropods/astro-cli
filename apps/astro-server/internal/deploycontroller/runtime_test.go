package deploycontroller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

// newTestWatcher builds a clusterWatcher backed by a fake clientset seeded with
// the given objects, with all informer caches synced.
func newTestWatcher(t *testing.T, objs ...runtime.Object) *clusterWatcher {
	t.Helper()
	client := fake.NewSimpleClientset(objs...)
	factory := informers.NewSharedInformerFactory(client, 0)
	w := &clusterWatcher{
		deploys:   factory.Apps().V1().Deployments().Lister(),
		statefuls: factory.Apps().V1().StatefulSets().Lister(),
		jobs:      factory.Batch().V1().Jobs().Lister(),
		cronJobs:  factory.Batch().V1().CronJobs().Lister(),
		pods:      factory.Core().V1().Pods().Lister(),
		services:  factory.Core().V1().Services().Lister(),
	}
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	factory.Start(stop)
	factory.WaitForCacheSync(stop)
	return w
}

func TestBuildRuntimeSnapshot(t *testing.T) {
	const ns = "astro-x-0"
	agentMatch := map[string]string{"app": "agent"}

	agentDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-dep", Namespace: ns,
			Labels: map[string]string{"app.kubernetes.io/component": "agent"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: i32(1),
			Selector: &metav1.LabelSelector{MatchLabels: agentMatch},
		},
		Status: appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1},
	}
	agentPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-dep-abc", Namespace: ns,
			Labels: map[string]string{"app": "agent", "app.kubernetes.io/version": "build-1"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				{Name: "messaging", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}
	// A provider scaled to zero must be skipped (no pods to show), matching the
	// live endpoint.
	scaledDown := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "model-dep", Namespace: ns,
			Labels: map[string]string{"app.kubernetes.io/component": "model-llm"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: i32(0),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "model-llm"}},
		},
	}
	msgSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-messaging", Namespace: ns,
			Labels: map[string]string{"app.kubernetes.io/component": "messaging"},
		},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP},
	}
	ingestJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ingest-job", Namespace: ns,
			Labels: map[string]string{"app.kubernetes.io/component": "ingestion-docs"},
		},
		Spec: batchv1.JobSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"job": "ingest"}}},
		Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{
			{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
		}},
	}

	w := newTestWatcher(t, agentDep, agentPod, scaledDown, msgSvc, ingestJob)

	snap, err := buildRuntimeSnapshot(w, ns)
	if err != nil {
		t.Fatalf("buildRuntimeSnapshot: %v", err)
	}

	if snap.Ready != 1 || snap.Replicas != 1 {
		t.Errorf("expected agent ready/replicas 1/1, got %d/%d", snap.Ready, snap.Replicas)
	}

	byName := map[string]struct{}{}
	for _, wl := range snap.Workloads {
		byName[wl.Name] = struct{}{}
	}
	if _, ok := byName["model-dep"]; ok {
		t.Errorf("scaled-to-zero deployment should be skipped, got it in workloads")
	}

	var sawAgent, sawJob bool
	for _, wl := range snap.Workloads {
		switch wl.Name {
		case "agent-dep":
			sawAgent = true
			if len(wl.Pods) != 1 || wl.Pods[0].Name != "agent-dep-abc" {
				t.Errorf("agent workload: expected pod agent-dep-abc, got %+v", wl.Pods)
			}
			if wl.Pods[0].BuildID != "build-1" {
				t.Errorf("agent pod: expected build_id build-1, got %q", wl.Pods[0].BuildID)
			}
			if len(wl.Pods[0].Containers) != 2 {
				t.Errorf("agent pod: expected 2 containers, got %d", len(wl.Pods[0].Containers))
			}
		case "ingest-job":
			sawJob = true
			if wl.Kind != "Job" || wl.Status != "Succeeded" {
				t.Errorf("ingest job: expected Kind=Job Status=Succeeded, got %q/%q", wl.Kind, wl.Status)
			}
		}
	}
	if !sawAgent || !sawJob {
		t.Errorf("expected agent + job workloads, sawAgent=%v sawJob=%v", sawAgent, sawJob)
	}

	if len(snap.Services) != 1 || snap.Services[0].Name != "agent-messaging" {
		t.Errorf("expected one messaging service, got %+v", snap.Services)
	}
}
