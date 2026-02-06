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
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/config"
	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/apps/astro-server/internal/k8s"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/middleware"
	"github.com/postman/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		applier := k8s.NewApplier(k8sClient, k8s.ApplierConfig{
			Namespace:         k8sNamespace,
			RegistryURL:       cfg.Deployment.RegistryURL,
			ProxyRegistryHost: cfg.Deployment.ProxyRegistryHost,
			IngressDomain:     cfg.Deployment.IngressDomain,
			ACMCertificateARN: cfg.Deployment.ACMCertificateARN,
			ALBGroupName:      cfg.Deployment.ALBGroupName,
		})
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

// ServiceEndpointInfo represents a service endpoint for a deployment
type ServiceEndpointInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ContainerStatus represents the status of a single container in a pod
type ContainerStatus struct {
	Name         string `json:"name"`
	State        string `json:"state"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
}

// PodDetail represents details about a single pod
type PodDetail struct {
	Name       string            `json:"name"`
	Phase      string            `json:"phase"`
	PodIP      string            `json:"pod_ip,omitempty"`
	Age        string            `json:"age"`
	Containers []ContainerStatus `json:"containers"`
}

// AgentDeployment represents information about a deployed agent
type AgentDeployment struct {
	Name            string               `json:"name"`
	Version         string               `json:"version"`
	Status          string               `json:"status"`
	Replicas        int32                `json:"replicas"`
	Ready           int32                `json:"ready"`
	CreatedAt       string               `json:"created_at"`
	Components      []string             `json:"components"`
	ServiceEndpoint *ServiceEndpointInfo `json:"service_endpoint,omitempty"`
	ExternalURL     string               `json:"external_url,omitempty"`
	Pods            []PodDetail          `json:"pods,omitempty"`
}

// ListDeployments returns a handler for listing deployed agents
func ListDeployments(log *logger.Logger, cfg *config.Config) gin.HandlerFunc {
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

		// Derive namespace from user ID
		k8sNamespace := sanitizeNamespace(user.ID)

		log.Info("Listing deployments",
			"k8s_namespace", k8sNamespace,
			"user_id", user.ID,
		)

		// Initialize EKS client
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

		// List deployments with astro label
		deployments, err := listAstroDeployments(c.Request.Context(), k8sClient, k8sNamespace)
		if err != nil {
			log.Error("Failed to list deployments", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to list deployments",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"deployments": deployments,
			"count":       len(deployments),
			"namespace":   k8sNamespace,
		})
	}
}

// listAstroDeployments lists all deployments managed by astro in a namespace
func listAstroDeployments(ctx context.Context, k8sClient *k8s.EKSClient, namespace string) ([]AgentDeployment, error) {
	clientset := k8sClient.Clientset()

	// List deployments with astro label selector
	labelSelector := "app.kubernetes.io/managed-by=astro-server"
	deploymentList, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	// List services for the namespace to get endpoints
	serviceList, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
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

	// Build a map of agent services (only agent component services)
	agentServices := make(map[string]string) // key: "agentName:version" -> URL
	for _, svc := range serviceList.Items {
		agentName := svc.Labels["astro.dev/agent"]
		version := svc.Labels["app.kubernetes.io/version"]
		component := svc.Labels["app.kubernetes.io/component"]

		// Only include the main agent service endpoint
		if agentName != "" && component == "agent" {
			key := agentName + ":" + version
			// Build the cluster DNS URL
			port := int32(8080)
			for _, p := range svc.Spec.Ports {
				if p.Name == "http" || p.Port == 8080 {
					port = p.Port
					break
				}
			}
			agentServices[key] = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", svc.Name, namespace, port)
		}
	}

	// Build a map of external URLs from ingresses
	agentExternalURLs := make(map[string]string) // key: "agentName:version" -> external URL
	if ingressList != nil {
		for _, ing := range ingressList.Items {
			agentName := ing.Labels["astro.dev/agent"]
			version := ing.Labels["app.kubernetes.io/version"]

			if agentName != "" && len(ing.Spec.Rules) > 0 {
				key := agentName + ":" + version
				host := ing.Spec.Rules[0].Host
				if host != "" {
					agentExternalURLs[key] = fmt.Sprintf("https://%s", host)
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
				Name:       agentName,
				Version:    version,
				Status:     status,
				Replicas:   dep.Status.Replicas,
				Ready:      dep.Status.ReadyReplicas,
				CreatedAt:  dep.CreationTimestamp.Format(time.RFC3339),
				Components: []string{},
			}

			// Add service endpoint if available
			if url, ok := agentServices[key]; ok {
				info.ServiceEndpoint = &ServiceEndpointInfo{
					Name: "agent",
					URL:  url,
				}
			}

			// Add external URL if available
			if url, ok := agentExternalURLs[key]; ok {
				info.ExternalURL = url
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

		for _, cs := range pod.Status.ContainerStatuses {
			container := ContainerStatus{
				Name:         cs.Name,
				Ready:        cs.Ready,
				RestartCount: cs.RestartCount,
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

// GetDeploymentLogs returns a handler for fetching pod logs
func GetDeploymentLogs(log *logger.Logger, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get authenticated user from context
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		k8sNamespace := sanitizeNamespace(user.ID)
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

		// Initialize EKS client
		k8sClient, err := k8s.NewEKSClient(c.Request.Context(), k8s.EKSClientConfig{
			ClusterName:     cfg.Deployment.EKSClusterName,
			ClusterEndpoint: cfg.Deployment.K8sMasterURL,
			Region:          cfg.Deployment.AWSRegion,
			Logger:          log,
		})
		if err != nil {
			log.Error("Failed to create EKS client", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to EKS cluster"})
			return
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
		defer stream.Close()

		logBytes, err := io.ReadAll(stream)
		if err != nil {
			log.Error("Failed to read pod logs", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read pod logs"})
			return
		}

		c.Data(http.StatusOK, "text/plain; charset=utf-8", logBytes)
	}
}
