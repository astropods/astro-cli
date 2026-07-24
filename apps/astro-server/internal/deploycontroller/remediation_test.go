package deploycontroller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func revPod(ns, name, rev string, cs corev1.ContainerStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
			Labels:    map[string]string{"app": "agent", controllerRevisionHashLabel: rev},
		},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{cs}},
	}
}

// TestRollWedgedStatefulSetPods covers the curative rollout-deadlock fix: a pod
// wedged on a permanent image-pull wait AND on a revision older than the
// StatefulSet's update revision is evicted so the controller recreates it on the
// update revision. Pods already on the update revision, healthy pods, and pods
// on a StatefulSet whose rollout is complete are all left alone.
func TestRollWedgedStatefulSetPods(t *testing.T) {
	const ns = "acct-1"
	wedged := corev1.ContainerStatus{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}
	running := corev1.ContainerStatus{Name: "app", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}

	tests := []struct {
		name                string
		ssCurrent, ssUpdate string
		pod                 *corev1.Pod
		wantDeleted         bool
	}{
		{"wedged on stale revision → evicted", "rev-old", "rev-new", revPod(ns, "agent-0", "rev-old", wedged), true},
		{"wedged on update revision → left alone", "rev-old", "rev-new", revPod(ns, "agent-0", "rev-new", wedged), false},
		{"healthy on stale revision → left alone", "rev-old", "rev-new", revPod(ns, "agent-0", "rev-old", running), false},
		{"rollout complete (current==update) → left alone", "rev-new", "rev-new", revPod(ns, "agent-0", "rev-old", wedged), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset(tt.pod)
			w := &clusterWatcher{pods: podListerWith(tt.pod), clientset: cs}
			ss := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "agent"},
				Spec:       appsv1.StatefulSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}}},
				Status:     appsv1.StatefulSetStatus{CurrentRevision: tt.ssCurrent, UpdateRevision: tt.ssUpdate},
			}
			c := &Controller{log: logger.New("error", "text")}

			c.rollWedgedStatefulSetPods(context.Background(), w, ns, ss)

			_, err := cs.CoreV1().Pods(ns).Get(context.Background(), tt.pod.Name, metav1.GetOptions{})
			deleted := apierrors.IsNotFound(err)
			if deleted != tt.wantDeleted {
				t.Fatalf("deleted=%v, want %v (get err: %v)", deleted, tt.wantDeleted, err)
			}
		})
	}
}

// TestPodShouldRollForward exercises the pure gating predicate, including init
// containers and pods mid-deletion.
func TestPodShouldRollForward(t *testing.T) {
	wedged := corev1.ContainerStatus{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}

	t.Run("wedged init container on stale revision → true", func(t *testing.T) {
		p := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{controllerRevisionHashLabel: "rev-old"}},
			Status:     corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{wedged}},
		}
		if !podShouldRollForward(p, "rev-new") {
			t.Fatal("expected true for wedged init container on stale revision")
		}
	})

	t.Run("pod being deleted → false", func(t *testing.T) {
		now := metav1.Now()
		p := revPod("ns", "agent-0", "rev-old", wedged)
		p.DeletionTimestamp = &now
		if podShouldRollForward(p, "rev-new") {
			t.Fatal("expected false for terminating pod")
		}
	})
}
