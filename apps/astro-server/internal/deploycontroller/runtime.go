package deploycontroller

import (
	"encoding/json"
	"fmt"
	"hash/fnv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

const (
	componentLabel = "app.kubernetes.io/component"
	versionLabel   = "app.kubernetes.io/version"
	// agentComponent is the primary workload; its observed replica counts are
	// the deployment-level ready/replicas the runtime endpoint reports.
	agentComponent = "agent"
)

// buildRuntimeSnapshot assembles the full live-runtime view of a deployment's
// namespace from the informer caches: every managed workload, all of its pods,
// all of their containers, and all managed services. This is the read model the
// runtime endpoint deserializes instead of hitting the K8s API per request.
//
// It re-lists from the (already synced) listers rather than reusing sync()'s
// workload slices — a cache read is cheap and keeps the two concerns readable.
func buildRuntimeSnapshot(w *clusterWatcher, ns string) (deploymentstore.RuntimeSnapshot, error) {
	sel := labels.Everything() // informers are already scoped to managed-by=astro-server
	var snap deploymentstore.RuntimeSnapshot

	// Pods are listed once and matched to each workload by its own selector.
	allPods, err := w.pods.Pods(ns).List(sel)
	if err != nil {
		return snap, fmt.Errorf("list pods: %w", err)
	}

	deploys, err := w.deploys.Deployments(ns).List(sel)
	if err != nil {
		return snap, fmt.Errorf("list deployments: %w", err)
	}
	for _, d := range deploys {
		component := d.Labels[componentLabel]
		if component == agentComponent || component == "" {
			snap.Ready, snap.Replicas = d.Status.ReadyReplicas, d.Status.Replicas
		}
		if scaledToZero(d.Spec.Replicas) {
			continue // no pods to show, matches the live endpoint's behavior
		}
		snap.Workloads = append(snap.Workloads, deploymentstore.RuntimeWorkload{
			Name:      d.Name,
			Kind:      "Deployment",
			Component: component,
			CreatedAt: d.CreationTimestamp.Time,
			Pods:      podsForSelector(allPods, d.Spec.Selector),
		})
	}

	statefuls, err := w.statefuls.StatefulSets(ns).List(sel)
	if err != nil {
		return snap, fmt.Errorf("list statefulsets: %w", err)
	}
	for _, s := range statefuls {
		component := s.Labels[componentLabel]
		if component == agentComponent || component == "" {
			snap.Ready, snap.Replicas = s.Status.ReadyReplicas, s.Status.Replicas
		}
		if scaledToZero(s.Spec.Replicas) {
			continue
		}
		snap.Workloads = append(snap.Workloads, deploymentstore.RuntimeWorkload{
			Name:      s.Name,
			Kind:      "StatefulSet",
			Component: component,
			CreatedAt: s.CreationTimestamp.Time,
			Pods:      podsForSelector(allPods, s.Spec.Selector),
		})
	}

	jobs, err := w.jobs.Jobs(ns).List(sel)
	if err != nil {
		return snap, fmt.Errorf("list jobs: %w", err)
	}
	for _, j := range jobs {
		snap.Workloads = append(snap.Workloads, deploymentstore.RuntimeWorkload{
			Name:      j.Name,
			Kind:      "Job",
			Component: j.Labels[componentLabel],
			CreatedAt: j.CreationTimestamp.Time,
			Status:    jobStatus(j),
			Pods:      podsForSelector(allPods, j.Spec.Selector),
		})
	}

	cronJobs, err := w.cronJobs.CronJobs(ns).List(sel)
	if err != nil {
		return snap, fmt.Errorf("list cronjobs: %w", err)
	}
	for _, cj := range cronJobs {
		// CronJobs own no pods directly (their child Jobs do); the runtime view
		// shows their status + schedule, not pods.
		snap.Workloads = append(snap.Workloads, deploymentstore.RuntimeWorkload{
			Name:      cj.Name,
			Kind:      "CronJob",
			Component: cj.Labels[componentLabel],
			CreatedAt: cj.CreationTimestamp.Time,
			Status:    cronJobStatus(cj),
			Schedule:  cj.Spec.Schedule,
		})
	}

	services, err := w.services.Services(ns).List(sel)
	if err != nil {
		return snap, fmt.Errorf("list services: %w", err)
	}
	for _, svc := range services {
		snap.Services = append(snap.Services, deploymentstore.RuntimeService{
			Name:      svc.Name,
			Type:      string(svc.Spec.Type),
			Component: svc.Labels[componentLabel],
		})
	}

	return snap, nil
}

func scaledToZero(replicas *int32) bool {
	return replicas != nil && *replicas == 0
}

// podsForSelector returns the snapshot form of every pod in `pods` matching the
// workload's label selector.
func podsForSelector(pods []*corev1.Pod, sel *metav1.LabelSelector) []deploymentstore.RuntimePod {
	if sel == nil {
		return nil
	}
	selector, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return nil
	}
	var out []deploymentstore.RuntimePod
	for _, p := range pods {
		if !selector.Matches(labels.Set(p.Labels)) {
			continue
		}
		out = append(out, deploymentstore.RuntimePod{
			Name:       p.Name,
			Phase:      string(p.Status.Phase),
			BuildID:    p.Labels[versionLabel],
			CreatedAt:  p.CreationTimestamp.Time,
			Containers: containerStatuses(p),
		})
	}
	return out
}

// containerStatuses extracts per-container state from a pod, merging init and
// main containers. Mirrors the handler's buildContainerStatuses so the runtime
// endpoint renders identically whether it reads live K8s or this snapshot.
func containerStatuses(pod *corev1.Pod) []deploymentstore.RuntimeContainer {
	all := append([]corev1.ContainerStatus{}, pod.Status.ContainerStatuses...)
	all = append(all, pod.Status.InitContainerStatuses...)
	out := make([]deploymentstore.RuntimeContainer, 0, len(all))
	for _, cs := range all {
		c := deploymentstore.RuntimeContainer{
			Name:         cs.Name,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
		}
		switch {
		case cs.State.Running != nil:
			c.State = "Running"
		case cs.State.Waiting != nil:
			c.State = "Waiting"
			c.Reason = cs.State.Waiting.Reason
			c.Message = cs.State.Waiting.Message
		case cs.State.Terminated != nil:
			c.State = "Terminated"
			c.Reason = cs.State.Terminated.Reason
			c.Message = cs.State.Terminated.Message
		default:
			c.State = "Unknown"
		}
		out = append(out, c)
	}
	return out
}

// jobStatus / cronJobStatus mirror the handler's pure status derivations so the
// snapshot carries the same Job/CronJob vocabulary the UI already renders.
func jobStatus(job *batchv1.Job) string {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return "Succeeded"
		}
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return "Failed"
		}
	}
	if job.Status.Active > 0 {
		return "Running"
	}
	return "Pending"
}

func cronJobStatus(cj *batchv1.CronJob) string {
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		return "Suspended"
	}
	if len(cj.Status.Active) > 0 {
		return "Active"
	}
	return "Idle"
}

// snapshotHash is a content hash used to skip persisting an unchanged snapshot,
// so a resync of a steady-state deployment doesn't rewrite the row.
func snapshotHash(snap deploymentstore.RuntimeSnapshot) (uint64, error) {
	data, err := json.Marshal(snap)
	if err != nil {
		return 0, err
	}
	h := fnv.New64a()
	_, _ = h.Write(data)
	return h.Sum64(), nil
}
