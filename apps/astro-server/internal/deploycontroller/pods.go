package deploycontroller

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// podFailureGrace is how long a pod may sit on a permanent-failure waiting
// reason before we treat it as a real workload failure. Long enough that a
// transient registry blip resolves on its own, short enough to fail well before
// the Deployment's progressDeadlineSeconds (180s) — so failures surface fast and
// with a specific reason instead of a generic "ProgressDeadlineExceeded".
const podFailureGrace = 90 * time.Second

// crashLoopRestartLimit: CrashLoopBackOff counts as failure only past this many
// restarts, so a container that crashed once and is retrying isn't failed.
const crashLoopRestartLimit = 3

// permanentWaitReasons are container waiting reasons that do not self-resolve
// without operator action (a fixed image reference, a fresh build).
var permanentWaitReasons = map[string]bool{
	"ImagePullBackOff":     true,
	"ErrImagePull":         true,
	"InvalidImageName":     true,
	"CreateContainerError": true,
}

// enrichFromPods overrides a still-settling workload status to failed when one
// of its pods is permanently wedged, attaching the specific pod reason. Already
// ready/complete/failed statuses are returned unchanged. The pod set is selected
// by the workload's own label selector.
func enrichFromPods(pl corelisters.PodLister, ns string, sel *metav1.LabelSelector, ws deploymentstore.WorkloadStatus) deploymentstore.WorkloadStatus {
	switch ws.Phase {
	case deploymentstore.WorkloadPhaseReady,
		deploymentstore.WorkloadPhaseComplete,
		deploymentstore.WorkloadPhaseFailed:
		return ws
	}
	if pl == nil || sel == nil {
		return ws
	}
	selector, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return ws
	}
	pods, err := pl.Pods(ns).List(selector)
	if err != nil {
		return ws
	}
	if reason, msg := classifyPods(pods); reason != "" {
		ws.Phase = deploymentstore.WorkloadPhaseFailed
		ws.Reason = reason
		ws.Message = msg
	}
	return ws
}

// classifyPods returns the first permanent failure found across the pods, or
// ("","") when none is wedged.
func classifyPods(pods []*corev1.Pod) (reason, message string) {
	for _, p := range pods {
		if r, m := classifyPodFailure(p); r != "" {
			return r, m
		}
	}
	return "", ""
}

// classifyPodFailure inspects one pod's init + main container statuses and
// returns a (reason, message) when it is permanently wedged. Pods mid-deletion
// or already succeeded are ignored.
func classifyPodFailure(pod *corev1.Pod) (reason, message string) {
	if pod == nil || pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded {
		return "", ""
	}
	age := time.Since(pod.CreationTimestamp.Time)

	all := append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...)
	all = append(all, pod.Status.ContainerStatuses...)
	for _, cs := range all {
		// OOMKilled surfaces as a terminated state (current or last).
		if oom := oomKilled(cs); oom {
			return "OOMKilled", fmt.Sprintf("container %s was OOMKilled", cs.Name)
		}
		w := cs.State.Waiting
		if w == nil {
			continue
		}
		switch {
		case permanentWaitReasons[w.Reason]:
			if age >= podFailureGrace {
				return w.Reason, fmt.Sprintf("%s: %s", cs.Name, waitingDetail(w))
			}
		case w.Reason == "CrashLoopBackOff":
			if cs.RestartCount > crashLoopRestartLimit {
				return w.Reason, fmt.Sprintf("%s restarted %d times", cs.Name, cs.RestartCount)
			}
		}
	}
	return "", ""
}

func oomKilled(cs corev1.ContainerStatus) bool {
	if t := cs.State.Terminated; t != nil && t.Reason == "OOMKilled" {
		return true
	}
	if t := cs.LastTerminationState.Terminated; t != nil && t.Reason == "OOMKilled" {
		return true
	}
	return false
}

func waitingDetail(w *corev1.ContainerStateWaiting) string {
	if w.Message != "" {
		return w.Message
	}
	return w.Reason
}
