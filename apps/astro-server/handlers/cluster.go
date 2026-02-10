package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/k8s"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterStatusResponse contains cluster resource information
type ClusterStatusResponse struct {
	Timestamp   string                 `json:"timestamp"`
	Namespace   string                 `json:"namespace"`
	Deployments []DeploymentInfo       `json:"deployments"`
	Pods        []PodInfo              `json:"pods"`
	Services    []ServiceInfo          `json:"services"`
	Summary     ClusterSummary         `json:"summary"`
}

// ClusterSummary provides counts of resources
type ClusterSummary struct {
	TotalDeployments int `json:"total_deployments"`
	TotalPods        int `json:"total_pods"`
	RunningPods      int `json:"running_pods"`
	PendingPods      int `json:"pending_pods"`
	FailedPods       int `json:"failed_pods"`
	TotalServices    int `json:"total_services"`
}

// DeploymentInfo contains deployment details
type DeploymentInfo struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Replicas          int32             `json:"replicas"`
	ReadyReplicas     int32             `json:"ready_replicas"`
	AvailableReplicas int32             `json:"available_replicas"`
	Labels            map[string]string `json:"labels,omitempty"`
	CreatedAt         string            `json:"created_at"`
}

// PodInfo contains pod details
type PodInfo struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Phase     string            `json:"phase"`
	NodeName  string            `json:"node_name,omitempty"`
	PodIP     string            `json:"pod_ip,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt string            `json:"created_at"`
}

// ServiceInfo contains service details
type ServiceInfo struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Type       string            `json:"type"`
	ClusterIP  string            `json:"cluster_ip,omitempty"`
	ExternalIP []string          `json:"external_ip,omitempty"`
	Ports      []ServicePort     `json:"ports,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	CreatedAt  string            `json:"created_at"`
}

// ServicePort contains port information
type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port"`
	Protocol   string `json:"protocol"`
}

// ClusterStatus returns a handler that lists cluster resources
func ClusterStatus(log *logger.Logger, k8sClient k8s.ClusterClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "kubernetes client not configured",
			})
			return
		}

		// Get namespace from query param, default to all namespaces
		namespace := c.Query("namespace")
		if namespace == "" {
			namespace = "" // empty string means all namespaces for k8s client
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		clientset := k8sClient.Clientset()
		response := ClusterStatusResponse{
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Namespace:   namespace,
			Deployments: []DeploymentInfo{},
			Pods:        []PodInfo{},
			Services:    []ServiceInfo{},
		}

		// List deployments
		deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Error("Failed to list deployments", "error", err, "namespace", namespace)
		} else {
			for _, d := range deployments.Items {
				response.Deployments = append(response.Deployments, DeploymentInfo{
					Name:              d.Name,
					Namespace:         d.Namespace,
					Replicas:          *d.Spec.Replicas,
					ReadyReplicas:     d.Status.ReadyReplicas,
					AvailableReplicas: d.Status.AvailableReplicas,
					Labels:            d.Labels,
					CreatedAt:         d.CreationTimestamp.Format(time.RFC3339),
				})
			}
		}

		// List pods
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Error("Failed to list pods", "error", err, "namespace", namespace)
		} else {
			for _, p := range pods.Items {
				response.Pods = append(response.Pods, PodInfo{
					Name:      p.Name,
					Namespace: p.Namespace,
					Phase:     string(p.Status.Phase),
					NodeName:  p.Spec.NodeName,
					PodIP:     p.Status.PodIP,
					Labels:    p.Labels,
					CreatedAt: p.CreationTimestamp.Format(time.RFC3339),
				})
			}
		}

		// List services
		services, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Error("Failed to list services", "error", err, "namespace", namespace)
		} else {
			for _, s := range services.Items {
				svcInfo := ServiceInfo{
					Name:       s.Name,
					Namespace:  s.Namespace,
					Type:       string(s.Spec.Type),
					ClusterIP:  s.Spec.ClusterIP,
					Labels:     s.Labels,
					CreatedAt:  s.CreationTimestamp.Format(time.RFC3339),
				}

				// Collect external IPs
				for _, ingress := range s.Status.LoadBalancer.Ingress {
					if ingress.IP != "" {
						svcInfo.ExternalIP = append(svcInfo.ExternalIP, ingress.IP)
					}
					if ingress.Hostname != "" {
						svcInfo.ExternalIP = append(svcInfo.ExternalIP, ingress.Hostname)
					}
				}

				// Collect ports
				for _, port := range s.Spec.Ports {
					svcInfo.Ports = append(svcInfo.Ports, ServicePort{
						Name:       port.Name,
						Port:       port.Port,
						TargetPort: port.TargetPort.String(),
						Protocol:   string(port.Protocol),
					})
				}

				response.Services = append(response.Services, svcInfo)
			}
		}

		// Build summary
		response.Summary = ClusterSummary{
			TotalDeployments: len(response.Deployments),
			TotalPods:        len(response.Pods),
			TotalServices:    len(response.Services),
		}

		for _, p := range response.Pods {
			switch p.Phase {
			case "Running":
				response.Summary.RunningPods++
			case "Pending":
				response.Summary.PendingPods++
			case "Failed":
				response.Summary.FailedPods++
			}
		}

		if namespace == "" {
			response.Namespace = "all"
		}

		c.JSON(http.StatusOK, response)
	}
}
