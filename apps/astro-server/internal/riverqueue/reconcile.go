package riverqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
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

		report := w.buildDeploymentDriftReport(ctx, dep)
		if report == nil {
			continue
		}

		// Always save the report (queen needs "all match" state too)
		if err := w.store.SaveDriftReport(dep.ID, report); err != nil {
			w.log.Error("Reconcile: failed to save drift report", "error", err, "deployment_id", dep.ID)
		}

		// Trigger re-deploy only when there's actual drift or missing resources
		if report.Summary.Missing+report.Summary.Drift > 0 {
			driftMsg := fmt.Sprintf("Drift detected: %d missing, %d drifted (of %d total)", report.Summary.Missing, report.Summary.Drift, report.Summary.Total)
			w.log.Warn("Reconcile: drift detected, enqueuing re-apply",
				"deployment_id", dep.ID,
				"missing", report.Summary.Missing,
				"drift", report.Summary.Drift,
			)
			if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusPending, driftMsg, nil); err != nil {
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
				staleMsg := fmt.Sprintf("Stuck in provisioning since %s (>15m), marking failed", dep.StatusChangedAt.Format(time.RFC3339))
				w.log.Error("Reconcile: deployment stuck in provisioning",
					"deployment_id", dep.ID,
					"since", dep.StatusChangedAt,
				)
				if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusFailed, staleMsg, nil); err != nil {
					w.log.Warn("Failed to mark stale deployment as failed", "error", err, "deployment_id", dep.ID)
				}
			}
		}
	}

	// Stuck pending — job insert may have failed after DB commit, or the
	// River job was deduplicated. Re-enqueue after 5m, mark failed after 30m.
	pending, err := w.store.GetDeploymentsInStatus(deploymentstore.StatusPending)
	if err != nil {
		w.log.Error("Reconcile: failed to list pending deployments", "error", err)
	} else {
		for _, dep := range pending {
			stuckFor := time.Since(dep.StatusChangedAt)
			if stuckFor > 30*time.Minute {
				staleMsg := fmt.Sprintf("Stuck in pending since %s (>30m), marking failed", dep.StatusChangedAt.Format(time.RFC3339))
				w.log.Error("Reconcile: deployment stuck in pending too long, marking failed",
					"deployment_id", dep.ID,
					"since", dep.StatusChangedAt,
				)
				if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusFailed, staleMsg, nil); err != nil {
					w.log.Warn("Failed to mark stale pending deployment as failed", "error", err, "deployment_id", dep.ID)
				}
			} else if stuckFor > 5*time.Minute {
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
	deps, err := w.store.GetDeploymentsInStatus(
		deploymentstore.StatusActive,
		deploymentstore.StatusScaledDown,
		deploymentstore.StatusPending,
		deploymentstore.StatusProvisioning,
		deploymentstore.StatusFailed,
		deploymentstore.StatusUndeploying,
	)
	if err != nil {
		w.log.Error("Reconcile: failed to list deployments for ownership", "error", err)
		return
	}

	scanTime := time.Now()
	dbNamespaces := make(map[string]*deploymentstore.Deployment, len(deps))

	for _, dep := range deps {
		dbNamespaces[dep.Namespace] = dep
		sourceAccount := sourceAccountFromSpec(dep.DeploymentSpecJSON)
		_, err := w.db.ExecContext(ctx, `
			INSERT INTO namespace_ownership (namespace, account_id, agent_name, deployment_id, source_account, scanned_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (namespace) DO UPDATE
			SET account_id = EXCLUDED.account_id,
			    agent_name = EXCLUDED.agent_name,
			    deployment_id = EXCLUDED.deployment_id,
			    source_account = EXCLUDED.source_account,
			    scanned_at = EXCLUDED.scanned_at
		`, dep.Namespace, dep.AccountID, dep.AgentName, dep.ID, sourceAccount, scanTime)
		if err != nil {
			w.log.Warn("Reconcile: failed to upsert namespace_ownership", "namespace", dep.Namespace, "error", err)
		}
	}

	// Detect orphaned K8s namespaces and recover them as failed deployments.
	nsList, err := w.k8s.Clientset().CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if err != nil {
		w.log.Warn("Reconcile: failed to list K8s namespaces", "error", err)
		return
	}

	for _, ns := range nsList.Items {
		if _, ok := dbNamespaces[ns.Name]; ok {
			continue
		}

		accountID := ns.Labels["astro.dev/account-id"]
		agentName := ns.Labels["astro.dev/agent"]
		buildID := ns.Labels["astro.dev/build"]

		if accountID == "" || agentName == "" {
			w.log.Warn("Reconcile: orphaned K8s namespace missing labels, skipping recovery",
				"namespace", ns.Name,
			)
			continue
		}

		newID := deployid.FromNamespace(ns.Name)
		if newID == "" {
			newID = deployid.New()
		}
		if err := w.store.RecoverOrphanedDeployment(newID, accountID, agentName, buildID, ns.Name); err != nil {
			w.log.Error("Reconcile: failed to recover orphaned namespace",
				"namespace", ns.Name,
				"error", err,
			)
			continue
		}

		w.log.Warn("Reconcile: recovered orphaned K8s namespace as failed deployment",
			"namespace", ns.Name,
			"deployment_id", newID,
			"account_id", accountID,
			"agent", agentName,
		)
	}
}

// sourceAccountFromSpec extracts the source.account field from a deployment spec JSON.
// Returns empty string if the spec is empty or unparseable.
func sourceAccountFromSpec(specJSON string) string {
	if specJSON == "" || specJSON == "{}" {
		return ""
	}
	var spec struct {
		Source struct {
			Account string `json:"account"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return ""
	}
	return spec.Source.Account
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

// buildDeploymentDriftReport fetches expected state from DB and compares against live K8s.
func (w *ReconcileWorker) buildDeploymentDriftReport(ctx context.Context, dep *deploymentstore.Deployment) *deploymentstore.DriftReport {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	workloads, err := w.store.GetWorkloads(dep.ID)
	if err != nil {
		return nil
	}

	services, _ := w.store.GetServices(dep.ID)
	ingresses, _ := w.store.GetIngresses(dep.ID)

	// Build service ID -> name map for ingress display
	svcNameByID := map[int]string{}
	for _, svc := range services {
		svcNameByID[svc.ID] = svc.WorkloadName
	}

	return BuildDriftReport(ctx, w.k8s.Clientset(), dep.Namespace, workloads, services, ingresses, svcNameByID)
}

// BuildDriftReport is the core drift detection logic.
// It compares expected workloads, services, and ingresses against the live K8s state,
// producing a structured DriftReport. Used by both the reconciler and the admin gRPC server.
func BuildDriftReport(ctx context.Context, clientset *kubernetes.Clientset, namespace string, workloads []*deploymentstore.Workload, services []*deploymentstore.Service, ingresses []*deploymentstore.Ingress, svcNameByID map[int]string) *deploymentstore.DriftReport {
	report := &deploymentstore.DriftReport{
		DetectedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// --- Workloads ---
	expectedWorkloadNames := map[string]bool{}
	for _, wl := range workloads {
		expectedWorkloadNames[wl.Name] = true
		item := deploymentstore.DriftResourceItem{
			Name:     wl.Name,
			Type:     wl.WorkloadType,
			Expected: map[string]string{"Image": wl.Image, "Replicas": fmt.Sprintf("%d", wl.Replicas)},
		}

		switch wl.WorkloadType {
		case "deployment":
			actual, err := clientset.AppsV1().Deployments(namespace).Get(ctx, wl.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					item.Status = "missing"
					item.Actual = map[string]string{}
				} else {
					continue // transient error, skip
				}
			} else {
				actualReplicas := int32(1)
				if actual.Spec.Replicas != nil {
					actualReplicas = *actual.Spec.Replicas
				}
				actualImage := ""
				if len(actual.Spec.Template.Spec.Containers) > 0 {
					actualImage = actual.Spec.Template.Spec.Containers[0].Image
				}
				item.Actual = map[string]string{
					"Image":    actualImage,
					"Replicas": fmt.Sprintf("%d/%d", actual.Status.ReadyReplicas, actualReplicas),
				}
				if int(actualReplicas) != wl.Replicas || actualImage != wl.Image {
					item.Status = "drift"
				} else {
					item.Status = "match"
				}
			}

		case "statefulset":
			actual, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, wl.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					item.Status = "missing"
					item.Actual = map[string]string{}
				} else {
					continue
				}
			} else {
				actualReplicas := int32(1)
				if actual.Spec.Replicas != nil {
					actualReplicas = *actual.Spec.Replicas
				}
				actualImage := ""
				if len(actual.Spec.Template.Spec.Containers) > 0 {
					actualImage = actual.Spec.Template.Spec.Containers[0].Image
				}
				item.Actual = map[string]string{
					"Image":    actualImage,
					"Replicas": fmt.Sprintf("%d/%d", actual.Status.ReadyReplicas, actualReplicas),
				}
				if int(actualReplicas) != wl.Replicas || actualImage != wl.Image {
					item.Status = "drift"
				} else {
					item.Status = "match"
				}
			}

		case "cronjob":
			actual, err := clientset.BatchV1().CronJobs(namespace).Get(ctx, wl.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					item.Status = "missing"
					item.Actual = map[string]string{}
				} else {
					continue
				}
			} else {
				item.Expected = map[string]string{}
				if wl.TriggerSchedule != nil {
					item.Expected["Schedule"] = *wl.TriggerSchedule
				}
				item.Actual = map[string]string{"Schedule": actual.Spec.Schedule}
				if wl.TriggerSchedule != nil && actual.Spec.Schedule != *wl.TriggerSchedule {
					item.Status = "drift"
				} else {
					item.Status = "match"
				}
			}

		default:
			continue // skip unknown types
		}

		report.Workloads = append(report.Workloads, item)
	}

	// Detect extra deployments/statefulsets not in expected set
	if liveDeployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{}); err == nil {
		for _, d := range liveDeployments.Items {
			if !expectedWorkloadNames[d.Name] {
				actualReplicas := int32(1)
				if d.Spec.Replicas != nil {
					actualReplicas = *d.Spec.Replicas
				}
				actualImage := ""
				if len(d.Spec.Template.Spec.Containers) > 0 {
					actualImage = d.Spec.Template.Spec.Containers[0].Image
				}
				report.Workloads = append(report.Workloads, deploymentstore.DriftResourceItem{
					Name:     d.Name,
					Type:     "deployment",
					Status:   "extra",
					Expected: map[string]string{},
					Actual: map[string]string{
						"Image":    actualImage,
						"Replicas": fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, actualReplicas),
					},
				})
			}
		}
	}
	if liveStatefulSets, err := clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{}); err == nil {
		for _, ss := range liveStatefulSets.Items {
			if !expectedWorkloadNames[ss.Name] {
				actualReplicas := int32(1)
				if ss.Spec.Replicas != nil {
					actualReplicas = *ss.Spec.Replicas
				}
				actualImage := ""
				if len(ss.Spec.Template.Spec.Containers) > 0 {
					actualImage = ss.Spec.Template.Spec.Containers[0].Image
				}
				report.Workloads = append(report.Workloads, deploymentstore.DriftResourceItem{
					Name:     ss.Name,
					Type:     "statefulset",
					Status:   "extra",
					Expected: map[string]string{},
					Actual: map[string]string{
						"Image":    actualImage,
						"Replicas": fmt.Sprintf("%d/%d", ss.Status.ReadyReplicas, actualReplicas),
					},
				})
			}
		}
	}

	// --- Services (deduplicate by WorkloadName) ---
	expectedSvcNames := map[string]bool{}
	checked := map[string]bool{}
	// Group expected services by workload name for port comparison
	svcPortsByName := map[string]string{}
	for _, svc := range services {
		svcName := svc.WorkloadName
		if svcName == "" {
			svcName = svc.Name
		}
		expectedSvcNames[svcName] = true
		if existing, ok := svcPortsByName[svcName]; ok {
			svcPortsByName[svcName] = existing + ", " + fmt.Sprintf("%d", svc.Port)
		} else {
			svcPortsByName[svcName] = fmt.Sprintf("%d", svc.Port)
		}
	}

	for _, svc := range services {
		svcName := svc.WorkloadName
		if svcName == "" {
			svcName = svc.Name
		}
		if checked[svcName] {
			continue
		}
		checked[svcName] = true

		item := deploymentstore.DriftResourceItem{
			Name:     svcName,
			Type:     "service",
			Expected: map[string]string{"Ports": svcPortsByName[svcName]},
		}

		actual, err := clientset.CoreV1().Services(namespace).Get(ctx, svcName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				item.Status = "missing"
				item.Actual = map[string]string{}
			} else {
				continue
			}
		} else {
			var actualPorts []string
			for _, p := range actual.Spec.Ports {
				actualPorts = append(actualPorts, fmt.Sprintf("%d", p.Port))
			}
			item.Actual = map[string]string{"Ports": strings.Join(actualPorts, ", ")}
			item.Status = "match"
		}
		report.Services = append(report.Services, item)
	}

	// Detect extra services
	if liveSvcs, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{}); err == nil {
		for _, s := range liveSvcs.Items {
			if !expectedSvcNames[s.Name] {
				var actualPorts []string
				for _, p := range s.Spec.Ports {
					actualPorts = append(actualPorts, fmt.Sprintf("%d", p.Port))
				}
				report.Services = append(report.Services, deploymentstore.DriftResourceItem{
					Name:     s.Name,
					Type:     "service",
					Status:   "extra",
					Expected: map[string]string{},
					Actual:   map[string]string{"Ports": strings.Join(actualPorts, ", ")},
				})
			}
		}
	}

	// --- Ingresses ---
	expectedIngressHosts := map[string]bool{}
	for _, ing := range ingresses {
		expectedIngressHosts[ing.Hostname] = true
		item := deploymentstore.DriftResourceItem{
			Name: ing.Hostname,
			Type: "ingress",
			Expected: map[string]string{
				"Hostname": ing.Hostname,
				"Path":     ing.Path,
				"Service":  svcNameByID[ing.ServiceID],
			},
		}

		// Check if any live ingress has this host
		found := false
		if liveIngresses, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{}); err == nil {
			for _, li := range liveIngresses.Items {
				for _, rule := range li.Spec.Rules {
					if rule.Host == ing.Hostname {
						found = true
						paths := ""
						backend := ""
						if rule.HTTP != nil {
							for _, p := range rule.HTTP.Paths {
								paths = p.Path
								if p.Backend.Service != nil {
									backend = fmt.Sprintf("%s:%d", p.Backend.Service.Name, p.Backend.Service.Port.Number)
								}
							}
						}
						item.Actual = map[string]string{
							"Hostname": rule.Host,
							"Path":     paths,
							"Service":  backend,
						}
						item.Status = "match"
						break
					}
				}
				if found {
					break
				}
			}
		}
		if !found {
			item.Status = "missing"
			item.Actual = map[string]string{}
		}
		report.Ingresses = append(report.Ingresses, item)
	}

	// Detect extra ingresses
	if liveIngresses, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{}); err == nil {
		for _, li := range liveIngresses.Items {
			for _, rule := range li.Spec.Rules {
				if !expectedIngressHosts[rule.Host] {
					paths := ""
					backend := ""
					if rule.HTTP != nil {
						for _, p := range rule.HTTP.Paths {
							paths = p.Path
							if p.Backend.Service != nil {
								backend = fmt.Sprintf("%s:%d", p.Backend.Service.Name, p.Backend.Service.Port.Number)
							}
						}
					}
					report.Ingresses = append(report.Ingresses, deploymentstore.DriftResourceItem{
						Name:     rule.Host,
						Type:     "ingress",
						Status:   "extra",
						Expected: map[string]string{},
						Actual: map[string]string{
							"Hostname": rule.Host,
							"Path":     paths,
							"Service":  backend,
						},
					})
				}
			}
		}
	}

	// Compute summary
	allItems := append(append(report.Workloads, report.Services...), report.Ingresses...)
	report.Summary.Total = len(allItems)
	for _, item := range allItems {
		switch item.Status {
		case "match":
			report.Summary.Match++
		case "missing":
			report.Summary.Missing++
		case "extra":
			report.Summary.Extra++
		case "drift":
			report.Summary.Drift++
		}
	}

	return report
}
