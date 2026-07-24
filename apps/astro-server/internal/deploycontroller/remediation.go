package deploycontroller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// controllerRevisionHashLabel is stamped by the StatefulSet controller on every
// pod, naming the ControllerRevision (pod-template hash) the pod was created from.
const controllerRevisionHashLabel = "controller-revision-hash"

// rollWedgedStatefulSetPods breaks a StatefulSet rollout deadlock. A pod wedged
// in an image-pull/create failure is never Running-and-Ready, and a StatefulSet
// RollingUpdate refuses to replace a not-Ready pod — so once the pod is stuck on
// an outdated revision, a template update (e.g. a corrected image) never reaches
// it and the rollout hangs indefinitely. For each pod that is both wedged and on
// a revision older than the StatefulSet's update revision, we delete it so the
// controller recreates it on the update revision.
//
// Gating on the stale revision is what keeps this safe: a pod wedged on the
// *current* update revision has no newer revision to roll to, so it is left
// alone — that avoids a delete/recreate churn loop on a genuinely bad new image.
// Best-effort: errors are logged and the next resync retries.
func (c *Controller) rollWedgedStatefulSetPods(ctx context.Context, w *clusterWatcher, ns string, ss *appsv1.StatefulSet) {
	updateRev := ss.Status.UpdateRevision
	if updateRev == "" || updateRev == ss.Status.CurrentRevision {
		// No rollout in flight (or status not yet populated): nothing to unblock.
		return
	}
	sel, err := metav1.LabelSelectorAsSelector(ss.Spec.Selector)
	if err != nil {
		return
	}
	pods, err := w.pods.Pods(ns).List(sel)
	if err != nil {
		return
	}
	for _, p := range pods {
		if !podShouldRollForward(p, updateRev) {
			continue
		}
		if err := w.clientset.CoreV1().Pods(ns).Delete(ctx, p.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			c.log.Warn("deploycontroller: evict wedged pod failed",
				"namespace", ns, "pod", p.Name, "error", err)
			continue
		}
		c.log.Info("deploycontroller: evicted pod wedged on stale revision to unblock StatefulSet rollout",
			"namespace", ns, "statefulset", ss.Name, "pod", p.Name,
			"pod_revision", p.Labels[controllerRevisionHashLabel], "update_revision", updateRev)
	}
}

// podShouldRollForward reports whether pod is a wedged, stale-revision pod the
// StatefulSet controller cannot roll on its own: not being deleted, on a
// revision other than updateRev, and stuck on a permanent image-pull/create wait.
func podShouldRollForward(pod *corev1.Pod, updateRev string) bool {
	if pod == nil || pod.DeletionTimestamp != nil {
		return false
	}
	if pod.Labels[controllerRevisionHashLabel] == updateRev {
		return false // already on the update revision
	}
	return podWedgedOnImagePull(pod)
}

// podWedgedOnImagePull reports whether any of the pod's init or main containers
// is waiting on a permanent image-pull/create failure (see permanentWaitReasons).
func podWedgedOnImagePull(pod *corev1.Pod) bool {
	statuses := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...)
	for _, cs := range statuses {
		if w := cs.State.Waiting; w != nil && permanentWaitReasons[w.Reason] {
			return true
		}
	}
	return false
}
