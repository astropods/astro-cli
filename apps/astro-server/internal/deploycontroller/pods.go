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

const (
	crashLoopRestartLimit = 3
	crashLoopStableWindow = 5 * time.Minute
)

// permanentWaitReasons are container waiting reasons that do not self-resolve
// without operator action (a fixed image reference, a fresh build).
//
// CreateContainerConfigError is the kubelet reason when a container references a
// ConfigMap/Secret (via envFrom or env) that does not exist — e.g. an env source
// that failed to apply. It never self-resolves; without it here the pod sits on
// "configmap ... not found" and the deployment hangs in "deploying" until the
// stale watchdog fires ~30m later, blocking redeploys the whole time.
var permanentWaitReasons = map[string]bool{
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"InvalidImageName":           true,
	"CreateContainerError":       true,
	"CreateContainerConfigError": true,
}

func enrichFromPods(pl corelisters.PodLister, ns string, sel *metav1.LabelSelector, ws deploymentstore.WorkloadStatus) deploymentstore.WorkloadStatus {
	switch ws.Phase {
	case deploymentstore.WorkloadPhaseComplete, deploymentstore.WorkloadPhaseFailed:
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
	classify := classifyPods
	if ws.Phase == deploymentstore.WorkloadPhaseReady {
		classify = classifyUnstablePods
	}
	if reason, msg := classify(pods); reason != "" {
		ws.Phase = deploymentstore.WorkloadPhaseFailed
		ws.Reason = reason
		ws.Message = msg
	}
	return ws
}

func classifyPods(pods []*corev1.Pod) (reason, message string) {
	for _, p := range pods {
		if r, m := classifyPodFailure(p); r != "" {
			return r, m
		}
	}
	return "", ""
}

func classifyUnstablePods(pods []*corev1.Pod) (reason, message string) {
	for _, p := range pods {
		if p == nil || p.DeletionTimestamp != nil || !podReady(p) {
			continue
		}
		for _, cs := range allContainerStatuses(p) {
			if cs.RestartCount > crashLoopRestartLimit && !runStable(cs) {
				return "CrashLoopBackOff", fmt.Sprintf("%s restarted %d times", cs.Name, cs.RestartCount)
			}
		}
	}
	return "", ""
}

func runStable(cs corev1.ContainerStatus) bool {
	r := cs.State.Running
	return r != nil && time.Since(r.StartedAt.Time) >= crashLoopStableWindow
}

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func allContainerStatuses(p *corev1.Pod) []corev1.ContainerStatus {
	all := append([]corev1.ContainerStatus{}, p.Status.InitContainerStatuses...)
	return append(all, p.Status.ContainerStatuses...)
}

func classifyPodFailure(pod *corev1.Pod) (reason, message string) {
	if pod == nil || pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded {
		return "", ""
	}
	age := time.Since(pod.CreationTimestamp.Time)

	for _, cs := range allContainerStatuses(pod) {
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

	// A pod the scheduler can't place — no fit-able node, or an unbound PVC that
	// leaves it unschedulable — never produces container statuses, so the loop
	// above can't see it. Past the grace window a standing Unschedulable is a real
	// failure. This is the only fast-fail path for StatefulSet/Job pods, which
	// have no progressDeadlineSeconds equivalent and would otherwise sit in
	// "deploying" until the staleness watchdog's much longer deadline.
	if age >= podFailureGrace {
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse && c.Reason == corev1.PodReasonUnschedulable {
				msg := c.Message
				if msg == "" {
					msg = "pod is unschedulable"
				}
				return corev1.PodReasonUnschedulable, msg
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
