package k8s

import (
	"context"
	"fmt"

	"log"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ApplierConfig holds configuration for the Applier
type ApplierConfig struct {
	Namespace         string
	RegistryURL       string
	ProxyRegistryHost string
	Environment       string
	ImagePullPolicy   corev1.PullPolicy // Defaults to PullAlways; set PullNever for local dev
	// Ingress configuration for agent workloads
	IngressDomain     string
	ACMCertificateARN string
	ALBGroupName      string
	// Ingress configuration for ingestion workloads (separate ALB)
	IngestionIngressDomain string
	IngestionACMCertARN    string
	IngestionALBGroupName  string
	// Observability (Galileo) — injected into collector sidecar
	GalileoAPIKey  string
	GalileoProject string
	// Observability (Langfuse) — per-account auth token for collector sidecar
	LangfuseAuthToken string
	LangfuseBaseURL   string
	// DeploymentID is the database deployment ID (passed to collector as ASTRO_DEPLOYMENT_ID)
	DeploymentID string
	// NamespaceLabels are merged into the namespace metadata on create/update
	NamespaceLabels map[string]string
	// NamespaceAnnotations are merged into the namespace metadata on create/update
	NamespaceAnnotations map[string]string
	// PodSubnetCIDRs are the private subnet CIDRs where cluster pods run.
	// When non-empty, NetworkPolicies enforcing namespace isolation are applied.
	PodSubnetCIDRs []string
	// LocalMode relaxes pod security hardening for third-party provider
	// containers (qdrant, neo4j, etc.) that expect to run as their image's
	// default user. Only set true for local K8s (Docker Desktop / kind).
	LocalMode bool
}

// Applier applies Kubernetes manifests to a cluster
type Applier struct {
	clientset       kubernetes.Interface
	namespace       string
	registryURL     string
	imageResolver   *ImageResolver
	imagePullPolicy corev1.PullPolicy
	// Ingress configuration for agent workloads
	ingressDomain     string
	acmCertificateARN string
	albGroupName      string
	// Ingress configuration for ingestion workloads (separate ALB)
	ingestionIngressDomain string
	ingestionACMCertARN    string
	ingestionALBGroupName  string
	// Observability
	galileoAPIKey     string
	galileoProject    string
	langfuseAuthToken string
	langfuseBaseURL   string
	deploymentID      string
	// Per-namespace labels
	namespaceLabels map[string]string
	// Per-namespace annotations
	namespaceAnnotations map[string]string
	// Pod subnet CIDRs for NetworkPolicy isolation
	podSubnetCIDRs []string
	localMode      bool
}

// NewApplier creates a new applier
func NewApplier(client ClusterClient, cfg ApplierConfig) *Applier {
	pullPolicy := cfg.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}
	return &Applier{
		clientset:              client.Clientset(),
		namespace:              cfg.Namespace,
		registryURL:            cfg.RegistryURL,
		imageResolver:          NewImageResolver(cfg.ProxyRegistryHost, cfg.RegistryURL, cfg.Environment),
		imagePullPolicy:        pullPolicy,
		ingressDomain:          cfg.IngressDomain,
		acmCertificateARN:      cfg.ACMCertificateARN,
		albGroupName:           cfg.ALBGroupName,
		ingestionIngressDomain: cfg.IngestionIngressDomain,
		ingestionACMCertARN:    cfg.IngestionACMCertARN,
		ingestionALBGroupName:  cfg.IngestionALBGroupName,
		galileoAPIKey:          cfg.GalileoAPIKey,
		galileoProject:         cfg.GalileoProject,
		langfuseAuthToken:      cfg.LangfuseAuthToken,
		langfuseBaseURL:        cfg.LangfuseBaseURL,
		deploymentID:           cfg.DeploymentID,
		namespaceLabels:        cfg.NamespaceLabels,
		namespaceAnnotations:   cfg.NamespaceAnnotations,
		podSubnetCIDRs:         cfg.PodSubnetCIDRs,
		localMode:              cfg.LocalMode,
	}
}

// resolveContainerImage resolves a container image reference to its ECR path
func (a *Applier) resolveContainerImage(container spec.ContainerConfig) (spec.ContainerConfig, error) {
	if container.Image == "" {
		return container, nil
	}

	resolvedImage, err := a.imageResolver.ResolveImage(container.Image)
	if err != nil {
		return container, err
	}

	// Create a copy with resolved image
	resolved := container
	resolved.Image = resolvedImage
	return resolved, nil
}

// ApplyResult holds the result of applying manifests
type ApplyResult struct {
	Resources        []deployment.ResourceStatus
	ServiceEndpoints []deployment.ServiceEndpoint
	Errors           []deployment.DeploymentError
}

// applySecret creates or updates a Secret
func (a *Applier) applySecret(ctx context.Context, secret *corev1.Secret) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "Secret",
		Name:      secret.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.CoreV1().Secrets(a.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = a.clientset.CoreV1().Secrets(a.namespace).Update(ctx, secret, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyConfigMap creates or updates a ConfigMap
func (a *Applier) applyConfigMap(ctx context.Context, cm *corev1.ConfigMap) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "ConfigMap",
		Name:      cm.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.CoreV1().ConfigMaps(a.namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = a.clientset.CoreV1().ConfigMaps(a.namespace).Update(ctx, cm, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyService creates or updates a Service
func (a *Applier) applyService(ctx context.Context, svc *corev1.Service) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "Service",
		Name:      svc.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.CoreV1().Services(a.namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// For services, we need to preserve the ClusterIP
			existing, err := a.clientset.CoreV1().Services(a.namespace).Get(ctx, svc.Name, metav1.GetOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			svc.Spec.ClusterIP = existing.Spec.ClusterIP
			svc.ResourceVersion = existing.ResourceVersion

			_, err = a.clientset.CoreV1().Services(a.namespace).Update(ctx, svc, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyDeployment creates or updates a Deployment
func (a *Applier) applyDeployment(ctx context.Context, depl *appsv1.Deployment) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "Deployment",
		Name:      depl.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.AppsV1().Deployments(a.namespace).Create(ctx, depl, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = a.clientset.AppsV1().Deployments(a.namespace).Update(ctx, depl, metav1.UpdateOptions{})
			if err != nil {
				// Selector is immutable — if it changed, delete and recreate.
				if errors.IsInvalid(err) {
					if delErr := a.clientset.AppsV1().Deployments(a.namespace).Delete(ctx, depl.Name, metav1.DeleteOptions{}); delErr != nil {
						status.Status = "failed"
						status.Message = fmt.Sprintf("delete for recreate: %v", delErr)
						return status, delErr
					}
					if _, createErr := a.clientset.AppsV1().Deployments(a.namespace).Create(ctx, depl, metav1.CreateOptions{}); createErr != nil {
						status.Status = "failed"
						status.Message = createErr.Error()
						return status, createErr
					}
					status.Status = "recreated"
					return status, nil
				}
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyStatefulSet creates or updates a StatefulSet.
// On update it preserves immutable fields (selector, serviceName,
// volumeClaimTemplates) from the existing resource so that Kubernetes
// does not reject the request. If the selector has changed (e.g. label
// scheme migration), the StatefulSet is deleted and recreated.
func (a *Applier) applyStatefulSet(ctx context.Context, ss *appsv1.StatefulSet) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "StatefulSet",
		Name:      ss.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.AppsV1().StatefulSets(a.namespace).Create(ctx, ss, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, getErr := a.clientset.AppsV1().StatefulSets(a.namespace).Get(ctx, ss.Name, metav1.GetOptions{})
			if getErr != nil {
				status.Status = "failed"
				status.Message = getErr.Error()
				return status, getErr
			}

			// If the selector has changed, we must delete and recreate —
			// K8s does not allow updating the selector or template labels
			// to not match it.
			if !selectorMatchesLabels(existing.Spec.Selector, ss.Spec.Template.Labels) {
				if delErr := a.clientset.AppsV1().StatefulSets(a.namespace).Delete(ctx, ss.Name, metav1.DeleteOptions{}); delErr != nil {
					status.Status = "failed"
					status.Message = fmt.Sprintf("delete for recreate: %v", delErr)
					return status, delErr
				}
				if _, createErr := a.clientset.AppsV1().StatefulSets(a.namespace).Create(ctx, ss, metav1.CreateOptions{}); createErr != nil {
					status.Status = "failed"
					status.Message = createErr.Error()
					return status, createErr
				}
				status.Status = "recreated"
				return status, nil
			}

			// Preserve immutable fields from the existing StatefulSet
			ss.Spec.Selector = existing.Spec.Selector
			ss.Spec.ServiceName = existing.Spec.ServiceName
			ss.Spec.VolumeClaimTemplates = existing.Spec.VolumeClaimTemplates
			ss.ResourceVersion = existing.ResourceVersion

			_, err = a.clientset.AppsV1().StatefulSets(a.namespace).Update(ctx, ss, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// selectorMatchesLabels returns true if every key/value in the selector's
// MatchLabels exists in the given labels map.
func selectorMatchesLabels(sel *metav1.LabelSelector, labels map[string]string) bool {
	if sel == nil {
		return true
	}
	for k, v := range sel.MatchLabels {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// applyCronJob creates or updates a CronJob
func (a *Applier) applyCronJob(ctx context.Context, cj *batchv1.CronJob) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "CronJob",
		Name:      cj.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.BatchV1().CronJobs(a.namespace).Create(ctx, cj, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = a.clientset.BatchV1().CronJobs(a.namespace).Update(ctx, cj, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyJob creates a Job, deleting any existing one first (Jobs are immutable)
func (a *Applier) applyJob(ctx context.Context, job *batchv1.Job) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "Job",
		Name:      job.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.BatchV1().Jobs(a.namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Jobs are immutable once created — delete and recreate
			propagation := metav1.DeletePropagationForeground
			deleteErr := a.clientset.BatchV1().Jobs(a.namespace).Delete(ctx, job.Name, metav1.DeleteOptions{
				PropagationPolicy: &propagation,
			})
			if deleteErr != nil {
				status.Status = "failed"
				status.Message = fmt.Sprintf("failed to delete existing job: %v", deleteErr)
				return status, deleteErr
			}
			// Recreate
			_, err = a.clientset.BatchV1().Jobs(a.namespace).Create(ctx, job, metav1.CreateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyNetworkPolicy creates or updates a NetworkPolicy
func (a *Applier) applyNetworkPolicy(ctx context.Context, np *networkingv1.NetworkPolicy) error {
	_, err := a.clientset.NetworkingV1().NetworkPolicies(a.namespace).Create(ctx, np, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, getErr := a.clientset.NetworkingV1().NetworkPolicies(a.namespace).Get(ctx, np.Name, metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			np.ResourceVersion = existing.ResourceVersion
			_, err = a.clientset.NetworkingV1().NetworkPolicies(a.namespace).Update(ctx, np, metav1.UpdateOptions{})
			return err
		}
		return err
	}
	return nil
}

// cleanupStaleBuildResources deletes resources from a previous build of the same
// agent. This prevents stale Deployments/Services/Ingresses from lingering when
// resource names change between builds (e.g. a container is renamed).
// Errors are logged but not returned — stale resources are annoying but not fatal.
func (a *Applier) cleanupStaleBuildResources(ctx context.Context, accountName, agentName, buildID string) []error {
	labelSelector := fmt.Sprintf("app.kubernetes.io/managed-by=astro-server,%s=%s", deployment.LabelKeyAgent, deployment.AgentLabelValue(accountName, agentName))
	propagation := metav1.DeletePropagationBackground
	deleteOpts := metav1.DeleteOptions{PropagationPolicy: &propagation}
	listOpts := metav1.ListOptions{LabelSelector: labelSelector}

	var errs []error
	sanitizedBuildID := deployment.SanitizeName(buildID)
	isStale := func(labels map[string]string) bool {
		return labels["app.kubernetes.io/version"] != "" && labels["app.kubernetes.io/version"] != sanitizedBuildID
	}

	// Ingresses
	ingresses, err := a.clientset.NetworkingV1().Ingresses(a.namespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range ingresses.Items {
			if isStale(item.Labels) {
				log.Printf("[cleanup] deleting stale Ingress %s (build %s)", item.Name, item.Labels["app.kubernetes.io/version"])
				if err := a.clientset.NetworkingV1().Ingresses(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
					errs = append(errs, fmt.Errorf("delete ingress %s: %w", item.Name, err))
				}
			}
		}
	}

	// Deployments
	deployments, err := a.clientset.AppsV1().Deployments(a.namespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range deployments.Items {
			if isStale(item.Labels) {
				log.Printf("[cleanup] deleting stale Deployment %s (build %s)", item.Name, item.Labels["app.kubernetes.io/version"])
				if err := a.clientset.AppsV1().Deployments(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
					errs = append(errs, fmt.Errorf("delete deployment %s: %w", item.Name, err))
				}
			}
		}
	}

	// StatefulSets
	statefulSets, err := a.clientset.AppsV1().StatefulSets(a.namespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range statefulSets.Items {
			if isStale(item.Labels) {
				log.Printf("[cleanup] deleting stale StatefulSet %s (build %s)", item.Name, item.Labels["app.kubernetes.io/version"])
				if err := a.clientset.AppsV1().StatefulSets(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
					errs = append(errs, fmt.Errorf("delete statefulset %s: %w", item.Name, err))
				}
			}
		}
	}

	// CronJobs
	cronJobs, err := a.clientset.BatchV1().CronJobs(a.namespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range cronJobs.Items {
			if isStale(item.Labels) {
				log.Printf("[cleanup] deleting stale CronJob %s (build %s)", item.Name, item.Labels["app.kubernetes.io/version"])
				if err := a.clientset.BatchV1().CronJobs(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
					errs = append(errs, fmt.Errorf("delete cronjob %s: %w", item.Name, err))
				}
			}
		}
	}

	// Jobs
	jobs, err := a.clientset.BatchV1().Jobs(a.namespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range jobs.Items {
			if isStale(item.Labels) {
				log.Printf("[cleanup] deleting stale Job %s (build %s)", item.Name, item.Labels["app.kubernetes.io/version"])
				if err := a.clientset.BatchV1().Jobs(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
					errs = append(errs, fmt.Errorf("delete job %s: %w", item.Name, err))
				}
			}
		}
	}

	// Services
	services, err := a.clientset.CoreV1().Services(a.namespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range services.Items {
			if isStale(item.Labels) {
				log.Printf("[cleanup] deleting stale Service %s (build %s)", item.Name, item.Labels["app.kubernetes.io/version"])
				if err := a.clientset.CoreV1().Services(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
					errs = append(errs, fmt.Errorf("delete service %s: %w", item.Name, err))
				}
			}
		}
	}

	// ConfigMaps
	configMaps, err := a.clientset.CoreV1().ConfigMaps(a.namespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range configMaps.Items {
			if isStale(item.Labels) {
				log.Printf("[cleanup] deleting stale ConfigMap %s (build %s)", item.Name, item.Labels["app.kubernetes.io/version"])
				if err := a.clientset.CoreV1().ConfigMaps(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
					errs = append(errs, fmt.Errorf("delete configmap %s: %w", item.Name, err))
				}
			}
		}
	}

	// Secrets
	secrets, err := a.clientset.CoreV1().Secrets(a.namespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range secrets.Items {
			if isStale(item.Labels) {
				log.Printf("[cleanup] deleting stale Secret %s (build %s)", item.Name, item.Labels["app.kubernetes.io/version"])
				if err := a.clientset.CoreV1().Secrets(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
					errs = append(errs, fmt.Errorf("delete secret %s: %w", item.Name, err))
				}
			}
		}
	}

	return errs
}

// applyIngress creates or updates an Ingress.
// It refuses to create an ingress whose backend service port is zero — doing so
// would produce a target group with no healthy targets, causing ALB health checks
// to fail and pod readiness gates to block the rollout indefinitely.
func (a *Applier) applyIngress(ctx context.Context, ing *networkingv1.Ingress) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "Ingress",
		Name:      ing.Name,
		Namespace: a.namespace,
	}

	// Safety: reject ingress if any rule references a backend with port == 0.
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service != nil && path.Backend.Service.Port.Number == 0 {
				status.Status = "failed"
				status.Message = "refusing to create ingress: backend service port is 0 (container port not exposed)"
				return status, fmt.Errorf("%s", status.Message)
			}
		}
	}

	_, err := a.clientset.NetworkingV1().Ingresses(a.namespace).Create(ctx, ing, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Get existing ingress to preserve resource version
			existing, getErr := a.clientset.NetworkingV1().Ingresses(a.namespace).Get(ctx, ing.Name, metav1.GetOptions{})
			if getErr != nil {
				status.Status = "failed"
				status.Message = getErr.Error()
				return status, getErr
			}
			ing.ResourceVersion = existing.ResourceVersion

			_, err = a.clientset.NetworkingV1().Ingresses(a.namespace).Update(ctx, ing, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}
