package riverqueue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/riverqueue/river"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	spec "github.com/astropods/astro/packages/astro-spec"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
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
	queue     *Queue
	log       *logger.Logger
}

func (w *ReconcileWorker) Work(ctx context.Context, _ *river.Job[ReconcileArgs]) error {
	if w.k8s == nil {
		return nil
	}

	// Each step gets its own timeout so one slow step can't starve the others.
	run := func(fn func(context.Context)) {
		stepCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		fn(stepCtx)
	}

	run(w.reconcileActive)
	run(w.reconcileOIDCIssuer)
	run(w.detectStaleJobs)
	run(w.escalatePodFailures)
	run(w.maintainNamespaceOwnership)

	return nil
}

// Pod-failure escalation thresholds. We use pod creation time as a proxy for
// "long enough that a normal image pull would have completed" — there's no
// per-state timestamp on container statuses, so the pod's age is the cheapest
// stable signal. Tuned to be conservative: fast networks pull cold images in
// <30s, so a 2 minute wait is well past "transient pull retry" territory.
const (
	podFailureWaitGrace   = 2 * time.Minute
	crashLoopRestartLimit = 5
)

// permanentWaitReasons are container waiting reasons we treat as a hard
// failure once the pod has been around longer than podFailureWaitGrace. These
// are all states that do not self-recover without operator action (a fresh
// build, a fixed image reference, etc.).
var permanentWaitReasons = map[string]bool{
	"ImagePullBackOff":     true,
	"ErrImagePull":         true,
	"InvalidImageName":     true,
	"CreateContainerError": true,
}

// escalatePodFailures inspects pods belonging to active/provisioning/pending
// deployments and flips them to failed when a container has been wedged on a
// permanent failure reason long enough to count as a real failure. This is the
// reconciliation companion to the deploy-time image preflight: anything the
// preflight misses (image deleted post-deploy, init failure, OOM crash loop)
// gets escalated here instead of silently sitting in "deploying" forever.
//
// Performance: a single cluster-wide pod List filtered by the
// app.kubernetes.io/managed-by=astro-server label is one round-trip
// regardless of how many deployments are active. Pods are then grouped by
// namespace in memory so each deployment is matched O(1).
//
// Idempotent: deployments already in status=failed are filtered out at the
// query layer.
func (w *ReconcileWorker) escalatePodFailures(ctx context.Context) {
	deps, err := w.store.GetDeploymentsInStatus(
		deploymentstore.StatusActive,
		deploymentstore.StatusProvisioning,
		deploymentstore.StatusPending,
	)
	if err != nil {
		w.log.Error("Reconcile: failed to list deployments for pod escalation", "error", err)
		return
	}
	if len(deps) == 0 {
		return
	}

	pods, err := w.k8s.Clientset().CoreV1().Pods("").List(ctx, metav1.ListOptions{
		LabelSelector: managedByAstroLabelSelector,
	})
	if err != nil {
		w.log.Warn("Reconcile: failed to list managed pods for escalation", "error", err)
		return
	}
	byNamespace := groupPodsByNamespace(pods.Items)

	for _, dep := range deps {
		reason, image, podName := findEscalatablePodFailure(byNamespace[dep.Namespace])
		if reason == "" {
			continue
		}
		msg := formatPodFailureMessage(reason, image, podName)
		if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusFailed, msg, nil); err != nil {
			w.log.Warn("Reconcile: failed to escalate deployment to failed",
				"error", err, "deployment_id", dep.ID)
			continue
		}
		w.log.Warn("Reconcile: escalated deployment to failed (pod failure)",
			"deployment_id", dep.ID,
			"namespace", dep.Namespace,
			"pod", podName,
			"reason", reason,
			"image", image,
		)
	}
}

// managedByAstroLabelSelector matches every workload pod the Applier renders
// (k8s.ManagedByLabel constant). Knowledge-store pods provisioned by us also
// carry this label so they participate in escalation.
const managedByAstroLabelSelector = "app.kubernetes.io/managed-by=astro-server"

// groupPodsByNamespace bins a flat pod slice by metadata.namespace. Pods that
// are mid-deletion (DeletionTimestamp set) or have already finished
// successfully (PodSucceeded; e.g. ingestion-startup Jobs) are dropped so
// stale waiting-state on a terminating pod cannot trigger an escalation.
func groupPodsByNamespace(pods []corev1.Pod) map[string][]corev1.Pod {
	out := make(map[string][]corev1.Pod, len(pods))
	for i := range pods {
		p := &pods[i]
		if p.DeletionTimestamp != nil {
			continue
		}
		if p.Status.Phase == corev1.PodSucceeded {
			continue
		}
		out[p.Namespace] = append(out[p.Namespace], *p)
	}
	return out
}

// findEscalatablePodFailure returns (reason, image, pod_name) for the first
// pod in the slice whose containers indicate a permanent failure, or
// ("", "", "") when nothing in the slice is escalatable yet.
func findEscalatablePodFailure(pods []corev1.Pod) (reason, image, podName string) {
	for i := range pods {
		if r, img := classifyPodFailure(pods[i]); r != "" {
			return r, img, pods[i].Name
		}
	}
	return "", "", ""
}

// classifyPodFailure inspects a single pod's container statuses and returns
// (reason, image) for the first permanent failure detected, or ("", "") when
// the pod is healthy or still in transient-failure territory.
func classifyPodFailure(pod corev1.Pod) (reason, image string) {
	age := time.Since(pod.CreationTimestamp.Time)
	all := append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...)
	all = append(all, pod.Status.ContainerStatuses...)
	for _, cs := range all {
		if cs.State.Waiting == nil {
			continue
		}
		r := cs.State.Waiting.Reason
		switch {
		case permanentWaitReasons[r]:
			if age >= podFailureWaitGrace {
				return r, cs.Image
			}
		case r == "CrashLoopBackOff":
			if cs.RestartCount > crashLoopRestartLimit {
				return r, cs.Image
			}
		}
	}
	return "", ""
}

// formatPodFailureMessage renders a short human-readable error for the
// deployments.error_message column. UI surfaces this verbatim in tooltips.
func formatPodFailureMessage(reason, image, podName string) string {
	return fmt.Sprintf("%s on pod %s (image=%s)", reason, podName, image)
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

		// Log drift but do NOT auto-remediate. The drift report is saved above
		// for visibility in the admin UI; operators can trigger a manual reapply.
		if report.Summary.Missing+report.Summary.Drift > 0 {
			w.log.Warn("Reconcile: drift detected",
				"deployment_id", dep.ID,
				"namespace", dep.Namespace,
				"missing", report.Summary.Missing,
				"drift", report.Summary.Drift,
				"total", report.Summary.Total,
			)
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

// reconcileOIDCConfig detects when any OIDC configuration (issuer, endpoints,
// client credentials, session timeout) has changed relative to the live K8s
// state. Any active deployment with auth.web.type=oidc whose ingress
// annotations or credentials secret are stale is set back to pending and
// re-enqueued for deploy.
func (w *ReconcileWorker) reconcileOIDCIssuer(ctx context.Context) {
	dcfg := w.deployer.Cfg.Deployment
	if dcfg.MessagingOIDCIssuer == "" {
		return // OIDC disabled — nothing to reconcile
	}

	deps, err := w.store.GetDeploymentsInStatus(deploymentstore.StatusActive)
	if err != nil {
		w.log.Error("Reconcile OIDC: failed to list active deployments", "error", err)
		return
	}

	// Build expected annotation JSON — mirrors k8s/ingress.go BuildIngress.
	expectedAnnotation := buildExpectedOIDCAnnotation(dcfg.MessagingOIDCIssuer,
		dcfg.MessagingOIDCAuthEndpoint, dcfg.MessagingOIDCTokenEndpoint,
		dcfg.MessagingOIDCUserInfoEndpoint)

	expectedTimeout := dcfg.MessagingOIDCSessionTimeout
	if expectedTimeout == 0 {
		expectedTimeout = 3600
	}

	// Collect OIDC deployments and their namespaces (DB-only, no K8s calls).
	var oidcDeps []*deploymentstore.Deployment
	oidcNamespaces := map[string]bool{}
	for _, dep := range deps {
		if specHasOIDCAuth(dep.DeploymentSpecJSON) {
			oidcDeps = append(oidcDeps, dep)
			oidcNamespaces[dep.Namespace] = true
		}
	}
	if len(oidcDeps) == 0 {
		return
	}

	// Single cluster-wide ingress list filtered by managed-by label, then
	// bucket by namespace. One K8s call instead of N.
	clientset := w.k8s.Clientset()
	allIngresses, err := clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if err != nil {
		w.log.Warn("Reconcile OIDC: failed to list cluster ingresses", "error", err)
		return
	}

	ingressByNs := map[string][]networkingv1.Ingress{}
	for _, ing := range allIngresses.Items {
		if oidcNamespaces[ing.Namespace] {
			ingressByNs[ing.Namespace] = append(ingressByNs[ing.Namespace], ing)
		}
	}

	// Check every OIDC deployment against the pre-fetched ingresses so
	// partially-applied rollouts recover on the next tick.
	const maxReappliesPerTick = 10
	reapplied := 0
	stale := 0
	for _, dep := range oidcDeps {
		if oidcAnnotationCurrent(ingressByNs[dep.Namespace], expectedAnnotation, expectedTimeout) {
			continue
		}
		stale++

		if reapplied >= maxReappliesPerTick {
			continue // count remaining stale but don't enqueue
		}

		if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusPending, "OIDC config changed — reapplying", nil); err != nil {
			w.log.Error("Reconcile OIDC: failed to set pending", "deployment_id", dep.ID, "error", err)
			continue
		}
		if err := w.queue.InsertDeployJob(ctx, dep.ID); err != nil {
			w.log.Error("Reconcile OIDC: failed to enqueue deploy job", "deployment_id", dep.ID, "error", err)
			continue
		}
		reapplied++
	}
	if stale > 0 {
		w.log.Info("Reconcile OIDC: stale deployments found",
			"stale", stale, "reapplied", reapplied, "deferred", stale-reapplied)
	}
}

// specHasOIDCAuth returns true if the deployment spec JSON has auth.web.type == "oidc".
func specHasOIDCAuth(specJSON string) bool {
	var ds spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(specJSON), &ds); err != nil {
		return false
	}
	return ds.Interfaces != nil &&
		ds.Interfaces.Auth != nil &&
		ds.Interfaces.Auth.Web != nil &&
		ds.Interfaces.Auth.Web.Type == "oidc"
}

// buildExpectedOIDCAnnotation produces the JSON string that BuildIngress would
// write to the auth-idp-oidc annotation, so we can compare exactly.
func buildExpectedOIDCAnnotation(issuer, authEP, tokenEP, userInfoEP string) string {
	b, err := json.Marshal(map[string]string{ //nolint:gosec // G101 false positive: "messaging-oidc" is a K8s secret name, not a credential
		"issuer":                issuer,
		"authorizationEndpoint": authEP,
		"tokenEndpoint":         tokenEP,
		"userInfoEndpoint":      userInfoEP,
		"secretName":            "messaging-oidc",
	})
	if err != nil {
		return ""
	}
	return string(b)
}

// oidcAnnotationCurrent checks whether the OIDC ingress annotations are up to
// date. Returns true (current) when either:
//   - an ingress has the correct OIDC annotation, or
//   - no ingress has an OIDC annotation at all (nothing to fix).
//
// Returns false (stale) only when an ingress exists with an OIDC annotation
// that doesn't match the expected config.
func oidcAnnotationCurrent(ingresses []networkingv1.Ingress, expectedAnnotation string, expectedTimeout int) bool {
	for _, ing := range ingresses {
		liveOIDC := ing.Annotations["alb.ingress.kubernetes.io/auth-idp-oidc"]
		if liveOIDC == "" {
			continue
		}
		if liveOIDC != expectedAnnotation {
			return false // annotation exists but is stale
		}
		liveTimeout := ing.Annotations["alb.ingress.kubernetes.io/auth-session-timeout"]
		if liveTimeout != fmt.Sprintf("%d", expectedTimeout) {
			return false
		}
		return true // found a matching OIDC ingress
	}
	return true // no OIDC annotation found — nothing to fix
}

// detectOrphanedNamespaces finds K8s namespaces managed by astro-server that
// have no corresponding deployment in the database, and recovers them as failed
// deployments so operators can investigate.
func (w *ReconcileWorker) maintainNamespaceOwnership(ctx context.Context) {
	deps, err := w.store.GetDeploymentsInStatus(
		deploymentstore.StatusActive,
		deploymentstore.StatusScaledDown,
		deploymentstore.StatusPending,
		deploymentstore.StatusProvisioning,
		deploymentstore.StatusFailed,
		deploymentstore.StatusUndeploying,
		deploymentstore.StatusUndeployed,
		deploymentstore.StatusStopped,
	)
	if err != nil {
		w.log.Error("Reconcile: failed to list deployments for orphan detection", "error", err)
		return
	}

	dbNamespaces := make(map[string]*deploymentstore.Deployment, len(deps))
	for _, dep := range deps {
		dbNamespaces[dep.Namespace] = dep
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
		agentName := ns.Labels[deployment.LabelKeyAgent]
		buildID := ns.Labels["astro.dev/build"]

		if accountID == "" || agentName == "" {
			w.log.Warn("Reconcile: orphaned K8s namespace missing labels, skipping recovery",
				"namespace", ns.Name,
			)
			continue
		}

		// The source-account-id label was added in the PR2 work — namespaces
		// stamped before that commit lack it. Default the missing case to the
		// deployer account: same-account deploys are the common case, and
		// any genuine cross-account orphan stamped pre-PR2 is rare enough
		// that the warning + manual triage is preferable to silently
		// recording a wrong lineage.
		sourceAccountID := ns.Labels[deployment.LabelKeySourceAccountID]
		if sourceAccountID == "" {
			w.log.Warn("Reconcile: orphaned K8s namespace missing source-account-id label, defaulting to deployer account",
				"namespace", ns.Name,
				"account_id", accountID,
			)
			sourceAccountID = accountID
		}

		newID := deployid.FromNamespace(ns.Name)
		if newID == "" {
			newID = deployid.New()
		}
		if err := w.store.RecoverOrphanedDeployment(newID, accountID, sourceAccountID, agentName, buildID, ns.Name); err != nil {
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
			"source_account_id", sourceAccountID,
			"agent", agentName,
		)
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
	variables, _ := w.store.GetDeploymentVariables(dep.ID)
	resolvedKeys, _ := w.store.GetResolvedKeys(dep.ID)

	// Build service ID -> name map for ingress display
	svcNameByID := map[int]string{}
	for _, svc := range services {
		svcNameByID[svc.ID] = svc.WorkloadName
	}

	return BuildDriftReport(ctx, w.k8s.Clientset(), dep.Namespace, dep.AgentName, dep.BuildID, workloads, services, ingresses, svcNameByID, variables, resolvedKeys)
}

// BuildDriftReport is the core drift detection logic.
// It compares expected workloads, services, ingresses, env vars, and secrets
// against the live K8s state, producing a structured DriftReport.
// Used by both the reconciler and the admin gRPC server.
func BuildDriftReport(ctx context.Context, clientset *kubernetes.Clientset, namespace string, agentName string, buildID string, workloads []*deploymentstore.Workload, services []*deploymentstore.Service, ingresses []*deploymentstore.Ingress, svcNameByID map[int]string, variables []deploymentstore.Variable, resolvedKeys *deploymentstore.ResolvedKeys) *deploymentstore.DriftReport {
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

	// --- Environment Variables (ConfigMap) ---
	// Use resolved keys (the actual ConfigMap/Secret key sets computed during
	// deploy) when available. Fall back to deriving from deployment_variables
	// for deployments that pre-date the resolved keys table.
	var expectedEnvKeys []string
	var expectedSecretKeys []string
	if resolvedKeys != nil {
		expectedEnvKeys = resolvedKeys.ConfigMapKeys
		expectedSecretKeys = resolvedKeys.SecretKeys
	} else {
		for _, v := range variables {
			upperKey := strings.ToUpper(v.Name)
			if v.Secret {
				expectedSecretKeys = append(expectedSecretKeys, upperKey)
			} else {
				expectedEnvKeys = append(expectedEnvKeys, upperKey)
			}
		}
	}

	if len(expectedEnvKeys) > 0 {
		configMapName := deployment.GenerateConfigMapName(agentName, buildID)
		item := deploymentstore.DriftResourceItem{
			Name: configMapName,
			Type: "configmap",
			Expected: map[string]string{
				"Keys": strings.Join(sortedKeys(expectedEnvKeys), ", "),
			},
		}

		actual, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				item.Status = "missing"
				item.Actual = map[string]string{}
			} else {
				goto skipConfigMap
			}
		} else {
			actualKeys := sortedKeys(mapKeys(actual.Data))
			item.Actual = map[string]string{
				"Keys": strings.Join(actualKeys, ", "),
			}
			missingKeys, _ := diffKeys(expectedEnvKeys, mapKeys(actual.Data))
			if len(missingKeys) > 0 {
				item.Status = "drift"
				item.Expected["Missing"] = strings.Join(sortedKeys(missingKeys), ", ")
			} else if resolvedKeys != nil && len(resolvedKeys.ConfigMapHashes) > 0 {
				// Keys match — check values via hash comparison
				var changed []string
				for k, expectedHash := range resolvedKeys.ConfigMapHashes {
					if actualVal, ok := actual.Data[k]; ok {
						if hashString(actualVal) != expectedHash {
							changed = append(changed, k)
						}
					}
				}
				if len(changed) > 0 {
					item.Status = "drift"
					item.Expected["Changed"] = strings.Join(sortedKeys(changed), ", ")
				} else {
					item.Status = "match"
				}
			} else {
				item.Status = "match"
			}
		}
		report.EnvVars = append(report.EnvVars, item)
	}
skipConfigMap:

	// --- Secrets ---
	if len(expectedSecretKeys) > 0 {
		secretName := deployment.GenerateSecretName(agentName, buildID)
		item := deploymentstore.DriftResourceItem{
			Name: secretName,
			Type: "secret",
			Expected: map[string]string{
				"Keys": strings.Join(sortedKeys(expectedSecretKeys), ", "),
			},
		}

		actual, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				item.Status = "missing"
				item.Actual = map[string]string{}
			} else {
				goto skipSecret
			}
		} else {
			actualKeys := sortedKeys(byteMapKeys(actual.Data))
			item.Actual = map[string]string{
				"Keys": strings.Join(actualKeys, ", "),
			}
			secretMissing, secretExtra := diffKeys(expectedSecretKeys, byteMapKeys(actual.Data))
			if len(secretMissing) > 0 || len(secretExtra) > 0 {
				item.Status = "drift"
				if len(secretMissing) > 0 {
					item.Expected["Missing"] = strings.Join(sortedKeys(secretMissing), ", ")
				}
				if len(secretExtra) > 0 {
					item.Actual["Extra"] = strings.Join(sortedKeys(secretExtra), ", ")
				}
			} else if resolvedKeys != nil && len(resolvedKeys.SecretHashes) > 0 {
				// Keys match — check values via hash comparison
				var changed []string
				for k, expectedHash := range resolvedKeys.SecretHashes {
					if actualVal, ok := actual.Data[k]; ok {
						if hashBytes(actualVal) != expectedHash {
							changed = append(changed, k)
						}
					}
				}
				if len(changed) > 0 {
					item.Status = "drift"
					item.Expected["Changed"] = strings.Join(sortedKeys(changed), ", ")
				} else {
					item.Status = "match"
				}
			} else {
				item.Status = "match"
			}
		}
		report.Secrets = append(report.Secrets, item)
	}
skipSecret:

	// Compute summary
	allItems := append(append(report.Workloads, report.Services...), report.Ingresses...)
	allItems = append(allItems, report.EnvVars...)
	allItems = append(allItems, report.Secrets...)
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

// sortedKeys returns a sorted copy of a string slice.
func sortedKeys(keys []string) []string {
	sorted := make([]string, len(keys))
	copy(sorted, keys)
	sort.Strings(sorted)
	return sorted
}

// mapKeys returns the keys of a map[string]string.
func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// byteMapKeys returns the keys of a map[string][]byte.
func byteMapKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// diffKeys returns keys present in expected but not in actual (missing),
// and keys present in actual but not in expected (extra).
// hashString returns the hex-encoded SHA-256 of a string.
func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// hashBytes returns the hex-encoded SHA-256 of a byte slice.
func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func diffKeys(expected, actual []string) (missing, extra []string) {
	expectedSet := make(map[string]bool, len(expected))
	for _, k := range expected {
		expectedSet[k] = true
	}
	actualSet := make(map[string]bool, len(actual))
	for _, k := range actual {
		actualSet[k] = true
	}
	for _, k := range expected {
		if !actualSet[k] {
			missing = append(missing, k)
		}
	}
	for _, k := range actual {
		if !expectedSet[k] {
			extra = append(extra, k)
		}
	}
	return
}
