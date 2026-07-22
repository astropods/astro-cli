package deploycontroller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

func i32(n int32) *int32 { return &n }

func TestDeriveDeploymentHealth(t *testing.T) {
	tests := []struct {
		name      string
		dep       *appsv1.Deployment
		wantPhase string
		wantReady int
	}{
		{
			name: "ready",
			dep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Generation: 2},
				Spec:       appsv1.DeploymentSpec{Replicas: i32(3)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 2, ReadyReplicas: 3, UpdatedReplicas: 3, AvailableReplicas: 3,
				},
			},
			wantPhase: deploymentstore.WorkloadPhaseReady, wantReady: 3,
		},
		{
			name: "generation lag → progressing",
			dep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Generation: 5},
				Spec:       appsv1.DeploymentSpec{Replicas: i32(2)},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 4, ReadyReplicas: 2},
			},
			wantPhase: deploymentstore.WorkloadPhaseProgressing, wantReady: 2,
		},
		{
			name: "partial rollout → progressing",
			dep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: i32(3)},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, ReadyReplicas: 1, UpdatedReplicas: 1},
			},
			wantPhase: deploymentstore.WorkloadPhaseProgressing, wantReady: 1,
		},
		{
			name: "no pods yet → pending",
			dep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: i32(2)},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 1},
			},
			wantPhase: deploymentstore.WorkloadPhasePending,
		},
		{
			name: "progress deadline exceeded → failed",
			dep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: i32(1)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					Conditions: []appsv1.DeploymentCondition{{
						Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse,
						Reason: "ProgressDeadlineExceeded", Message: "exceeded its progress deadline",
					}},
				},
			},
			wantPhase: deploymentstore.WorkloadPhaseFailed,
		},
		{
			name: "scaled to zero → ready",
			dep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: i32(0)},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 1},
			},
			wantPhase: deploymentstore.WorkloadPhaseReady,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveDeploymentHealth(tt.dep)
			if got.Phase != tt.wantPhase {
				t.Errorf("phase = %q, want %q", got.Phase, tt.wantPhase)
			}
			if got.WorkloadType != "deployment" {
				t.Errorf("type = %q, want deployment", got.WorkloadType)
			}
			if tt.wantReady != 0 && got.ObservedReady != tt.wantReady {
				t.Errorf("ready = %d, want %d", got.ObservedReady, tt.wantReady)
			}
		})
	}
}

func TestDeriveStatefulSetHealth(t *testing.T) {
	ready := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Generation: 1},
		Spec:       appsv1.StatefulSetSpec{Replicas: i32(2)},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1, ReadyReplicas: 2, UpdatedReplicas: 2,
			CurrentRevision: "rev-1", UpdateRevision: "rev-1",
		},
	}
	if got := deriveStatefulSetHealth(ready); got.Phase != deploymentstore.WorkloadPhaseReady {
		t.Errorf("ready STS phase = %q, want ready", got.Phase)
	}

	rolling := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Generation: 1},
		Spec:       appsv1.StatefulSetSpec{Replicas: i32(2)},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1, ReadyReplicas: 2, UpdatedReplicas: 1,
			CurrentRevision: "rev-1", UpdateRevision: "rev-2",
		},
	}
	if got := deriveStatefulSetHealth(rolling); got.Phase != deploymentstore.WorkloadPhaseProgressing {
		t.Errorf("rolling STS phase = %q, want progressing", got.Phase)
	}

	// OnDelete keeps the pod on the old revision after a spec change: ready pod,
	// but UpdatedReplicas and revisions lag forever. Must still read as ready.
	onDelete := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Generation: 3},
		Spec: appsv1.StatefulSetSpec{
			Replicas:       i32(1),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType},
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 3, ReadyReplicas: 1, UpdatedReplicas: 0,
			CurrentRevision: "rev-1", UpdateRevision: "rev-2",
		},
	}
	if got := deriveStatefulSetHealth(onDelete); got.Phase != deploymentstore.WorkloadPhaseReady {
		t.Errorf("OnDelete STS phase = %q, want ready", got.Phase)
	}
}

func TestDeriveJobHealth(t *testing.T) {
	tests := []struct {
		name string
		job  *batchv1.Job
		want string
	}{
		{"complete", &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j"}, Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}},
			deploymentstore.WorkloadPhaseComplete},
		{"failed", &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j"}, Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded"}}}},
			deploymentstore.WorkloadPhaseFailed},
		{"active", &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j"}, Status: batchv1.JobStatus{Active: 1}},
			deploymentstore.WorkloadPhaseProgressing},
		{"pending", &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j"}, Status: batchv1.JobStatus{}},
			deploymentstore.WorkloadPhasePending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveJobHealth(tt.job); got.Phase != tt.want {
				t.Errorf("phase = %q, want %q", got.Phase, tt.want)
			}
		})
	}
}

func TestDeriveCronJobHealth(t *testing.T) {
	got := deriveCronJobHealth(&batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "cron"}})
	if got.Phase != deploymentstore.WorkloadPhaseReady || got.WorkloadType != "cronjob" {
		t.Errorf("cronjob = %+v", got)
	}
}
