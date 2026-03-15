package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TriggerIngestion returns a handler that creates a one-shot Job for a manual ingestion trigger
func TriggerIngestion(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, k8sClient k8s.ClusterClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		k8sNamespace := c.Param("id")
		ingestionName := c.Param("ingestion")

		// Get authenticated user from context
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
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

		// Verify membership
		if !isAccountMember(c, accountStore, acct.ID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		// Verify namespace ownership and get agent metadata from labels
		ns, err := k8sClient.Clientset().CoreV1().Namespaces().Get(c.Request.Context(), k8sNamespace, metav1.GetOptions{})
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "namespace not found"})
			return
		}
		if ns.Labels["astro.dev/account-id"] != acct.ID {
			c.JSON(http.StatusForbidden, gin.H{"error": "namespace does not belong to this account"})
			return
		}

		agentName := ns.Labels["astro.dev/agent"]
		buildID := ns.Labels["astro.dev/build"]
		if agentName == "" || buildID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "namespace missing agent or build labels"})
			return
		}

		// Fetch agent spec from index
		agentVersion, err := agentIndex.GetVersion(acct.ID, agentName, buildID)
		if err != nil {
			log.Error("Agent version not found", "error", err)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "agent version not found",
				"details": fmt.Sprintf("%s:%s not found in index", agentName, buildID),
			})
			return
		}

		// Parse spec
		var astroSpec spec.AstroSpec
		specBytes, err := json.Marshal(agentVersion.Spec)
		if err != nil {
			log.Error("Failed to marshal spec", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process spec"})
			return
		}
		if err := json.Unmarshal(specBytes, &astroSpec); err != nil {
			log.Error("Failed to unmarshal spec", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse spec"})
			return
		}

		// Look up the ingestion entry
		ingestion, ok := astroSpec.Ingestion[ingestionName]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"error": fmt.Sprintf("ingestion %q not found in spec", ingestionName),
			})
			return
		}

		if ingestion.Trigger.Type != "manual" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("ingestion %q has trigger type %q, not \"manual\"", ingestionName, ingestion.Trigger.Type),
			})
			return
		}

		// Generate unique job name with timestamp
		resourceName := deployment.GenerateResourceName(agentName, "ingestion", ingestionName)
		jobName := fmt.Sprintf("%s-%d", resourceName, time.Now().Unix())

		secretName := deployment.GenerateSecretName(agentName, buildID)
		configMapName := deployment.GenerateConfigMapName(agentName, buildID)

		jobCfg := k8s.JobConfig{
			Name:          jobName,
			Namespace:     k8sNamespace,
			AgentName:     agentName,
			BuildID:       buildID,
			Component:     fmt.Sprintf("ingestion-%s", ingestionName),
			SecretName:    secretName,
			ConfigMapName: configMapName,
			Ingestion:     ingestion,
		}
		job := k8s.BuildJob(jobCfg)

		_, err = k8sClient.Clientset().BatchV1().Jobs(k8sNamespace).Create(
			c.Request.Context(), job, metav1.CreateOptions{},
		)
		if err != nil {
			log.Error("Failed to create ingestion job", "error", err, "job", jobName)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to create ingestion job",
				"details": err.Error(),
			})
			return
		}

		log.Info("Manual ingestion triggered",
			"agent", agentName,
			"build_id", buildID,
			"ingestion", ingestionName,
			"job", jobName,
			"namespace", k8sNamespace,
			"user", user.ID,
		)

		c.JSON(http.StatusOK, gin.H{
			"status":    "triggered",
			"job_name":  jobName,
			"namespace": k8sNamespace,
		})
	}
}
