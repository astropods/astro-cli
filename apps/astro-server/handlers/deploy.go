package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/config"
	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/apps/astro-server/internal/deploymentstore"
	"github.com/postman/astro/apps/astro-server/internal/k8s"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/middleware"
	"github.com/postman/astro/packages/astro-spec"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// isAccountMember checks whether the user is a member of the account.
func isAccountMember(_ *gin.Context, accountStore *account.AccountStore, acctID, userID string) bool {
	isMember, err := accountStore.IsMember(acctID, userID)
	return err == nil && isMember
}

// deploymentNamespace derives a K8s namespace from a deployment UUID.
// The UUID is generated once per new deployment and stored in the DB,
// so the namespace is stable across redeploys.
func deploymentNamespace(deploymentID string) string {
	return "astro-" + strings.ReplaceAll(deploymentID, "-", "")[:20]
}

// verifyNamespaceOwnership checks that a K8s namespace belongs to the given account.
func verifyNamespaceOwnership(ctx context.Context, k8sClient k8s.ClusterClient, namespace, accountID string) error {
	ns, err := k8sClient.Clientset().CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("namespace not found: %w", err)
	}
	if ns.Labels["astro.dev/account-id"] != accountID {
		return fmt.Errorf("namespace %s does not belong to account %s", namespace, accountID)
	}
	return nil
}

// DeployAgent returns a handler for deploying agents to Kubernetes
// parseDeploySpec reads and parses a deployment spec from the request body.
// Supports both YAML and JSON (detected automatically).
func parseDeploySpec(c *gin.Context) (*spec.AstroDeploymentSpec, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	return spec.ParseDeploymentSpec(body)
}

// prepareDeployment parses the submitted spec, authenticates the caller, looks up
// the registered agent build, regenerates the server's template, enforces Rule 19,
// and returns everything needed to proceed with deployment or validation.
type deployContext struct {
	acct          *account.Account
	agentName     string
	displayName   string
	deploymentID  string
	buildID       string
	k8sNS         string
	resolveResult *deployment.ResolveResult
}

func prepareDeployment(
	c *gin.Context,
	log *logger.Logger,
	submittedSpec *spec.AstroDeploymentSpec,
	accountStore *account.AccountStore,
	agentIndex *agentindex.Index,
	cfg *config.Config,
	deployStore *deploymentstore.Store,
) (*deployContext, bool) {
	user, exists := middleware.GetUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return nil, false
	}

	// Only accept fulfilled specs — not templates
	if submittedSpec.Spec != "deployment/v1" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("deploy endpoint requires spec: deployment/v1, got %q", submittedSpec.Spec),
		})
		return nil, false
	}

	sourceAccountName := submittedSpec.Source.Account
	if sourceAccountName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source.account is required in the deployment spec"})
		return nil, false
	}

	// Determine target account: prefer target.account, fall back to source.account
	targetAccountName := submittedSpec.Target.Account
	if targetAccountName == "" {
		targetAccountName = sourceAccountName
	}

	// Look up source account for build resolution
	sourceAcct, err := accountStore.GetByName(sourceAccountName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source account not found"})
		return nil, false
	}

	// Look up target account for auth check and namespace derivation
	var targetAcct *account.Account
	if targetAccountName == sourceAccountName {
		targetAcct = sourceAcct
	} else {
		targetAcct, err = accountStore.GetByName(targetAccountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "target account not found"})
			return nil, false
		}
	}

	// Auth: user must be a member of the target account
	isMember, err := accountStore.IsMember(targetAcct.ID, user.ID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for target account"})
		return nil, false
	}

	// Sanitize and validate the optional display name
	submittedSpec.Target.DisplayName = strings.TrimSpace(submittedSpec.Target.DisplayName)
	if dn := submittedSpec.Target.DisplayName; dn != "" {
		if len(dn) > 64 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target.display_name must be 64 characters or fewer"})
			return nil, false
		}
		for _, r := range dn {
			if r < 0x20 || r == 0x7f {
				c.JSON(http.StatusBadRequest, gin.H{"error": "target.display_name contains invalid control characters"})
				return nil, false
			}
		}
	}

	agentName := submittedSpec.Source.Name
	buildID := submittedSpec.Source.Build

	log.Info("Processing deployment spec",
		"agent", agentName,
		"build", buildID,
		"source_account", sourceAccountName,
		"target_account", targetAccountName,
		"user_id", user.ID,
	)

	// Look up the exact build referenced in the spec (from source account)
	agentVersion, err := agentIndex.GetVersion(sourceAcct.ID, agentName, buildID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "agent build not found",
			"details": fmt.Sprintf("no build %q found for agent %q", buildID, agentName),
		})
		return nil, false
	}

	// Parse the registered astro-spec for this build
	var astroSpec spec.AstroSpec
	specBytes, err := json.Marshal(agentVersion.Spec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process registered spec"})
		return nil, false
	}
	if err := json.Unmarshal(specBytes, &astroSpec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse registered spec"})
		return nil, false
	}

	// Re-generate the server's canonical template for this build
	template, err := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
		Spec:              &astroSpec,
		Account:           sourceAccountName,
		BuildID:           buildID,
		RegistryURL:       cfg.Deployment.RegistryURL,
		ProxyRegistryHost: cfg.Deployment.ProxyRegistryHost,
		Environment:       cfg.Deployment.Environment,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to generate deployment template",
			"details": err.Error(),
		})
		return nil, false
	}

	displayName := submittedSpec.Target.DisplayName

	// Check display name uniqueness within the account (if non-empty)
	if displayName != "" && deployStore != nil {
		existing, lookupErr := deployStore.GetActiveDeploymentByDisplayName(targetAcct.ID, displayName)
		if lookupErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check display name uniqueness"})
			return nil, false
		}
		// Allow redeploy: same agent + same display name is a redeploy, not a conflict
		if existing != nil && existing.AgentName != agentName {
			c.JSON(http.StatusConflict, gin.H{"error": "display_name already in use by another active deployment"})
			return nil, false
		}
	}

	// Resolve namespace: reuse existing if this is a redeploy, otherwise generate new
	var k8sNamespace, deploymentID string
	if displayName != "" && deployStore != nil {
		existing, _ := deployStore.GetActiveDeploymentByDisplayName(targetAcct.ID, displayName)
		if existing != nil {
			// Redeploy — reuse namespace
			k8sNamespace = existing.Namespace
			deploymentID = uuid.New().String()
		}
	}
	if k8sNamespace == "" && deployStore != nil {
		// Check for single active deployment of this agent (backward compat)
		existing, _ := deployStore.GetActiveDeployment(targetAcct.ID, agentName)
		if existing != nil && displayName == "" {
			k8sNamespace = existing.Namespace
			deploymentID = uuid.New().String()
		}
	}
	if k8sNamespace == "" {
		// New deployment — generate UUID-based namespace
		deploymentID = uuid.New().String()
		k8sNamespace = deploymentNamespace(deploymentID)
	}

	submittedSpec.Target.Namespace = k8sNamespace

	// Rule 19: reject any change to server-owned fields
	// Sync user-supplied target fields so EnforceEditable doesn't reject them
	template.Target.Account = submittedSpec.Target.Account
	template.Target.DisplayName = submittedSpec.Target.DisplayName
	if editErrs := spec.EnforceEditable(template, submittedSpec); len(editErrs) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "server-owned fields were modified",
			"validation_errors": toValidationErrors(editErrs),
		})
		return nil, false
	}

	// Validate and resolve
	resolveResult, err := deployment.ValidateAndResolve(submittedSpec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to resolve deployment spec",
			"details": err.Error(),
		})
		return nil, false
	}
	if len(resolveResult.Errors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "deployment spec validation failed",
			"validation_errors": toValidationErrors(resolveResult.Errors),
		})
		return nil, false
	}

	return &deployContext{
		acct:          targetAcct,
		agentName:     agentName,
		displayName:   displayName,
		deploymentID:  deploymentID,
		buildID:       buildID,
		k8sNS:         k8sNamespace,
		resolveResult: resolveResult,
	}, true
}

// DeployAgent returns a handler for deploying agents to Kubernetes.
// POST /api/v1/deploy
// Content-Type: application/yaml (or application/json)
// Body: fulfilled deployment spec (spec: deployment/v1)
func DeployAgent(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		submittedSpec, err := parseDeploySpec(c)
		if err != nil {
			log.Error("Failed to parse deployment spec", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid deployment spec",
				"details": err.Error(),
			})
			return
		}

		dctx, ok := prepareDeployment(c, log, submittedSpec, accountStore, agentIndex, cfg, deployStore)
		if !ok {
			return
		}

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		log.Info("Deploying to Kubernetes", "k8s_namespace", dctx.k8sNS)

		applier := k8s.NewApplier(k8sClient, k8s.ApplierConfig{
			Namespace:              dctx.k8sNS,
			RegistryURL:            cfg.Deployment.RegistryURL,
			ProxyRegistryHost:      cfg.Deployment.ProxyRegistryHost,
			Environment:            cfg.Deployment.Environment,
			ImagePullPolicy:        imagePullPolicyForMode(cfg.Deployment.K8sClientMode),
			IngressDomain:          cfg.Deployment.IngressDomain,
			ACMCertificateARN:      cfg.Deployment.ACMCertificateARN,
			ALBGroupName:           cfg.Deployment.ALBGroupName,
			IngestionIngressDomain: cfg.Deployment.IngestionIngressDomain,
			IngestionACMCertARN:    cfg.Deployment.IngestionACMCertARN,
			IngestionALBGroupName:  cfg.Deployment.IngestionALBGroupName,
			GalileoAPIKey:          cfg.Deployment.GalileoAPIKey,
			GalileoProject:         cfg.Deployment.GalileoProject,
			PodSubnetCIDRs:         cfg.Deployment.PodSubnetCIDRs,
			NamespaceLabels: map[string]string{
				"astro.dev/account-id": dctx.acct.ID,
				"astro.dev/account":    dctx.acct.Name,
				"astro.dev/agent":      dctx.agentName,
				"astro.dev/build":      dctx.buildID,
			},
			NamespaceAnnotations: map[string]string{
				"astro.dev/display-name": dctx.displayName,
			},
		})
		applyResult, err := applier.ApplyDeploymentSpec(c.Request.Context(), dctx.resolveResult.Spec)
		if err != nil {
			log.Error("Deployment failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "deployment failed",
				"details": err.Error(),
			})
			return
		}

		// Persist resolved spec (secret values stripped)
		if deployStore != nil {
			stripped := spec.StripSecretVariableValues(dctx.resolveResult.Spec)
			if specJSON, marshalErr := json.Marshal(stripped); marshalErr != nil {
				log.Error("Failed to marshal stripped spec for storage", "error", marshalErr)
			} else if _, storeErr := deployStore.SaveDeployment(dctx.deploymentID, dctx.acct.ID, dctx.agentName, dctx.displayName, dctx.buildID, dctx.k8sNS, string(specJSON)); storeErr != nil {
				log.Error("Failed to save deployment record", "error", storeErr)
			}
		}

		status := "success"
		statusCode := http.StatusOK
		if len(applyResult.Errors) > 0 {
			status = "partial"
			statusCode = http.StatusMultiStatus
			log.Warn("Deployment completed with errors", "error_count", len(applyResult.Errors))
		}

		log.Info("Deployment completed",
			"status", status,
			"resources", len(applyResult.Resources),
			"errors", len(applyResult.Errors),
		)

		c.JSON(statusCode, deployment.DeployResponse{
			Status:           status,
			Name:             dctx.agentName,
			BuildID:          dctx.buildID,
			K8sNamespace:     dctx.k8sNS,
			DeployedAt:       time.Now().UTC(),
			Resources:        applyResult.Resources,
			ServiceEndpoints: applyResult.ServiceEndpoints,
			Errors:           applyResult.Errors,
		})
	}
}

// ValidateDeployment validates a fulfilled deployment spec without applying it.
// POST /api/v1/deploy/validate
// Content-Type: application/yaml (or application/json)
// Body: fulfilled deployment spec (spec: deployment/v1)
func ValidateDeployment(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		submittedSpec, err := parseDeploySpec(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid deployment spec",
				"details": err.Error(),
			})
			return
		}

		dctx, ok := prepareDeployment(c, log, submittedSpec, accountStore, agentIndex, cfg, nil)
		if !ok {
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"valid":    true,
			"name":     dctx.agentName,
			"build_id": dctx.buildID,
		})
	}
}

// UndeployAgent returns a handler for undeploying agents from Kubernetes
func UndeployAgent(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req deployment.UndeployRequest

		// Parse request body
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Error("Failed to parse undeploy request", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request",
				"details": err.Error(),
			})
			return
		}

		// Get authenticated user from context
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		if deployStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment store not configured"})
			return
		}

		// Look up deployment by ID
		dep, err := deployStore.GetDeploymentByID(req.DeploymentID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up deployment"})
			return
		}
		if dep == nil || dep.Status != "active" {
			c.JSON(http.StatusNotFound, gin.H{"error": "active deployment not found"})
			return
		}

		// Verify user is a member of the deployment's account
		isMember, err := accountStore.IsMember(dep.AccountID, user.ID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
			return
		}

		log.Info("Received undeploy request",
			"deployment_id", req.DeploymentID,
			"agent", dep.AgentName,
			"namespace", dep.Namespace,
			"user_id", user.ID,
		)

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		// Delete the entire namespace — cascades to all resources
		err = k8sClient.Clientset().CoreV1().Namespaces().Delete(
			c.Request.Context(), dep.Namespace, metav1.DeleteOptions{},
		)
		if err != nil {
			log.Error("Failed to delete namespace", "error", err, "namespace", dep.Namespace)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "undeploy failed",
				"details": err.Error(),
			})
			return
		}

		// Mark deployment record as undeployed
		if err := deployStore.MarkUndeployedByID(dep.ID); err != nil {
			log.Warn("Failed to mark deployment as undeployed", "error", err)
		}

		response := deployment.UndeployResponse{
			Status:       "success",
			Name:         dep.AgentName,
			BuildID:      dep.BuildID,
			K8sNamespace: dep.Namespace,
			UndeployedAt: time.Now().UTC(),
		}

		log.Info("Undeploy completed",
			"status", "success",
			"namespace", dep.Namespace,
		)

		c.JSON(http.StatusOK, response)
	}
}

// ServiceEndpointInfo represents a service endpoint for a deployment
type ServiceEndpointInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Type string `json:"type,omitempty"`
}

// EnvVar represents a single environment variable in a container
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	From  string `json:"from,omitempty"` // e.g. "secret:my-secret/key" or "configmap:cm/key"
}

// ContainerStatus represents the status of a single container in a pod
type ContainerStatus struct {
	Name         string   `json:"name"`
	State        string   `json:"state"`
	Ready        bool     `json:"ready"`
	RestartCount int32    `json:"restart_count"`
	Reason       string   `json:"reason,omitempty"`
	Message      string   `json:"message,omitempty"`
	Env          []EnvVar `json:"env,omitempty"`
}

// PodDetail represents details about a single pod
type PodDetail struct {
	Name       string            `json:"name"`
	Phase      string            `json:"phase"`
	PodIP      string            `json:"pod_ip,omitempty"`
	Age        string            `json:"age"`
	Containers []ContainerStatus `json:"containers"`
}

// JobDetail represents details about a single K8s Job (e.g. ingestion run)
type JobDetail struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // "Running", "Succeeded", "Failed", "Pending"
	Component   string `json:"component"`
	Age         string `json:"age"`
	StartTime   string `json:"start_time,omitempty"`
	Completions string `json:"completions"` // "1/1", "0/1"
}

// AgentDeployment represents information about a deployed agent
type AgentDeployment struct {
	ID               string                `json:"id,omitempty"`
	Name             string                `json:"name"`
	DisplayName      string                `json:"display_name,omitempty"`
	BuildID          string                `json:"build_id"`
	Namespace        string                `json:"namespace"`
	Status           string                `json:"status"`
	Replicas         int32                 `json:"replicas"`
	Ready            int32                 `json:"ready"`
	CreatedAt        string                `json:"created_at"`
	Components       []string              `json:"components"`
	ManualIngestions []string              `json:"manual_ingestions,omitempty"`
	ExternalURLs     []ServiceEndpointInfo `json:"external_urls,omitempty"`
	Pods             []PodDetail           `json:"pods,omitempty"`
	Jobs             []JobDetail           `json:"jobs,omitempty"`
}

// ListDeployments returns a handler for listing deployed agents
func ListDeployments(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get authenticated user from context
		user, exists := middleware.GetUser(c)
		if !exists {
			log.Error("User not found in context")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			return
		}

		// Resolve account from query param
		accountName := c.Query("account")
		if accountName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "account query parameter is required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		// Verify user is a member
		isMember, err := accountStore.IsMember(acct.ID, user.ID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
			return
		}

		log.Info("Listing deployments",
			"account", accountName,
			"user_id", user.ID,
		)

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "kubernetes client not configured",
			})
			return
		}

		// List namespaces belonging to this account
		nsSelector := fmt.Sprintf("astro.dev/account-id=%s,app.kubernetes.io/managed-by=astro-server", acct.ID)
		nsList, err := k8sClient.Clientset().CoreV1().Namespaces().List(
			c.Request.Context(), metav1.ListOptions{LabelSelector: nsSelector},
		)
		if err != nil {
			log.Error("Failed to list namespaces", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to list deployments",
				"details": err.Error(),
			})
			return
		}

		// Build namespace → DB deployment info map
		type nsInfo struct {
			ID          string
			DisplayName string
		}
		nsDeployments := make(map[string]nsInfo)
		if deployStore != nil {
			dbDeps, dbErr := deployStore.GetActiveDeploymentsByAccount(acct.ID)
			if dbErr != nil {
				log.Warn("Failed to load deployments from DB", "error", dbErr)
			} else {
				for _, d := range dbDeps {
					nsDeployments[d.Namespace] = nsInfo{ID: d.ID, DisplayName: d.DisplayName}
				}
			}
		}

		// Aggregate deployments across all per-deployment namespaces
		var allDeployments []AgentDeployment
		for _, ns := range nsList.Items {
			manualIngestions := parseManualIngestions(ns.Annotations)
			deps, err := listAstroDeployments(c.Request.Context(), k8sClient, ns.Name, manualIngestions)
			if err != nil {
				log.Warn("Failed to list deployments in namespace", "namespace", ns.Name, "error", err)
				continue
			}
			if info, ok := nsDeployments[ns.Name]; ok {
				for i := range deps {
					deps[i].ID = info.ID
					deps[i].DisplayName = info.DisplayName
				}
			}
			allDeployments = append(allDeployments, deps...)
		}

		c.JSON(http.StatusOK, gin.H{
			"deployments": allDeployments,
			"count":       len(allDeployments),
		})
	}
}

// parseManualIngestions reads the "astro.dev/manual-ingestions" annotation from a namespace.
func parseManualIngestions(annotations map[string]string) []string {
	val := annotations["astro.dev/manual-ingestions"]
	if val == "" {
		return nil
	}
	return strings.Split(val, ",")
}

// listAstroDeployments lists all deployments managed by astro in a namespace
func listAstroDeployments(ctx context.Context, k8sClient k8s.ClusterClient, namespace string, manualIngestions []string) ([]AgentDeployment, error) {
	clientset := k8sClient.Clientset()

	// List deployments with astro label selector
	labelSelector := "app.kubernetes.io/managed-by=astro-server"
	deploymentList, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	// List ingresses for the namespace to get external URLs
	ingressList, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		// Ingress listing failure is not critical, log and continue
		ingressList = nil
	}

	// List pods for the namespace
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Build a map of external URLs from ingresses
	agentExternalURLs := make(map[string][]ServiceEndpointInfo) // key: "agentName:version" -> endpoints
	if ingressList != nil {
		for _, ing := range ingressList.Items {
			agentName := ing.Labels["astro.dev/agent"]
			version := ing.Labels["app.kubernetes.io/version"]
			component := ing.Labels["app.kubernetes.io/component"]

			if agentName != "" && len(ing.Spec.Rules) > 0 {
				key := agentName + ":" + version
				host := ing.Spec.Rules[0].Host
				if host != "" {
					agentExternalURLs[key] = append(agentExternalURLs[key], ServiceEndpointInfo{
						Name: component,
						URL:  fmt.Sprintf("https://%s", host),
						Type: component,
					})
				}
			}
		}
	}

	// Group deployments by agent name
	agentDeployments := make(map[string]*AgentDeployment)

	for _, dep := range deploymentList.Items {
		agentName := dep.Labels["astro.dev/agent"]
		version := dep.Labels["app.kubernetes.io/version"]
		component := dep.Labels["app.kubernetes.io/component"]

		if agentName == "" {
			continue
		}

		key := agentName + ":" + version
		info, exists := agentDeployments[key]
		if !exists {
			status := "Running"
			if dep.Status.ReadyReplicas < dep.Status.Replicas {
				status = "Pending"
			}
			if dep.Status.Replicas == 0 {
				status = "Stopped"
			}

			info = &AgentDeployment{
				Name:             agentName,
				BuildID:          version,
				Namespace:        namespace,
				Status:           status,
				Replicas:         dep.Status.Replicas,
				Ready:            dep.Status.ReadyReplicas,
				CreatedAt:        dep.CreationTimestamp.Format(time.RFC3339),
				Components:       []string{},
				ManualIngestions: manualIngestions,
			}

			// Add external URLs if available
			if urls, ok := agentExternalURLs[key]; ok {
				info.ExternalURLs = urls
			}

			agentDeployments[key] = info
		}

		// Add component
		if component != "" {
			info.Components = append(info.Components, component)
		}

		// Update status if any deployment is not ready
		if dep.Status.ReadyReplicas < dep.Status.Replicas {
			info.Status = "Pending"
		}
	}

	// List Jobs (e.g. ingestion runs) for the namespace
	jobList, err := clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		jobList = &batchv1.JobList{} // non-critical, continue without jobs
	}

	// Attach Jobs to their respective agent deployments (and create entries for job-only agents)
	for _, job := range jobList.Items {
		agentName := job.Labels["astro.dev/agent"]
		version := job.Labels["app.kubernetes.io/version"]
		component := job.Labels["app.kubernetes.io/component"]
		if agentName == "" {
			continue
		}

		key := agentName + ":" + version
		info, exists := agentDeployments[key]
		if !exists {
			// Job exists but no Deployment entry — create a stub so it's visible
			info = &AgentDeployment{
				Name:             agentName,
				BuildID:          version,
				Namespace:        namespace,
				Status:           "Running",
				CreatedAt:        job.CreationTimestamp.Format(time.RFC3339),
				Components:       []string{},
				ManualIngestions: manualIngestions,
			}
			agentDeployments[key] = info
		}

		jobDetail := JobDetail{
			Name:      job.Name,
			Component: component,
			Age:       formatAge(job.CreationTimestamp.Time),
		}

		if job.Status.StartTime != nil {
			jobDetail.StartTime = job.Status.StartTime.Format(time.RFC3339)
		}

		// Derive completions string
		desired := int32(1)
		if job.Spec.Completions != nil {
			desired = *job.Spec.Completions
		}
		jobDetail.Completions = fmt.Sprintf("%d/%d", job.Status.Succeeded, desired)

		// Derive status from conditions and counters
		jobDetail.Status = jobStatus(&job)

		info.Jobs = append(info.Jobs, jobDetail)
	}

	// Attach pods to their respective agent deployments
	for _, pod := range podList.Items {
		agentName := pod.Labels["astro.dev/agent"]
		version := pod.Labels["app.kubernetes.io/version"]
		if agentName == "" {
			continue
		}

		key := agentName + ":" + version
		info, exists := agentDeployments[key]
		if !exists {
			continue
		}

		podDetail := PodDetail{
			Name:       pod.Name,
			Phase:      string(pod.Status.Phase),
			PodIP:      pod.Status.PodIP,
			Age:        formatAge(pod.CreationTimestamp.Time),
			Containers: []ContainerStatus{},
		}

		// Build a map of spec containers for env var lookup
		specContainers := map[string][]EnvVar{}
		for _, sc := range pod.Spec.Containers {
			var envVars []EnvVar

			// Collect envFrom sources (ConfigMap/Secret whole-mounts)
			for _, ef := range sc.EnvFrom {
				if ef.ConfigMapRef != nil {
					envVars = append(envVars, EnvVar{
						Name: ef.Prefix + "*",
						From: "configmap:" + ef.ConfigMapRef.Name,
					})
				}
				if ef.SecretRef != nil {
					envVars = append(envVars, EnvVar{
						Name: ef.Prefix + "*",
						From: "secret:" + ef.SecretRef.Name,
					})
				}
			}

			// Collect individual env entries
			for _, e := range sc.Env {
				ev := EnvVar{Name: e.Name}
				if e.ValueFrom != nil {
					switch {
					case e.ValueFrom.SecretKeyRef != nil:
						ev.From = "secret:" + e.ValueFrom.SecretKeyRef.Name + "/" + e.ValueFrom.SecretKeyRef.Key
					case e.ValueFrom.ConfigMapKeyRef != nil:
						ev.From = "configmap:" + e.ValueFrom.ConfigMapKeyRef.Name + "/" + e.ValueFrom.ConfigMapKeyRef.Key
					case e.ValueFrom.FieldRef != nil:
						ev.From = "field:" + e.ValueFrom.FieldRef.FieldPath
					default:
						ev.From = "ref"
					}
				} else {
					ev.Value = e.Value
				}
				envVars = append(envVars, ev)
			}
			specContainers[sc.Name] = envVars
		}

		for _, cs := range pod.Status.ContainerStatuses {
			container := ContainerStatus{
				Name:         cs.Name,
				Ready:        cs.Ready,
				RestartCount: cs.RestartCount,
				Env:          specContainers[cs.Name],
			}
			switch {
			case cs.State.Running != nil:
				container.State = "Running"
			case cs.State.Waiting != nil:
				container.State = "Waiting"
				container.Reason = cs.State.Waiting.Reason
				container.Message = cs.State.Waiting.Message
			case cs.State.Terminated != nil:
				container.State = "Terminated"
				container.Reason = cs.State.Terminated.Reason
				container.Message = cs.State.Terminated.Message
			default:
				container.State = "Unknown"
			}
			podDetail.Containers = append(podDetail.Containers, container)
		}

		info.Pods = append(info.Pods, podDetail)
	}

	// Convert map to slice
	result := make([]AgentDeployment, 0, len(agentDeployments))
	for _, info := range agentDeployments {
		result = append(result, *info)
	}

	return result, nil
}

// formatAge returns a human-readable age string
func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// jobStatus derives a human-readable status from a K8s Job's conditions and counters.
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

// RestartPod deletes a pod in a deployment's namespace, causing Kubernetes to recreate it.
// POST /api/v1/deployments/:namespace/pods/:pod/restart
func RestartPod(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		accountName := c.Query("account")
		if accountName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "account query parameter is required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		if !isAccountMember(c, accountStore, acct.ID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		k8sNamespace := c.Param("namespace")
		if err := verifyNamespaceOwnership(c.Request.Context(), k8sClient, k8sNamespace, acct.ID); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "namespace does not belong to this account"})
			return
		}

		podName := c.Param("pod")
		err = k8sClient.Clientset().CoreV1().Pods(k8sNamespace).Delete(c.Request.Context(), podName, metav1.DeleteOptions{})
		if err != nil {
			log.Error("Failed to delete pod", "error", err, "pod", podName, "namespace", k8sNamespace)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restart pod", "details": err.Error()})
			return
		}

		log.Info("Pod restarted (deleted)", "pod", podName, "namespace", k8sNamespace, "user", user.ID)
		c.JSON(http.StatusOK, gin.H{"status": "restarting", "pod": podName})
	}
}

func GetDeploymentLogs(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		accountName := c.Query("account")
		if accountName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "account query parameter is required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		if !isAccountMember(c, accountStore, acct.ID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		k8sNamespace := c.Param("namespace")
		if err := verifyNamespaceOwnership(c.Request.Context(), k8sClient, k8sNamespace, acct.ID); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "namespace does not belong to this account"})
			return
		}

		podName := c.Query("pod")
		containerName := c.Query("container")

		if podName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pod query parameter is required"})
			return
		}

		tailLines := int64(200)
		if tl := c.Query("tailLines"); tl != "" {
			if parsed, err := strconv.ParseInt(tl, 10, 64); err == nil && parsed > 0 {
				tailLines = parsed
			}
		}

		logOpts := &corev1.PodLogOptions{
			TailLines: &tailLines,
		}
		if containerName != "" {
			logOpts.Container = containerName
		}

		req := k8sClient.Clientset().CoreV1().Pods(k8sNamespace).GetLogs(podName, logOpts)
		stream, err := req.Stream(c.Request.Context())
		if err != nil {
			log.Error("Failed to get pod logs", "error", err, "pod", podName)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to get pod logs",
				"details": err.Error(),
			})
			return
		}
		defer stream.Close() //nolint:errcheck

		logBytes, err := io.ReadAll(stream)
		if err != nil {
			log.Error("Failed to read pod logs", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read pod logs"})
			return
		}

		c.Data(http.StatusOK, "text/plain; charset=utf-8", logBytes)
	}
}

// GetDeploymentTemplate returns a handler for generating deployment spec templates.
// GET /api/v1/agents/:account/:name/deployment-template
// Optional query: ?build=<build_id>
func GetDeploymentTemplate(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		name := c.Param("name")

		log.Info("Generating deployment template",
			"account", accountName,
			"name", name,
		)

		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		accountID := acct.ID

		if !isAccountMember(c, accountStore, accountID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
			return
		}

		// Resolve build — specific build_id or latest
		var agentVersion *agentindex.AgentVersion
		if buildParam := c.Query("build"); buildParam != "" {
			agentVersion, err = agentIndex.GetVersion(accountID, name, buildParam)
		} else {
			agentVersion, err = agentIndex.GetLatestVersion(accountID, name)
		}
		if err != nil {
			log.Error("Failed to get agent build", "error", err)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "no builds found for agent",
				"details": err.Error(),
			})
			return
		}

		// Parse spec from stored map
		specBytes, err := json.Marshal(agentVersion.Spec)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process spec"})
			return
		}
		var astroSpec spec.AstroSpec
		if err := json.Unmarshal(specBytes, &astroSpec); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse spec"})
			return
		}

		// Generate deployment template
		template, err := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
			Spec:              &astroSpec,
			Account:           accountName,
			BuildID:           agentVersion.BuildID,
			RegistryURL:       cfg.Deployment.RegistryURL,
			ProxyRegistryHost: cfg.Deployment.ProxyRegistryHost,
			Environment:       cfg.Deployment.Environment,
		})
		if err != nil {
			log.Error("Failed to generate deployment template", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to generate deployment template",
				"details": err.Error(),
			})
			return
		}

		// Namespace is generated at deploy time from the deployment UUID
		template.Target.Namespace = "<generated-on-deploy>"

		// Return JSON or YAML based on format query param
		if c.Query("format") == "json" {
			c.JSON(http.StatusOK, template)
			return
		}

		// Default: serialize to YAML
		yamlBytes, err := spec.SerializeDeploymentSpec(template)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize template"})
			return
		}

		c.Data(http.StatusOK, "application/yaml", yamlBytes)
	}
}

// GetActiveDeploymentSpec returns the stored deployment spec for the currently active deployment.
// GET /api/v1/agents/:account/:name/deployment
func GetActiveDeploymentSpec(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		agentName := c.Param("name")

		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		if !isAccountMember(c, accountStore, acct.ID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
			return
		}

		d, err := deployStore.GetActiveDeployment(acct.ID, agentName)
		if err != nil {
			log.Error("Failed to get active deployment", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get deployment"})
			return
		}
		if d == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "no active deployment found"})
			return
		}

		var specObj json.RawMessage
		if err := json.Unmarshal([]byte(d.DeploymentSpecJSON), &specObj); err != nil {
			log.Error("Failed to parse stored deployment spec", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse stored spec"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"id":          d.ID,
			"agent_name":  d.AgentName,
			"build_id":    d.BuildID,
			"namespace":   d.Namespace,
			"status":      d.Status,
			"deployed_at": d.DeployedAt,
			"spec":        specObj,
		})
	}
}

// GetDeploymentHistory returns all deployment records for an agent.
// GET /api/v1/agents/:account/:name/deployment/history
func GetDeploymentHistory(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		agentName := c.Param("name")

		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		if !isAccountMember(c, accountStore, acct.ID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
			return
		}

		history, err := deployStore.GetDeploymentHistory(acct.ID, agentName)
		if err != nil {
			log.Error("Failed to get deployment history", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get deployment history"})
			return
		}

		type deploymentRecord struct {
			ID           string          `json:"id"`
			AgentName    string          `json:"agent_name"`
			BuildID      string          `json:"build_id"`
			Namespace    string          `json:"namespace"`
			Status       string          `json:"status"`
			DeployedAt   time.Time       `json:"deployed_at"`
			UndeployedAt *time.Time      `json:"undeployed_at,omitempty"`
			Spec         json.RawMessage `json:"spec"`
		}

		records := make([]deploymentRecord, 0, len(history))
		for _, d := range history {
			var specObj json.RawMessage
			if err := json.Unmarshal([]byte(d.DeploymentSpecJSON), &specObj); err != nil {
				log.Warn("Failed to parse stored spec, skipping", "deployment_id", d.ID, "error", err)
				specObj = json.RawMessage(`{}`)
			}
			records = append(records, deploymentRecord{
				ID:           d.ID,
				AgentName:    d.AgentName,
				BuildID:      d.BuildID,
				Namespace:    d.Namespace,
				Status:       d.Status,
				DeployedAt:   d.DeployedAt,
				UndeployedAt: d.UndeployedAt,
				Spec:         specObj,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"deployments": records,
			"count":       len(records),
		})
	}
}

// GetConfigMapData returns the key-value data of a ConfigMap in a deployment's namespace.
// GET /api/v1/deployments/:namespace/configmap/:cmname
func GetConfigMapData(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		accountName := c.Query("account")
		if accountName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "account query parameter is required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		if !isAccountMember(c, accountStore, acct.ID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		k8sNamespace := c.Param("namespace")
		if err := verifyNamespaceOwnership(c.Request.Context(), k8sClient, k8sNamespace, acct.ID); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "namespace does not belong to this account"})
			return
		}

		cmName := c.Param("cmname")
		cm, err := k8sClient.Clientset().CoreV1().ConfigMaps(k8sNamespace).Get(c.Request.Context(), cmName, metav1.GetOptions{})
		if err != nil {
			log.Error("Failed to get configmap", "error", err, "configmap", cmName, "namespace", k8sNamespace)
			c.JSON(http.StatusNotFound, gin.H{"error": "configmap not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"name":      cm.Name,
			"namespace": cm.Namespace,
			"data":      cm.Data,
		})
	}
}

// GetSecretKeys returns the key names (but NOT values) of a Secret in a deployment's namespace.
// GET /api/v1/deployments/:name/:build_id/secret/:secretname/keys
func GetSecretKeys(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		accountName := c.Query("account")
		if accountName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "account query parameter is required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		if !isAccountMember(c, accountStore, acct.ID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		k8sNamespace := c.Param("namespace")
		if err := verifyNamespaceOwnership(c.Request.Context(), k8sClient, k8sNamespace, acct.ID); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "namespace does not belong to this account"})
			return
		}

		secretName := c.Param("secretname")
		secret, err := k8sClient.Clientset().CoreV1().Secrets(k8sNamespace).Get(c.Request.Context(), secretName, metav1.GetOptions{})
		if err != nil {
			log.Error("Failed to get secret", "error", err, "secret", secretName, "namespace", k8sNamespace)
			c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
			return
		}

		keys := make([]string, 0, len(secret.Data))
		for k := range secret.Data {
			keys = append(keys, k)
		}

		c.JSON(http.StatusOK, gin.H{
			"name":      secret.Name,
			"namespace": secret.Namespace,
			"keys":      keys,
		})
	}
}

// toValidationErrors converts a slice of "field: message" error strings into
// structured {field, message} objects expected by the frontend.
func toValidationErrors(errs []string) []gin.H {
	out := make([]gin.H, 0, len(errs))
	for _, e := range errs {
		field, message, _ := strings.Cut(e, ": ")
		if message == "" {
			field, message = "", e
		}
		out = append(out, gin.H{"field": field, "message": message})
	}
	return out
}

// imagePullPolicyForMode returns PullNever for local mode (images must be
// built locally and available in the cluster), PullAlways otherwise.
func imagePullPolicyForMode(mode string) corev1.PullPolicy {
	if mode == "local" {
		return corev1.PullNever
	}
	return corev1.PullAlways
}
