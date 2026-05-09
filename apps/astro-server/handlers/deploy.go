package handlers

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"sync"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/authorizationstore"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/colorextract"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/astropods/astro/apps/astro-server/internal/specsign"
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

// templateCache caches generated base templates (after generateTemplate + mergeDeploymentPrefill)
// so that adapter re-triggers in the POST template endpoint only run ShapeTemplate.
type templateCache struct {
	m   sync.Map
	ttl time.Duration
}

type templateCacheEntry struct {
	template  *spec.AstroDeploymentSpec
	expiresAt time.Time
}

func (tc *templateCache) get(key string) (*spec.AstroDeploymentSpec, bool) {
	val, ok := tc.m.Load(key)
	if !ok {
		return nil, false
	}
	entry := val.(*templateCacheEntry)
	if time.Now().After(entry.expiresAt) {
		tc.m.Delete(key)
		return nil, false
	}
	return entry.template, true
}

func (tc *templateCache) set(key string, tmpl *spec.AstroDeploymentSpec) {
	tc.m.Store(key, &templateCacheEntry{
		template:  tmpl,
		expiresAt: time.Now().Add(tc.ttl),
	})
}

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

// validateAgentDisplayName trims and validates a display name.
func validateAgentDisplayName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("display_name must not be empty")
	}
	if len(name) > 64 {
		return "", fmt.Errorf("display_name must be 64 characters or fewer")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("display_name contains invalid control characters")
		}
	}
	return name, nil
}

// UpdateDeploymentDisplayName returns a handler that updates only the display name
// of a deployment without triggering a redeploy.
func UpdateDeploymentDisplayName(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, auditStore *auditlog.Store) gin.HandlerFunc {
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

		var body struct {
			DisplayName string `json:"display_name"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		name, err := validateAgentDisplayName(body.DisplayName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := deployStore.UpdateDisplayName(dep.ID, name); err != nil {
			log.Error("failed to update display name", "deployment_id", dep.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update display name"})
			return
		}

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = "deployment.rename"
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Renamed deployment to " + name
		evt.Metadata = map[string]any{"display_name": name}
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, gin.H{"display_name": name})
	}
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
	sourceAccountID   string
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

	// Auth: user must have deployment visibility on the source agent.
	// Same-account deploys may use private blueprints. Cross-account deploys
	// require a public blueprint, even if the caller belongs to both accounts:
	// membership in the publisher account should not let a private org
	// blueprint be deployed into a personal account (or vice versa).
	sourceAgent, err := agentIndex.Get(sourceAcct.ID, submittedSpec.Source.Name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source agent not found"})
		return nil, false
	}
	if !canDeploySourceAgent(sourceAcct, targetAcct, sourceAgent) {
		c.JSON(http.StatusNotFound, gin.H{"error": "source agent not found"})
		return nil, false
	}

	// Sanitize and validate the optional display name
	if dn := submittedSpec.Target.DisplayName; strings.TrimSpace(dn) != "" {
		validated, err := validateAgentDisplayName(dn)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return nil, false
		}
		submittedSpec.Target.DisplayName = validated
	}

	agentName := submittedSpec.Source.Name
	if !spec.IsValidName(agentName) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid agent name %q: must be lowercase alphanumeric and hyphens only, 1–63 characters", agentName),
		})
		return nil, false
	}
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

	// Verify template signature — if valid the spec is exactly what the template
	// endpoint produced and we can skip re-generation + field enforcement.
	signatureVerified := specsign.Verify(cfg.Deployment.TemplateSigningKey, submittedSpec, c.GetHeader("X-Template-Signature"))

	if !signatureVerified {
		// No valid signature — fall back to re-generation + EnforceEditable.
		var astroSpec spec.AstroSpec
		specBytes, specErr := json.Marshal(agentVersion.Spec)
		if specErr != nil {
			log.Error("Failed to marshal stored spec", "error", specErr, "agent", agentName, "build", buildID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process registered spec", "details": specErr.Error()})
			return nil, false
		}
		if specErr = json.Unmarshal(specBytes, &astroSpec); specErr != nil {
			log.Error("Failed to unmarshal spec into AstroSpec", "error", specErr, "agent", agentName, "build", buildID, "raw", string(specBytes))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse registered spec", "details": specErr.Error()})
			return nil, false
		}

		template, tmplErr := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
			Spec:              &astroSpec,
			AgentName:         sourceAgent.Name,
			Account:           sourceAcct.Name,
			ECRNamespace:      agentVersion.ECRNamespace,
			BuildID:           agentVersion.BuildID,
			RegistryURL:       cfg.Deployment.RegistryURL,
			ProxyRegistryHost: cfg.Deployment.ProxyRegistryHost,
			Environment:       cfg.Deployment.Environment,
		})
		if tmplErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to generate deployment template",
				"details": tmplErr.Error(),
			})
			return nil, false
		}

		// Rule 19: reject any change to server-owned fields.
		template.Target.Account = submittedSpec.Target.Account
		template.Target.DisplayName = submittedSpec.Target.DisplayName
		template.Target.DeploymentID = submittedSpec.Target.DeploymentID
		if submittedSpec.Interfaces != nil {
			deployment.ApplyAdapterShaping(template, submittedSpec.Interfaces.Adapters)
			deployment.ApplyAdapterShaping(submittedSpec, submittedSpec.Interfaces.Adapters)
		}
		deployment.ApplyBindingShaping(template, submittedSpec)
		if editErrs := spec.EnforceEditable(template, submittedSpec); len(editErrs) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "server-owned fields were modified",
				"validation_errors": toValidationErrors(editErrs),
			})
			return nil, false
		}
	}

	displayName := submittedSpec.Target.DisplayName

	// Resolve identity. The deployment id is the only handle for "redeploy this
	// thing" — it is what binds a row to its K8s namespace forever. Without an
	// explicit Target.DeploymentID, every deploy is a brand-new row with a
	// fresh id and a namespace derived from that id. display_name uniqueness
	// is enforced atomically by the (account_id, display_name) partial unique
	// index at INSERT time — surfaced below as a 409 — so no pre-check is
	// needed here.
	var k8sNamespace, deploymentID string
	var isUpdate bool
	if submittedSpec.Target.DeploymentID != "" && deployStore != nil {
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

	if k8sNamespace == "" {
		deploymentID = deployid.New()
		k8sNamespace = deploymentNamespace(deploymentID)
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

	// Deploy-time invariant: a slack-enabled deploy must always carry at
	// least one slack grant. Without it the messaging container's signed
	// token has an empty anyone_adapters claim and the bot is unreachable.
	// The template-path seed already covers fresh deploys, but CLI-direct
	// deploys and templates with web-only grants fall through to here.
	ensureSlackAnyoneGrant(submittedSpec)
	ensureSlackAnyoneGrant(resolveResult.Spec)

	if submittedSpec.Interfaces != nil {
		if authErrs := validateAuthorizationSpec(submittedSpec.Interfaces.Auth); len(authErrs) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "interfaces.auth invalid",
				"validation_errors": toValidationErrors(authErrs),
			})
			return nil, false
		}
	}

	return &deployContext{
		acct:              targetAcct,
		sourceAccountName: sourceAcct.Name,
		sourceAccountID:   sourceAcct.ID,
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

func canDeploySourceAgent(sourceAcct, targetAcct *account.Account, sourceAgent *agentindex.Agent) bool {
	if sourceAcct.ID == targetAcct.ID {
		return true
	}
	return sourceAgent.Visibility == "public"
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

func DeployAgent(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, varsStore *accountvars.Store, entCheck EntitlementChecker, queue DeployQueue, avatarStore *avatar.Store, omClient *openmeter.Client, db *sql.DB, auditStore *auditlog.Store, ksStore *knowledgestore.Store, authzStore *authorizationstore.Store, imagePreflighter *k8s.ImagePreflighter) gin.HandlerFunc {
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

		// Image preflight: HEAD the agent image's manifest before we write
		// any deployment rows or enqueue any work. Catches the redeploy-
		// against-vanished-tag case fast (sub-second 422) instead of leaving
		// kubelet to surface ImagePullBackOff 35 minutes later. ErrImageNotFound
		// is the only typed result that blocks; transport errors fail open.
		agentImage := dctx.resolveResult.Spec.Agent.Image
		if perr := imagePreflighter.PreflightWithBuildID(c.Request.Context(), agentImage, dctx.buildID); perr != nil {
			if nf, isNF := k8s.AsImageNotFound(perr); isNF {
				log.Warn("Deploy rejected: agent image not found in registry",
					"agent", dctx.agentName,
					"build", dctx.buildID,
					"image", nf.Image,
					"reason", nf.Reason,
				)
				c.JSON(http.StatusUnprocessableEntity, gin.H{
					"error":    "image_not_found",
					"image":    nf.Image,
					"build_id": dctx.buildID,
					"details":  nf.Reason,
					"hint":     "the build's image is no longer in the registry — push a new build or deploy a different existing one",
				})
				return
			}
			log.Warn("Image preflight returned unexpected error (failing open)", "error", perr)
		}

		// Validate knowledge store bindings authoritatively.
		var resolvedBindings map[string]deployment.ResolvedBinding
		if ksStore != nil {
			requested := make(map[string]string)
			for name, k := range submittedSpec.Knowledge {
				if k.IsBound() {
					requested[name] = k.Binding
				}
			}
			if len(requested) > 0 {
				var bindingErrs []spec.ValidationError
				resolvedBindings, bindingErrs = deployment.ResolveBindings(c.Request.Context(), ksStore, dctx.acct.ID, submittedSpec.Knowledge, requested)
				if len(bindingErrs) > 0 {
					c.JSON(http.StatusBadRequest, gin.H{
						"error":             "binding validation failed",
						"validation_errors": bindingErrs,
					})
					return
				}
			}
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
			SourceAccountID: dctx.sourceAccountID,
			AgentName:       dctx.agentName, DisplayName: dctx.displayName,
			BuildID: dctx.buildID, Namespace: dctx.k8sNS,
			SpecJSON: string(specJSON),
		}
		if enc != nil {
			params.EncryptedDataKey = enc.EncryptedDataKey
			params.KMSKeyARN = enc.KMSKeyARN
		}

		// Collect binding store IDs for persistence from already-resolved
		// bindings. The map may be empty — a deploy that submits no bindings
		// must still clear any rows from a prior revision.
		bindingStoreIDs := make(map[string]string, len(resolvedBindings))
		for name, rb := range resolvedBindings {
			bindingStoreIDs[name] = rb.Store.ID
		}

		// Save deployment as pending with normalized spec AND authorization
		// grants in a single transaction. Folding the grants write into the
		// same tx prevents the failure mode where the deployment row commits
		// but the grants write rolls back, leaving the deployment exposed to
		// the no-grants owner-fallback path.
		//
		// E11 semantics still hold: when the spec's auth block is omitted
		// entirely, we leave grants untouched. When present (even with
		// grants:[]), we atomically replace.
		applyAuth := authzStore != nil && submittedSpec.Interfaces != nil && submittedSpec.Interfaces.Auth != nil
		txFn := func(tx *sql.Tx, deploymentID string) error {
			nsCfg := &deploymentstore.NormalizedSpecConfig{
				Namespace:              dctx.k8sNS,
				IngressDomain:          cfg.Deployment.IngressDomain,
				IngestionIngressDomain: cfg.Deployment.IngestionIngressDomain,
				VarRefs:                dctx.varRefs,
			}
			if err := deploymentstore.SaveNormalizedSpec(tx, deploymentID, dctx.resolveResult.Spec, resolved, enc, nsCfg); err != nil {
				return err
			}
			if ksStore != nil {
				if err := ksStore.SaveBindings(c.Request.Context(), tx, deploymentID, bindingStoreIDs); err != nil {
					return err
				}
			}
			if applyAuth {
				grants := buildAuthorizationGrants(deploymentID, submittedSpec.Interfaces.Auth)
				if err := authorizationstore.ReplaceGrantsTx(tx, deploymentID, grants); err != nil {
					return fmt.Errorf("replace grants: %w", err)
				}
			}
			return nil
		}
		var storeErr error
		if dctx.isUpdate {
			_, storeErr = deployStore.UpdateDeploymentPending(params, txFn)
		} else {
			_, storeErr = deployStore.SaveDeploymentPending(params, txFn)
		}
		if storeErr != nil {
			if errors.Is(storeErr, deploymentstore.ErrDuplicateDisplayName) {
				c.JSON(http.StatusConflict, gin.H{"error": storeErr.Error()})
				return
			}
			log.Error("Failed to save deployment record", "error", storeErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule deployment"})
			return
		}

		// Reserve the blueprint name on first deploy — best-effort, never blocks the response.
		if !dctx.isUpdate {
			go func() {
				if err := agentIndex.MarkNameReserved(dctx.sourceAccountID, dctx.agentName); err != nil {
					log.Warn("Failed to mark blueprint name as reserved", "agent", dctx.agentName, "error", err)
				}
			}()
		}

		// Copy the blueprint's avatar and colors to the new deployment (best-effort).
		if avatarStore != nil && !dctx.isUpdate {
			if _, copyErr := avatarStore.CopyAgentToDeployment(c.Request.Context(), dctx.sourceAccountName, dctx.agentName, dctx.deploymentID); copyErr != nil {
				log.Warn("Failed to copy blueprint avatar to deployment", "error", copyErr, "deployment_id", dctx.deploymentID)
			}
			if agent, err := agentIndex.Get(dctx.sourceAccountID, dctx.agentName); err == nil && agent.AvatarColors != nil {
				_ = deployStore.SetAvatarColors(dctx.deploymentID, *agent.AvatarColors)
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

		// Look up deployment by ID
		dep, err := deployStore.GetDeploymentByID(req.DeploymentID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up deployment"})
			return
		}
		if dep == nil || dep.Status == deploymentstore.StatusUndeploying || dep.Status == deploymentstore.StatusUndeployed {
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

// annotateWorkloadsWithRows overlays Source and IsSecret on each
// container env entry by looking up the matching deployment_build_env
// row by (role, env_name). Values from K8s are preserved; this only
// adds provenance metadata.
//
// Role mapping: workload component label + container name → Role.
//   - component "agent" + container "messaging" → RoleMessaging
//   - component "agent" + container "*"         → RoleAgent
//   - component "collector"                     → RoleCollector
//   - component "knowledge-<name>"              → KnowledgeRole(<name>)
//   - component "ingestion-<name>"              → IngestionRole(<name>)
func annotateWorkloadsWithRows(workloads []WorkloadDetail, rows []deploymentstore.BuildEnvRow) {
	idx := indexBuildEnvRows(rows)
	for wi := range workloads {
		wl := &workloads[wi]
		for ci := range wl.Containers {
			c := &wl.Containers[ci]
			role := roleFor(wl.Component, c.Name)
			if role == "" {
				continue
			}
			for ei := range c.Env {
				ev := &c.Env[ei]
				key := role + "|" + ev.Name
				if r, ok := idx[key]; ok {
					ev.Source = r.Source
					ev.IsSecret = r.IsSecret
				}
			}
		}
	}
}

// indexBuildEnvRows returns a "<role>|<env_name>" lookup over rows.
func indexBuildEnvRows(rows []deploymentstore.BuildEnvRow) map[string]deploymentstore.BuildEnvRow {
	out := make(map[string]deploymentstore.BuildEnvRow, len(rows))
	for _, r := range rows {
		out[r.Role+"|"+r.EnvName] = r
	}
	return out
}

// roleFor maps a (workload component, container name) to the
// deployment_build_env role string. Returns "" when the pair doesn't
// correspond to a tracked role (e.g. integration containers, which
// aren't represented in the unified env table today).
func roleFor(component, containerName string) string {
	switch {
	case component == "agent" && containerName == "messaging":
		return string(deployment.RoleMessaging)
	case component == "agent":
		return string(deployment.RoleAgent)
	case component == "collector":
		return string(deployment.RoleCollector)
	case strings.HasPrefix(component, "knowledge-"):
		return string(deployment.KnowledgeRole(strings.TrimPrefix(component, "knowledge-")))
	case strings.HasPrefix(component, "ingestion-"):
		return string(deployment.IngestionRole(strings.TrimPrefix(component, "ingestion-")))
	}
	return ""
}

// EnvVar represents a single environment variable in a container
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	From  string `json:"from,omitempty"` // e.g. "secret:my-secret/key" or "configmap:cm/key"
	// Source is the categorical provenance from deployment_build_env when
	// available (one of: 'user_var', 'platform_meta', 'service_url',
	// 'knowledge_cred', 'auth_token', 'adapter_config', 'derived'). Empty
	// when no row exists for this (role, env_name) — in that case clients
	// fall back to inferring from From.
	Source string `json:"source,omitempty"`
	// IsSecret is the authoritative secret flag from deployment_build_env
	// when available. Replaces the client-side `isSensitiveEnvVar` name
	// heuristic; clients should redact when this is true. Defaults to
	// false; callers that need a value rely on the existing redaction
	// behavior (•••••• already in Value for K8s-sourced secrets).
	IsSecret bool `json:"is_secret,omitempty"`
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

// WorkloadDetail represents a k8s workload — Deployment, StatefulSet, Job
// (one-shot ingestion / startup), or CronJob (scheduled ingestion).
//
// Field population by Kind:
//   - Deployment / StatefulSet: Phase, PodName, Containers, URLs
//   - Job:                       Status, StartTime, Completions, Containers
//                                (from the executing pod, when present)
//   - CronJob:                   Status, Schedule, StartTime (last fire),
//                                Runs (child Jobs)
type WorkloadDetail struct {
	Name      string `json:"name"`      // k8s resource name
	Kind      string `json:"kind"`      // "Deployment", "StatefulSet", "Job", "CronJob"
	Component string `json:"component"` // from app.kubernetes.io/component label
	Age       string `json:"age"`

	// Long-running workload fields (Deployment / StatefulSet)
	Phase      string                `json:"phase,omitempty"`    // k8s pod phase (Running, Pending, Succeeded, Failed, Unknown)
	PodName    string                `json:"pod_name,omitempty"` // name of the representative pod (for restarts)
	Containers []ContainerStatus     `json:"containers"`
	URLs       []ServiceEndpointInfo `json:"urls,omitempty"`

	// Batch workload fields. Status carries the Job/CronJob-flavored vocab:
	//   Job:     Pending / Running / Succeeded / Failed
	//   CronJob: Idle / Active / Suspended
	// Long-running kinds leave Status empty — their health is read from
	// Containers[].Ready instead.
	Status      string      `json:"status,omitempty"`
	Schedule    string      `json:"schedule,omitempty"`     // cron expression (CronJob only)
	StartTime   string      `json:"start_time,omitempty"`   // Job: pod start. CronJob: last schedule time.
	Completions string      `json:"completions,omitempty"`  // "succeeded/desired" (Job only)
	Runs        []JobDetail `json:"runs,omitempty"`         // CronJob children, oldest-first
}

// JobDetail represents a single K8s Job execution — used both for standalone
// (startup) Jobs surfaced as their own Workload run and for CronJob children
// listed under Workload.Runs.
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
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name,omitempty"`
	AvatarURL    string          `json:"avatar_url,omitempty"`
	AvatarColors json.RawMessage `json:"avatar_colors,omitempty"`
	BuildID      string          `json:"build_id"`
	// LatestBuildID is the most-recent published build for the agent the
	// deployment was sourced from (lineage = source_account_id || account_id +
	// agent_name). Empty when there are no published builds, when the lookup
	// fails, or when the lineage agent is private and not owned by the viewer.
	// The UI compares against BuildID to render the "new build available"
	// upgrade affordance — the server doing the join saves N blueprint
	// fetches on the dashboard.
	LatestBuildID string `json:"latest_build_id,omitempty"`
	// SourceAccount is the publishing account name the deployment was built from.
	// Resolved from deployments.source_account_id (post-migration always
	// populated, falls back to deployment_spec_json's source.account for legacy
	// rows). Empty when neither source is available; in that case clients should
	// treat the URL/owning account as the lineage source. Clients consume this to
	// look up blueprint upgrade signals against the right account, since the
	// owning account may have a same-named blueprint with no shared lineage
	// (cross-account deploys).
	SourceAccount string `json:"source_account,omitempty"`
	Namespace     string `json:"namespace"`
	Status        string `json:"status"`
	// ErrorMessage mirrors deployments.error_message. Populated whenever the DB
	// row has a non-empty error (failed status from preflight/escalation, or
	// any other status that recorded an error). The UI surfaces this verbatim
	// on the deployment status badge so operators see WHY a deployment is
	// failed/error without opening admin tools.
	ErrorMessage       string                `json:"error_message,omitempty"`
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

// DeploymentSummaryItem is a lightweight deployment projection for quick-switch UIs.
type DeploymentSummaryItem struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name,omitempty"`
	Status       string          `json:"status"`
	AvatarURL    string          `json:"avatar_url,omitempty"`
	AvatarColors json.RawMessage `json:"avatar_colors,omitempty"`
}

// AccountDeploymentsSummary groups deployment summaries under an account.
type AccountDeploymentsSummary struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Type        string                  `json:"type"`
	DisplayName string                  `json:"display_name"`
	Deployments []DeploymentSummaryItem `json:"deployments"`
}

// DeploymentsSummaryResponse is the response for GET /api/v1/deployments/summary.
type DeploymentsSummaryResponse struct {
	Accounts []AccountDeploymentsSummary `json:"accounts"`
}

// ListDeploymentsSummary returns lightweight deployment summaries across all
// accounts the authenticated user belongs to. No K8s enrichment — DB only.
func ListDeploymentsSummary(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, avatarStore *avatar.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		accounts, err := accountStore.GetAccountsForUser(user.ID)
		if err != nil {
			log.Error("Failed to get accounts for user", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get accounts"})
			return
		}

		accountIDs := make([]string, len(accounts))
		accountMap := make(map[string]account.AccountWithRole, len(accounts))
		for i, a := range accounts {
			accountIDs[i] = a.ID
			accountMap[a.ID] = a
		}

		summaries, err := deployStore.GetSummariesForAccounts(accountIDs)
		if err != nil {
			log.Error("Failed to get deployment summaries", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get deployment summaries"})
			return
		}

		// Group by account, preserving DB sort order (deployed_at DESC).
		grouped := make(map[string][]DeploymentSummaryItem, len(accounts))
		for _, d := range summaries {
			item := DeploymentSummaryItem{
				ID:          d.ID,
				Name:        d.AgentName,
				DisplayName: d.DisplayName,
				Status:      d.Status,
			}
			if avatarStore != nil {
				item.AvatarURL = avatarStore.DeploymentAvatarURL(d.ID)
			}
			if d.AvatarColors != nil {
				item.AvatarColors = *d.AvatarColors
			}
			grouped[d.AccountID] = append(grouped[d.AccountID], item)
		}

		result := make([]AccountDeploymentsSummary, 0, len(accounts))
		for _, a := range accounts {
			deps := grouped[a.ID]
			if deps == nil {
				deps = []DeploymentSummaryItem{}
			}
			result = append(result, AccountDeploymentsSummary{
				ID:          a.ID,
				Name:        a.Name,
				Type:        a.Type,
				DisplayName: a.DisplayName,
				Deployments: deps,
			})
		}

		c.JSON(http.StatusOK, DeploymentsSummaryResponse{Accounts: result})
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
				enriched[i] = enrichDeployment(gctx, log, accountStore, k8sClient, deployStore, dbDep, listAstroDeploymentsLight, cache, k8scache.ListKeyPrefix, k8scache.ListTTL)
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

		// Build a lookup of avatar colors from the original DB records.
		dbColorsByID := make(map[string]json.RawMessage, len(dbDeps))
		for _, dep := range dbDeps {
			if dep.AvatarColors != nil {
				dbColorsByID[dep.ID] = *dep.AvatarColors
			}
		}

		// Resolve avatar URLs and colors for each deployment.
		if avatarStore != nil {
			for i, d := range allDeployments {
				allDeployments[i].AvatarURL = avatarStore.DeploymentAvatarURL(d.ID)
			}
		}
		for i, d := range allDeployments {
			if len(allDeployments[i].AvatarColors) == 0 {
				if colors, ok := dbColorsByID[d.ID]; ok {
					allDeployments[i].AvatarColors = colors
				}
			}
			if avatarStore != nil {
				allDeployments[i].AvatarColors = colorextract.EnsureCurrent(c.Request.Context(), allDeployments[i].AvatarColors,
					func(ctx context.Context) ([]byte, error) { return avatarStore.ReadDeploymentAvatar(ctx, d.ID) },
					func(ctx context.Context, j []byte) error { return deployStore.SetAvatarColors(d.ID, j) },
				)
			}
		}

		populateLatestBuildIDs(log, agentIdx, accountStore, dbDeps, allDeployments)

		c.JSON(http.StatusOK, gin.H{
			"deployments": allDeployments,
			"count":       len(allDeployments),
		})
	}
}

// populateLatestBuildIDs fills LatestBuildID on each deployment in `deps` from
// a single batch query against agent_versions. Looks up the lineage agent —
// which is the source account when set, falling back to the owning account —
// so cross-account deploys still see upgrade signals from the publisher.
//
// Cross-account refs whose source blueprint is private are suppressed: the
// deploy endpoint refuses to honor a private blueprint across an account
// boundary (canDeploySourceAgent), so advertising an upgrade the user can't
// act on would be a false positive.
//
// Quietly leaves LatestBuildID empty on lookup failure rather than failing the
// whole list response: this is a UX hint, not load-bearing data.
func populateLatestBuildIDs(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore, dbDeps []*deploymentstore.Deployment, deps []AgentDeployment) {
	if index == nil || len(dbDeps) == 0 || len(deps) == 0 {
		return
	}

	type lineageInfo struct {
		ref          agentindex.AgentVersionRef
		crossAccount bool
	}
	lineageByDepID := make(map[string]lineageInfo, len(dbDeps))
	refSet := make(map[agentindex.AgentVersionRef]struct{})
	crossAccountRefs := make(map[agentindex.AgentVersionRef]struct{})
	for _, dbDep := range dbDeps {
		acctID := lineageAccountID(log, accountStore, dbDep)
		if acctID == "" || dbDep.AgentName == "" {
			continue
		}
		ref := agentindex.AgentVersionRef{AccountID: acctID, Name: dbDep.AgentName}
		crossAccount := acctID != dbDep.AccountID
		lineageByDepID[dbDep.ID] = lineageInfo{ref: ref, crossAccount: crossAccount}
		refSet[ref] = struct{}{}
		if crossAccount {
			crossAccountRefs[ref] = struct{}{}
		}
	}

	if len(refSet) == 0 {
		return
	}
	refs := make([]agentindex.AgentVersionRef, 0, len(refSet))
	for r := range refSet {
		refs = append(refs, r)
	}
	latest, err := index.BatchLatestBuildIDs(refs)
	if err != nil {
		log.Warn("Failed to load latest build IDs for deployments", "error", err)
		return
	}

	// Private cross-account blueprints aren't deployable across the boundary,
	// so an upgrade signal would advertise an action the deploy endpoint
	// would reject. Resolve visibility for each unique cross-account ref
	// (typically 0 — the steady-state dashboard is single-account) and
	// suppress those refs from the result map.
	blockedRefs := make(map[agentindex.AgentVersionRef]struct{})
	for ref := range crossAccountRefs {
		agent, err := index.Get(ref.AccountID, ref.Name)
		if err != nil || agent == nil {
			continue
		}
		if agent.Visibility == "private" {
			blockedRefs[ref] = struct{}{}
		}
	}

	for i, d := range deps {
		info, ok := lineageByDepID[d.ID]
		if !ok {
			continue
		}
		if _, blocked := blockedRefs[info.ref]; blocked {
			continue
		}
		if buildID, ok := latest[info.ref.AccountID+"/"+info.ref.Name]; ok && buildID != "" {
			deps[i].LatestBuildID = buildID
		}
	}
}

// lineageAccountID returns the account_id whose agent_versions table holds
// the upgrade signal for a deployment. Source account wins when populated
// (cross-account deploys), otherwise the owning account is the publisher too.
func lineageAccountID(log *logger.Logger, accountStore *account.AccountStore, dep *deploymentstore.Deployment) string {
	if dep.SourceAccountID != nil && *dep.SourceAccountID != "" {
		return *dep.SourceAccountID
	}
	if dep.AccountID != "" {
		return dep.AccountID
	}
	// Pre-migration legacy: source account name lives in spec JSON; resolve
	// it via the AccountStore so we have the ID for the join. Best-effort.
	name := resolveSourceAccountName(log, accountStore, dep)
	if name == "" || accountStore == nil {
		return ""
	}
	acct, err := accountStore.GetByName(name)
	if err != nil || acct == nil {
		return ""
	}
	return acct.ID
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

		deps := enrichDeployment(c.Request.Context(), log, accountStore, k8sClient, deployStore, dbDep, listAstroDeployments, k8scache.NoopCache{}, "", 0)
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

		// Annotate K8s-derived env entries with authoritative metadata
		// (Source, IsSecret) from deployment_build_env. The values still
		// come from K8s; the rows just supply provenance the UI uses for
		// badge color and redaction, replacing the client-side
		// isSensitiveEnvVar name heuristic. No-op when no rows exist
		// (legacy deployments fall back to From-based inference client-side).
		if rows, rowErr := deployStore.GetBuildEnv(dbDep.ID); rowErr == nil && len(rows) > 0 {
			annotateWorkloadsWithRows(result.Workloads, rows)
		} else if rowErr != nil {
			log.Warn("GetBuildEnv failed for deployment annotation",
				"error", rowErr, "deployment_id", dbDep.ID)
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

		if avatarStore != nil {
			result.AvatarURL = avatarStore.DeploymentAvatarURL(dbDep.ID)
			result.AvatarColors = colorextract.EnsureCurrent(c.Request.Context(), result.AvatarColors,
				func(ctx context.Context) ([]byte, error) { return avatarStore.ReadDeploymentAvatar(ctx, dbDep.ID) },
				func(ctx context.Context, j []byte) error { return deployStore.SetAvatarColors(dbDep.ID, j) },
			)
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
func agentDeploymentFromDB(log *logger.Logger, accountStore *account.AccountStore, dep *deploymentstore.Deployment) AgentDeployment {
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
		ID:            dep.ID,
		Name:          dep.AgentName,
		DisplayName:   dep.DisplayName,
		BuildID:       dep.BuildID,
		SourceAccount: resolveSourceAccountName(log, accountStore, dep),
		Namespace:     dep.Namespace,
		Status:        status,
		Replicas:      0,
		Ready:         0,
		CreatedAt:     dep.DeployedAt.Format(time.RFC3339),
		Components:    []string{},
	}
	if dep.AvatarColors != nil {
		ad.AvatarColors = *dep.AvatarColors
	}

	if dep.ErrorMessage != nil && *dep.ErrorMessage != "" {
		ad.ErrorMessage = *dep.ErrorMessage
	}
	// status=failed is the canonical truth-from-DB signal (preflight,
	// pod-failure escalation, stale-job sweep). Surface it as "error" so
	// the UI's error badge fires even on the lightweight DB-only path.
	if dep.Status == deploymentstore.StatusFailed || (dep.ErrorMessage != nil && *dep.ErrorMessage != "") {
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

func enrichDeployment(ctx context.Context, log *logger.Logger, accountStore *account.AccountStore, k8sClient k8s.ClusterClient, deployStore *deploymentstore.Store, dbDep *deploymentstore.Deployment, listFn k8sListFn, cache k8scache.Cache, keyPrefix string, cacheTTL time.Duration) []AgentDeployment {
	// Source account name resolved once per dbDep so the K8s and DB-only paths
	// return identical SourceAccount values. Looked up via
	// resolveSourceAccountName which prefers source_account_id and falls back to
	// deployment_spec_json.source.account on legacy rows.
	sourceAccount := resolveSourceAccountName(log, accountStore, dbDep)

	applyDBFields := func(deps []AgentDeployment, createdAt time.Time) {
		for i := range deps {
			deps[i].ID = dbDep.ID
			deps[i].Name = dbDep.AgentName
			deps[i].DisplayName = dbDep.DisplayName
			deps[i].SourceAccount = sourceAccount
			deps[i].CreatedAt = createdAt.Format(time.RFC3339)
			if dbDep.AvatarColors != nil {
				deps[i].AvatarColors = *dbDep.AvatarColors
			}
			if dbDep.ErrorMessage != nil && *dbDep.ErrorMessage != "" {
				deps[i].ErrorMessage = *dbDep.ErrorMessage
			}
			// DB status overrides whatever the K8s replica scan inferred.
			// failed/undeploying/pending are authoritative — without this, a
			// failed deployment with 0/N ready replicas reads as "deploying"
			// indefinitely (the original 35-minute stuck bug).
			switch dbDep.Status {
			case deploymentstore.StatusPending, deploymentstore.StatusProvisioning:
				deps[i].Status = "pending"
			case deploymentstore.StatusUndeploying:
				deps[i].Status = "undeploying"
			case deploymentstore.StatusFailed:
				deps[i].Status = "error"
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
		entry := agentDeploymentFromDB(log, accountStore, dbDep)
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

		// Skip workloads scaled to zero — no pods to show
		if dep.Spec.Replicas != nil && *dep.Spec.Replicas == 0 {
			continue
		}

		wl := WorkloadDetail{
			Name:       dep.Name,
			Kind:       "Deployment",
			Component:  component,
			Age:        formatAge(dep.CreationTimestamp.Time),
			Containers: containersFromSpec(dep.Spec.Template.Spec),
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

		// Skip workloads scaled to zero — no pods to show
		if sts.Spec.Replicas != nil && *sts.Spec.Replicas == 0 {
			continue
		}

		wl := WorkloadDetail{
			Name:       sts.Name,
			Kind:       "StatefulSet",
			Component:  component,
			Age:        formatAge(sts.CreationTimestamp.Time),
			Containers: containersFromSpec(sts.Spec.Template.Spec),
		}
		if urls, ok := workloadURLs[key+":"+component]; ok {
			wl.URLs = urls
		}
		info.Workloads = append(info.Workloads, wl)
	}

	// List CronJobs first so we can route CronJob-owned Jobs into the
	// matching Workload's Runs slice in the loop below.
	cronJobList, err := clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		cronJobList = &batchv1.CronJobList{} // non-critical, continue without cronjobs
	}

	// cronWorkloadIdx points from a CronJob's K8s name to its Workload entry,
	// so child Jobs can append themselves to Runs[] without re-scanning.
	type cronWorkloadRef struct {
		info *AgentDeployment
		idx  int
	}
	cronWorkloadIdx := make(map[string]cronWorkloadRef)

	for _, cj := range cronJobList.Items {
		agentKey := cj.Labels[deployment.LabelKeyAgent]
		version := cj.Labels["app.kubernetes.io/version"]
		component := cj.Labels["app.kubernetes.io/component"]
		if agentKey == "" {
			continue
		}

		key := agentKey + ":" + version
		info, exists := agentDeployments[key]
		if !exists {
			info = &AgentDeployment{
				BuildID:          version,
				Namespace:        namespace,
				Status:           "Running",
				CreatedAt:        cj.CreationTimestamp.Format(time.RFC3339),
				Components:       []string{},
				ManualIngestions: manualIngestions,
			}
			agentDeployments[key] = info
		}

		wl := WorkloadDetail{
			Name:       cj.Name,
			Kind:       "CronJob",
			Component:  component,
			Age:        formatAge(cj.CreationTimestamp.Time),
			Status:     cronJobStatus(&cj),
			Schedule:   cj.Spec.Schedule,
			Containers: containersFromSpecWithEnv(ctx, clientset, namespace, cj.Spec.JobTemplate.Spec.Template.Spec),
		}
		if cj.Status.LastScheduleTime != nil {
			wl.StartTime = cj.Status.LastScheduleTime.Format(time.RFC3339)
		}
		info.Workloads = append(info.Workloads, wl)
		cronWorkloadIdx[cj.Name] = cronWorkloadRef{info: info, idx: len(info.Workloads) - 1}
	}

	// List Jobs and route each one based on ownerReferences:
	//   - owned by a CronJob → append as a JobDetail to that CronJob's Runs[]
	//   - standalone (startup ingestion) → create its own Kind="Job" Workload
	jobList, err := clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		jobList = &batchv1.JobList{} // non-critical, continue without jobs
	}

	for _, job := range jobList.Items {
		agentKey := job.Labels[deployment.LabelKeyAgent]
		version := job.Labels["app.kubernetes.io/version"]
		component := job.Labels["app.kubernetes.io/component"]
		if agentKey == "" {
			continue
		}

		desired := int32(1)
		if job.Spec.Completions != nil {
			desired = *job.Spec.Completions
		}
		runDetail := JobDetail{
			Name:        job.Name,
			Component:   component,
			Age:         formatAge(job.CreationTimestamp.Time),
			Completions: fmt.Sprintf("%d/%d", job.Status.Succeeded, desired),
			Status:      jobStatus(&job),
		}
		if job.Status.StartTime != nil {
			runDetail.StartTime = job.Status.StartTime.Format(time.RFC3339)
		}

		// CronJob-owned Job: attach as a Run to the parent Workload.
		if parent := cronJobOwner(job.OwnerReferences); parent != "" {
			if ref, ok := cronWorkloadIdx[parent]; ok {
				ref.info.Workloads[ref.idx].Runs = append(ref.info.Workloads[ref.idx].Runs, runDetail)
				continue
			}
			// Parent CronJob wasn't listed (e.g. label-mismatched or pruned).
			// Fall through and surface the orphan Job as its own Workload so
			// it's still visible.
		}

		// Standalone Job → its own Workload row.
		key := agentKey + ":" + version
		info, exists := agentDeployments[key]
		if !exists {
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
		info.Workloads = append(info.Workloads, WorkloadDetail{
			Name:        job.Name,
			Kind:        "Job",
			Component:   component,
			Age:         runDetail.Age,
			Status:      runDetail.Status,
			StartTime:   runDetail.StartTime,
			Completions: runDetail.Completions,
			Containers:  containersFromSpecWithEnv(ctx, clientset, namespace, job.Spec.Template.Spec),
		})
	}

	// Index pods by agent key + component for matching to workloads.
	// Version is intentionally excluded from the key so that pods with stale version
	// labels (e.g. OnDelete StatefulSets not yet recycled after a redeploy) are still matched.
	type podKey struct{ agent, component string }
	bestPod := make(map[podKey]corev1.Pod)
	for _, pod := range podList.Items {
		agentKey := pod.Labels[deployment.LabelKeyAgent]
		component := pod.Labels["app.kubernetes.io/component"]
		if agentKey == "" {
			continue
		}
		pk := podKey{agentKey, component}
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
		for i := range info.Workloads {
			wl := &info.Workloads[i]
			pod, ok := bestPod[podKey{agentKey, wl.Component}]
			if !ok {
				continue
			}
			wl.Containers = enrichContainerStatuses(wl.Containers, buildContainerStatuses(ctx, clientset, pod))
			wl.PodName = pod.Name
			wl.Phase = string(pod.Status.Phase)
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

// containersFromSpec returns a ContainerStatus list with only names populated from a pod template spec.
// Runtime fields (State, Ready, RestartCount) are left zero and enriched later if a pod is found.
func containersFromSpec(podSpec corev1.PodSpec) []ContainerStatus {
	out := make([]ContainerStatus, len(podSpec.Containers)+len(podSpec.InitContainers))
	for i, c := range podSpec.Containers {
		out[i] = ContainerStatus{Name: c.Name}
	}
	for i, c := range podSpec.InitContainers {
		out[len(podSpec.Containers)+i] = ContainerStatus{Name: c.Name}
	}
	return out
}

// containersFromSpecWithEnv mirrors containersFromSpec but also resolves env vars
// directly from the pod template, without requiring a live pod. Used for Jobs and
// CronJobs where the pod may have been GC'd or never run yet — env wiring is
// identical to what would land on a pod, so the workload's own template is the
// authoritative source.
func containersFromSpecWithEnv(ctx context.Context, clientset kubernetes.Interface, ns string, podSpec corev1.PodSpec) []ContainerStatus {
	envByContainer := resolvePodSpecEnv(ctx, clientset, ns, podSpec)
	out := make([]ContainerStatus, 0, len(podSpec.Containers)+len(podSpec.InitContainers))
	for _, c := range podSpec.Containers {
		out = append(out, ContainerStatus{Name: c.Name, Env: envByContainer[c.Name]})
	}
	for _, c := range podSpec.InitContainers {
		out = append(out, ContainerStatus{Name: c.Name, Env: envByContainer[c.Name]})
	}
	return out
}

// enrichContainerStatuses merges runtime status from a pod into the spec-seeded container list.
// For each container in base, if a matching runtime status exists it updates the runtime fields
// (State, Ready, RestartCount, Reason, Message, Env) in-place. Containers present in base but
// absent from runtime are left with zero runtime fields. Containers present only in runtime are
// ignored — the spec is the source of truth for which containers exist.
func enrichContainerStatuses(specContainers, podContainers []ContainerStatus) []ContainerStatus {
	podStatusByName := make(map[string]ContainerStatus, len(podContainers))
	for _, podContainer := range podContainers {
		podStatusByName[podContainer.Name] = podContainer
	}
	result := make([]ContainerStatus, len(specContainers))
	for i, specContainer := range specContainers {
		if podStatus, ok := podStatusByName[specContainer.Name]; ok {
			result[i] = podStatus
		} else {
			result[i] = specContainer
		}
	}
	return result
}

// resolvePodSpecEnv resolves the full env list for each container in a PodSpec,
// mirroring K8s runtime precedence: envFrom is resolved first, then direct env
// entries overlay (a direct entry with the same name as an envFrom-resolved key
// wins). Secret values are redacted; only keys are surfaced. Returns a map keyed
// by container name covering both Containers and InitContainers.
func resolvePodSpecEnv(ctx context.Context, clientset kubernetes.Interface, ns string, podSpec corev1.PodSpec) map[string][]EnvVar {
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

	out := map[string][]EnvVar{}
	for _, sc := range append(podSpec.Containers, podSpec.InitContainers...) {
		byName := map[string]EnvVar{}
		order := []string{}
		put := func(ev EnvVar) {
			if _, exists := byName[ev.Name]; !exists {
				order = append(order, ev.Name)
			}
			byName[ev.Name] = ev
		}

		for _, ef := range sc.EnvFrom {
			if ef.ConfigMapRef != nil {
				for k, v := range resolveConfigMap(ef.ConfigMapRef.Name) {
					put(EnvVar{
						Name:  ef.Prefix + k,
						Value: v,
						From:  "configmap:" + ef.ConfigMapRef.Name,
					})
				}
			}
			if ef.SecretRef != nil {
				for _, k := range resolveSecretKeys(ef.SecretRef.Name) {
					put(EnvVar{
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
			put(ev)
		}

		envVars := make([]EnvVar, 0, len(order))
		for _, name := range order {
			envVars = append(envVars, byName[name])
		}
		out[sc.Name] = envVars
	}
	return out
}

// buildContainerStatuses extracts container statuses and env vars from a k8s pod.
// It resolves envFrom references (ConfigMaps and Secrets) into individual key-value pairs.
func buildContainerStatuses(ctx context.Context, clientset kubernetes.Interface, pod corev1.Pod) []ContainerStatus {
	envByContainer := resolvePodSpecEnv(ctx, clientset, pod.Namespace, pod.Spec)

	var containers []ContainerStatus
	for _, cs := range append(pod.Status.ContainerStatuses, pod.Status.InitContainerStatuses...) {
		container := ContainerStatus{
			Name:         cs.Name,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
			Env:          envByContainer[cs.Name],
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

// cronJobStatus reports Suspended when explicitly suspended, Active when at
// least one child Job is currently running, and Idle otherwise (scheduled but
// nothing in-flight). This is distinct from jobStatus's vocabulary because a
// CronJob has no "Succeeded"/"Failed" terminal state — its children do.
func cronJobStatus(cj *batchv1.CronJob) string {
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		return "Suspended"
	}
	if len(cj.Status.Active) > 0 {
		return "Active"
	}
	return "Idle"
}

// cronJobOwner returns the name of the CronJob that owns a Job, or "" if the
// Job is standalone. K8s sets a single batch/v1 CronJob ownerRef on every
// child Job, so we don't need to walk the chain.
func cronJobOwner(refs []metav1.OwnerReference) string {
	for _, r := range refs {
		if r.Kind == "CronJob" {
			return r.Name
		}
	}
	return ""
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
		// workload is the K8s Deployment/StatefulSet name for the agent container,
		// typically prefixed by the agent name (e.g. "my-agent-agent"). Used as a
		// pod-label prefix filter in Loki: pod=~"<workload>-.+".
		workloadName := c.Query("workload")
		if !spec.IsValidName(workloadName) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid workload name %q", workloadName)})
			return
		}
		containerName := c.Query("container")

		loc := getTimezoneLocation(c)

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
		if lvl := c.Query("level"); lvl != "" {
			lokiParams.LevelFilter = lvl
		}
		if dir := c.Query("direction"); dir == "backward" {
			lokiParams.Direction = "backward"
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

		streamLogs(c, log, lokiClient, lokiParams, k8sClient, dep.Namespace, podName, logOpts, loc)
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

		// workload is the K8s Deployment/StatefulSet name for the agent container,
		// typically prefixed by the agent name (e.g. "my-agent-agent"). Used as a
		// pod-label prefix filter in Loki: pod=~"<workload>-.+".
		workloadName := c.Query("workload")
		if !spec.IsValidName(workloadName) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid workload name %q", workloadName)})
			return
		}
		containerName := c.Query("container")
		podName := c.Query("pod")

		loc := getTimezoneLocation(c)

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
			payload, _ := json.Marshal(lokiLineToEntry(ll, loc))
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

// resolveSourceAccountName returns the publishing account's name for a
// deployment. It prefers the deployments.source_account_id column (always
// set on writes after the migration) and falls back to parsing
// deployment_spec_json.source.account for legacy rows that predate the
// column. Returns "" when neither source is available; callers treat that
// as "same account" and use the URL account.
func resolveSourceAccountName(log *logger.Logger, accountStore *account.AccountStore, d *deploymentstore.Deployment) string {
	if d.SourceAccountID != nil && *d.SourceAccountID != "" {
		if acct, err := accountStore.GetByID(*d.SourceAccountID); err == nil && acct != nil {
			return acct.Name
		} else if err != nil {
			log.Warn("Failed to resolve source_account_id; falling back to spec JSON",
				"deployment_id", d.ID, "source_account_id", *d.SourceAccountID, "error", err)
		}
	}
	return deploymentstore.SourceAccountFromSpec(d.DeploymentSpecJSON)
}

// resolveAgentForTemplate loads the account + agent used to build a
// deployment template. The caller is responsible for any access-control
// decisions on the returned agent (e.g. private-visibility membership
// checks) — this function only resolves the records.
func resolveAgentForTemplate(
	c *gin.Context,
	accountStore *account.AccountStore,
	agentIndex *agentindex.Index,
	accountName, agentName string,
) (*account.Account, *agentindex.Agent, bool) {
	acct, err := accountStore.GetByName(accountName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found", "error_code": "account_not_found"})
		return nil, nil, false
	}
	agent, err := agentIndex.Get(acct.ID, agentName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found", "error_code": "blueprint_not_found"})
		return nil, nil, false
	}
	return acct, agent, true
}

// enforcePrivateAgentMembership is the private-visibility gate: a private
// agent is only visible to members of its owning account. Returns true if
// access is allowed; on denial it writes the response and returns false.
func enforcePrivateAgentMembership(
	c *gin.Context,
	accountStore *account.AccountStore,
	acct *account.Account,
	agent *agentindex.Agent,
) bool {
	if agent.Visibility != "private" {
		return true
	}
	user, exists := middleware.GetUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return false
	}
	if !isAccountMember(c, accountStore, acct.ID, user.ID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found", "error_code": "blueprint_not_found"})
		return false
	}
	return true
}

// generateTemplate resolves an agent build and generates a deployment
// template from a pre-resolved (account, agent) pair.
// buildIDOverride, when non-empty, pins the build instead of using latest.
//
// Access control is the caller's responsibility: fresh-template callers
// run enforcePrivateAgentMembership before calling in; prefill callers rely
// on the deployment's own target-account membership check and do not need
// to re-check source-agent visibility.
func generateTemplate(
	c *gin.Context,
	log *logger.Logger,
	agentIndex *agentindex.Index,
	cfg *config.Config,
	acct *account.Account,
	agent *agentindex.Agent,
	buildIDOverride string,
) (*spec.AstroDeploymentSpec, bool) {
	accountID := acct.ID
	name := agent.Name

	var agentVersion *agentindex.AgentVersion
	var err error
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
			"error":      "no builds found for agent",
			"error_code": "build_not_found",
			"details":    err.Error(),
		})
		return nil, false
	}

	specBytes, err := json.Marshal(agentVersion.Spec)
	if err != nil {
		log.Error("Failed to marshal stored spec", "error", err, "account", acct.Name, "agent", name)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process spec", "details": err.Error()})
		return nil, false
	}
	var astroSpec spec.AstroSpec
	if err := json.Unmarshal(specBytes, &astroSpec); err != nil {
		log.Error("Failed to unmarshal spec into AstroSpec", "error", err, "account", acct.Name, "agent", name, "raw", string(specBytes))
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

// PostDeploymentTemplate returns a handler for the interactive POST deployment-template endpoint.
// POST /api/v1/agents/:account/:name/deployment-template
func PostDeploymentTemplate(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, ksStore *knowledgestore.Store, authzStore *authorizationstore.Store) gin.HandlerFunc {
	cache := &templateCache{ttl: 5 * time.Minute}

	return func(c *gin.Context) {
		var req spec.TemplateRequest
		if c.Request.Body != nil && c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
				return
			}
		}

		accountName := c.Param("account")
		agentName := c.Param("name")
		buildIDOverride := req.Build

		// Build shape options for binding resolution.
		var shapeOpts *deployment.ShapeOptions
		if ksStore != nil {
			acct, acctErr := accountStore.GetByName(accountName)
			if acctErr == nil && acct != nil {
				shapeOpts = &deployment.ShapeOptions{
					KnowledgeStore: ksStore,
					AccountID:      acct.ID,
				}
			}
		}

		// Prefill from existing deployment when deployment_id is provided.
		if req.DeploymentID != "" {
			existing, err := deployStore.GetDeploymentByID(req.DeploymentID)
			if err != nil {
				log.Error("Failed to get deployment", "error", err, "deployment_id", req.DeploymentID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up deployment"})
				return
			}
			if existing == nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
				return
			}

			// Auth check always runs — never skipped by cache.
			user, _ := middleware.GetUser(c)
			if !isAccountMember(c, accountStore, existing.AccountID, user.ID) {
				c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for deployment's account"})
				return
			}

			// Use deployment's build unless the request overrides it.
			if buildIDOverride == "" {
				buildIDOverride = existing.BuildID
			}

			// Resolve historical revision when requested.
			prefillExisting := existing
			if req.Revision > 0 {
				rev, revErr := deployStore.GetRevisionByNumber(req.DeploymentID, req.Revision)
				if revErr != nil {
					log.Error("Failed to get revision", "error", revErr, "deployment_id", req.DeploymentID, "revision", req.Revision)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get revision"})
					return
				}
				if rev == nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "revision not found"})
					return
				}
				if buildIDOverride == existing.BuildID {
					buildIDOverride = rev.BuildID
				}
				revExisting := *existing
				revExisting.DeploymentSpecJSON = string(rev.SpecJSON)
				prefillExisting = &revExisting
			}

			// Cross-account deployments resolve the publisher account from the
			// deployments.source_account_id column. Legacy rows (pre-migration)
			// leave the column NULL; fall back to parsing
			// deployment_spec_json.source.account, then to the URL account.
			// When Revision > 0, prefillExisting.DeploymentSpecJSON already
			// holds the historical revision's spec, so the JSON fallback
			// picks up the source account as it was at that revision.
			sourceAccountName := resolveSourceAccountName(log, accountStore, prefillExisting)
			lookupAccountName := accountName
			if sourceAccountName != "" {
				lookupAccountName = sourceAccountName
			}

			// Restore knowledge bindings from the stored spec so that the
			// initial template load (before the user sends explicit bindings)
			// correctly shapes bound entries and populates the binding picker.
			deployment.ApplyStoredBindingsToRequest(log, &req, prefillExisting.DeploymentSpecJSON)

			// Check cache — skips generateTemplate + DB var fetch + merge on hit.
			cacheKey := accountName + ":" + sourceAccountName + ":" + agentName + ":" + buildIDOverride + ":" + req.DeploymentID + ":" + strconv.Itoa(req.Revision)
			if base, ok := cache.get(cacheKey); ok {
				resp := deployment.ShapeTemplate(c.Request.Context(), base, &req, shapeOpts)
				// Display name is mutable outside the deploy flow — always
				// apply the current value from the DB, not the cached one.
				resp.Template.Target.DisplayName = prefillExisting.DisplayName
				if req.Finalize {
					resp.Signature = specsign.Sign(cfg.Deployment.TemplateSigningKey, &resp.Template)
				}
				c.JSON(http.StatusOK, resp)
				return
			}

			// Cache miss. Resolve the source account + agent. No private-
			// visibility gate here: auth was enforced above against the
			// deployment's account, and requiring source-account membership
			// would break cross-account Configure for any private agent that
			// has already been published and deployed downstream.
			acct, agent, ok := resolveAgentForTemplate(c, accountStore, agentIndex, lookupAccountName, agentName)
			if !ok {
				return
			}
			template, ok := generateTemplate(c, log, agentIndex, cfg, acct, agent, buildIDOverride)
			if !ok {
				return
			}

			storedVars, err := deployStore.GetDeploymentVariables(req.DeploymentID)
			if err != nil {
				log.Error("Failed to get deployment variables", "error", err, "deployment_id", req.DeploymentID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get deployment variables"})
				return
			}

			mergeDeploymentPrefill(log, template, prefillExisting, storedVars, accountStore, authzStore)

			// For historical revisions, also override display name from the stored spec.
			if req.Revision > 0 {
				var storedSpec spec.AstroDeploymentSpec
				if jsonErr := json.Unmarshal([]byte(prefillExisting.DeploymentSpecJSON), &storedSpec); jsonErr == nil {
					if storedSpec.Target.DisplayName != "" {
						template.Target.DisplayName = storedSpec.Target.DisplayName
					}
				}
			}

			cache.set(cacheKey, template)
			resp := deployment.ShapeTemplate(c.Request.Context(), template, &req, shapeOpts)
			if req.Finalize {
				resp.Signature = specsign.Sign(cfg.Deployment.TemplateSigningKey, &resp.Template)
			}
			c.JSON(http.StatusOK, resp)
			return
		}

		// No deployment_id — fresh template.
		cacheKey := accountName + ":" + agentName + ":" + buildIDOverride
		if base, ok := cache.get(cacheKey); ok {
			resp := deployment.ShapeTemplate(c.Request.Context(), base, &req, shapeOpts)
			if req.Finalize {
				resp.Signature = specsign.Sign(cfg.Deployment.TemplateSigningKey, &resp.Template)
			}
			c.JSON(http.StatusOK, resp)
			return
		}

		// Fresh requests gate access on the agent's visibility because there
		// is no prior deployment to anchor authorization against.
		acct, agent, ok := resolveAgentForTemplate(c, accountStore, agentIndex, accountName, agentName)
		if !ok {
			return
		}
		if !enforcePrivateAgentMembership(c, accountStore, acct, agent) {
			return
		}
		template, ok := generateTemplate(c, log, agentIndex, cfg, acct, agent, buildIDOverride)
		if !ok {
			return
		}

		// Fresh-deploy prefill: pre-populate sensible auth grants so a default
		// deploy isn't dead-on-arrival. The user sees these in the editable
		// template before submitting and may remove or extend them. See spec
		// section "Defaults and prefill".
		if user, ok := middleware.GetUser(c); ok {
			seedFreshAuthGrants(template, user.ID)
		}

		cache.set(cacheKey, template)
		resp := deployment.ShapeTemplate(c.Request.Context(), template, &req, shapeOpts)
		if req.Finalize {
			resp.Signature = specsign.Sign(cfg.Deployment.TemplateSigningKey, &resp.Template)
		}
		c.JSON(http.StatusOK, resp)
	}
}

// seedFreshAuthGrants populates interfaces.auth grants with sensible defaults
// on a fresh deploy. The deployer always gets a user grant under web. When
// slack is in the adapter list, an `anyone` grant is seeded under slack so the
// channel is reachable out of the box — slack identity resolves to the owner
// account regardless of grant shape, so `anyone` and `org: <owner>`
// collapse to the same effective scope (anyone in the bot's workspace).
//
// Existing grants in the template (e.g. coming from the agent's astropods.yml)
// are preserved — we only seed when both blocks are empty.
func seedFreshAuthGrants(template *spec.AstroDeploymentSpec, deployerUserID string) {
	if template.Interfaces == nil {
		return
	}
	if template.Interfaces.Auth == nil {
		template.Interfaces.Auth = &spec.DeploymentInterfacesAuth{}
	}
	auth := template.Interfaces.Auth
	if (auth.Web != nil && len(auth.Web.Grants) > 0) || (auth.Slack != nil && len(auth.Slack.Grants) > 0) {
		return
	}

	if auth.Web == nil {
		auth.Web = &spec.DeploymentWebAuth{}
	}
	auth.Web.Grants = []spec.DeploymentAuthorizationGrant{{UserID: deployerUserID}}

	if slices.Contains(template.Interfaces.Adapters, "slack") {
		auth.Slack = &spec.DeploymentSlackAuth{
			Grants: []spec.DeploymentAuthorizationGrant{{Anyone: true}},
		}
	}
}

// ensureSlackAnyoneGrant seeds an `anyone` grant under slack at deploy time
// when the slack adapter is enabled but no slack grants are mentioned. The
// template-path seed (seedFreshAuthGrants) only fires when both web and
// slack blocks are empty, so it misses the common case where the user (or
// astropods.yml) sets web grants but leaves slack unset. Without a slack
// grant the signed token's anyone_adapters claim ends up empty and the
// channel is unreachable. Existing slack grants are preserved.
func ensureSlackAnyoneGrant(ds *spec.AstroDeploymentSpec) {
	if ds.Interfaces == nil {
		return
	}
	if !slices.Contains(ds.Interfaces.Adapters, "slack") {
		return
	}
	if ds.Interfaces.Auth == nil {
		ds.Interfaces.Auth = &spec.DeploymentInterfacesAuth{}
	}
	auth := ds.Interfaces.Auth
	if auth.Slack != nil && len(auth.Slack.Grants) > 0 {
		return
	}
	auth.Slack = &spec.DeploymentSlackAuth{
		Grants: []spec.DeploymentAuthorizationGrant{{Anyone: true}},
	}
}

// mergeDeploymentPrefill merges stored deployment values into a template.
// Used by PostDeploymentTemplate when deployment_id is provided.
func mergeDeploymentPrefill(log *logger.Logger, template *spec.AstroDeploymentSpec, existing *deploymentstore.Deployment, storedVars []deploymentstore.Variable, accountStore *account.AccountStore, authzStore *authorizationstore.Store) {
	template.Target.DeploymentID = existing.ID
	template.Target.DisplayName = existing.DisplayName

	// Resolve account name for target.account
	acct, err := accountStore.GetByID(existing.AccountID)
	if err == nil && acct != nil {
		template.Target.Account = acct.Name
	}

	// Merge variable values from the stored deployment into the template.
	for _, sv := range storedVars {
		if tv, ok := template.Variables[sv.Name]; ok {
			if sv.Ref != "" {
				// Variable was originally set via an account variable reference —
				// restore the ref so the UI shows which account variable was selected.
				tv.Ref = sv.Ref
			} else if !sv.Secret {
				// Only expose plaintext values for non-secret variables.
				// Secret values without refs are encrypted blobs — not useful to the UI.
				tv.Value = sv.Value
			}
			template.Variables[sv.Name] = tv
		}
	}

	// Merge adapters and ingestion schedules from stored spec
	if existing.DeploymentSpecJSON != "" {
		var storedSpec spec.AstroDeploymentSpec
		if jsonErr := json.Unmarshal([]byte(existing.DeploymentSpecJSON), &storedSpec); jsonErr == nil {
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
		}
	}

	// Pull live authorization (default_role + grants) from the DB, not the stored spec.
	// The DB is the source of truth — admin endpoints can mutate it between deploys.
	if authzStore != nil && template.Interfaces != nil {
		if template.Interfaces.Auth == nil {
			template.Interfaces.Auth = &spec.DeploymentInterfacesAuth{}
		}
		mergeAuthorizationFromStore(log, authzStore, existing.ID, template.Interfaces.Auth)
	}
}

// buildAuthorizationGrants flattens the spec's web and slack grant blocks
// into a single store-shape list, ready for ReplaceGrantsTx.
//
// This is the only writer-side translator from spec form to store form. The
// deploy handler runs the resulting list through ReplaceGrantsTx inside the
// same transaction that creates the deployment row, so the grants table is
// never out of sync with the deployments table.
func buildAuthorizationGrants(deploymentID string, auth *spec.DeploymentInterfacesAuth) []authorizationstore.Grant {
	var grants []authorizationstore.Grant
	if auth == nil {
		return grants
	}
	if auth.Web != nil {
		for _, g := range auth.Web.Grants {
			grants = append(grants, specGrantToStore(deploymentID, g, authorizationstore.AdapterWeb))
		}
	}
	if auth.Slack != nil {
		for _, g := range auth.Slack.Grants {
			grants = append(grants, specGrantToStore(deploymentID, g, authorizationstore.AdapterSlack))
		}
	}
	return grants
}

// specGrantToStore translates a spec-shape grant (org|user_id|anyone)
// into the store's polymorphic (subject_type, subject_id) shape. Adapter is
// supplied by the caller since it's implied by the parent block in the spec.
//
// Caller must have run validateAuthorizationSpec first to ensure exactly one
// of the three subject fields is set.
func specGrantToStore(deploymentID string, g spec.DeploymentAuthorizationGrant, adapter string) authorizationstore.Grant {
	out := authorizationstore.Grant{DeploymentID: deploymentID, Adapter: adapter}
	switch {
	case g.Anyone:
		out.SubjectType = authorizationstore.SubjectTypeAnyone
		out.SubjectID = ""
	case g.UserID != "":
		out.SubjectType = authorizationstore.SubjectTypeUser
		out.SubjectID = g.UserID
	default:
		out.SubjectType = authorizationstore.SubjectTypeOrg
		out.SubjectID = g.Org
	}
	return out
}

// storeGrantToSpec is the inverse of specGrantToStore. Returns a spec grant
// (without an adapter field — that's encoded by where the grant ends up in
// the auth block).
func storeGrantToSpec(g *authorizationstore.Grant) spec.DeploymentAuthorizationGrant {
	out := spec.DeploymentAuthorizationGrant{}
	switch g.SubjectType {
	case authorizationstore.SubjectTypeAnyone:
		out.Anyone = true
	case authorizationstore.SubjectTypeUser:
		out.UserID = g.SubjectID
	case authorizationstore.SubjectTypeOrg:
		out.Org = g.SubjectID
	}
	return out
}

// validateAuthorizationSpec checks that every grant has exactly one subject
// (org/user/anyone) and that slack grants are org-scoped only.
// Returns a list of human-readable error strings, empty when the block is
// valid.
func validateAuthorizationSpec(auth *spec.DeploymentInterfacesAuth) []string {
	if auth == nil {
		return nil
	}
	var errs []string
	seen := map[string]struct{}{}

	check := func(adapter string, grants []spec.DeploymentAuthorizationGrant) {
		for i, g := range grants {
			prefix := fmt.Sprintf("interfaces.auth.%s.grants[%d]", adapter, i)

			subjectCount := 0
			if g.Org != "" {
				subjectCount++
			}
			if g.UserID != "" {
				subjectCount++
			}
			if g.Anyone {
				subjectCount++
			}
			switch subjectCount {
			case 0:
				errs = append(errs, prefix+": exactly one of org, user_id, anyone is required")
			case 1:
				// ok
			default:
				errs = append(errs, prefix+": only one of org, user_id, anyone may be set")
			}

			// user grants on slack are now allowed: the messaging container
			// forwards (team_id, slack_user_id) and the resolver looks up
			// the linked WorkOS user via slack_identity_mappings. Slack
			// users who haven't linked their identity still fall through
			// to the owning-account / anyone candidates — a user grant
			// just won't match for them.

			// Detect duplicates within the same spec.
			key := adapter + "|"
			switch {
			case g.Anyone:
				key += "anyone:"
			case g.UserID != "":
				key += "user:" + g.UserID
			case g.Org != "":
				key += "org:" + g.Org
			}
			if _, dup := seen[key]; dup {
				errs = append(errs, prefix+": duplicate grant")
			}
			seen[key] = struct{}{}
		}
	}

	if auth.Web != nil {
		check(authorizationstore.AdapterWeb, auth.Web.Grants)
	}
	if auth.Slack != nil {
		check(authorizationstore.AdapterSlack, auth.Slack.Grants)
	}
	return errs
}

// mergeAuthorizationFromStore overlays the deployment's stored grants onto the
// template's interfaces.auth so the UI reflects the live access state. Used
// by the deployment-template prefill path on redeploys. Grants are dispatched
// into auth.web.grants or auth.slack.grants based on each row's adapter.
func mergeAuthorizationFromStore(log *logger.Logger, authzStore *authorizationstore.Store, deploymentID string, auth *spec.DeploymentInterfacesAuth) {
	grants, err := authzStore.ListGrants(deploymentID)
	if err != nil {
		log.Error("Failed to list authorization grants", "error", err, "deployment_id", deploymentID)
		return
	}

	if auth.Web != nil {
		auth.Web.Grants = nil
	}
	if auth.Slack != nil {
		auth.Slack.Grants = nil
	}
	for _, g := range grants {
		sg := storeGrantToSpec(g)
		switch g.Adapter {
		case authorizationstore.AdapterWeb:
			if auth.Web == nil {
				auth.Web = &spec.DeploymentWebAuth{}
			}
			auth.Web.Grants = append(auth.Web.Grants, sg)
		case authorizationstore.AdapterSlack:
			if auth.Slack == nil {
				auth.Slack = &spec.DeploymentSlackAuth{}
			}
			auth.Slack.Grants = append(auth.Slack.Grants, sg)
		}
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
			ID            string    `json:"id"`
			AgentName     string    `json:"agent_name"`
			Revision      int       `json:"revision"`
			BuildID       string    `json:"build_id"`
			Namespace     string    `json:"namespace"`
			DisplayName   string    `json:"display_name"`
			IsCurrent     bool      `json:"is_current"`
			Status        string    `json:"status"`
			DeployedAt    time.Time `json:"deployed_at"`
			Spec          any       `json:"spec"`
			Source        string    `json:"source"`
			CommitSHA     string    `json:"commit_sha,omitempty"`
			Branch        string    `json:"branch,omitempty"`
			CommitMessage string    `json:"commit_message,omitempty"`
			RepoFullName  string    `json:"repo_full_name,omitempty"`
		}

		records := make([]revisionRecord, 0, len(history))
		for _, r := range history {
			records = append(records, revisionRecord{
				ID:            r.DeploymentID,
				AgentName:     r.AgentName,
				Revision:      r.Revision,
				BuildID:       r.BuildID,
				Namespace:     r.Namespace,
				DisplayName:   r.DisplayName,
				IsCurrent:     r.IsCurrent,
				Status:        r.Status,
				DeployedAt:    r.DeployedAt,
				Spec:          map[string]any{},
				Source:        r.Source,
				CommitSHA:     r.CommitSHA,
				Branch:        r.Branch,
				CommitMessage: r.CommitMessage,
				RepoFullName:  r.RepoFullName,
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
