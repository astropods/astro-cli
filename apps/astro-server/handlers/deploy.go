package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	spec "github.com/astropods/astro/packages/astro-spec"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/gin-gonic/gin"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// isAccountMember checks whether the user is a member of the account.
func isAccountMember(_ *gin.Context, accountStore *account.AccountStore, acctID, userID string) bool {
	isMember, err := accountStore.IsMember(acctID, userID)
	return err == nil && isMember
}

// deploymentNamespace derives a K8s namespace from a deployment ID (xxx-xxx-xxx).
// The ID is generated once per new deployment and stored in the DB,
// so the namespace is stable across redeploys.
func deploymentNamespace(id string) string {
	return "astro-" + deployid.Compact(id) + "-0"
}

// verifyNamespaceOwnership checks that a K8s namespace belongs to the given account.
// resolveDeployment looks up a deployment by ID, verifies the caller's account owns it,
// and returns the deployment. Used by handlers that accept a deployment ID in the URL path
// so the frontend never needs to know about K8s namespaces.
func resolveDeployment(c *gin.Context, deployStore *deploymentstore.Store, accountStore *account.AccountStore) (*deploymentstore.Deployment, error) {
	deploymentID := c.Param("id")
	if deploymentID == "" {
		return nil, fmt.Errorf("deployment ID is required")
	}

	dep, err := deployStore.GetDeploymentByID(deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to look up deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found")
	}

	// Verify account ownership
	user, _ := middleware.GetUser(c)
	if !isAccountMember(c, accountStore, dep.AccountID, user.ID) {
		return nil, fmt.Errorf("insufficient permissions")
	}

	return dep, nil
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
	isUpdate      bool // true when deployment_id was provided (in-place update)
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

	// Auth: user must have visibility on the source agent.
	// Public agents are deployable by anyone; private agents require source account membership.
	sourceAgent, err := agentIndex.Get(sourceAcct.ID, submittedSpec.Source.Name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source agent not found"})
		return nil, false
	}
	if sourceAgent.Visibility == "private" {
		isSourceMember, err := accountStore.IsMember(sourceAcct.ID, user.ID)
		if err != nil || !isSourceMember {
			c.JSON(http.StatusNotFound, gin.H{"error": "source agent not found"})
			return nil, false
		}
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
		log.Error("Failed to marshal stored spec", "error", err, "agent", agentName, "build", buildID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process registered spec", "details": err.Error()})
		return nil, false
	}
	if err := json.Unmarshal(specBytes, &astroSpec); err != nil {
		log.Error("Failed to unmarshal spec into AstroSpec", "error", err, "agent", agentName, "build", buildID, "raw", string(specBytes))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse registered spec", "details": err.Error()})
		return nil, false
	}

	// Re-generate the server's canonical template for this build
	template, err := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
		Spec:              &astroSpec,
		Account:           sourceAccountName,
		ECRNamespace:      agentVersion.ECRNamespace,
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
	var isUpdate bool
	if submittedSpec.Target.DeploymentID != "" && deployStore != nil {
		// Explicit deployment_id: in-place update
		existing, _ := deployStore.GetDeploymentByID(submittedSpec.Target.DeploymentID)
		if existing == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found for given deployment_id"})
			return nil, false
		}
		if existing.Status != deploymentstore.StatusActive && existing.Status != deploymentstore.StatusFailed {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deployment is not active or failed"})
			return nil, false
		}
		if !isAccountMember(c, accountStore, existing.AccountID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for deployment's account"})
			return nil, false
		}
		deploymentID = existing.ID
		k8sNamespace = existing.Namespace
		isUpdate = true
	}
	if k8sNamespace == "" && displayName != "" && deployStore != nil {
		existing, _ := deployStore.GetActiveDeploymentByDisplayName(targetAcct.ID, displayName)
		if existing != nil {
			// Redeploy — reuse namespace
			k8sNamespace = existing.Namespace
			deploymentID = deployid.New()
		}
	}
	if k8sNamespace == "" && deployStore != nil {
		// Check for single active deployment of this agent (backward compat)
		existing, _ := deployStore.GetActiveDeployment(targetAcct.ID, agentName)
		if existing != nil && displayName == "" {
			k8sNamespace = existing.Namespace
			deploymentID = deployid.New()
		}
	}
	if k8sNamespace == "" {
		// New deployment — generate UUID-based namespace
		deploymentID = deployid.New()
		k8sNamespace = deploymentNamespace(deploymentID)
	}

	// Rule 19: reject any change to server-owned fields
	// Sync user-supplied target fields so EnforceEditable doesn't reject them
	template.Target.Account = submittedSpec.Target.Account
	template.Target.DisplayName = submittedSpec.Target.DisplayName
	template.Target.DeploymentID = submittedSpec.Target.DeploymentID
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
		isUpdate:      isUpdate,
		resolveResult: resolveResult,
	}, true
}

// DeployAgent returns a handler for deploying agents to Kubernetes.
// POST /api/v1/deploy
// Content-Type: application/yaml (or application/json)
// Body: fulfilled deployment spec (spec: deployment/v1)
// EntitlementChecker is the interface used by DeployAgent for entitlement checks.
// A nil EntitlementChecker skips all checks.
type EntitlementChecker interface {
	Check(ctx context.Context, accountID string, features ...string) (blocked bool, feature string, ent *openmeter.EntitlementValue)
}

// DeployQueue abstracts job insertion for deploy/undeploy/wakeup operations.
type DeployQueue interface {
	InsertDeployJob(ctx context.Context, deploymentID string) error
	InsertUndeployJob(ctx context.Context, deploymentID string) error
	InsertWakeUpJob(ctx context.Context, deploymentID string) error
}

func DeployAgent(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, entCheck EntitlementChecker, queue DeployQueue) gin.HandlerFunc {
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

		// Check entitlements for deploy (account resolved from spec, not middleware)
		if entCheck != nil {
			if blocked, feature, entResult := entCheck.Check(c.Request.Context(), dctx.acct.ID, "agent_deployments", "compute"); blocked {
				c.JSON(http.StatusPaymentRequired, gin.H{
					"error":   "entitlement limit reached",
					"feature": feature,
					"usage":   entResult.Usage,
					"limit":   entResult.TotalAvailableGrantAmount,
				})
				return
			}
		}

		// Persist resolved spec and enqueue async deploy job
		if deployStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment store not configured"})
			return
		}

		stripped := spec.StripSecretVariableValues(dctx.resolveResult.Spec)
		specJSON, marshalErr := json.Marshal(stripped)
		if marshalErr != nil {
			log.Error("Failed to marshal stripped spec for storage", "error", marshalErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process spec"})
			return
		}

		// Resolve env vars for normalized storage
		rctx := deployment.ResolveContext{
			Namespace:  dctx.k8sNS,
			AgentName:  dctx.agentName,
			BuildID:    dctx.buildID,
			SecretName: deployment.GenerateSecretName(dctx.agentName, dctx.buildID),
		}
		resolved := deployment.ResolveDeploymentSpecEnv(dctx.resolveResult.Spec, rctx)

		// Create encryptor if KMS is configured
		var enc *envelope.Encryptor
		if cfg.Deployment.KMSKeyARN != "" {
			awsCfg, awsErr := awsconfig.LoadDefaultConfig(c.Request.Context())
			if awsErr != nil {
				log.Error("Failed to load AWS config for KMS", "error", awsErr)
			} else {
				kmsClient := kms.NewFromConfig(awsCfg)
				enc, awsErr = envelope.NewEncryptor(c.Request.Context(), kmsClient, cfg.Deployment.KMSKeyARN)
				if awsErr != nil {
					log.Error("Failed to create KMS encryptor", "error", awsErr)
				}
			}
		}

		params := deploymentstore.SaveDeploymentParams{
			ID: dctx.deploymentID, AccountID: dctx.acct.ID,
			AgentName: dctx.agentName, DisplayName: dctx.displayName,
			BuildID: dctx.buildID, Namespace: dctx.k8sNS,
			SpecJSON: string(specJSON),
		}
		if enc != nil {
			params.EncryptedDataKey = enc.EncryptedDataKey
			params.KMSKeyARN = enc.KMSKeyARN
		}

		// Save deployment as pending with normalized spec in same transaction
		txFn := func(tx *sql.Tx, deploymentID string) error {
			nsCfg := &deploymentstore.NormalizedSpecConfig{
				Namespace:              dctx.k8sNS,
				IngressDomain:          cfg.Deployment.IngressDomain,
				IngestionIngressDomain: cfg.Deployment.IngestionIngressDomain,
			}
			return deploymentstore.SaveNormalizedSpec(tx, deploymentID, dctx.resolveResult.Spec, resolved, enc, nsCfg)
		}
		var storeErr error
		if dctx.isUpdate {
			_, storeErr = deployStore.UpdateDeploymentPending(params, txFn)
		} else {
			_, storeErr = deployStore.SaveDeploymentPending(params, txFn)
		}
		if storeErr != nil {
			log.Error("Failed to save deployment record", "error", storeErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule deployment"})
			return
		}

		// Enqueue deploy job (separate from DB transaction; UniqueOpts prevents duplicates)
		if err := queue.InsertDeployJob(c.Request.Context(), dctx.deploymentID); err != nil {
			log.Error("Failed to enqueue deploy job", "error", err, "deployment_id", dctx.deploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule deployment"})
			return
		}

		log.Info("Deployment queued",
			"deployment_id", dctx.deploymentID,
			"agent", dctx.agentName,
			"build", dctx.buildID,
			"namespace", dctx.k8sNS,
		)

		c.JSON(http.StatusAccepted, deployment.DeployResponse{
			Status:       "pending",
			DeploymentID: dctx.deploymentID,
			Name:         dctx.agentName,
			BuildID:      dctx.buildID,
			K8sNamespace: dctx.k8sNS,
			DeployedAt:   time.Now().UTC(),
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
func UndeployAgent(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, queue DeployQueue) gin.HandlerFunc {
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

		// Look up deployment by ID — accept active or scaled_down
		dep, err := deployStore.GetDeploymentByID(req.DeploymentID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up deployment"})
			return
		}
		if dep == nil || (dep.Status != deploymentstore.StatusActive && dep.Status != deploymentstore.StatusScaledDown && dep.Status != deploymentstore.StatusFailed) {
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

		// Set status to undeploying and enqueue async undeploy job
		if err := deployStore.UpdateStatus(dep.ID, deploymentstore.StatusUndeploying, "", nil); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
			return
		}

		if err := queue.InsertUndeployJob(c.Request.Context(), dep.ID); err != nil {
			log.Error("Failed to enqueue undeploy job", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule undeploy"})
			return
		}

		log.Info("Undeploy queued",
			"deployment_id", dep.ID,
			"namespace", dep.Namespace,
		)

		c.JSON(http.StatusAccepted, deployment.UndeployResponse{
			Status:       "undeploying",
			Name:         dep.AgentName,
			BuildID:      dep.BuildID,
			K8sNamespace: dep.Namespace,
			UndeployedAt: time.Now().UTC(),
		})
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
	ID               string                `json:"id"`
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

		if deployStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "deployment store not configured",
			})
			return
		}

		// DB is the source of truth — query all visible deployments (not just active)
		dbDeps, err := deployStore.GetVisibleDeploymentsByAccount(acct.ID)
		if err != nil {
			log.Error("Failed to load deployments from DB", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to list deployments",
				"details": err.Error(),
			})
			return
		}

		// For each DB deployment, fetch live K8s status from its namespace.
		// Deployments without K8s resources (failed, pending) get a DB-only entry.
		var allDeployments []AgentDeployment
		for _, dbDep := range dbDeps {
			// Read manual ingestions from namespace annotations
			ns, nsErr := k8sClient.Clientset().CoreV1().Namespaces().Get(
				c.Request.Context(), dbDep.Namespace, metav1.GetOptions{},
			)
			if nsErr != nil || ns.DeletionTimestamp != nil {
				// No K8s namespace — build entry from DB record alone
				allDeployments = append(allDeployments, agentDeploymentFromDB(dbDep))
				continue
			}

			manualIngestions := parseManualIngestions(ns.Annotations)
			deps, k8sErr := listAstroDeployments(c.Request.Context(), k8sClient, dbDep.Namespace, manualIngestions)
			if k8sErr != nil || len(deps) == 0 {
				// K8s resources missing or error — build entry from DB record
				allDeployments = append(allDeployments, agentDeploymentFromDB(dbDep))
				continue
			}

			// Populate DB-owned fields on all K8s deployment entries for this namespace.
			// The DB is the source of truth for agent name — k8s labels must not
			// leak into frontend responses.
			for i := range deps {
				deps[i].ID = dbDep.ID
				deps[i].Name = dbDep.AgentName
				deps[i].DisplayName = dbDep.DisplayName
			}
			allDeployments = append(allDeployments, deps...)
		}

		c.JSON(http.StatusOK, gin.H{
			"deployments": allDeployments,
			"count":       len(allDeployments),
		})
	}
}

// agentDeploymentFromDB builds an AgentDeployment entry from a DB record alone,
// used when K8s resources are unavailable (failed, pending, or missing namespace).
func agentDeploymentFromDB(dep *deploymentstore.Deployment) AgentDeployment {
	status := "error"
	switch dep.Status {
	case deploymentstore.StatusActive:
		status = "Running"
	case deploymentstore.StatusPending, deploymentstore.StatusProvisioning:
		status = "pending"
	case deploymentstore.StatusScaledDown:
		status = "Stopped"
	case deploymentstore.StatusUndeploying:
		status = "pending"
	}

	ad := AgentDeployment{
		ID:          dep.ID,
		Name:        dep.AgentName,
		DisplayName: dep.DisplayName,
		BuildID:     dep.BuildID,
		Namespace:   dep.Namespace,
		Status:      status,
		Replicas:    0,
		Ready:       0,
		CreatedAt:   dep.DeployedAt.Format(time.RFC3339),
		Components:  []string{},
	}

	if dep.ErrorMessage != nil && *dep.ErrorMessage != "" {
		ad.Status = "error"
	}

	return ad
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
			agentKey := ing.Labels[deployment.LabelKeyAgent]
			version := ing.Labels["app.kubernetes.io/version"]
			component := ing.Labels["app.kubernetes.io/component"]

			if agentKey != "" && len(ing.Spec.Rules) > 0 {
				key := agentKey + ":" + version
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
		agentKey := dep.Labels[deployment.LabelKeyAgent]
		version := dep.Labels["app.kubernetes.io/version"]
		component := dep.Labels["app.kubernetes.io/component"]

		if agentKey == "" {
			continue
		}

		key := agentKey + ":" + version
		info, exists := agentDeployments[key]
		if !exists {
			// Use Spec.Replicas (desired) rather than Status.Replicas (current).
			// Status.Replicas starts at 0 right after creation and increments as pods
			// are scheduled, so using it would cause a transient "Stopped" status
			// during normal deployment startup.
			desiredReplicas := int32(1)
			if dep.Spec.Replicas != nil {
				desiredReplicas = *dep.Spec.Replicas
			}

			status := "Running"
			if dep.Status.ReadyReplicas < desiredReplicas {
				status = "Pending"
			}
			if desiredReplicas == 0 {
				status = "Stopped"
			}

			info = &AgentDeployment{
				BuildID:          version,
				Namespace:        namespace,
				Status:           status,
				Replicas:         desiredReplicas,
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

		// Update status if any deployment is not ready (using same desired replica logic)
		desiredReplicas := int32(1)
		if dep.Spec.Replicas != nil {
			desiredReplicas = *dep.Spec.Replicas
		}
		if dep.Status.ReadyReplicas < desiredReplicas && desiredReplicas > 0 {
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
		agentKey := job.Labels[deployment.LabelKeyAgent]
		version := job.Labels["app.kubernetes.io/version"]
		component := job.Labels["app.kubernetes.io/component"]
		if agentKey == "" {
			continue
		}

		key := agentKey + ":" + version
		info, exists := agentDeployments[key]
		if !exists {
			// Job exists but no Deployment entry — create a stub so it's visible
			info = &AgentDeployment{
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
		agentKey := pod.Labels[deployment.LabelKeyAgent]
		version := pod.Labels["app.kubernetes.io/version"]
		if agentKey == "" {
			continue
		}

		key := agentKey + ":" + version
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
// POST /api/v1/deployments/:id/pods/:pod/restart
func RestartPod(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := middleware.GetUser(c); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		podName := c.Param("pod")
		err = k8sClient.Clientset().CoreV1().Pods(dep.Namespace).Delete(c.Request.Context(), podName, metav1.DeleteOptions{})
		if err != nil {
			log.Error("Failed to delete pod", "error", err, "pod", podName, "namespace", dep.Namespace)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restart pod", "details": err.Error()})
			return
		}

		user, _ := middleware.GetUser(c)
		log.Info("Pod restarted (deleted)", "pod", podName, "namespace", dep.Namespace, "user", user.ID)
		c.JSON(http.StatusOK, gin.H{"status": "restarting", "pod": podName})
	}
}

func GetDeploymentLogs(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store, lokiClient *loki.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := middleware.GetUser(c); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		podName := c.Query("pod")
		containerName := c.Query("container")

		tailLines := int64(200)
		if tl := c.Query("tailLines"); tl != "" {
			if parsed, err := strconv.ParseInt(tl, 10, 64); err == nil && parsed > 0 {
				tailLines = parsed
			}
		}

		// Loki path: query the centralized log store.
		if lokiClient != nil {
			p := loki.QueryParams{
				Namespace: dep.Namespace,
				Pod:       podName,
				Container: containerName,
				Limit:     tailLines,
			}
			if s := c.Query("since"); s != "" {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					p.Start = t
				} else if ns, err := strconv.ParseInt(s, 10, 64); err == nil {
					p.Start = time.Unix(0, ns)
				}
			}
			if u := c.Query("until"); u != "" {
				if t, err := time.Parse(time.RFC3339, u); err == nil {
					p.End = t
				} else if ns, err := strconv.ParseInt(u, 10, 64); err == nil {
					p.End = time.Unix(0, ns)
				}
			}

			lines, err := lokiClient.QueryLogs(c.Request.Context(), p)
			if err != nil {
				log.Error("Failed to query Loki logs", "error", err, "namespace", dep.Namespace)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "failed to query logs",
					"details": err.Error(),
				})
				return
			}

			var sb strings.Builder
			for _, l := range lines {
				sb.WriteString(l.Line)
			}
			c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(sb.String()))
			return
		}

		// K8s fallback: direct pod log stream (used when LOKI_URL is not configured).
		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "log backend not configured"})
			return
		}
		if podName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pod query parameter is required"})
			return
		}

		logOpts := &corev1.PodLogOptions{TailLines: &tailLines}
		if containerName != "" {
			logOpts.Container = containerName
		}

		req := k8sClient.Clientset().CoreV1().Pods(dep.Namespace).GetLogs(podName, logOpts)
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

// generateTemplate resolves an agent build and generates a deployment template.
// Shared by GetDeploymentTemplate and GetPrefilledDeploymentTemplate.
func generateTemplate(
	c *gin.Context,
	log *logger.Logger,
	agentIndex *agentindex.Index,
	accountStore *account.AccountStore,
	cfg *config.Config,
) (*spec.AstroDeploymentSpec, bool) {
	accountName := c.Param("account")
	name := c.Param("name")

	user, exists := middleware.GetUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return nil, false
	}

	acct, err := accountStore.GetByName(accountName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return nil, false
	}
	accountID := acct.ID

	agent, err := agentIndex.Get(accountID, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return nil, false
	}

	if agent.Visibility == "private" {
		if !isAccountMember(c, accountStore, accountID, user.ID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return nil, false
		}
	}

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
		return nil, false
	}

	specBytes, err := json.Marshal(agentVersion.Spec)
	if err != nil {
		log.Error("Failed to marshal stored spec", "error", err, "account", accountName, "agent", name)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process spec", "details": err.Error()})
		return nil, false
	}
	var astroSpec spec.AstroSpec
	if err := json.Unmarshal(specBytes, &astroSpec); err != nil {
		log.Error("Failed to unmarshal spec into AstroSpec", "error", err, "account", accountName, "agent", name, "raw", string(specBytes))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse spec", "details": err.Error()})
		return nil, false
	}

	template, err := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
		Spec:              &astroSpec,
		Account:           accountName,
		ECRNamespace:      agentVersion.ECRNamespace,
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
		return nil, false
	}

	return template, true
}

// respondWithTemplate sends the template as JSON or YAML based on query param.
func respondWithTemplate(c *gin.Context, template *spec.AstroDeploymentSpec) {
	if c.Query("format") == "json" {
		c.JSON(http.StatusOK, template)
		return
	}
	yamlBytes, err := spec.SerializeDeploymentSpec(template)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize template"})
		return
	}
	c.Data(http.StatusOK, "application/yaml", yamlBytes)
}

// GetDeploymentTemplate returns a handler for generating deployment spec templates.
// GET /api/v1/agents/:account/:name/deployment-template
// Optional query: ?build=<build_id>
func GetDeploymentTemplate(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		template, ok := generateTemplate(c, log, agentIndex, accountStore, cfg)
		if !ok {
			return
		}
		respondWithTemplate(c, template)
	}
}

// GetPrefilledDeploymentTemplate returns a handler for generating a deployment template
// pre-filled with values from an existing deployment.
// GET /api/v1/agents/:account/:name/deployment-template/:deploymentID
func GetPrefilledDeploymentTemplate(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		deploymentID := c.Param("deploymentID")

		template, ok := generateTemplate(c, log, agentIndex, accountStore, cfg)
		if !ok {
			return
		}

		// Look up existing deployment
		existing, err := deployStore.GetDeploymentByID(deploymentID)
		if err != nil {
			log.Error("Failed to get deployment", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up deployment"})
			return
		}
		if existing == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}

		// Verify requesting user is a member of deployment's account
		user, _ := middleware.GetUser(c)
		if !isAccountMember(c, accountStore, existing.AccountID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for deployment's account"})
			return
		}

		// Get stored variables
		storedVars, err := deployStore.GetDeploymentVariables(deploymentID)
		if err != nil {
			log.Error("Failed to get deployment variables", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get deployment variables"})
			return
		}

		// Decrypt secrets if KMS is configured and deployment has an encrypted data key
		var dec *envelope.Decryptor
		if len(existing.EncryptedDataKey) > 0 && cfg.Deployment.KMSKeyARN != "" {
			awsCfg, awsErr := awsconfig.LoadDefaultConfig(c.Request.Context())
			if awsErr == nil {
				kmsClient := kms.NewFromConfig(awsCfg)
				dec, _ = envelope.NewDecryptor(c.Request.Context(), kmsClient, existing.EncryptedDataKey)
			}
		}

		// Merge stored values into template
		template.Target.DeploymentID = deploymentID
		template.Target.DisplayName = existing.DisplayName

		// Resolve account name for target.account
		acct, err := accountStore.GetByID(existing.AccountID)
		if err == nil && acct != nil {
			template.Target.Account = acct.Name
		}

		// Merge variable values
		for _, sv := range storedVars {
			if tv, ok := template.Variables[sv.Name]; ok {
				val := sv.Value
				if sv.Secret && dec != nil && len(sv.Nonce) > 0 {
					ciphertext, b64Err := base64.StdEncoding.DecodeString(val)
					if b64Err == nil {
						plaintext, decErr := dec.Decrypt(ciphertext, sv.Nonce)
						if decErr == nil {
							val = string(plaintext)
						}
					}
				}
				tv.Value = val
				template.Variables[sv.Name] = tv
			}
		}

		// Merge adapters and ingestion schedules from stored spec
		if existing.DeploymentSpecJSON != "" {
			var storedSpec spec.AstroDeploymentSpec
			if jsonErr := json.Unmarshal([]byte(existing.DeploymentSpecJSON), &storedSpec); jsonErr == nil {
				if storedSpec.Interfaces != nil && template.Interfaces != nil {
					template.Interfaces.Adapters = storedSpec.Interfaces.Adapters
				}
				for name, storedIng := range storedSpec.Ingestion {
					if tmplIng, ok := template.Ingestion[name]; ok && storedIng.Trigger.Schedule != "" {
						tmplIng.Trigger.Schedule = storedIng.Trigger.Schedule
						template.Ingestion[name] = tmplIng
					}
				}
			}
		}

		respondWithTemplate(c, template)
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
// GET /api/v1/deployments/:id/configmap/:cmname
func GetConfigMapData(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := middleware.GetUser(c); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		cmName := c.Param("cmname")
		cm, err := k8sClient.Clientset().CoreV1().ConfigMaps(dep.Namespace).Get(c.Request.Context(), cmName, metav1.GetOptions{})
		if err != nil {
			log.Error("Failed to get configmap", "error", err, "configmap", cmName, "namespace", dep.Namespace)
			c.JSON(http.StatusNotFound, gin.H{"error": "configmap not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"name": cm.Name,
			"data": cm.Data,
		})
	}
}

// GetSecretKeys returns the key names (but NOT values) of a Secret in a deployment's namespace.
// GET /api/v1/deployments/:id/secret/:secretname/keys
func GetSecretKeys(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := middleware.GetUser(c); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		secretName := c.Param("secretname")
		secret, err := k8sClient.Clientset().CoreV1().Secrets(dep.Namespace).Get(c.Request.Context(), secretName, metav1.GetOptions{})
		if err != nil {
			log.Error("Failed to get secret", "error", err, "secret", secretName, "namespace", dep.Namespace)
			c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
			return
		}

		keys := make([]string, 0, len(secret.Data))
		for k := range secret.Data {
			keys = append(keys, k)
		}

		c.JSON(http.StatusOK, gin.H{
			"name": secret.Name,
			"keys": keys,
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

// GetDeploymentStatus returns the current status, events, and revisions for a deployment.
func GetDeploymentStatus(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dep, err := deployStore.GetDeploymentByID(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up deployment"})
			return
		}
		if dep == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}

		if !isAccountMember(c, accountStore, dep.AccountID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		events, evErr := deployStore.GetDeploymentEvents(dep.ID, 50)
		if evErr != nil {
			log.Warn("Failed to load deployment events", "error", evErr, "deployment_id", dep.ID)
		}
		revisions, revErr := deployStore.GetRevisions(dep.ID)
		if revErr != nil {
			log.Warn("Failed to load deployment revisions", "error", revErr, "deployment_id", dep.ID)
		}

		c.JSON(http.StatusOK, gin.H{
			"deployment_id":     dep.ID,
			"status":            dep.Status,
			"current_revision":  dep.CurrentRevision,
			"error_message":     dep.ErrorMessage,
			"error_details":     dep.ErrorDetails,
			"deployed_at":       dep.DeployedAt,
			"status_changed_at": dep.StatusChangedAt,
			"events":            events,
			"revisions":         revisions,
		})
	}
}

// WakeUpDeployment triggers re-provisioning of a KEDA-scaled-down deployment.
func WakeUpDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, queue DeployQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dep, err := deployStore.GetDeploymentByID(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up deployment"})
			return
		}
		if dep == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}
		if dep.Status != deploymentstore.StatusScaledDown {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deployment is not scaled down"})
			return
		}

		if !isAccountMember(c, accountStore, dep.AccountID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		if err := deployStore.UpdateStatus(dep.ID, deploymentstore.StatusPending, "", nil); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
			return
		}

		if err := queue.InsertWakeUpJob(c.Request.Context(), dep.ID); err != nil {
			log.Error("Failed to enqueue wakeup job", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule wakeup"})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"status":        "pending",
			"deployment_id": dep.ID,
		})
	}
}

// RollbackDeployment rolls back a deployment to a previous revision.
func RollbackDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, queue DeployQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		var req struct {
			Revision int `json:"revision" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
			return
		}

		dep, err := deployStore.GetDeploymentByID(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up deployment"})
			return
		}
		if dep == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}
		if dep.Status != deploymentstore.StatusActive && dep.Status != deploymentstore.StatusFailed {
			c.JSON(http.StatusBadRequest, gin.H{"error": "can only rollback active or failed deployments"})
			return
		}
		if dep.CurrentRevision != nil && *dep.CurrentRevision == req.Revision {
			c.JSON(http.StatusBadRequest, gin.H{"error": "already on this revision"})
			return
		}

		if !isAccountMember(c, accountStore, dep.AccountID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		// SetCurrentRevision atomically sets revision, status=pending, and records event.
		// Job enqueue happens after commit (store uses database/sql, River uses pgx).
		if err := deployStore.SetCurrentRevision(dep.ID, req.Revision, nil); err != nil {
			log.Error("Failed to set revision", "error", err, "deployment_id", dep.ID, "revision", req.Revision)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := queue.InsertDeployJob(c.Request.Context(), dep.ID); err != nil {
			log.Error("Failed to enqueue rollback deploy job", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule rollback"})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"status":           "pending",
			"deployment_id":    dep.ID,
			"current_revision": req.Revision,
			"message":          fmt.Sprintf("Rolling back to revision %d", req.Revision),
		})
	}
}

// TODO: move local-env pull policy into a dedicated PR for local K8s dev experience
// imagePullPolicyForMode returns IfNotPresent for local mode (locally-built
// images are used as-is; third-party images like qdrant are pulled on first
// use), PullAlways for production.
func imagePullPolicyForMode(mode string) corev1.PullPolicy {
	if mode == "local" {
		return corev1.PullIfNotPresent
	}
	return corev1.PullAlways
}
