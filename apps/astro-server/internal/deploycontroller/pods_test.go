package deploycontroller

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func waitingPod(age time.Duration, reason string, restarts int32) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(time.Now().Add(-age))},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "main",
				RestartCount: restarts,
				State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}},
			}},
		},
	}
}

func TestClassifyPodFailure(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{"ImagePullBackOff past grace → failed", waitingPod(2*time.Minute, "ImagePullBackOff", 0), "ImagePullBackOff"},
		{"ImagePullBackOff within grace → transient", waitingPod(10*time.Second, "ImagePullBackOff", 0), ""},
		{"InvalidImageName past grace → failed", waitingPod(2*time.Minute, "InvalidImageName", 0), "InvalidImageName"},
		{"CrashLoopBackOff few restarts → transient", waitingPod(10*time.Minute, "CrashLoopBackOff", 2), ""},
		{"CrashLoopBackOff many restarts → failed", waitingPod(10*time.Minute, "CrashLoopBackOff", 6), "CrashLoopBackOff"},
		{"PodInitializing → transient", waitingPod(10*time.Minute, "PodInitializing", 0), ""},
		{"running (no waiting) → healthy", &corev1.Pod{Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}},
		}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := classifyPodFailure(tt.pod)
			if got != tt.want {
				t.Errorf("reason = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyPodFailure_OOMKilled(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:                 "main",
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}},
				State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			}},
		},
	}
	if got, _ := classifyPodFailure(pod); got != "OOMKilled" {
		t.Errorf("reason = %q, want OOMKilled", got)
	}
}

func TestClassifyPodFailure_SkipsTerminating(t *testing.T) {
	p := waitingPod(5*time.Minute, "ImagePullBackOff", 0)
	now := metav1.Now()
	p.DeletionTimestamp = &now
	if got, _ := classifyPodFailure(p); got != "" {
		t.Errorf("terminating pod should not be classified, got %q", got)
	}
}

func unschedulablePod(age time.Duration, msg string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(time.Now().Add(-age))},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type:    corev1.PodScheduled,
				Status:  corev1.ConditionFalse,
				Reason:  corev1.PodReasonUnschedulable,
				Message: msg,
			}},
		},
	}
}

func TestClassifyPodFailure_Unschedulable(t *testing.T) {
	tests := []struct {
		name    string
		pod     *corev1.Pod
		want    string
		wantMsg string
	}{
		{"unschedulable past grace → failed",
			unschedulablePod(2*time.Minute, "0/3 nodes are available: insufficient cpu"),
			"Unschedulable", "0/3 nodes are available: insufficient cpu"},
		{"unschedulable within grace → transient (scheduler may still place it)",
			unschedulablePod(10*time.Second, "0/3 nodes are available"), "", ""},
		{"unbound PVC past grace → failed (surfaces as Unschedulable)",
			unschedulablePod(2*time.Minute, "pod has unbound immediate PersistentVolumeClaims"),
			"Unschedulable", "pod has unbound immediate PersistentVolumeClaims"},
		{"unschedulable with no message → default message",
			unschedulablePod(2*time.Minute, ""), "Unschedulable", "pod is unschedulable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, msg := classifyPodFailure(tt.pod)
			if got != tt.want {
				t.Errorf("reason = %q, want %q", got, tt.want)
			}
			if tt.want != "" && msg != tt.wantMsg {
				t.Errorf("message = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}
