package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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

// isAccountMember checks whether the user has access to the account.
// Admin users (authenticated via admin Basic Auth) bypass the membership check.
func isAccountMember(c *gin.Context, accountStore *account.AccountStore, acctID, userID string) bool {
	if middleware.IsAdmin(c) {
		return true
	}
	isMember, err := accountStore.IsMember(acctID, userID)
	return err == nil && isMember
}

// deploymentNamespace derives a deterministic K8s namespace from a deployment's
// identity. The namespace is stable across builds so that persistent data
// (volumes, secrets, configmaps) survives redeploys of the same agent.
func deploymentNamespace(accountID, sourceAccount, agentName string) string {
	h := sha256.Sum256([]byte(accountID + ":" + sourceAccount + ":" + agentName))
	return "astro-" + hex.EncodeToString(h[:])[:20]
}

// resolveDeploymentNamespace finds the K8s namespace for a deployment by
// first checking the DB record, then falling back to K8s label lookup.
func resolveDeploymentNamespace(ctx context.Context, deployStore *deploymentstore.Store, k8sClient k8s.ClusterClient, accountID, agentName string) (string, error) {
	// Try DB first
	if deployStore != nil {
		dep, err := deployStore.GetActiveDeployment(accountID, agentName)
		if err != nil {
			return "", fmt.Errorf("failed to look up deployment: %w", err)
		}
		if dep != nil {
			return dep.Namespace, nil
		}
	}

	// Fallback: find namespace from K8s labels
	if k8sClient != nil {
		selector := fmt.Sprintf("astro.dev/account-id=%s,astro.dev/agent=%s,app.kubernetes.io/managed-by=astro-server", accountID, agentName)
		nsList, err := k8sClient.Clientset().CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err == nil && len(nsList.Items) > 0 {
			return nsList.Items[0].Name, nil
		}
	}

	return "", fmt.Errorf("no active deployment found for agent %q", agentName)
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
func DeployAgent(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req deployment.DeployRequest

		// Parse request body
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Error("Failed to parse deploy request", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request",
				"details": err.Error(),
			})
			return
		}

		// Get authenticated user from context
		user, exists := middleware.GetUser(c)
		if !exists {
			log.Error("User not found in context")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			return
		}

		// Resolve account
		if req.Account == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "account is required"})
			return
		}

		acct, err := accountStore.GetByName(req.Account)
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

		log.Info("Received deploy request",
			"name", req.Name,
			"account", req.Account,
			"source_account", req.SourceAccount,
			"user_id", user.ID,
		)

		// Resolve latest build — own account uses latest build, cross-account uses latest published
		isCrossAccount := req.SourceAccount != "" && req.SourceAccount != req.Account
		var agentVersion *agentindex.AgentVersion

		if isCrossAccount {
			sourceAcct, err := accountStore.GetByName(req.SourceAccount)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "source account not found"})
				return
			}

			agentVersion, err = agentIndex.GetLatestPublishedVersion(sourceAcct.ID, req.Name)
			if err != nil {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "no published version available; cross-account deploys require a published version",
				})
				return
			}
		} else {
			agentVersion, err = agentIndex.GetLatestVersion(acct.ID, req.Name)
			if err != nil {
				log.Error("Agent not found", "error", err)
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "agent not found",
					"details": fmt.Sprintf("no builds found for %s", req.Name),
				})
				return
			}
		}

		buildID := agentVersion.BuildID

		sourceAccount := req.Account
		if req.SourceAccount != "" {
			sourceAccount = req.SourceAccount
		}

		// Derive per-deployment namespace (stable across builds)
		k8sNamespace := deploymentNamespace(acct.ID, sourceAccount, req.Name)

		// Parse spec from JSON
		var astroSpec spec.AstroSpec
		specBytes, err := json.Marshal(agentVersion.Spec)
		if err != nil {
			log.Error("Failed to marshal spec", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to process spec",
				"details": err.Error(),
			})
			return
		}

		if err := json.Unmarshal(specBytes, &astroSpec); err != nil {
			log.Error("Failed to unmarshal spec", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to parse spec",
				"details": err.Error(),
			})
			return
		}

		// Generate deployment spec from astro spec
		deploySpec, err := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
			Spec:              &astroSpec,
			Account:           req.Account,
			BuildID:           buildID,
			RegistryURL:       cfg.Deployment.RegistryURL,
			ProxyRegistryHost: cfg.Deployment.ProxyRegistryHost,
			Environment:       cfg.Deployment.Environment,
		})
		if err != nil {
			log.Error("Failed to generate deployment spec", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to generate deployment spec",
				"details": err.Error(),
			})
			return
		}

		// Fill in user-provided credentials
		for key, value := range req.UserCredentials {
			if cred, ok := deploySpec.Credentials[key]; ok {
				cred.Value = value
				deploySpec.Credentials[key] = cred
			} else {
				// Add credential not in template (e.g. user-supplied extras)
				deploySpec.Credentials[key] = spec.DeploymentCredential{Value: value}
			}
		}

		// Fill in interfaces from deploy request
		if len(req.Interfaces) > 0 && deploySpec.Interfaces != nil {
			deploySpec.Interfaces.Adapters = req.Interfaces
		} else if len(req.Interfaces) > 0 {
			deploySpec.Interfaces = &spec.DeploymentInterfaces{
				Adapters: req.Interfaces,
			}
		}

		// Fill in schedules from deploy request
		for name, schedule := range req.Schedules {
			if ing, ok := deploySpec.Ingestion[name]; ok {
				ing.Trigger.Schedule = schedule
				deploySpec.Ingestion[name] = ing
			}
		}

		// Set target namespace
		deploySpec.Target.Namespace = k8sNamespace

		// Validate and resolve the filled-in deployment spec
		resolveResult, err := deployment.ValidateAndResolve(deploySpec)
		if err != nil {
			log.Error("Deployment spec resolution failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to resolve deployment spec",
				"details": err.Error(),
			})
			return
		}

		if len(resolveResult.Errors) > 0 {
			log.Error("Deployment spec validation failed", "errors", resolveResult.Errors)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "deployment spec validation failed",
				"validation_errors": resolveResult.Errors,
			})
			return
		}

		log.Info("Deploying to Kubernetes",
			"k8s_namespace", k8sNamespace,
		)

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "kubernetes client not configured",
			})
			return
		}

		// Apply deployment spec to cluster
		applier := k8s.NewApplier(k8sClient, k8s.ApplierConfig{
			Namespace:              k8sNamespace,
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
				"astro.dev/account-id":      acct.ID,
				"astro.dev/account":         req.Account,
				"astro.dev/agent":           req.Name,
				"astro.dev/build":           buildID,
				"astro.dev/source-account": sourceAccount,
			},
		})
		applyResult, err := applier.ApplyDeploymentSpec(
			c.Request.Context(),
			resolveResult.Spec,
		)

		if err != nil {
			log.Error("Deployment failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "deployment failed",
				"details": err.Error(),
			})
			return
		}

		// Persist resolved spec (credentials stripped)
		if deployStore != nil {
			stripped := spec.StripCredentialValues(resolveResult.Spec)
			specJSON, marshalErr := json.Marshal(stripped)
			if marshalErr != nil {
				log.Error("Failed to marshal stripped spec for storage", "error", marshalErr)
			} else if _, storeErr := deployStore.SaveDeployment(acct.ID, req.Name, buildID, k8sNamespace, string(specJSON)); storeErr != nil {
				log.Error("Failed to save deployment record", "error", storeErr)
			}
		}

		// Check if there were any errors during deployment
		status := "success"
		statusCode := http.StatusOK
		if len(applyResult.Errors) > 0 {
			status = "partial"
			statusCode = http.StatusMultiStatus
			log.Warn("Deployment completed with errors", "error_count", len(applyResult.Errors))
		}

		// Build response
		response := deployment.DeployResponse{
			Status:           status,
			Name:             req.Name,
			BuildID:          buildID,
			K8sNamespace:     k8sNamespace,
			DeployedAt:       time.Now().UTC(),
			Resources:        applyResult.Resources,
			ServiceEndpoints: applyResult.ServiceEndpoints,
			Errors:           applyResult.Errors,
		}

		log.Info("Deployment completed",
			"status", status,
			"resources", len(applyResult.Resources),
			"errors", len(applyResult.Errors),
		)

		c.JSON(statusCode, response)
	}
}

// ValidateDeployment returns a handler that validates a deployment without applying it.
// POST /api/v1/deploy/validate
func ValidateDeployment(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req deployment.DeployRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
			return
		}

		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		if req.Account == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "account is required"})
			return
		}

		acct, err := accountStore.GetByName(req.Account)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		isMember, err := accountStore.IsMember(acct.ID, user.ID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
			return
		}

		log.Info("Validating deployment",
			"name", req.Name,
			"account", req.Account,
			"user_id", user.ID,
		)

		// Resolve latest build
		isCrossAccount := req.SourceAccount != "" && req.SourceAccount != req.Account
		var agentVersion *agentindex.AgentVersion

		if isCrossAccount {
			sourceAcct, err := accountStore.GetByName(req.SourceAccount)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "source account not found"})
				return
			}
			agentVersion, err = agentIndex.GetLatestPublishedVersion(sourceAcct.ID, req.Name)
			if err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "no published version available"})
				return
			}
		} else {
			agentVersion, err = agentIndex.GetLatestVersion(acct.ID, req.Name)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "agent not found", "details": fmt.Sprintf("no builds found for %s", req.Name)})
				return
			}
		}

		// Derive per-deployment namespace
		sourceAccount := req.Account
		if req.SourceAccount != "" {
			sourceAccount = req.SourceAccount
		}
		k8sNamespace := deploymentNamespace(acct.ID, sourceAccount, req.Name)

		// Parse spec
		var astroSpec spec.AstroSpec
		specBytes, err := json.Marshal(agentVersion.Spec)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process spec"})
			return
		}
		if err := json.Unmarshal(specBytes, &astroSpec); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse spec"})
			return
		}

		// Generate deployment spec
		deploySpec, err := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
			Spec:              &astroSpec,
			Account:           req.Account,
			BuildID:           agentVersion.BuildID,
			RegistryURL:       cfg.Deployment.RegistryURL,
			ProxyRegistryHost: cfg.Deployment.ProxyRegistryHost,
			Environment:       cfg.Deployment.Environment,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate deployment spec", "details": err.Error()})
			return
		}

		// Fill in user-provided credentials
		for key, value := range req.UserCredentials {
			if cred, ok := deploySpec.Credentials[key]; ok {
				cred.Value = value
				deploySpec.Credentials[key] = cred
			} else {
				deploySpec.Credentials[key] = spec.DeploymentCredential{Value: value}
			}
		}

		// Fill in interfaces
		if len(req.Interfaces) > 0 && deploySpec.Interfaces != nil {
			deploySpec.Interfaces.Adapters = req.Interfaces
		} else if len(req.Interfaces) > 0 {
			deploySpec.Interfaces = &spec.DeploymentInterfaces{
				Adapters: req.Interfaces,
			}
		}

		// Fill in schedules
		for name, schedule := range req.Schedules {
			if ing, ok := deploySpec.Ingestion[name]; ok {
				ing.Trigger.Schedule = schedule
				deploySpec.Ingestion[name] = ing
			}
		}

		deploySpec.Target.Namespace = k8sNamespace

		// Validate and resolve
		resolveResult, err := deployment.ValidateAndResolve(deploySpec)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve deployment spec", "details": err.Error()})
			return
		}

		if len(resolveResult.Errors) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"valid":             false,
				"validation_errors": resolveResult.Errors,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"valid":    true,
			"build_id": agentVersion.BuildID,
			"name":     req.Name,
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
			log.Error("User not found in context")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			return
		}

		// Resolve account
		if req.Account == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "account is required"})
			return
		}

		acct, err := accountStore.GetByName(req.Account)
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

		log.Info("Received undeploy request",
			"name", req.Name,
			"account", req.Account,
			"user_id", user.ID,
		)

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "kubernetes client not configured",
			})
			return
		}

		// Look up active deployment — try DB first, then K8s fallback
		var k8sNamespace, buildID string
		if deployStore != nil {
			dep, dbErr := deployStore.GetActiveDeployment(acct.ID, req.Name)
			if dbErr != nil {
				log.Warn("Failed to look up deployment from DB", "error", dbErr)
			}
			if dep != nil {
				k8sNamespace = dep.Namespace
				buildID = dep.BuildID
			}
		}

		if k8sNamespace == "" {
			ns, nsErr := resolveDeploymentNamespace(c.Request.Context(), nil, k8sClient, acct.ID, req.Name)
			if nsErr != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "no active deployment found for this agent"})
				return
			}
			k8sNamespace = ns
		}

		// Delete the entire namespace — cascades to all resources
		err = k8sClient.Clientset().CoreV1().Namespaces().Delete(
			c.Request.Context(), k8sNamespace, metav1.DeleteOptions{},
		)
		if err != nil {
			log.Error("Failed to delete namespace", "error", err, "namespace", k8sNamespace)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "undeploy failed",
				"details": err.Error(),
			})
			return
		}

		// Mark deployment record as undeployed
		if err := deployStore.MarkUndeployed(acct.ID, req.Name); err != nil {
			log.Warn("Failed to mark deployment as undeployed", "error", err)
		}

		response := deployment.UndeployResponse{
			Status:       "success",
			Name:         req.Name,
			BuildID:      buildID,
			K8sNamespace: k8sNamespace,
			UndeployedAt: time.Now().UTC(),
		}

		log.Info("Undeploy completed",
			"status", "success",
			"namespace", k8sNamespace,
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
	Status      string `json:"status"`      // "Running", "Succeeded", "Failed", "Pending"
	Component   string `json:"component"`
	Age         string `json:"age"`
	StartTime   string `json:"start_time,omitempty"`
	Completions string `json:"completions"` // "1/1", "0/1"
}

// AgentDeployment represents information about a deployed agent
type AgentDeployment struct {
	Name              string               `json:"name"`
	BuildID           string               `json:"build_id"`
	Namespace         string               `json:"namespace"`
	Status            string               `json:"status"`
	Replicas          int32                `json:"replicas"`
	Ready             int32                `json:"ready"`
	CreatedAt         string               `json:"created_at"`
	Components        []string             `json:"components"`
	ManualIngestions  []string             `json:"manual_ingestions,omitempty"`
	ExternalURLs      []ServiceEndpointInfo `json:"external_urls,omitempty"`
	Pods              []PodDetail          `json:"pods,omitempty"`
	Jobs              []JobDetail          `json:"jobs,omitempty"`
}

// ListDeployments returns a handler for listing deployed agents
func ListDeployments(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient) gin.HandlerFunc {
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

		// Aggregate deployments across all per-deployment namespaces
		var allDeployments []AgentDeployment
		for _, ns := range nsList.Items {
			manualIngestions := parseManualIngestions(ns.Annotations)
			deps, err := listAstroDeployments(c.Request.Context(), k8sClient, ns.Name, manualIngestions)
			if err != nil {
				log.Warn("Failed to list deployments in namespace", "namespace", ns.Name, "error", err)
				continue
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

		var accountID string
		if accountStore != nil && accountName != "" {
			acct, err := accountStore.GetByName(accountName)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
				return
			}
			accountID = acct.ID
		}

		// Resolve build — specific build_id or latest
		var agentVersion *agentindex.AgentVersion
		var err error
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

		// Set target namespace so the template preview shows it
		template.Target.Namespace = deploymentNamespace(accountID, accountName, name)

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

// imagePullPolicyForMode returns PullNever for local mode (images must be
// built locally and available in the cluster), PullAlways otherwise.
func imagePullPolicyForMode(mode string) corev1.PullPolicy {
	if mode == "local" {
		return corev1.PullNever
	}
	return corev1.PullAlways
}
