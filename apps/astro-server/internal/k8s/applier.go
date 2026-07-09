package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"log"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// registryPullSecretName is the dockerconfigjson Secret the applier writes into
// each tenant namespace (and links to the default ServiceAccount) so tenant
// pods pull tenant images through astro-registry.
const registryPullSecretName = "astro-registry-pull"

// ApplierConfig holds configuration for the Applier
type ApplierConfig struct {
	Namespace         string
	ProxyRegistryHost string
	// RegistryPullCredential is the cluster pull credential (CPC) the kubelet
	// presents to astro-registry to pull tenant images. When set (with
	// ProxyRegistryHost), the applier writes a dockerconfigjson pull secret into
	// the tenant namespace and links it to the default ServiceAccount so tenant
	// pods pull through the registry. See docs/01-spec/registry-pull-through-spec.md.
	RegistryPullCredential string
	ImagePullPolicy        corev1.PullPolicy // Defaults to PullAlways; set PullNever for local dev
	// ImagePreflighter, when set, performs a registry HEAD on tenant images
	// in resolveContainerImage and returns *ErrImageNotFound on missing tags.
	// Defense in depth — the deploy handler should already have preflighted
	// the agent image synchronously before enqueueing, but this catches
	// stale specs and bypassed code paths.
	ImagePreflighter *ImagePreflighter
	// TenantImageHosts is the allowlist of registry hosts whose images we
	// preflight. Empty disables preflight in the Applier (handler-only mode).
	// Typical values: the proxy registry host (registry.localhost) and the
	// resolved registry URL host (123.dkr.ecr....amazonaws.com).
	TenantImageHosts []string
	// Ingress configuration for agent workloads (front-door ALB owns TLS,
	// OIDC, and ALB grouping in astro-infra; we just need the domain to
	// generate per-tenant hostnames).
	IngressDomain string
	// AgentPublicIngressDomain is the open (no-OIDC) cohort base. Web surfaces
	// flagged public generate their host here so the front-door ALB skips OIDC.
	AgentPublicIngressDomain string
	// Ingress configuration for ingestion workloads
	IngestionIngressDomain string
	// Observability (Langfuse) — per-account auth token for collector sidecar
	LangfuseAuthToken string
	LangfuseBaseURL   string
	// DeploymentID is the database deployment ID (passed to collector as ASTRO_DEPLOYMENT_ID)
	DeploymentID string
	// NamespaceLabels are merged into the namespace metadata on create/update
	NamespaceLabels map[string]string
	// NamespaceAnnotations are merged into the namespace metadata on create/update
	NamespaceAnnotations map[string]string
	// PodSubnetCIDRs are the secondary-private subnet CIDRs where cluster pods run.
	// When non-empty, NetworkPolicies enforcing namespace isolation are applied.
	PodSubnetCIDRs []string
	// CPSubnetCIDRs are primary VPC private subnets hosting EKS apiserver ENIs.
	// When non-empty, a sibling `allow-apiserver-proxy` NetworkPolicy is generated
	// allowing service-proxy traffic to messaging sidecars on TCP 8090/9090.
	CPSubnetCIDRs []string
	// LangfuseVPCEIPs are the VPC endpoint ENI IPs for Langfuse PrivateLink.
	// When non-empty, an egress rule allowing port 3000 to these IPs is added.
	LangfuseVPCEIPs []string
	// LocalMode relaxes pod security hardening for third-party provider
	// containers (qdrant, neo4j, etc.) that expect to run as their image's
	// default user. Only set true for local K8s (Docker Desktop / kind).
	LocalMode bool
	// AstroGatewayAPIKey is the per-account LiteLLM virtual key minted at
	// deploy time. Injected under every resolver-derived credential name for
	// each models.* entry whose provider == "astro-gateway". Empty when no
	// such entry exists or when the AI Gateway is disabled in this env.
	AstroGatewayAPIKey string
	// AstroGatewayBaseURL is the gateway endpoint paired with AstroGatewayAPIKey,
	// surfaced through the resolver-derived *_BASE_URL env vars.
	AstroGatewayBaseURL string
	// Bound knowledge store resolution info (populated by deployer for specs with bindings)
	BoundKnowledge   map[string]deployment.BoundKnowledgeInfo
	BoundCredentials map[string]string // "name.key" → credential value
	// DeployTokenSecret is the HMAC secret used to sign per-deployment tokens
	// injected into messaging containers as ASTRO_AUTHZ_TOKEN.
	DeployTokenSecret string
	// AuthzCallbackURL is the base URL the messaging container uses to call
	// astro-server's /deployments/authorize endpoint. Injected as
	// ASTRO_AUTHZ_URL alongside ASTRO_AUTHZ_TOKEN. When empty (local dev),
	// the messaging container falls back to AllowAll.
	AuthzCallbackURL string
	// AuthTestUserID is the user_id (typically the account owner) that the
	// messaging container should treat every web request as coming from.
	// Set only in local mode where no ingress is in front to inject the
	// real OIDC identity header.
	AuthTestUserID string
	// DeploymentID is reused as the key for deployment_build_env writes.
	// Already set above for collector wiring; PersistResolutions uses it.
	// (Field reused, not redeclared.)

	// PersistResolutions is invoked once per ApplyDeploymentSpec, after
	// Resolve has run, to write rows to deployment_build_env. The applier
	// can't import deploymentstore directly (deploymentstore depends on
	// k8s); the deployer wires this callback to its Store + Encryptor.
	// Best-effort: returning an error logs a warning and the apply
	// continues.
	PersistResolutions func(deploymentID string, rows []deployment.Resolution) error
	// PersistMessagingHost is invoked in local mode after the messaging
	// Service has been created with an auto-allocated NodePort; the deployer
	// writes the resolved host:port back to the deployment_ingresses row so
	// GetMessagingURLs surfaces a working Launch URL. host is "localhost:<n>".
	PersistMessagingHost func(deploymentID, host string) error
}

// Applier applies Kubernetes manifests to a cluster
type Applier struct {
	clientset              kubernetes.Interface
	namespace              string
	proxyRegistryHost      string
	registryPullCredential string
	imagePullPolicy        corev1.PullPolicy
	imagePreflighter       *ImagePreflighter
	tenantImageHosts       []string
	// Ingress configuration
	ingressDomain            string
	agentPublicIngressDomain string
	ingestionIngressDomain   string
	// Observability
	langfuseAuthToken string
	langfuseBaseURL   string
	deploymentID      string
	// Per-namespace labels
	namespaceLabels map[string]string
	// Per-namespace annotations
	namespaceAnnotations map[string]string
	// Pod subnet CIDRs for NetworkPolicy isolation
	podSubnetCIDRs       []string
	cpSubnetCIDRs        []string
	langfuseVPCEIPs      []string
	localMode            bool
	astroGatewayAPIKey   string
	astroGatewayBaseURL  string
	boundKnowledge       map[string]deployment.BoundKnowledgeInfo
	boundCredentials     map[string]string
	deployTokenSecret    string
	authzCallbackURL     string
	authTestUserID       string
	persistResolutions   func(deploymentID string, rows []deployment.Resolution) error
	persistMessagingHost func(deploymentID, host string) error
}

// NewApplier creates a new applier
func NewApplier(client ClusterClient, cfg ApplierConfig) *Applier {
	pullPolicy := cfg.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}
	return &Applier{
		clientset:                client.Clientset(),
		namespace:                cfg.Namespace,
		proxyRegistryHost:        cfg.ProxyRegistryHost,
		registryPullCredential:   cfg.RegistryPullCredential,
		imagePullPolicy:          pullPolicy,
		imagePreflighter:         cfg.ImagePreflighter,
		tenantImageHosts:         cfg.TenantImageHosts,
		ingressDomain:            cfg.IngressDomain,
		agentPublicIngressDomain: cfg.AgentPublicIngressDomain,
		ingestionIngressDomain:   cfg.IngestionIngressDomain,
		langfuseAuthToken:        cfg.LangfuseAuthToken,
		langfuseBaseURL:          cfg.LangfuseBaseURL,
		deploymentID:             cfg.DeploymentID,
		namespaceLabels:          cfg.NamespaceLabels,
		namespaceAnnotations:     cfg.NamespaceAnnotations,
		podSubnetCIDRs:           cfg.PodSubnetCIDRs,
		cpSubnetCIDRs:            cfg.CPSubnetCIDRs,
		langfuseVPCEIPs:          cfg.LangfuseVPCEIPs,
		localMode:                cfg.LocalMode,
		astroGatewayAPIKey:       cfg.AstroGatewayAPIKey,
		astroGatewayBaseURL:      cfg.AstroGatewayBaseURL,
		boundKnowledge:           cfg.BoundKnowledge,
		boundCredentials:         cfg.BoundCredentials,
		deployTokenSecret:        cfg.DeployTokenSecret,
		authzCallbackURL:         cfg.AuthzCallbackURL,
		authTestUserID:           cfg.AuthTestUserID,
		persistResolutions:       cfg.PersistResolutions,
		persistMessagingHost:     cfg.PersistMessagingHost,
	}
}

// ensureRegistryPullSecret writes the tenant image-pull Secret (a
// dockerconfigjson for the proxy registry, built from the cluster pull
// credential) into the namespace and links it to the default ServiceAccount, so
// every tenant pod pulls tenant images through astro-registry. No-op when the
// credential or proxy host is unset (e.g. local dev).
func (a *Applier) ensureRegistryPullSecret(ctx context.Context) error {
	if a.registryPullCredential == "" || a.proxyRegistryHost == "" {
		return nil
	}

	dockercfg, err := dockerConfigJSON(a.proxyRegistryHost, a.registryPullCredential)
	if err != nil {
		return fmt.Errorf("build dockerconfigjson: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      registryPullSecretName,
			Namespace: a.namespace,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "astro-server"},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: dockercfg},
	}

	secrets := a.clientset.CoreV1().Secrets(a.namespace)
	if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create pull secret: %w", err)
		}
		if _, err := secrets.Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update pull secret: %w", err)
		}
	}

	return a.linkPullSecretToDefaultSA(ctx)
}

// linkPullSecretToDefaultSA adds the pull secret to the namespace default
// ServiceAccount's imagePullSecrets, so every pod using that SA (all tenant
// pods — none set a serviceAccountName) picks it up. The default SA is created
// asynchronously after the namespace, so we retry briefly; conflicts re-read.
func (a *Applier) linkPullSecretToDefaultSA(ctx context.Context) error {
	sas := a.clientset.CoreV1().ServiceAccounts(a.namespace)

	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		sa, err := sas.Get(ctx, "default", metav1.GetOptions{})
		if err != nil {
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}

		for _, ref := range sa.ImagePullSecrets {
			if ref.Name == registryPullSecretName {
				return nil // already linked
			}
		}

		sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{Name: registryPullSecretName})
		if _, err := sas.Update(ctx, sa, metav1.UpdateOptions{}); err != nil {
			if errors.IsConflict(err) {
				continue // someone else updated it; re-read and retry
			}
			return fmt.Errorf("link pull secret to default ServiceAccount: %w", err)
		}
		return nil
	}
	return fmt.Errorf("default ServiceAccount not ready in namespace %s: %w", a.namespace, lastErr)
}

// dockerConfigJSON builds a Docker config.json for a single registry host,
// authenticating as user "token" with the given credential in the password slot
// (the astro-registry v2 token flow reads the credential from the password).
func dockerConfigJSON(host, credential string) ([]byte, error) {
	auth := base64.StdEncoding.EncodeToString([]byte("token:" + credential))
	cfg := map[string]any{
		"auths": map[string]any{
			host: map[string]string{
				"username": "token",
				"password": credential,
				"auth":     auth,
			},
		},
	}
	return json.Marshal(cfg)
}

// resolveContainerImage preflights a container image (when an ImagePreflighter
// is configured for a tenant host) to fail fast on a vanished tag, returning
// *ErrImageNotFound so the deploy errors immediately instead of waiting for
// kubelet to surface ImagePullBackOff. Image references are already final pull
// paths — the deployment template produces them — so nothing is rewritten here.
func (a *Applier) resolveContainerImage(ctx context.Context, container spec.ContainerConfig) (spec.ContainerConfig, error) {
	if container.Image == "" {
		return container, nil
	}

	if a.imagePreflighter != nil && a.shouldPreflight(container.Image) {
		if perr := a.imagePreflighter.Preflight(ctx, container.Image); perr != nil {
			return container, perr
		}
	}

	return container, nil
}

// shouldPreflight returns true when image's host matches one of the tenant
// registry hosts configured on the Applier. Restricting preflight to tenant
// images keeps us from issuing HEAD requests to docker.io / quay.io / etc.
// for every public sidecar (postgres, qdrant, ...), which would be wasted
// network and noisy logs.
func (a *Applier) shouldPreflight(image string) bool {
	if len(a.tenantImageHosts) == 0 {
		return false
	}
	host, _, _, ok := parseImageRef(image)
	if !ok {
		return false
	}
	target := stripPort(host)
	for _, h := range a.tenantImageHosts {
		if h == "" {
			continue
		}
		if strings.EqualFold(stripPort(h), target) {
			return true
		}
	}
	return false
}

// ApplyResult holds the result of applying manifests
type ApplyResult struct {
	Resources        []deployment.ResourceStatus
	ServiceEndpoints []deployment.ServiceEndpoint
	Errors           []deployment.DeploymentError

	// AllCredentials is the union of bound and self-hosted knowledge
	// credentials that were available during this apply, keyed
	// "<knowledgeName>.<attr>". Surfaced to the caller so the orchestrator
	// (deployer) can run deployment.Resolve over the spec and persist
	// rows to deployment_build_env without re-deriving the cred set.
	AllCredentials map[string]string
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
			existing, getErr := a.clientset.BatchV1().CronJobs(a.namespace).Get(ctx, cj.Name, metav1.GetOptions{})
			if getErr != nil {
				status.Status = "failed"
				status.Message = getErr.Error()
				return status, getErr
			}
			cj.ResourceVersion = existing.ResourceVersion
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

// applyJob creates a Job. Because Jobs are immutable, an existing Job is
// deleted first; the delete is asynchronous, so applyJob waits for the object
// to disappear before recreating to avoid the "object is being deleted" race.
func (a *Applier) applyJob(ctx context.Context, job *batchv1.Job) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "Job",
		Name:      job.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.BatchV1().Jobs(a.namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Jobs are immutable — delete then recreate. Deletion is async, so
			// wait until the object is actually gone before recreating;
			// recreating while it is still terminating fails with
			// "object is being deleted".
			propagation := metav1.DeletePropagationBackground
			if deleteErr := a.clientset.BatchV1().Jobs(a.namespace).Delete(ctx, job.Name, metav1.DeleteOptions{
				PropagationPolicy: &propagation,
			}); deleteErr != nil && !errors.IsNotFound(deleteErr) {
				status.Status = "failed"
				status.Message = fmt.Sprintf("failed to delete existing job: %v", deleteErr)
				return status, deleteErr
			}

			// Bound the wait so a stuck finalizer can't hang the deploy worker.
			if waitErr := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 60*time.Second, true,
				func(ctx context.Context) (bool, error) {
					_, getErr := a.clientset.BatchV1().Jobs(a.namespace).Get(ctx, job.Name, metav1.GetOptions{})
					return errors.IsNotFound(getErr), nil
				}); waitErr != nil {
				status.Status = "failed"
				status.Message = fmt.Sprintf("existing job still terminating: %v", waitErr)
				return status, waitErr
			}

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
