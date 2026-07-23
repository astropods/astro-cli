package deploycontroller

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// podListerWith builds a PodLister backed by the given pods, matching how the
// informer-backed lister indexes by namespace.
func podListerWith(pods ...*corev1.Pod) corelisters.PodLister {
	idx := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	for _, p := range pods {
		_ = idx.Add(p)
	}
	return corelisters.NewPodLister(idx)
}

func labeledPod(ns, name string, labels map[string]string, cs corev1.ContainerStatus, age time.Duration) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         ns,
			Name:              name,
			Labels:            labels,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
		},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{cs}},
	}
}

func TestEnrichFromPods(t *testing.T) {
	const ns = "acct-1"
	sel := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}}
	appLabels := map[string]string{"app": "agent"}

	running := corev1.ContainerStatus{Name: "app", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}
	wedged := corev1.ContainerStatus{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}

	tests := []struct {
		name       string
		in         deploymentstore.WorkloadStatus
		lister     corelisters.PodLister
		sel        *metav1.LabelSelector
		wantPhase  string
		wantReason string
	}{
		{
			name:      "ready workload short-circuits — not flipped by a wedged pod",
			in:        deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhaseReady},
			lister:    podListerWith(labeledPod(ns, "agent-0", appLabels, wedged, 5*time.Minute)),
			sel:       sel,
			wantPhase: deploymentstore.WorkloadPhaseReady,
		},
		{
			name:       "progressing + wedged pod past grace → failed with reason",
			in:         deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhaseProgressing},
			lister:     podListerWith(labeledPod(ns, "agent-0", appLabels, wedged, 5*time.Minute)),
			sel:        sel,
			wantPhase:  deploymentstore.WorkloadPhaseFailed,
			wantReason: "ImagePullBackOff",
		},
		{
			name:      "progressing + healthy pod → unchanged",
			in:        deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhaseProgressing},
			lister:    podListerWith(labeledPod(ns, "agent-0", appLabels, running, 5*time.Minute)),
			sel:       sel,
			wantPhase: deploymentstore.WorkloadPhaseProgressing,
		},
		{
			name:      "selector matches no pods → unchanged",
			in:        deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhasePending},
			lister:    podListerWith(labeledPod(ns, "other-0", map[string]string{"app": "cache"}, wedged, 5*time.Minute)),
			sel:       sel,
			wantPhase: deploymentstore.WorkloadPhasePending,
		},
		{
			name:      "nil selector → unchanged",
			in:        deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhaseProgressing},
			lister:    podListerWith(labeledPod(ns, "agent-0", appLabels, wedged, 5*time.Minute)),
			sel:       nil,
			wantPhase: deploymentstore.WorkloadPhaseProgressing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enrichFromPods(tt.lister, ns, tt.sel, tt.in)
			if got.Phase != tt.wantPhase {
				t.Errorf("phase = %q, want %q", got.Phase, tt.wantPhase)
			}
			if tt.wantReason != "" && got.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}
