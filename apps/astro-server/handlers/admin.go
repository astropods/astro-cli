package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/deploymentstore"
	"github.com/postman/astro/apps/astro-server/internal/k8s"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AdminAgentDeployment extends AgentDeployment with account info.
type AdminAgentDeployment struct {
	AgentDeployment
	AccountName string `json:"account_name"`
}

// AdminListDeployments returns a handler that lists all active deployments across all accounts.
// It reuses listAstroDeployments to get full K8s data (pods, jobs, URLs).
func AdminListDeployments(log *logger.Logger, deployStore *deploymentstore.Store, k8sClient k8s.ClusterClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		dbDeployments, err := deployStore.ListAllActive()
		if err != nil {
			log.Error("Failed to list all active deployments", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
			return
		}

		var results []AdminAgentDeployment

		for _, d := range dbDeployments {
			ts := d.DeployedAt.Format("2006-01-02T15:04:05Z07:00")

			if k8sClient == nil {
				// No K8s client — return DB-only data
				results = append(results, AdminAgentDeployment{
					AgentDeployment: AgentDeployment{
						Name:       d.AgentName,
						BuildID:    d.BuildID,
						Namespace:  d.Namespace,
						Status:     d.Status,
						CreatedAt:  ts,
						Components: []string{},
					},
					AccountName: d.AccountName,
				})
				continue
			}

			// Read manual ingestions from namespace annotations
			ns, nsErr := k8sClient.Clientset().CoreV1().Namespaces().Get(
				c.Request.Context(), d.Namespace, metav1.GetOptions{},
			)
			var manualIngestions []string
			if nsErr == nil {
				manualIngestions = parseManualIngestions(ns.Annotations)
			}

			// Reuse the same function that powers the regular deployments list
			k8sDeps, err := listAstroDeployments(c.Request.Context(), k8sClient, d.Namespace, manualIngestions)
			if err != nil {
				log.Warn("Failed to list K8s deployments for namespace", "namespace", d.Namespace, "error", err)
				results = append(results, AdminAgentDeployment{
					AgentDeployment: AgentDeployment{
						Name:       d.AgentName,
						BuildID:    d.BuildID,
						Namespace:  d.Namespace,
						Status:     "Unknown",
						CreatedAt:  ts,
						Components: []string{},
					},
					AccountName: d.AccountName,
				})
				continue
			}

			if len(k8sDeps) == 0 {
				results = append(results, AdminAgentDeployment{
					AgentDeployment: AgentDeployment{
						Name:       d.AgentName,
						BuildID:    d.BuildID,
						Namespace:  d.Namespace,
						Status:     "NotFound",
						CreatedAt:  ts,
						Components: []string{},
					},
					AccountName: d.AccountName,
				})
				continue
			}

			// Match by agent name, or take the first if no exact match
			for _, dep := range k8sDeps {
				if strings.EqualFold(dep.Name, d.AgentName) || len(k8sDeps) == 1 {
					results = append(results, AdminAgentDeployment{
						AgentDeployment: dep,
						AccountName:     d.AccountName,
					})
					break
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"deployments": results,
			"count":       len(results),
		})
	}
}
