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

func readyPod(ns, name string, labels map[string]string, cs corev1.ContainerStatus) *corev1.Pod {
	p := labeledPod(ns, name, labels, cs, time.Hour)
	p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	return p
}

func runningContainer(name string, restarts int32, runFor time.Duration) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:         name,
		Ready:        true,
		RestartCount: restarts,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{
			StartedAt: metav1.NewTime(time.Now().Add(-runFor)),
		}},
	}
}

func TestEnrichFromPods(t *testing.T) {
	const ns = "acct-1"
	sel := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}}
	appLabels := map[string]string{"app": "agent"}

	running := corev1.ContainerStatus{Name: "app", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}
	wedged := corev1.ContainerStatus{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}
	restartedRecently := runningContainer("app", 7, 30*time.Second)
	restartedLongAgo := runningContainer("app", 7, 30*time.Minute)

	tests := []struct {
		name       string
		in         deploymentstore.WorkloadStatus
		lister     corelisters.PodLister
		sel        *metav1.LabelSelector
		wantPhase  string
		wantReason string
	}{
		{
			name:      "ready workload not flipped by a wedged, not-ready pod",
			in:        deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhaseReady},
			lister:    podListerWith(labeledPod(ns, "agent-0", appLabels, wedged, 5*time.Minute)),
			sel:       sel,
			wantPhase: deploymentstore.WorkloadPhaseReady,
		},
		{
			name:       "ready workload whose ready pod is between crashes → failed",
			in:         deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhaseReady},
			lister:     podListerWith(readyPod(ns, "agent-0", appLabels, restartedRecently)),
			sel:        sel,
			wantPhase:  deploymentstore.WorkloadPhaseFailed,
			wantReason: "CrashLoopBackOff",
		},
		{
			name:      "ready workload whose ready pod outlasted the stable window → ready",
			in:        deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhaseReady},
			lister:    podListerWith(readyPod(ns, "agent-0", appLabels, restartedLongAgo)),
			sel:       sel,
			wantPhase: deploymentstore.WorkloadPhaseReady,
		},
		{
			name:      "ready workload with a few restarts → ready",
			in:        deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhaseReady},
			lister:    podListerWith(readyPod(ns, "agent-0", appLabels, runningContainer("app", crashLoopRestartLimit, time.Second))),
			sel:       sel,
			wantPhase: deploymentstore.WorkloadPhaseReady,
		},
		{
			name: "rollout: crash-looping old pod is not ready, new pod is → ready",
			in:   deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhaseReady},
			lister: podListerWith(
				labeledPod(ns, "agent-old", appLabels, corev1.ContainerStatus{
					Name: "app", RestartCount: 9,
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				}, 20*time.Minute),
				readyPod(ns, "agent-new", appLabels, runningContainer("app", 0, 10*time.Second)),
			),
			sel:       sel,
			wantPhase: deploymentstore.WorkloadPhaseReady,
		},
		{
			name: "ready workload whose unstable pod is terminating → ready",
			in:   deploymentstore.WorkloadStatus{Phase: deploymentstore.WorkloadPhaseReady},
			lister: podListerWith(func() *corev1.Pod {
				p := readyPod(ns, "agent-0", appLabels, restartedRecently)
				now := metav1.Now()
				p.DeletionTimestamp = &now
				return p
			}()),
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
