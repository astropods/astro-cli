package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	spec "github.com/astropods/astro-spec"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TriggerIngestion returns a handler that creates a one-shot Job for a manual ingestion trigger
func TriggerIngestion(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, k8sReg *k8s.Registry, deployStore *deploymentstore.Store, cfg *config.Config, auditStore *auditlog.Store, entCheck EntitlementChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		ingestionName := c.Param("ingestion")

		if _, exists := middleware.GetUser(c); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		if blockedByBilling(c, entCheck, dep.AccountID) {
			return
		}

		k8sClient, ok := clusterClientForDeployment(c, k8sReg, dep)
		if !ok {
			return
		}

		k8sNamespace := dep.Namespace
		agentName := dep.AgentName
		buildID := dep.BuildID

		// Fetch agent spec from index
		agentVersion, err := agentIndex.GetVersion(dep.AccountID, agentName, buildID)
		if err != nil {
			log.Error("ingestion: agent version not found", "error", err)
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
			log.Error("ingestion: marshal spec failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process spec"})
			return
		}
		if err := json.Unmarshal(specBytes, &astroSpec); err != nil {
			log.Error("ingestion: unmarshal spec failed", "error", err)
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
			Name:            jobName,
			Namespace:       k8sNamespace,
			AgentName:       agentName,
			BuildID:         buildID,
			Component:       fmt.Sprintf("ingestion-%s", ingestionName),
			SecretName:      secretName,
			ConfigMapName:   configMapName,
			Ingestion:       ingestion,
			ImagePullPolicy: imagePullPolicyForMode(cfg.Deployment.K8sClientMode),
		}
		job := k8s.BuildJob(jobCfg)

		_, err = k8sClient.Clientset().BatchV1().Jobs(k8sNamespace).Create(
			c.Request.Context(), job, metav1.CreateOptions{},
		)
		if err != nil {
			log.Error("ingestion: create ingestion job failed", "error", err, "job", jobName)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to create ingestion job",
				"details": err.Error(),
			})
			return
		}

		u, _ := middleware.GetUser(c)
		log.Info("ingestion: manual ingestion triggered",
			"agent", agentName,
			"build_id", buildID,
			"ingestion", ingestionName,
			"job", jobName,
			"namespace", k8sNamespace,
			"user", u.ID,
		)

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.DeploymentTriggerIngestion
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Triggered ingestion " + ingestionName
		evt.Metadata = map[string]any{"ingestion": ingestionName, "job": jobName}
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, gin.H{
			"status":   "triggered",
			"job_name": jobName,
		})
	}
}
