package handlers

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	spec "github.com/astropods/astro/packages/astro-spec"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
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
	acct              *account.Account
	sourceAccountName string // account that owns the blueprint (may differ from acct on cross-account deploys)
	agentName         string
	displayName       string
	deploymentID      string
	buildID           string
	k8sNS             string
	isUpdate          bool // true when deployment_id was provided (in-place update)
	resolveResult     *deployment.ResolveResult
	varRefs           map[string]string // variable name → original account variable ref (before resolution)
}

func prepareDeployment(
	c *gin.Context,
	log *logger.Logger,
	submittedSpec *spec.AstroDeploymentSpec,
	accountStore *account.AccountStore,
	agentIndex *agentindex.Index,
	cfg *config.Config,
	deployStore *deploymentstore.Store,
	varsStore *accountvars.Store,
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

	// Reject org-scoped names (e.g. "@org/agent") — the CLI should strip these before pushing
	if strings.Contains(submittedSpec.Source.Name, "/") || strings.HasPrefix(submittedSpec.Source.Name, "@") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid source.name %q: must not contain @org/ prefix — re-push your agent with an updated CLI", submittedSpec.Source.Name),
		})
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
		AgentName:         sourceAgent.Name,
		Account:           sourceAcct.Name,
		ECRNamespace:      agentVersion.ECRNamespace,
		BuildID:           agentVersion.BuildID,
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

	// Resolve account variable refs before validation; capture the original refs
	// so they can be persisted and restored when building the prefilled template.
	var varRefs map[string]string
	if varsStore != nil {
		var refErr error
		varRefs, refErr = resolveVarReferences(c, log, submittedSpec, targetAcct.ID, varsStore, cfg)
		if refErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to resolve variable references",
				"details": refErr.Error(),
			})
			return nil, false
		}
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
		acct:              targetAcct,
		sourceAccountName: sourceAcct.Name,
		agentName:         agentName,
		displayName:       displayName,
		deploymentID:      deploymentID,
		buildID:           buildID,
		k8sNS:             k8sNamespace,
		isUpdate:          isUpdate,
		resolveResult:     resolveResult,
		varRefs:           varRefs,
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

// EnqueueUndeploy transitions a deployment to "undeploying" and inserts an
// async undeploy job. Used by both UndeployAgent and DeleteAccount.
func EnqueueUndeploy(ctx context.Context, deployStore *deploymentstore.Store, queue DeployQueue, deploymentID string) error {
	if err := deployStore.UpdateStatus(deploymentID, deploymentstore.StatusUndeploying, "", nil); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if err := queue.InsertUndeployJob(ctx, deploymentID); err != nil {
		return fmt.Errorf("insert undeploy job: %w", err)
	}
	return nil
}

func DeployAgent(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, varsStore *accountvars.Store, entCheck EntitlementChecker, queue DeployQueue, avatarStore *avatar.Store, omClient *openmeter.Client, db *sql.DB, auditStore *auditlog.Store) gin.HandlerFunc {
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

		dctx, ok := prepareDeployment(c, log, submittedSpec, accountStore, agentIndex, cfg, deployStore, varsStore)
		if !ok {
			return
		}

		// Check entitlements for deploy (account resolved from spec, not middleware)
		if entCheck != nil {
			if blocked, feature, entResult := entCheck.Check(c.Request.Context(), dctx.acct.ID, "agent_deployments", "compute"); blocked {
				c.JSON(http.StatusPaymentRequired, middleware.LimitResponse(feature, entResult))
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

		// Inject managed provider credentials into secret data.
		// These are platform-provided (e.g. anthropic-managed) and bypass user variables entirely.
		injectManagedCredentials(resolved, cfg)

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
				VarRefs:                dctx.varRefs,
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

		// Copy the blueprint's avatar to the new deployment (best-effort).
		if avatarStore != nil && !dctx.isUpdate {
			if _, copyErr := avatarStore.CopyAgentToDeployment(c.Request.Context(), dctx.sourceAccountName, dctx.agentName, dctx.deploymentID); copyErr != nil {
				log.Warn("Failed to copy blueprint avatar to deployment", "error", copyErr, "deployment_id", dctx.deploymentID)
			}
		}

		// Emit updated deployment count immediately so the next entitlement check
		// doesn't see stale OpenMeter data (heartbeat only runs every 5 minutes).
		if !dctx.isUpdate {
			go openmeter.EmitActiveDeployments(context.Background(), omClient, db, log, dctx.acct.ID)
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

		evt := auditlog.FromGinContext(c, dctx.acct.ID)
		evt.Action = auditlog.DeploymentDeploy
		evt.ResourceType = "deployment"
		evt.ResourceID = dctx.deploymentID
		evt.ResourceName = dctx.agentName
		evt.Description = "Deployed agent " + dctx.agentName
		evt.Metadata = map[string]any{"build_id": dctx.buildID, "namespace": dctx.k8sNS}
		auditStore.LogAsync(log, evt)

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

		dctx, ok := prepareDeployment(c, log, submittedSpec, accountStore, agentIndex, cfg, nil, nil)
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
func UndeployAgent(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, queue DeployQueue, omClient *openmeter.Client, db *sql.DB, auditStore *auditlog.Store) gin.HandlerFunc {
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
		if dep == nil || (dep.Status != deploymentstore.StatusActive && dep.Status != deploymentstore.StatusScaledDown && dep.Status != deploymentstore.StatusStopped && dep.Status != deploymentstore.StatusFailed) {
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
		if err := EnqueueUndeploy(c.Request.Context(), deployStore, queue, dep.ID); err != nil {
			log.Error("Failed to enqueue undeploy job", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule undeploy"})
			return
		}

		go openmeter.EmitActiveDeployments(context.Background(), omClient, db, log, dep.AccountID)

		log.Info("Undeploy queued",
			"deployment_id", dep.ID,
			"namespace", dep.Namespace,
		)

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.DeploymentUndeploy
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Undeployed agent " + dep.AgentName
		auditStore.LogAsync(log, evt)

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

// WorkloadDetail represents a k8s workload (Deployment, StatefulSet, etc.)
type WorkloadDetail struct {
	Name       string                `json:"name"`      // k8s resource name
	Kind       string                `json:"kind"`      // "Deployment" or "StatefulSet"
	Component  string                `json:"component"` // from app.kubernetes.io/component label
	Age        string                `json:"age"`
	PodName    string                `json:"pod_name,omitempty"` // name of the representative pod (for restarts)
	Containers []ContainerStatus     `json:"containers"`
	URLs       []ServiceEndpointInfo `json:"urls,omitempty"`
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
	ID                 string                `json:"id"`
	Name               string                `json:"name"`
	DisplayName        string                `json:"display_name,omitempty"`
	AvatarURL          string                `json:"avatar_url,omitempty"`
	BuildID            string                `json:"build_id"`
	Namespace          string                `json:"namespace"`
	Status             string                `json:"status"`
	Replicas           int32                 `json:"replicas"`
	Ready              int32                 `json:"ready"`
	CreatedAt          string                `json:"created_at"`
	UpdatedAt          string                `json:"updated_at,omitempty"`
	UpdatedBy          string                `json:"updated_by,omitempty"`
	Components         []string              `json:"components"`
	ManualIngestions   []string              `json:"manual_ingestions,omitempty"`
	ExternalURLs       []ServiceEndpointInfo `json:"external_urls,omitempty"`
	MessagingAvailable bool                  `json:"messaging_available,omitempty"`
	Workloads          []WorkloadDetail      `json:"workloads,omitempty"`
	Jobs               []JobDetail           `json:"jobs,omitempty"`
}

// CountDeployments returns a handler that returns the number of visible deployments for an account.
// This is a lightweight DB-only query with no K8s calls, suitable for skeleton rendering.
func CountDeployments(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store) gin.HandlerFunc {
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

		isMember, err := accountStore.IsMember(acct.ID, user.ID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
			return
		}

		count, err := deployStore.CountVisibleDeploymentsByAccount(acct.ID)
		if err != nil {
			log.Error("Failed to count deployments", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count deployments"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"count": count})
	}
}

// ListDeployments returns a handler for listing deployed agents
func ListDeployments(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store, agentIdx *agentindex.Index, avatarStore *avatar.Store, auditStore *auditlog.Store, cache k8scache.Cache) gin.HandlerFunc {
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

		// For each DB deployment, fetch live K8s status from its namespace in parallel.
		// Deployments without K8s resources (failed, pending) get a DB-only entry.
		// Results are pre-allocated by index to avoid a mutex; each slice is merged after.
		enriched := make([][]AgentDeployment, len(dbDeps))
		g, gctx := errgroup.WithContext(c.Request.Context())
		for i, dbDep := range dbDeps {
			g.Go(func() error {
				enriched[i] = enrichDeployment(gctx, log, k8sClient, deployStore, dbDep, listAstroDeploymentsLight, cache, k8scache.ListKeyPrefix, k8scache.ListTTL)
				return nil
			})
		}
		_ = g.Wait()

		var allDeployments []AgentDeployment
		for _, deps := range enriched {
			allDeployments = append(allDeployments, deps...)
		}

		// Resolve updated_at from the latest audit log entry per deployment.
		if auditStore != nil && len(allDeployments) > 0 {
			depIDs := make([]string, len(allDeployments))
			for i, d := range allDeployments {
				depIDs[i] = d.ID
			}
			latestMap, err := auditStore.LatestPerResource(c.Request.Context(), acct.ID, "deployment", depIDs)
			if err != nil {
				log.Warn("Failed to load audit timestamps for deployments", "error", err)
			} else {
				for i, d := range allDeployments {
					if latest, ok := latestMap[d.ID]; ok {
						allDeployments[i].UpdatedAt = latest.UpdatedAt.Format(time.RFC3339)
						allDeployments[i].UpdatedBy = latest.ActorID
					}
				}
			}
		}

		// Resolve avatar URLs for deployments that have their own custom avatar.
		if avatarStore != nil {
			dbDepByID := make(map[string]*deploymentstore.Deployment, len(dbDeps))
			for _, d := range dbDeps {
				dbDepByID[d.ID] = d
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"deployments": allDeployments,
			"count":       len(allDeployments),
		})
	}
}

// GetDeployment returns live K8s status for a single deployment.
// GET /api/v1/deployments/:id
func GetDeployment(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store, avatarStore *avatar.Store, auditStore *auditlog.Store, cache k8scache.Cache) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := middleware.GetUser(c); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return
		}

		dbDep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		deps := enrichDeployment(c.Request.Context(), log, k8sClient, deployStore, dbDep, listAstroDeployments, k8scache.NoopCache{}, "", 0)
		result := deps[0]
		// During a rolling build update the namespace contains workloads for both the old and
		// new build ID. Pick the entry matching the DB's current build so the client always
		// sees the new ingress URL, not the stale entry from the outgoing build.
		for _, dep := range deps {
			if dep.BuildID == dbDep.BuildID {
				result = dep
				break
			}
		}

		if auditStore != nil {
			latestMap, auditErr := auditStore.LatestPerResource(c.Request.Context(), dbDep.AccountID, "deployment", []string{dbDep.ID})
			if auditErr != nil {
				log.Warn("Failed to load audit timestamps for deployment", "error", auditErr)
			} else if latest, ok := latestMap[dbDep.ID]; ok {
				result.UpdatedAt = latest.UpdatedAt.Format(time.RFC3339)
				result.UpdatedBy = latest.ActorID
			}
		}

		// Check if the messaging ClusterIP service exists in K8s.
		// ExternalURLs only contains Ingress-exposed endpoints, so an internal-only
		// messaging sidecar would never appear there.
		messagingServiceName := deployment.GenerateAgentResourceName(dbDep.AgentName, "messaging")
		_, svcErr := k8sClient.Clientset().CoreV1().Services(dbDep.Namespace).Get(
			c.Request.Context(), messagingServiceName, metav1.GetOptions{},
		)
		result.MessagingAvailable = svcErr == nil

		if override := cfg.Deployment.MessagingURLOverride; override != "" {
			result.MessagingAvailable = true
			result.ExternalURLs = append(result.ExternalURLs, ServiceEndpointInfo{
				Name: "messaging", Type: "messaging", URL: override,
			})
		}

		c.JSON(http.StatusOK, gin.H{"deployment": result})
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
	case deploymentstore.StatusScaledDown, deploymentstore.StatusStopped:
		status = "Stopped"
	case deploymentstore.StatusUndeploying:
		status = "undeploying"
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

// enrichDeployment fetches live K8s state for a single DB deployment record and
// returns the resulting AgentDeployment entries (one per workload in the namespace).
// Falls back to a DB-only entry if the namespace is missing or K8s calls fail.
// When cache and keyPrefix are provided, a cache hit skips all K8s calls entirely.
// k8sListFn is the function signature shared by listAstroDeployments and listAstroDeploymentsLight.
type k8sListFn func(ctx context.Context, k8sClient k8s.ClusterClient, namespace string, manualIngestions []string) ([]AgentDeployment, error)

func enrichDeployment(ctx context.Context, log *logger.Logger, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store, dbDep *deploymentstore.Deployment, listFn k8sListFn, cache k8scache.Cache, keyPrefix string, cacheTTL time.Duration) []AgentDeployment {
	applyDBFields := func(deps []AgentDeployment, createdAt time.Time) {
		for i := range deps {
			deps[i].ID = dbDep.ID
			deps[i].Name = dbDep.AgentName
			deps[i].DisplayName = dbDep.DisplayName
			deps[i].CreatedAt = createdAt.Format(time.RFC3339)
			switch dbDep.Status {
			case deploymentstore.StatusPending, deploymentstore.StatusProvisioning:
				deps[i].Status = "pending"
			case deploymentstore.StatusUndeploying:
				deps[i].Status = "undeploying"
			}
		}
	}

	// Check cache before any DB or K8s calls. On a hit, use dbDep.DeployedAt for
	// CreatedAt to avoid the GetDeploymentFirstEventAt round-trip.
	cacheKey := keyPrefix + dbDep.Namespace
	if data, ok := cache.Get(ctx, cacheKey); ok {
		var deps []AgentDeployment
		if err := json.Unmarshal(data, &deps); err == nil && len(deps) > 0 {
			applyDBFields(deps, dbDep.DeployedAt)
			return deps
		}
	}

	// Cache miss: fetch firstSeenAt from DB for accurate CreatedAt.
	firstSeenAt := dbDep.DeployedAt
	if firstEventAt, evErr := deployStore.GetDeploymentFirstEventAt(dbDep.ID); evErr != nil {
		log.Warn("Failed to load first deployment event", "error", evErr, "deployment_id", dbDep.ID)
	} else if firstEventAt != nil {
		firstSeenAt = *firstEventAt
	}

	dbOnly := func() []AgentDeployment {
		entry := agentDeploymentFromDB(dbDep)
		entry.CreatedAt = firstSeenAt.Format(time.RFC3339)
		return []AgentDeployment{entry}
	}

	ns, nsErr := k8sClient.Clientset().CoreV1().Namespaces().Get(ctx, dbDep.Namespace, metav1.GetOptions{})
	if nsErr != nil || ns.DeletionTimestamp != nil {
		return dbOnly()
	}

	manualIngestions := parseManualIngestions(ns.Annotations)
	deps, k8sErr := listFn(ctx, k8sClient, dbDep.Namespace, manualIngestions)
	if k8sErr != nil || len(deps) == 0 {
		return dbOnly()
	}

	if data, err := json.Marshal(deps); err == nil {
		_ = cache.Set(ctx, cacheKey, data, cacheTTL)
	}

	applyDBFields(deps, firstSeenAt)
	return deps
}

// parseManualIngestions reads the "astro.dev/manual-ingestions" annotation from a namespace.
func parseManualIngestions(annotations map[string]string) []string {
	val := annotations["astro.dev/manual-ingestions"]
	if val == "" {
		return nil
	}
	return strings.Split(val, ",")
}

func deploymentReadinessStatus(replicas, readyReplicas int32) string {
	status := "Running"
	if readyReplicas < replicas {
		status = "Pending"
	}
	if replicas == 0 {
		status = "Stopped"
	}
	return status
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

	// List StatefulSets
	statefulSetList, err := clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list statefulsets: %w", err)
	}

	// List ingresses for the namespace to get external URLs
	ingressList, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		// Ingress listing failure is not critical, log and continue
		ingressList = nil
	}

	// List pods for the namespace (needed for container runtime status)
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Build maps of URLs from ingresses — both per-agent and per-workload (component).
	agentExternalURLs := make(map[string][]ServiceEndpointInfo) // key: "agent:version"
	workloadURLs := make(map[string][]ServiceEndpointInfo)      // key: "agent:version:component"
	if ingressList != nil {
		for _, ing := range ingressList.Items {
			agentKey := ing.Labels[deployment.LabelKeyAgent]
			version := ing.Labels["app.kubernetes.io/version"]
			component := ing.Labels["app.kubernetes.io/component"]

			if agentKey == "" || len(ing.Spec.Rules) == 0 {
				continue
			}
			for _, rule := range ing.Spec.Rules {
				if rule.Host == "" {
					continue
				}
				ep := ServiceEndpointInfo{
					Name: component,
					URL:  fmt.Sprintf("https://%s", rule.Host),
					Type: component,
				}
				agentKey := agentKey + ":" + version
				agentExternalURLs[agentKey] = append(agentExternalURLs[agentKey], ep)
				wlKey := agentKey + ":" + component
				workloadURLs[wlKey] = append(workloadURLs[wlKey], ep)
			}
		}
	}

	// Group deployments by agent name
	agentDeployments := make(map[string]*AgentDeployment)
	agentStatusFromPrimary := make(map[string]bool)

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
			info = &AgentDeployment{
				BuildID:          version,
				Namespace:        namespace,
				Status:           deploymentReadinessStatus(dep.Status.Replicas, dep.Status.ReadyReplicas),
				Replicas:         dep.Status.Replicas,
				Ready:            dep.Status.ReadyReplicas,
				CreatedAt:        dep.CreationTimestamp.Format(time.RFC3339),
				Components:       []string{},
				ManualIngestions: manualIngestions,
			}
			if urls, ok := agentExternalURLs[key]; ok {
				info.ExternalURLs = urls
			}
			agentDeployments[key] = info
		}

		// "agent" is the source of truth for deployment readiness. Other components
		// (collector, ingestion, etc.) can have independent readiness without blocking
		// the top-level deployment status.
		isPrimary := component == "agent" || component == ""
		if isPrimary || !agentStatusFromPrimary[key] {
			info.Status = deploymentReadinessStatus(dep.Status.Replicas, dep.Status.ReadyReplicas)
			info.Replicas = dep.Status.Replicas
			info.Ready = dep.Status.ReadyReplicas
			info.CreatedAt = dep.CreationTimestamp.Format(time.RFC3339)
			if isPrimary {
				agentStatusFromPrimary[key] = true
			}
		}

		if component != "" {
			info.Components = append(info.Components, component)
		}

		wl := WorkloadDetail{
			Name:      dep.Name,
			Kind:      "Deployment",
			Component: component,
			Age:       formatAge(dep.CreationTimestamp.Time),
		}
		if urls, ok := workloadURLs[key+":"+component]; ok {
			wl.URLs = urls
		}
		info.Workloads = append(info.Workloads, wl)
	}

	for _, sts := range statefulSetList.Items {
		agentKey := sts.Labels[deployment.LabelKeyAgent]
		version := sts.Labels["app.kubernetes.io/version"]
		component := sts.Labels["app.kubernetes.io/component"]
		if agentKey == "" {
			continue
		}

		key := agentKey + ":" + version
		info, exists := agentDeployments[key]
		if !exists {
			info = &AgentDeployment{
				BuildID:          version,
				Namespace:        namespace,
				Status:           deploymentReadinessStatus(sts.Status.Replicas, sts.Status.ReadyReplicas),
				Replicas:         sts.Status.Replicas,
				Ready:            sts.Status.ReadyReplicas,
				CreatedAt:        sts.CreationTimestamp.Format(time.RFC3339),
				Components:       []string{},
				ManualIngestions: manualIngestions,
			}
			if urls, ok := agentExternalURLs[key]; ok {
				info.ExternalURLs = urls
			}
			agentDeployments[key] = info
		}

		if component != "" {
			info.Components = append(info.Components, component)
		}

		wl := WorkloadDetail{
			Name:      sts.Name,
			Kind:      "StatefulSet",
			Component: component,
			Age:       formatAge(sts.CreationTimestamp.Time),
		}
		if urls, ok := workloadURLs[key+":"+component]; ok {
			wl.URLs = urls
		}
		info.Workloads = append(info.Workloads, wl)
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

	// Index pods by agent key + component for matching to workloads.
	// Key: "agentKey:version:component" → best pod (prefer Running phase, then newest).
	type podKey struct{ agent, version, component string }
	bestPod := make(map[podKey]corev1.Pod)
	for _, pod := range podList.Items {
		agentKey := pod.Labels[deployment.LabelKeyAgent]
		version := pod.Labels["app.kubernetes.io/version"]
		component := pod.Labels["app.kubernetes.io/component"]
		if agentKey == "" {
			continue
		}
		pk := podKey{agentKey, version, component}
		existing, ok := bestPod[pk]
		if !ok {
			bestPod[pk] = pod
			continue
		}
		// Prefer Running pods; among same phase prefer newest
		if pod.Status.Phase == corev1.PodRunning && existing.Status.Phase != corev1.PodRunning {
			bestPod[pk] = pod
		} else if pod.Status.Phase == existing.Status.Phase && pod.CreationTimestamp.After(existing.CreationTimestamp.Time) {
			bestPod[pk] = pod
		}
	}

	// Attach container statuses from best pod to each workload
	for key, info := range agentDeployments {
		parts := strings.SplitN(key, ":", 2)
		agentKey := parts[0]
		version := ""
		if len(parts) > 1 {
			version = parts[1]
		}
		for i := range info.Workloads {
			wl := &info.Workloads[i]
			pk := podKey{agentKey, version, wl.Component}
			pod, ok := bestPod[pk]
			if !ok {
				continue
			}
			wl.Containers = buildContainerStatuses(ctx, clientset, pod)
			wl.PodName = pod.Name
		}
	}

	// Convert map to slice
	result := make([]AgentDeployment, 0, len(agentDeployments))
	for _, info := range agentDeployments {
		result = append(result, *info)
	}

	return result, nil
}

// listAstroDeploymentsLight fetches only Deployments and StatefulSets for a namespace,
// skipping ingresses, pods, and jobs. Returns status, replicas, ready, and components.
func listAstroDeploymentsLight(ctx context.Context, k8sClient k8s.ClusterClient, namespace string, _ []string) ([]AgentDeployment, error) {
	clientset := k8sClient.Clientset()
	labelSelector := "app.kubernetes.io/managed-by=astro-server"

	deploymentList, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	statefulSetList, err := clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list statefulsets: %w", err)
	}

	agentDeployments := make(map[string]*AgentDeployment)
	agentStatusFromPrimary := make(map[string]bool)

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
			info = &AgentDeployment{
				BuildID:    version,
				Namespace:  namespace,
				Status:     deploymentReadinessStatus(dep.Status.Replicas, dep.Status.ReadyReplicas),
				Replicas:   dep.Status.Replicas,
				Ready:      dep.Status.ReadyReplicas,
				CreatedAt:  dep.CreationTimestamp.Format(time.RFC3339),
				Components: []string{},
			}
			agentDeployments[key] = info
		}

		isPrimary := component == "agent" || component == ""
		if isPrimary || !agentStatusFromPrimary[key] {
			info.Status = deploymentReadinessStatus(dep.Status.Replicas, dep.Status.ReadyReplicas)
			info.Replicas = dep.Status.Replicas
			info.Ready = dep.Status.ReadyReplicas
			info.CreatedAt = dep.CreationTimestamp.Format(time.RFC3339)
			if isPrimary {
				agentStatusFromPrimary[key] = true
			}
		}

		if component != "" {
			info.Components = append(info.Components, component)
		}
	}

	for _, sts := range statefulSetList.Items {
		agentKey := sts.Labels[deployment.LabelKeyAgent]
		version := sts.Labels["app.kubernetes.io/version"]
		component := sts.Labels["app.kubernetes.io/component"]
		if agentKey == "" {
			continue
		}

		key := agentKey + ":" + version
		info, exists := agentDeployments[key]
		if !exists {
			info = &AgentDeployment{
				BuildID:    version,
				Namespace:  namespace,
				Status:     deploymentReadinessStatus(sts.Status.Replicas, sts.Status.ReadyReplicas),
				Replicas:   sts.Status.Replicas,
				Ready:      sts.Status.ReadyReplicas,
				CreatedAt:  sts.CreationTimestamp.Format(time.RFC3339),
				Components: []string{},
			}
			agentDeployments[key] = info
		}

		if component != "" {
			info.Components = append(info.Components, component)
		}
	}

	result := make([]AgentDeployment, 0, len(agentDeployments))
	for _, info := range agentDeployments {
		result = append(result, *info)
	}
	return result, nil
}

// buildContainerStatuses extracts container statuses and env vars from a k8s pod.
// It resolves envFrom references (ConfigMaps and Secrets) into individual key-value pairs.
func buildContainerStatuses(ctx context.Context, clientset kubernetes.Interface, pod corev1.Pod) []ContainerStatus {
	ns := pod.Namespace

	// Cache resolved ConfigMaps and Secrets to avoid duplicate fetches.
	cmCache := map[string]map[string]string{}
	secCache := map[string]map[string]string{}

	resolveConfigMap := func(name string) map[string]string {
		if data, ok := cmCache[name]; ok {
			return data
		}
		cm, err := clientset.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			cmCache[name] = nil
			return nil
		}
		cmCache[name] = cm.Data
		return cm.Data
	}

	// Only resolve secret keys — never store or return secret values.
	resolveSecretKeys := func(name string) []string {
		if keys, ok := secCache[name]; ok {
			if keys == nil {
				return nil
			}
			result := make([]string, 0, len(keys))
			for k := range keys {
				result = append(result, k)
			}
			return result
		}
		sec, err := clientset.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			secCache[name] = nil
			return nil
		}
		redacted := make(map[string]string, len(sec.Data))
		for k := range sec.Data {
			redacted[k] = ""
		}
		secCache[name] = redacted
		result := make([]string, 0, len(redacted))
		for k := range redacted {
			result = append(result, k)
		}
		return result
	}

	// Build a map of spec containers (regular + init) for env var lookup
	specContainers := map[string][]EnvVar{}
	for _, sc := range append(pod.Spec.Containers, pod.Spec.InitContainers...) {
		var envVars []EnvVar

		// Resolve envFrom references into individual key-value pairs
		for _, ef := range sc.EnvFrom {
			if ef.ConfigMapRef != nil {
				data := resolveConfigMap(ef.ConfigMapRef.Name)
				for k, v := range data {
					envVars = append(envVars, EnvVar{
						Name:  ef.Prefix + k,
						Value: v,
						From:  "configmap:" + ef.ConfigMapRef.Name,
					})
				}
			}
			if ef.SecretRef != nil {
				keys := resolveSecretKeys(ef.SecretRef.Name)
				for _, k := range keys {
					envVars = append(envVars, EnvVar{
						Name:  ef.Prefix + k,
						Value: "••••••••",
						From:  "secret:" + ef.SecretRef.Name,
					})
				}
			}
		}

		for _, e := range sc.Env {
			ev := EnvVar{Name: e.Name}
			if e.ValueFrom != nil {
				switch {
				case e.ValueFrom.SecretKeyRef != nil:
					ev.Value = "••••••••"
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

	var containers []ContainerStatus
	for _, cs := range append(pod.Status.ContainerStatuses, pod.Status.InitContainerStatuses...) {
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
		containers = append(containers, container)
	}
	return containers
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

// RestartDeployment triggers a rolling restart of all Deployments and StatefulSets for an agent
// by patching spec.template.metadata.annotations with kubectl.kubernetes.io/restartedAt.
// Kubernetes performs a rolling update: new pod starts and becomes ready before the old one is terminated.
// POST /api/v1/deployments/:id/restart
func RestartDeployment(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
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

		acct, acctErr := accountStore.GetByID(dep.AccountID)
		if acctErr != nil || acct == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve account"})
			return
		}

		labelSelector := fmt.Sprintf("app.kubernetes.io/managed-by=astro-server,%s=%s,app.kubernetes.io/version=%s",
			deployment.LabelKeyAgent, deployment.AgentLabelValue(acct.Name, dep.AgentName), dep.BuildID)

		// Patch payload: setting restartedAt on the pod template triggers a rolling update
		// without changing any spec — Kubernetes detects the annotation change and cycles pods one at a time.
		patchPayload, _ := json.Marshal(map[string]any{
			"spec": map[string]any{
				"template": map[string]any{
					"metadata": map[string]any{
						"annotations": map[string]string{
							"kubectl.kubernetes.io/restartedAt": time.Now().UTC().Format(time.RFC3339),
						},
					},
				},
			},
		})

		clientset := k8sClient.Clientset()
		ctx := c.Request.Context()
		var restarted []string

		depList, err := clientset.AppsV1().Deployments(dep.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			log.Error("Failed to list deployments for restart", "error", err, "namespace", dep.Namespace)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workloads", "details": err.Error()})
			return
		}
		for _, d := range depList.Items {
			log.Info("Initiating rolling restart for Deployment",
				"deployment", d.Name,
				"namespace", dep.Namespace,
				"replicas", d.Spec.Replicas,
				"ready", d.Status.ReadyReplicas,
				"user", user.ID,
			)
			if _, patchErr := clientset.AppsV1().Deployments(dep.Namespace).Patch(ctx, d.Name, k8stypes.StrategicMergePatchType, patchPayload, metav1.PatchOptions{}); patchErr != nil {
				log.Error("Failed to patch Deployment for rolling restart", "error", patchErr, "deployment", d.Name)
				continue
			}
			restarted = append(restarted, d.Name)
			log.Info("Rolling restart patch applied — K8s will start new pod before terminating old",
				"deployment", d.Name,
				"namespace", dep.Namespace,
				"strategy", "RollingUpdate",
				"annotation", "kubectl.kubernetes.io/restartedAt",
			)
		}

		stsList, err := clientset.AppsV1().StatefulSets(dep.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			log.Warn("Failed to list StatefulSets for restart", "error", err, "namespace", dep.Namespace)
		} else {
			for _, ss := range stsList.Items {
				log.Info("Initiating rolling restart for StatefulSet",
					"statefulset", ss.Name,
					"namespace", dep.Namespace,
					"replicas", ss.Spec.Replicas,
					"ready", ss.Status.ReadyReplicas,
					"user", user.ID,
				)
				if _, patchErr := clientset.AppsV1().StatefulSets(dep.Namespace).Patch(ctx, ss.Name, k8stypes.StrategicMergePatchType, patchPayload, metav1.PatchOptions{}); patchErr != nil {
					log.Error("Failed to patch StatefulSet for rolling restart", "error", patchErr, "statefulset", ss.Name)
					continue
				}
				restarted = append(restarted, ss.Name)
				log.Info("Rolling restart patch applied — K8s will start new pod before terminating old",
					"statefulset", ss.Name,
					"namespace", dep.Namespace,
					"strategy", "RollingUpdate",
					"annotation", "kubectl.kubernetes.io/restartedAt",
				)
			}
		}

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.DeploymentRestart
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = fmt.Sprintf("Rolling restart triggered for %d workload(s)", len(restarted))
		evt.Metadata = map[string]any{"workloads": restarted, "namespace": dep.Namespace}
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, &RestartDeploymentResponse{Status: "restarting", Pods: restarted})
	}
}

// RestartPod deletes a pod in a deployment's namespace, causing Kubernetes to recreate it.
// POST /api/v1/deployments/:id/pods/:pod/restart
func RestartPod(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store, auditStore *auditlog.Store) gin.HandlerFunc {
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

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.DeploymentRestartPod
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Restarted pod " + podName
		evt.Metadata = map[string]any{"pod": podName, "namespace": dep.Namespace}
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, gin.H{"status": "restarting", "pod": podName})
	}
}

// GetDeploymentEvents returns Kubernetes events for a deployment's namespace.
func GetDeploymentEvents(log *logger.Logger, accountStore *account.AccountStore, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store, cache k8scache.Cache) gin.HandlerFunc {
	const cachePrefix = "astro:k8s:events:"
	const cacheTTL = 10 * time.Minute

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

		ctx := c.Request.Context()
		cacheKey := cachePrefix + dep.Namespace
		transitional := dep.Status == deploymentstore.StatusPending ||
			dep.Status == deploymentstore.StatusProvisioning ||
			dep.Status == deploymentstore.StatusUndeploying

		if !transitional {
			if data, ok := cache.Get(ctx, cacheKey); ok {
				var resp DeploymentEventsResponse
				if err := json.Unmarshal(data, &resp); err == nil {
					c.JSON(http.StatusOK, resp)
					return
				}
			}
		}

		eventList, err := k8sClient.Clientset().CoreV1().Events(dep.Namespace).List(ctx, metav1.ListOptions{Limit: 200})
		if err != nil {
			log.Error("Failed to list events", "error", err, "namespace", dep.Namespace)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list kubernetes events"})
			return
		}

		items := make([]K8sEventItem, 0, len(eventList.Items))
		for _, evt := range eventList.Items {
			lastTS := evt.LastTimestamp.Time
			if lastTS.IsZero() {
				lastTS = evt.EventTime.Time
			}
			if lastTS.IsZero() {
				lastTS = evt.CreationTimestamp.Time
			}
			firstTS := evt.FirstTimestamp.Time
			if firstTS.IsZero() {
				firstTS = lastTS
			}
			items = append(items, K8sEventItem{
				Type:           evt.Type,
				Reason:         evt.Reason,
				Message:        evt.Message,
				ObjectKind:     evt.InvolvedObject.Kind,
				ObjectName:     evt.InvolvedObject.Name,
				Count:          evt.Count,
				FirstTimestamp: firstTS.UTC().Format(time.RFC3339),
				LastTimestamp:  lastTS.UTC().Format(time.RFC3339),
			})
		}

		sort.Slice(items, func(i, j int) bool {
			return items[i].LastTimestamp > items[j].LastTimestamp
		})

		resp := DeploymentEventsResponse{Events: items}
		if data, err := json.Marshal(resp); err == nil {
			_ = cache.Set(ctx, cacheKey, data, cacheTTL)
		}
		c.JSON(http.StatusOK, resp)
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
		workloadName := c.Query("workload")
		containerName := c.Query("container")

		tailLines := int64(200)
		if tl := c.Query("tailLines"); tl != "" {
			if parsed, err := strconv.ParseInt(tl, 10, 64); err == nil && parsed > 0 {
				tailLines = parsed
			}
		}

		// Build Loki params (includes optional time range from query string).
		lokiParams := loki.QueryParams{
			Namespace: dep.Namespace,
			Pod:       podName,
			Workload:  workloadName,
			Container: containerName,
			Limit:     tailLines,
		}
		if s := c.Query("since"); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				lokiParams.Start = t
			} else if ns, err := strconv.ParseInt(s, 10, 64); err == nil {
				lokiParams.Start = time.Unix(0, ns)
			}
		}
		if u := c.Query("until"); u != "" {
			if t, err := time.Parse(time.RFC3339, u); err == nil {
				lokiParams.End = t
			} else if ns, err := strconv.ParseInt(u, 10, 64); err == nil {
				lokiParams.End = time.Unix(0, ns)
			}
		}

		// For the K8s fallback, resolve pod name from workload if needed.
		if lokiClient == nil && podName == "" && workloadName != "" {
			resolved, listErr := resolvePodForStream(c.Request.Context(), k8sClient, dep.Namespace, workloadName, containerName)
			if listErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list pods", "details": listErr.Error()})
				return
			}
			podName = resolved
		}

		if lokiClient == nil && podName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pod or workload query parameter is required"})
			return
		}

		logOpts := &corev1.PodLogOptions{TailLines: &tailLines, Timestamps: true}
		if containerName != "" {
			logOpts.Container = containerName
		}

		streamLogs(c, log, lokiClient, lokiParams, k8sClient, dep.Namespace, podName, logOpts)
	}
}

// resolvePodForStream finds a running pod for the given workload in the namespace.
// It prefers a Running pod; falls back to any pod with the right prefix and container.
func resolvePodForStream(ctx context.Context, k8sClient k8s.ClusterClient, namespace, workloadName, containerName string) (string, error) {
	pods, err := k8sClient.Clientset().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if err != nil {
		return "", err
	}
	prefix := workloadName + "-"
	var podName string
	for _, p := range pods.Items {
		if !strings.HasPrefix(p.Name, prefix) {
			continue
		}
		if containerName != "" {
			found := false
			for _, cs := range append(p.Status.ContainerStatuses, p.Status.InitContainerStatuses...) {
				if cs.Name == containerName {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if podName == "" || p.Status.Phase == corev1.PodRunning {
			podName = p.Name
		}
		if p.Status.Phase == corev1.PodRunning {
			break
		}
	}
	return podName, nil
}

// StreamDeploymentLogs streams log lines for a deployment workload as Server-Sent Events.
// heartbeatInterval overrides the 5s default keepalive cadence (useful in tests).
func StreamDeploymentLogs(log *logger.Logger, accountStore *account.AccountStore, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store, lokiClient *loki.Client, heartbeatInterval ...time.Duration) gin.HandlerFunc {
	hbInterval := 5 * time.Second
	if len(heartbeatInterval) > 0 && heartbeatInterval[0] > 0 {
		hbInterval = heartbeatInterval[0]
	}
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

		workloadName := c.Query("workload")
		containerName := c.Query("container")
		podName := c.Query("pod")
		backend := "none"
		if lokiClient != nil {
			backend = "loki"
		} else if k8sClient != nil {
			backend = "k8s"
		}

		log.Debug("SSE stream requested",
			"deployment", dep.ID, "namespace", dep.Namespace,
			"workload", workloadName, "container", containerName, "pod", podName,
			"backend", backend)

		if lokiClient == nil && k8sClient == nil {
			log.Warn("SSE stream rejected: no log backend configured", "deployment", dep.ID)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "log backend not configured"})
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		// Disable the per-connection write deadline for streaming responses.
		// The server-level WriteTimeout would otherwise kill the SSE connection
		// after the configured timeout (default 10s).
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})

		flusher := c.Writer.(http.Flusher)

		// Send the ready event immediately so the browser confirms the SSE
		// handshake before we block on Loki dial or K8s pod resolution.
		fmt.Fprintf(c.Writer, "event: ready\ndata: {}\n\n") //nolint:errcheck
		flusher.Flush()
		log.Debug("SSE ready event sent", "deployment", dep.ID)

		writeEvent := func(ll loki.LogLine) bool {
			payload, _ := json.Marshal(lokiLineToEntry(ll))
			_, writeErr := fmt.Fprintf(c.Writer, "id: %d\ndata: %s\n\n", ll.Timestamp.UnixNano(), payload)
			if writeErr != nil {
				log.Debug("SSE write failed, client likely disconnected", "deployment", dep.ID, "error", writeErr)
				return false
			}
			flusher.Flush()
			return true
		}

		writeStatusEvent := func(status string) bool {
			_, err := fmt.Fprintf(c.Writer, "event: status\ndata: {\"status\":%q}\n\n", status)
			if err != nil {
				log.Debug("SSE status write failed, client likely disconnected", "deployment", dep.ID, "error", err)
				return false
			}
			flusher.Flush()
			return true
		}

		writeErrorEvent := func(message string) {
			log.Debug("SSE sending error event", "deployment", dep.ID, "message", message)
			_, err := fmt.Fprintf(c.Writer, "event: error\ndata: {\"message\":%q}\n\n", message)
			if err != nil {
				log.Debug("SSE error write failed, client likely disconnected", "deployment", dep.ID, "error", err)
				return
			}
			flusher.Flush()
		}

		writeHeartbeat := func() bool {
			_, err := fmt.Fprintf(c.Writer, "event: heartbeat\ndata: {}\n\n")
			if err != nil {
				log.Debug("SSE heartbeat write failed, client likely disconnected", "deployment", dep.ID, "error", err)
				return false
			}
			flusher.Flush()
			return true
		}

		// reconnectPause waits 500ms before the next Loki dial attempt.
		// Returns false if the client disconnected during the wait.
		reconnectPause := func() bool {
			select {
			case <-time.After(500 * time.Millisecond):
				return true
			case <-c.Request.Context().Done():
				log.Debug("SSE client disconnected during Loki reconnect backoff", "deployment", dep.ID)
				return false
			}
		}

		if lokiClient != nil {
			// Loki's tail WebSocket closes periodically. Reconnect server-side so the
			// SSE connection stays open.
			heartbeat := time.NewTicker(hbInterval)
			defer heartbeat.Stop()
			firstConnect := true
			connectCount := 0

			for {
				connectCount++
				if !writeStatusEvent("connecting") {
					return
				}
				log.Debug("Loki tail dialing", "deployment", dep.ID, "attempt", connectCount,
					"namespace", dep.Namespace, "workload", workloadName, "container", containerName)

				ch, tailErr := lokiClient.TailLogs(c.Request.Context(), loki.QueryParams{
					Namespace: dep.Namespace,
					Pod:       podName,
					Workload:  workloadName,
					Container: containerName,
					Start:     time.Now(),
				})
				if tailErr != nil {
					if firstConnect {
						log.Error("Loki tail initial dial failed", "error", tailErr,
							"deployment", dep.ID, "namespace", dep.Namespace)
						writeErrorEvent("failed to connect to log stream")
						return
					}
					log.Warn("Loki tail reconnect failed, retrying", "error", tailErr,
						"deployment", dep.ID, "attempt", connectCount)
					if !reconnectPause() {
						return
					}
					continue
				}
				firstConnect = false
				if !writeStatusEvent("streaming") {
					return
				}
				log.Debug("Loki tail connected", "deployment", dep.ID, "attempt", connectCount)

			inner:
				for {
					select {
					case ll, ok := <-ch:
						if !ok {
							log.Debug("Loki tail channel closed, will reconnect", "deployment", dep.ID)
							if !writeStatusEvent("reconnecting") {
								return
							}
							break inner
						}
						if !writeEvent(ll) {
							return
						}
					case <-heartbeat.C:
						if !writeHeartbeat() {
							return
						}
					case <-c.Request.Context().Done():
						log.Debug("SSE client disconnected", "deployment", dep.ID)
						return
					}
				}

				if !reconnectPause() {
					return
				}
			}
		}

		// K8s fallback: resolve pod then stream with Follow=true.
		if podName == "" && workloadName != "" {
			log.Debug("Resolving pod for K8s stream", "deployment", dep.ID,
				"namespace", dep.Namespace, "workload", workloadName, "container", containerName)
			resolved, resolveErr := resolvePodForStream(c.Request.Context(), k8sClient, dep.Namespace, workloadName, containerName)
			if resolveErr != nil {
				log.Error("Failed to list pods for stream", "error", resolveErr,
					"deployment", dep.ID, "namespace", dep.Namespace, "workload", workloadName)
				writeErrorEvent("failed to list pods")
				return
			}
			podName = resolved
			log.Debug("Resolved pod for K8s stream", "deployment", dep.ID, "pod", podName)
		}

		if podName == "" {
			log.Warn("SSE stream rejected: no pod or workload specified", "deployment", dep.ID)
			writeErrorEvent("pod or workload required")
			return
		}

		sinceTime := metav1.NewTime(time.Now())
		logOpts := &corev1.PodLogOptions{Follow: true, SinceTime: &sinceTime, Timestamps: true}
		if containerName != "" {
			logOpts.Container = containerName
		}

		if !writeStatusEvent("connecting") {
			return
		}
		log.Debug("K8s pod log stream starting", "deployment", dep.ID, "namespace", dep.Namespace, "pod", podName)
		stream, streamErr := k8sClient.Clientset().CoreV1().Pods(dep.Namespace).GetLogs(podName, logOpts).Stream(c.Request.Context())
		if streamErr != nil {
			log.Error("Failed to stream pod logs", "error", streamErr,
				"deployment", dep.ID, "namespace", dep.Namespace, "pod", podName)
			writeErrorEvent("failed to stream pod logs")
			return
		}
		defer stream.Close() //nolint:errcheck
		if !writeStatusEvent("streaming") {
			return
		}
		log.Debug("K8s pod log stream connected", "deployment", dep.ID, "pod", podName)

		// Pipe scanner into a channel so we can select with the heartbeat ticker.
		lines := make(chan loki.LogLine)
		go func() {
			defer close(lines)
			scanner := bufio.NewScanner(stream)
			for scanner.Scan() {
				line := scanner.Text()
				ts := time.Now()
				msg := line
				if idx := strings.IndexByte(line, ' '); idx > 0 {
					if t, parseErr := time.Parse(time.RFC3339Nano, line[:idx]); parseErr == nil {
						ts = t
						msg = line[idx+1:]
					}
				}
				select {
				case lines <- loki.LogLine{Timestamp: ts, Line: msg}:
				case <-c.Request.Context().Done():
					return
				}
			}
			if err := scanner.Err(); err != nil {
				log.Debug("K8s pod log scanner error", "deployment", dep.ID, "pod", podName, "error", err)
			} else {
				log.Debug("K8s pod log stream ended", "deployment", dep.ID, "pod", podName)
			}
		}()

		heartbeat := time.NewTicker(hbInterval)
		defer heartbeat.Stop()

		for {
			select {
			case ll, ok := <-lines:
				if !ok {
					log.Debug("K8s log channel closed, ending SSE stream", "deployment", dep.ID, "pod", podName)
					return
				}
				if !writeEvent(ll) {
					return
				}
			case <-heartbeat.C:
				if !writeHeartbeat() {
					return
				}
			case <-c.Request.Context().Done():
				log.Debug("SSE client disconnected", "deployment", dep.ID)
				return
			}
		}
	}
}

// generateTemplate resolves an agent build and generates a deployment template.
// Shared by GetDeploymentTemplate and GetPrefilledDeploymentTemplate.
// buildIDOverride, when non-empty, pins the build instead of reading ?build= or using latest.
func generateTemplate(
	c *gin.Context,
	log *logger.Logger,
	agentIndex *agentindex.Index,
	accountStore *account.AccountStore,
	cfg *config.Config,
	buildIDOverride string,
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
	if buildIDOverride != "" {
		agentVersion, err = agentIndex.GetVersion(accountID, name, buildIDOverride)
	} else if buildParam := c.Query("build"); buildParam != "" {
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
		AgentName:         agent.Name,
		Account:           acct.Name,
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
		template, ok := generateTemplate(c, log, agentIndex, accountStore, cfg, "")
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

		// Look up existing deployment first so we can pin the build to what is currently deployed.
		// The config panel must always redeploy the same build — upgrading to a newer build is
		// only possible via the "new build available" banner.
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

		// Resolve the revision first (if requested) so we can use its build_id for template generation.
		specJSONToMerge := existing.DeploymentSpecJSON
		buildIDForTemplate := existing.BuildID
		revisionRequested := false
		// Allow the client to override the build ID (e.g. for new-build upgrades from the CTA).
		if buildOverride := c.Query("build"); buildOverride != "" {
			buildIDForTemplate = buildOverride
		}
		if revisionStr := c.Query("revision"); revisionStr != "" {
			revNum, convErr := strconv.Atoi(revisionStr)
			if convErr != nil || revNum < 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid revision"})
				return
			}
			rev, revErr := deployStore.GetRevisionByNumber(deploymentID, revNum)
			if revErr != nil {
				log.Error("Failed to get revision", "error", revErr, "deployment_id", deploymentID, "revision", revNum)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get revision"})
				return
			}
			if rev == nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "revision not found"})
				return
			}
			specJSONToMerge = string(rev.SpecJSON)
			buildIDForTemplate = rev.BuildID
			revisionRequested = true
		}

		template, ok := generateTemplate(c, log, agentIndex, accountStore, cfg, buildIDForTemplate)
		if !ok {
			return
		}

		// Get stored variables
		storedVars, err := deployStore.GetDeploymentVariables(deploymentID)
		if err != nil {
			log.Error("Failed to get deployment variables", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get deployment variables"})
			return
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
				if sv.Ref != "" {
					// Variable was originally set via an account variable reference —
					// restore the ref so the UI shows which account variable was selected.
					// Never return the resolved value regardless of whether it's secret.
					tv.Ref = sv.Ref
				} else if !sv.Secret {
					// Non-secret direct value: safe to return as-is.
					tv.Value = sv.Value
				}
				// Secret with no ref: leave value empty — never expose plaintext secrets.
				template.Variables[sv.Name] = tv
			}
		}

		// Merge adapters, ingestion schedules, and (for historical revisions) display name from stored spec
		if specJSONToMerge != "" {
			var storedSpec spec.AstroDeploymentSpec
			if jsonErr := json.Unmarshal([]byte(specJSONToMerge), &storedSpec); jsonErr == nil {
				if storedSpec.Interfaces != nil && template.Interfaces != nil {
					template.Interfaces.Adapters = storedSpec.Interfaces.Adapters
					template.Interfaces.Auth = storedSpec.Interfaces.Auth
				}
				for name, storedIng := range storedSpec.Ingestion {
					if tmplIng, ok := template.Ingestion[name]; ok && storedIng.Trigger.Schedule != "" {
						tmplIng.Trigger.Schedule = storedIng.Trigger.Schedule
						template.Ingestion[name] = tmplIng
					}
				}
				if revisionRequested && storedSpec.Target.DisplayName != "" {
					template.Target.DisplayName = storedSpec.Target.DisplayName
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

		history, err := deployStore.GetDeploymentHistoryByRevisions(acct.ID, agentName)
		if err != nil {
			log.Error("Failed to get deployment history", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get deployment history"})
			return
		}

		if deploymentID := c.Query("deployment_id"); deploymentID != "" {
			filtered := make([]deploymentstore.RevisionHistoryRecord, 0)
			for _, r := range history {
				if r.DeploymentID == deploymentID {
					filtered = append(filtered, r)
				}
			}
			history = filtered
		}

		type revisionRecord struct {
			ID          string    `json:"id"`
			AgentName   string    `json:"agent_name"`
			Revision    int       `json:"revision"`
			BuildID     string    `json:"build_id"`
			Namespace   string    `json:"namespace"`
			DisplayName string    `json:"display_name"`
			IsCurrent   bool      `json:"is_current"`
			Status      string    `json:"status"`
			DeployedAt  time.Time `json:"deployed_at"`
			Spec        any       `json:"spec"`
		}

		records := make([]revisionRecord, 0, len(history))
		for _, r := range history {
			records = append(records, revisionRecord{
				ID:          r.DeploymentID,
				AgentName:   r.AgentName,
				Revision:    r.Revision,
				BuildID:     r.BuildID,
				Namespace:   r.Namespace,
				DisplayName: r.DisplayName,
				IsCurrent:   r.IsCurrent,
				Status:      r.Status,
				DeployedAt:  r.DeployedAt,
				Spec:        map[string]any{},
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

// injectManagedCredentials adds platform-provided credentials to the resolved
// secret data. Managed providers (e.g. anthropic-managed) have their API keys
// supplied by the server rather than the user. The env var uses the canonical
// name (ANTHROPIC_API_KEY) so agent code works identically.
func injectManagedCredentials(resolved *deployment.ResolvedEnv, cfg *config.Config) {
	if val := cfg.Deployment.ManagedAnthropicAPIKey; val != "" {
		resolved.SecretData["ANTHROPIC_API_KEY"] = val
	}
}

// GetDeploymentStatus returns the current status, events, and revisions for a deployment.
func GetDeploymentStatus(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, agentIdx *agentindex.Index, avatarStore *avatar.Store) gin.HandlerFunc {
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

		acct, acctErr := accountStore.GetByID(dep.AccountID)
		if acctErr != nil || acct == nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		isMember, _ := accountStore.IsMember(acct.ID, user.ID)
		if !isMember {
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

// StopDeployment scales all workloads to zero without deleting resources.
// POST /api/v1/deployments/:id/stop
func StopDeployment(log *logger.Logger, accountStore *account.AccountStore, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store, auditStore *auditlog.Store, cache k8scache.Cache) gin.HandlerFunc {
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

		if dep.Status != deploymentstore.StatusActive && dep.Status != deploymentstore.StatusScaledDown {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deployment is not active or scaled down"})
			return
		}

		if err := k8s.StopNamespaceWorkloads(c.Request.Context(), k8sClient.Clientset(), dep.Namespace); err != nil {
			log.Error("Failed to stop deployment workloads", "error", err, "namespace", dep.Namespace, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stop deployment"})
			return
		}
		k8scache.InvalidateNamespace(c.Request.Context(), cache, dep.Namespace)

		if err := deployStore.UpdateStatus(dep.ID, deploymentstore.StatusStopped, "", nil); err != nil {
			log.Error("Failed to mark deployment stopped", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update deployment status"})
			return
		}

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.DeploymentStop
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Stopped deployment " + dep.AgentName
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusAccepted, gin.H{
			"status":        deploymentstore.StatusStopped,
			"deployment_id": dep.ID,
		})
	}
}

// WakeUpDeployment triggers re-provisioning of a KEDA-scaled-down deployment.
func WakeUpDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, queue DeployQueue, auditStore *auditlog.Store) gin.HandlerFunc {
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
		if dep.Status != deploymentstore.StatusScaledDown && dep.Status != deploymentstore.StatusStopped {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deployment is not stopped or scaled down"})
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

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.DeploymentWakeup
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Woke up deployment " + dep.AgentName
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusAccepted, gin.H{
			"status":        "pending",
			"deployment_id": dep.ID,
		})
	}
}

// RollbackDeployment rolls back a deployment to a previous revision.
func RollbackDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, queue DeployQueue, auditStore *auditlog.Store) gin.HandlerFunc {
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

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.DeploymentRollback
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = fmt.Sprintf("Rolled back deployment to revision %d", req.Revision)
		evt.Metadata = map[string]any{"revision": req.Revision}
		auditStore.LogAsync(log, evt)

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
