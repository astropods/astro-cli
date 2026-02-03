package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/config"
	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/apps/astro-server/internal/k8s"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/middleware"
	"github.com/postman/astro/packages/astro-spec"
)

// sanitizeNamespace converts a user ID to a valid Kubernetes namespace name
// Kubernetes namespace names must:
// - contain only lowercase alphanumeric characters or '-'
// - start and end with an alphanumeric character
// - be at most 63 characters long
func sanitizeNamespace(userID string) string {
	// Convert to lowercase
	ns := strings.ToLower(userID)

	// Replace any non-alphanumeric characters (except hyphens) with hyphens
	var sanitized strings.Builder
	for _, ch := range ns {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			sanitized.WriteRune(ch)
		} else {
			sanitized.WriteRune('-')
		}
	}

	result := sanitized.String()

	// Trim leading/trailing hyphens
	result = strings.Trim(result, "-")

	// Ensure it starts with alphanumeric if it doesn't
	if len(result) == 0 || result[0] == '-' {
		result = "user-" + result
	}

	// Truncate to 63 characters if needed
	if len(result) > 63 {
		result = result[:63]
	}

	// Ensure it ends with alphanumeric
	result = strings.TrimRight(result, "-")

	return result
}

// DeployAgent returns a handler for deploying agents to Kubernetes
func DeployAgent(log *logger.Logger, agentIndex *agentindex.Index, cfg *config.Config) gin.HandlerFunc {
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

		// Derive namespace from user ID (use user ID as namespace, with sanitization)
		k8sNamespace := sanitizeNamespace(user.ID)

		log.Info("Received deploy request",
			"name", req.Name,
			"version", req.Version,
			"k8s_namespace", k8sNamespace,
			"user_id", user.ID,
		)

		// Fetch agent spec from index
		agentVersion, err := agentIndex.GetVersion(req.Name, req.Version)
		if err != nil {
			log.Error("Agent version not found", "error", err)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "agent version not found",
				"details": fmt.Sprintf("%s:%s not found in index", req.Name, req.Version),
			})
			return
		}

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

		// Validate spec and credentials
		validator := deployment.NewValidator()
		validationResult := validator.ValidateSpec(&astroSpec, req.UserCredentials)

		if !validationResult.Valid {
			log.Error("Spec validation failed", "errors", validationResult.Errors)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":               "spec validation failed",
				"validation_errors":   validationResult.Errors,
				"missing_credentials": validationResult.MissingCredentials,
			})
			return
		}

		log.Info("Deploying to Kubernetes",
			"k8s_namespace", k8sNamespace,
		)

		// Translate spec to K8s manifests
		translator := deployment.NewTranslator(
			req.Name,
			req.Version,
			k8sNamespace,
			cfg.Deployment.RegistryURL,
			req.UserCredentials,
		)

		translationResult, err := translator.Translate(&astroSpec)
		if err != nil {
			log.Error("Failed to translate spec", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to translate spec to Kubernetes manifests",
				"details": err.Error(),
			})
			return
		}

		// Initialize EKS client for managed cluster
		k8sClient, err := k8s.NewEKSClient(c.Request.Context(), k8s.EKSClientConfig{
			ClusterName:     cfg.Deployment.EKSClusterName,
			ClusterEndpoint: cfg.Deployment.K8sMasterURL,
			Region:          cfg.Deployment.AWSRegion,
			Logger:          log,
		})
		if err != nil {
			log.Error("Failed to create EKS client", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to connect to EKS cluster",
				"details": err.Error(),
			})
			return
		}

		// Apply manifests to cluster
		applier := k8s.NewApplier(k8sClient, k8sNamespace, cfg.Deployment.RegistryURL, cfg.Deployment.ProxyRegistryHost)
		applyResult, err := applier.Apply(
			c.Request.Context(),
			&astroSpec,
			translationResult,
			req.Name,
			req.Version,
			req.UserCredentials,
		)

		if err != nil {
			log.Error("Deployment failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "deployment failed",
				"details": err.Error(),
			})
			return
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
			Version:          req.Version,
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

// UndeployAgent returns a handler for undeploying agents from Kubernetes
func UndeployAgent(log *logger.Logger, agentIndex *agentindex.Index, cfg *config.Config) gin.HandlerFunc {
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

		// Derive namespace from user ID (use user ID as namespace, with sanitization)
		k8sNamespace := sanitizeNamespace(user.ID)

		log.Info("Received undeploy request",
			"name", req.Name,
			"version", req.Version,
			"k8s_namespace", k8sNamespace,
			"user_id", user.ID,
		)

		log.Info("Undeploying from Kubernetes",
			"k8s_namespace", k8sNamespace,
		)

		// Initialize EKS client for managed cluster
		k8sClient, err := k8s.NewEKSClient(c.Request.Context(), k8s.EKSClientConfig{
			ClusterName:     cfg.Deployment.EKSClusterName,
			ClusterEndpoint: cfg.Deployment.K8sMasterURL,
			Region:          cfg.Deployment.AWSRegion,
			Logger:          log,
		})
		if err != nil {
			log.Error("Failed to create EKS client", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to connect to EKS cluster",
				"details": err.Error(),
			})
			return
		}

		// Delete resources from cluster
		deleter := k8s.NewDeleter(k8sClient, k8sNamespace)
		deleteResult, err := deleter.Delete(
			c.Request.Context(),
			req.Name,
			req.Version,
		)

		if err != nil {
			log.Error("Undeploy failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "undeploy failed",
				"details": err.Error(),
			})
			return
		}

		// Check if there were any errors during undeploy
		status := "success"
		statusCode := http.StatusOK
		if len(deleteResult.Errors) > 0 {
			status = "partial"
			statusCode = http.StatusMultiStatus
			log.Warn("Undeploy completed with errors", "error_count", len(deleteResult.Errors))
		}

		// Build response
		response := deployment.UndeployResponse{
			Status:       status,
			Name:         req.Name,
			Version:      req.Version,
			K8sNamespace: k8sNamespace,
			UndeployedAt: time.Now().UTC(),
			Resources:    deleteResult.Resources,
			Errors:       deleteResult.Errors,
		}

		log.Info("Undeploy completed",
			"status", status,
			"resources", len(deleteResult.Resources),
			"errors", len(deleteResult.Errors),
		)

		c.JSON(statusCode, response)
	}
}
