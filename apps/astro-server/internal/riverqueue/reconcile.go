package riverqueue

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// ReconcileArgs are the job arguments for the reconcile worker.
type ReconcileArgs struct{}

func (ReconcileArgs) Kind() string { return "reconcile" }

// ReconcileWorker replaces both DriftCheckWorker and NsScanWorker. It runs
// periodically to detect KEDA scale-downs, drift, stale provisioning, and
// orphaned namespaces.
type ReconcileWorker struct {
	river.WorkerDefaults[ReconcileArgs]
	deployer  *deployer.Deployer
	store     *deploymentstore.Store
	k8s       k8s.ClusterClient
	dynClient dynamic.Interface
	db        *sql.DB
	queue     *Queue
	log       *logger.Logger
}

func (w *ReconcileWorker) Work(ctx context.Context, _ *river.Job[ReconcileArgs]) error {
	if w.k8s == nil {
		return nil
	}

	w.reconcileActive(ctx)
	w.detectStaleJobs(ctx)
	w.maintainNamespaceOwnership(ctx)

	return nil
}

// reconcileActive checks active deployments for KEDA scale-down and drift.
func (w *ReconcileWorker) reconcileActive(ctx context.Context) {
	deps, err := w.store.GetDeploymentsInStatus(deploymentstore.StatusActive)
	if err != nil {
		w.log.Error("Reconcile: failed to list active deployments", "error", err)
		return
	}

	for _, dep := range deps {
		scaledDown, _ := w.store.IsScaledDown(dep.Namespace)
		if scaledDown {
			continue
		}

		if w.isKEDAScaledDown(ctx, dep.Namespace) {
			if err := w.store.MarkScaledDown(dep.ID, dep.Namespace); err != nil {
				w.log.Error("Reconcile: failed to mark scaled down", "error", err, "deployment_id", dep.ID)
			} else {
				w.log.Info("Reconcile: marked KEDA-scaled-down", "deployment_id", dep.ID, "namespace", dep.Namespace)
			}
			continue
		}

		// Check for reconciliation opt-out
		if w.hasAnnotation(ctx, dep.Namespace, "astro.dev/reconcile", "paused") {
			w.log.Info("Reconcile: paused via annotation", "deployment_id", dep.ID, "namespace", dep.Namespace)
			continue
		}

		drifts := w.detectDrift(ctx, dep)
		if len(drifts) > 0 {
			w.log.Warn("Reconcile: drift detected, enqueuing re-apply",
				"deployment_id", dep.ID,
				"drifts", len(drifts),
			)
			if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusPending, "", nil); err != nil {
				w.log.Error("Reconcile: failed to set pending for drift", "error", err, "deployment_id", dep.ID)
				continue
			}
			if _, err := w.queue.Insert(ctx, DeployArgs{DeploymentID: dep.ID}, nil); err != nil {
				w.log.Error("Reconcile: failed to enqueue deploy job", "error", err, "deployment_id", dep.ID)
			}
		}
	}
}

// detectStaleJobs marks deployments stuck in provisioning or pending as failed,
// and re-enqueues pending deployments that may have lost their River job.
func (w *ReconcileWorker) detectStaleJobs(ctx context.Context) {
	// Stale provisioning — worker crashed mid-apply
	provisioning, err := w.store.GetDeploymentsInStatus(deploymentstore.StatusProvisioning)
	if err != nil {
		w.log.Error("Reconcile: failed to list provisioning deployments", "error", err)
	} else {
		for _, dep := range provisioning {
			if time.Since(dep.StatusChangedAt) > 15*time.Minute {
				w.log.Error("Reconcile: deployment stuck in provisioning",
					"deployment_id", dep.ID,
					"since", dep.StatusChangedAt,
				)
				if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusFailed, "timed out in provisioning", nil); err != nil {
					w.log.Warn("Failed to mark stale deployment as failed", "error", err, "deployment_id", dep.ID)
				}
			}
		}
	}

	// Stuck pending — job insert may have failed after DB commit
	pending, err := w.store.GetDeploymentsInStatus(deploymentstore.StatusPending)
	if err != nil {
		w.log.Error("Reconcile: failed to list pending deployments", "error", err)
	} else {
		for _, dep := range pending {
			if time.Since(dep.StatusChangedAt) > 5*time.Minute {
				w.log.Warn("Reconcile: deployment stuck in pending, re-enqueuing",
					"deployment_id", dep.ID,
					"since", dep.StatusChangedAt,
				)
				if _, err := w.queue.Insert(ctx, DeployArgs{DeploymentID: dep.ID}, nil); err != nil {
					w.log.Error("Reconcile: failed to re-enqueue stuck pending deployment", "error", err, "deployment_id", dep.ID)
				}
			}
		}
	}
}

// maintainNamespaceOwnership upserts namespace_ownership rows for active deployments
// and detects orphaned K8s namespaces. Adapted from nsscan.Scanner.Scan.
func (w *ReconcileWorker) maintainNamespaceOwnership(ctx context.Context) {
	deps, err := w.store.GetDeploymentsInStatus(deploymentstore.StatusActive, deploymentstore.StatusScaledDown)
	if err != nil {
		w.log.Error("Reconcile: failed to list deployments for ownership", "error", err)
		return
	}

	scanTime := time.Now()
	dbNamespaces := make(map[string]*deploymentstore.Deployment, len(deps))

	for _, dep := range deps {
		dbNamespaces[dep.Namespace] = dep
		_, err := w.db.ExecContext(ctx, `
			INSERT INTO namespace_ownership (namespace, account_id, agent_name, deployment_id, scanned_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (namespace) DO UPDATE
			SET account_id = EXCLUDED.account_id,
			    agent_name = EXCLUDED.agent_name,
			    deployment_id = EXCLUDED.deployment_id,
			    scanned_at = EXCLUDED.scanned_at
		`, dep.Namespace, dep.AccountID, dep.AgentName, dep.ID, scanTime)
		if err != nil {
			w.log.Warn("Reconcile: failed to upsert namespace_ownership", "namespace", dep.Namespace, "error", err)
		}
	}

	// Detect orphaned K8s namespaces (log only)
	nsList, err := w.k8s.Clientset().CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if err != nil {
		w.log.Warn("Reconcile: failed to list K8s namespaces", "error", err)
		return
	}

	for _, ns := range nsList.Items {
		if _, ok := dbNamespaces[ns.Name]; !ok {
			w.log.Warn("Reconcile: orphaned K8s namespace",
				"namespace", ns.Name,
				"account_id", ns.Labels["astro.dev/account-id"],
				"agent", ns.Labels["astro.dev/agent"],
			)
		}
	}
}

// isKEDAScaledDown checks if all ScaledObjects in the namespace have Active=False.
func (w *ReconcileWorker) isKEDAScaledDown(ctx context.Context, namespace string) bool {
	if w.dynClient == nil {
		return false
	}

	gvr := schema.GroupVersionResource{Group: "keda.sh", Version: "v1alpha1", Resource: "scaledobjects"}
	objects, err := w.dynClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false // KEDA CRD not installed
		}
		return false
	}
	if len(objects.Items) == 0 {
		return false // no ScaledObjects → not KEDA-managed
	}

	for _, obj := range objects.Items {
		if !scaledObjectIsInactive(obj) {
			return false // at least one is active or scaling up
		}
	}

	return true
}

// scaledObjectIsInactive returns true if the ScaledObject's Active condition is False.
func scaledObjectIsInactive(obj unstructured.Unstructured) bool {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false // can't determine, assume active
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _, _ := unstructured.NestedString(cond, "type")
		condStatus, _, _ := unstructured.NestedString(cond, "status")
		if condType == "Active" && condStatus == "False" {
			return true
		}
	}

	return false // Active condition not found or is True
}

// hasAnnotation checks if a K8s namespace has a specific annotation value.
func (w *ReconcileWorker) hasAnnotation(ctx context.Context, namespace, key, value string) bool {
	ns, err := w.k8s.Clientset().CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return false
	}
	return ns.Annotations[key] == value
}

// detectDrift checks a deployment's K8s resources against the normalized DB state.
// Adapted from driftcheck.Checker.checkDeployment.
func (w *ReconcileWorker) detectDrift(ctx context.Context, dep *deploymentstore.Deployment) []string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var drifts []string
	clientset := w.k8s.Clientset()

	workloads, err := w.store.GetWorkloads(dep.ID)
	if err != nil || len(workloads) == 0 {
		return nil
	}

	for _, wl := range workloads {
		switch wl.WorkloadType {
		case "deployment":
			actual, err := clientset.AppsV1().Deployments(dep.Namespace).Get(ctx, wl.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					drifts = append(drifts, fmt.Sprintf("Deployment %q missing", wl.Name))
				}
				continue
			}
			actualReplicas := int32(1)
			if actual.Spec.Replicas != nil {
				actualReplicas = *actual.Spec.Replicas
			}
			if int(actualReplicas) != wl.Replicas {
				drifts = append(drifts, fmt.Sprintf("Deployment %q replicas: desired=%d actual=%d", wl.Name, wl.Replicas, actualReplicas))
			}
			if len(actual.Spec.Template.Spec.Containers) > 0 && actual.Spec.Template.Spec.Containers[0].Image != wl.Image {
				drifts = append(drifts, fmt.Sprintf("Deployment %q image mismatch", wl.Name))
			}

		case "statefulset":
			actual, err := clientset.AppsV1().StatefulSets(dep.Namespace).Get(ctx, wl.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					drifts = append(drifts, fmt.Sprintf("StatefulSet %q missing", wl.Name))
				}
				continue
			}
			actualReplicas := int32(1)
			if actual.Spec.Replicas != nil {
				actualReplicas = *actual.Spec.Replicas
			}
			if int(actualReplicas) != wl.Replicas {
				drifts = append(drifts, fmt.Sprintf("StatefulSet %q replicas: desired=%d actual=%d", wl.Name, wl.Replicas, actualReplicas))
			}
			if len(actual.Spec.Template.Spec.Containers) > 0 && actual.Spec.Template.Spec.Containers[0].Image != wl.Image {
				drifts = append(drifts, fmt.Sprintf("StatefulSet %q image mismatch", wl.Name))
			}

		case "cronjob":
			actual, err := clientset.BatchV1().CronJobs(dep.Namespace).Get(ctx, wl.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					drifts = append(drifts, fmt.Sprintf("CronJob %q missing", wl.Name))
				}
				continue
			}
			if wl.TriggerSchedule != nil && actual.Spec.Schedule != *wl.TriggerSchedule {
				drifts = append(drifts, fmt.Sprintf("CronJob %q schedule mismatch", wl.Name))
			}
		}
	}

	// Check services
	services, err := w.store.GetServices(dep.ID)
	if err == nil {
		for _, svc := range services {
			_, err := clientset.CoreV1().Services(dep.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
			if err != nil && apierrors.IsNotFound(err) {
				drifts = append(drifts, fmt.Sprintf("Service %q missing", svc.Name))
			}
		}
	}

	return drifts
}
