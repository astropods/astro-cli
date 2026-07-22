// Package deploycontroller implements the event-driven deployment controller.
// It watches managed K8s workloads via informers and persists their observed
// health to deployment_workload_status. Phase 1 is observe-and-persist only
// (shadow mode): it does not drive the deployment-level lifecycle.
package deploycontroller

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// reasonProgressDeadlineExceeded is the Deployment Progressing-condition reason
// K8s sets when a rollout exceeds progressDeadlineSeconds — a deterministic
// terminal-failure signal that does not require inspecting pods.
const reasonProgressDeadlineExceeded = "ProgressDeadlineExceeded"

// deriveDeploymentHealth maps a live Deployment to an observed WorkloadStatus
// using K8s rollout semantics: observedGeneration must have caught up to the
// object generation, and updated/available/ready replicas must all reach the
// desired count before a rollout is considered ready. A Progressing=False /
// ProgressDeadlineExceeded condition is a terminal failure.
func deriveDeploymentHealth(d *appsv1.Deployment) deploymentstore.WorkloadStatus {
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	ws := deploymentstore.WorkloadStatus{
		WorkloadName:       d.Name,
		WorkloadType:       "deployment",
		ObservedReady:      int(d.Status.ReadyReplicas),
		ObservedDesired:    int(desired),
		ObservedGeneration: d.Status.ObservedGeneration,
	}

	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Status == corev1.ConditionFalse && c.Reason == reasonProgressDeadlineExceeded {
			ws.Phase = deploymentstore.WorkloadPhaseFailed
			ws.Reason = c.Reason
			ws.Message = c.Message
			return ws
		}
	}

	switch {
	case desired == 0:
		// Intentionally scaled to zero (e.g. paused) — nothing to run is satisfied.
		ws.Phase = deploymentstore.WorkloadPhaseReady
	case d.Status.ObservedGeneration < d.Generation:
		// The controller hasn't observed the latest spec yet — mid-rollout.
		ws.Phase = deploymentstore.WorkloadPhaseProgressing
		ws.Reason = "GenerationLag"
	case d.Status.UpdatedReplicas >= desired &&
		d.Status.AvailableReplicas >= desired &&
		d.Status.ReadyReplicas >= desired:
		ws.Phase = deploymentstore.WorkloadPhaseReady
	case d.Status.ReadyReplicas > 0 || d.Status.UpdatedReplicas > 0:
		ws.Phase = deploymentstore.WorkloadPhaseProgressing
	default:
		ws.Phase = deploymentstore.WorkloadPhasePending
	}
	return ws
}

// deriveStatefulSetHealth maps a live StatefulSet to an observed WorkloadStatus.
// Ready requires the update to have fully rolled out: updated == ready ==
// desired and the current revision equal to the update revision.
func deriveStatefulSetHealth(s *appsv1.StatefulSet) deploymentstore.WorkloadStatus {
	desired := int32(1)
	if s.Spec.Replicas != nil {
		desired = *s.Spec.Replicas
	}
	ws := deploymentstore.WorkloadStatus{
		WorkloadName:       s.Name,
		WorkloadType:       "statefulset",
		ObservedReady:      int(s.Status.ReadyReplicas),
		ObservedDesired:    int(desired),
		ObservedGeneration: s.Status.ObservedGeneration,
	}

	rolloutComplete := s.Status.CurrentRevision == s.Status.UpdateRevision
	// OnDelete never recreates pods on a spec change, so UpdatedReplicas and the
	// revision pointers lag until a pod is manually deleted. A ready pod on the
	// serving revision is the steady state — gate readiness on the ready count.
	onDelete := s.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType
	switch {
	case desired == 0:
		ws.Phase = deploymentstore.WorkloadPhaseReady
	case s.Status.ObservedGeneration < s.Generation:
		ws.Phase = deploymentstore.WorkloadPhaseProgressing
		ws.Reason = "GenerationLag"
	case onDelete && s.Status.ReadyReplicas >= desired:
		ws.Phase = deploymentstore.WorkloadPhaseReady
	case rolloutComplete && s.Status.UpdatedReplicas >= desired && s.Status.ReadyReplicas >= desired:
		ws.Phase = deploymentstore.WorkloadPhaseReady
	case s.Status.ReadyReplicas > 0 || s.Status.UpdatedReplicas > 0:
		ws.Phase = deploymentstore.WorkloadPhaseProgressing
	default:
		ws.Phase = deploymentstore.WorkloadPhasePending
	}
	return ws
}

// deriveJobHealth maps a live Job to an observed WorkloadStatus using its
// Complete / Failed conditions.
func deriveJobHealth(j *batchv1.Job) deploymentstore.WorkloadStatus {
	desired := int32(1)
	if j.Spec.Completions != nil {
		desired = *j.Spec.Completions
	}
	ws := deploymentstore.WorkloadStatus{
		WorkloadName:    j.Name,
		WorkloadType:    "job",
		ObservedReady:   int(j.Status.Succeeded),
		ObservedDesired: int(desired),
	}

	for _, c := range j.Status.Conditions {
		if c.Status != corev1.ConditionTrue {
			continue
		}
		switch c.Type {
		case batchv1.JobComplete:
			ws.Phase = deploymentstore.WorkloadPhaseComplete
			return ws
		case batchv1.JobFailed:
			ws.Phase = deploymentstore.WorkloadPhaseFailed
			ws.Reason = c.Reason
			ws.Message = c.Message
			return ws
		}
	}

	if j.Status.Active > 0 {
		ws.Phase = deploymentstore.WorkloadPhaseProgressing
	} else {
		ws.Phase = deploymentstore.WorkloadPhasePending
	}
	return ws
}

// deriveCronJobHealth maps a live CronJob to an observed WorkloadStatus. A
// CronJob is a schedule, not a running workload, so its presence is treated as
// ready; individual runs are tracked as Jobs.
func deriveCronJobHealth(c *batchv1.CronJob) deploymentstore.WorkloadStatus {
	return deploymentstore.WorkloadStatus{
		WorkloadName: c.Name,
		WorkloadType: "cronjob",
		Phase:        deploymentstore.WorkloadPhaseReady,
	}
}
