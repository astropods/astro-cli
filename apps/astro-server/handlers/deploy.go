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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"sync"

	spec "github.com/astropods/astro-spec"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/authorizationstore"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/clustercfg"
	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/colorextract"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploycontroller"
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
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	"github.com/astropods/astro/apps/astro-server/internal/specsign"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// TemplateCache caches generated base templates (after generateTemplate + mergeDeploymentPrefill)
// so that adapter re-triggers in the POST template endpoint only run ShapeTemplate.
type TemplateCache struct {
	m   sync.Map
	ttl time.Duration
}

const templateCacheTTL = 5 * time.Minute

func NewTemplateCache() *TemplateCache {
	return &TemplateCache{ttl: templateCacheTTL}
}

type templateCacheEntry struct {
	template  *deployment.AstroDeploymentSpec
	models    []deployment.ModelSelection // gateway model selectors (template-layer)
	expiresAt time.Time
}

func (tc *TemplateCache) get(key string) (*deployment.AstroDeploymentSpec, []deployment.ModelSelection, bool) {
	if tc == nil {
		return nil, nil, false
	}
	val, ok := tc.m.Load(key)
	if !ok {
		return nil, nil, false
	}
	entry := val.(*templateCacheEntry)
	if time.Now().After(entry.expiresAt) {
		tc.m.Delete(key)
		return nil, nil, false
	}
	return entry.template, entry.models, true
}

func (tc *TemplateCache) set(key string, tmpl *deployment.AstroDeploymentSpec, models []deployment.ModelSelection) {
	if tc == nil {
		return
	}
	tc.m.Store(key, &templateCacheEntry{
		template:  tmpl,
		models:    models,
		expiresAt: time.Now().Add(tc.ttl),
	})
}

// DeleteByDeploymentID removes all cached entries for the given deployment ID so
// the next template prefill re-fetches live state (e.g. after a redeploy updates grants).
func (tc *TemplateCache) DeleteByDeploymentID(deploymentID string) {
	if tc == nil {
		return
	}
	needle := ":" + deploymentID + ":"
	tc.m.Range(func(k, _ any) bool {
		if key, ok := k.(string); ok && strings.Contains(key, needle) {
			tc.m.Delete(key)
		}
		return true
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

// Keep in sync with DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH in
// apps/astro-client/src/components/deploy/constants.ts.
const deploymentDisplayNameMaxLength = 42

// validateAgentDisplayName trims and validates a deployment display name.
func validateAgentDisplayName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("display_name must not be empty")
	}
	if utf8.RuneCountInString(name) > deploymentDisplayNameMaxLength {
		return "", fmt.Errorf("display_name must be %d characters or fewer", deploymentDisplayNameMaxLength)
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
func UpdateDeploymentDisplayName(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, auditStore *auditlog.Store, cache k8scache.Cache, fgaSync *authz.DeploymentFGASyncStore, queue DeployQueue) gin.HandlerFunc {
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

		var fgaRecorded bool
		var txFn func(*sql.Tx) error
		if name != dep.DisplayName {
			txFn = func(tx *sql.Tx) error {
				var recordErr error
				fgaRecorded, recordErr = fgaSync.RecordNameUpdateTx(c.Request.Context(), tx, dep.ID)
				return recordErr
			}
		}
		if err := deployStore.UpdateDisplayNameWithTx(dep.ID, name, txFn); err != nil {
			log.Error("failed to update display name", "deployment_id", dep.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update display name"})
			return
		}
		if fgaQueue, ok := queue.(deploymentFGAQueue); ok && fgaRecorded {
			if err := fgaQueue.InsertDeploymentFGAReconcileJob(c.Request.Context(), dep.ID); err != nil {
				log.Warn("Failed to enqueue deployment FGA reconciliation",
					"deployment_id", dep.ID,
					"desired_state", authz.DeploymentFGARegistered,
					"error", err,
				)
			}
		}
		_ = deploycache.Invalidate(c.Request.Context(), cache, dep.AccountID)

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
func parseDeploySpec(c *gin.Context) (*deployment.AstroDeploymentSpec, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	return deployment.ParseDeploymentSpec(body)
}

// applyAccountClusterPlacement sets target.cluster_id from the target account's
// placement binding. Empty binding clears cluster_id (primary cluster).
func applyAccountClusterPlacement(ds *deployment.AstroDeploymentSpec, targetAcct *account.Account) {
	if ds == nil || targetAcct == nil {
		return
	}
	if targetAcct.ClusterID != nil && *targetAcct.ClusterID != "" {
		ds.Target.ClusterID = *targetAcct.ClusterID
	} else {
		ds.Target.ClusterID = ""
	}
}

// validateDeployTargetCluster rejects deploys to unknown, disabled, or unhealthy
// additional clusters. Empty clusterID means primary and skips validation.
func validateDeployTargetCluster(
	c *gin.Context,
	log *logger.Logger,
	clusterStore *clusterstore.Store,
	k8sReg *k8s.Registry,
	clusterID string,
) bool {
	if clusterID == "" {
		return true
	}
	if clusterStore == nil {
		log.Error("Deploy specifies cluster_id but clusterStore is not configured", "cluster_id", clusterID)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster store not configured"})
		return false
	}
	// Route through k8sReg.GetEntry so subsequent calls in this request
	// (clustercfg.Resolve at deploy submit time) reuse the cached entry
	// instead of issuing a second SELECT.
	_ = clusterStore // kept in the signature for tests that pass it in
	_, lookupErr := k8sReg.GetEntry(c.Request.Context(), clusterID)
	if lookupErr != nil {
		if errors.Is(lookupErr, k8s.ErrClusterNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "unknown cluster_id",
				"cluster_id": clusterID,
			})
			return false
		}
		log.Error("Failed to look up cluster", "cluster_id", clusterID, "error", lookupErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cluster lookup failed"})
		return false
	}
	if healthErr := clusterHealthForDeploy(c.Request.Context(), k8sReg, clusterID); healthErr != nil {
		if !k8sRegistryReady(k8sReg) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
			return false
		}
		log.Warn("Deploy rejected: cluster unhealthy",
			"cluster_id", clusterID,
			"error", healthErr,
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "cluster is unhealthy",
			"cluster_id": clusterID,
			"details":    k8s.PublicClusterHealthDetail(healthErr),
		})
		return false
	}
	return true
}

func respondDeploymentTemplate(
	c *gin.Context,
	cfg *config.Config,
	resp *deployment.TemplateResponse,
	targetAcct *account.Account,
	finalize bool,
) {
	applyAccountClusterPlacement(&resp.Template, targetAcct)
	if finalize {
		resp.Signature = specsign.Sign(cfg.Deployment.TemplateSigningKey, &resp.Template)
	}
	c.JSON(http.StatusOK, resp)
}

// prepareDeployment parses the submitted spec, authenticates the caller, looks up
// the registered agent build, regenerates the server's template, enforces Rule 19,
// and returns everything needed to proceed with deployment or validation.
type deployContext struct {
	acct                *account.Account
	sourceAccountName   string // account that owns the blueprint (may differ from acct on cross-account deploys)
	sourceAccountID     string
	agentName           string
	displayName         string
	previousDisplayName string
	deploymentID        string
	buildID             string
	k8sNS               string
	isUpdate            bool // true when deployment_id was provided (in-place update)
	resolveResult       *deployment.ResolveResult
	varRefs             map[string]string // variable name → original account variable ref (before resolution)
}

type configuredSecretMode int

const (
	restoreConfiguredSecrets configuredSecretMode = iota
	validateConfiguredSecrets
)

// hydratePreservedInlineSecrets resolves signed Configured sentinels entirely
// inside the deploy request. Finalized configure templates never contain the
// stored plaintext; after signature verification and account authorization,
// this function restores it from encrypted deployment storage and clears the
// template-only marker before validation and persistence. Preflight validation
// only validates the marker shape and leaves it opaque.
func hydratePreservedInlineSecrets(
	ctx context.Context,
	ds *deployment.AstroDeploymentSpec,
	existing *deploymentstore.Deployment,
	store *deploymentstore.Store,
	cfg *config.Config,
	mode configuredSecretMode,
) error {
	preserved := make([]string, 0)
	for name, variable := range ds.Variables {
		if !variable.Configured {
			continue
		}
		if !variable.Secret || variable.Value != "" || variable.Ref != "" {
			return fmt.Errorf("variables.%s has an invalid configured-secret marker", name)
		}
		preserved = append(preserved, name)
	}
	if len(preserved) == 0 {
		return nil
	}
	if existing == nil || store == nil {
		if mode == validateConfiguredSecrets {
			return nil
		}
		return fmt.Errorf("configured secrets can only be preserved for an existing deployment")
	}

	stored, err := store.GetDeploymentVariables(existing.ID)
	if err != nil {
		return fmt.Errorf("load stored deployment variables: %w", err)
	}
	byName := make(map[string]deploymentstore.Variable, len(stored))
	for _, variable := range stored {
		if deploymentstore.IsInlineSecret(variable) {
			byName[variable.Name] = variable
		}
	}

	dec, err := deploymentstore.NewDeploymentDecryptor(
		ctx,
		existing.EncryptedDataKey,
		cfg.Deployment.KMSKeyARN,
	)
	if err != nil {
		return fmt.Errorf("initialize deployment secret decryptor: %w", err)
	}

	for _, name := range preserved {
		storedVariable, ok := byName[name]
		if !ok {
			return fmt.Errorf("variables.%s has no stored inline secret to preserve", name)
		}
		plaintext, ok := deploymentstore.PlaintextValue(dec, storedVariable)
		if !ok {
			return fmt.Errorf("variables.%s could not be decrypted", name)
		}
		variable := ds.Variables[name]
		variable.Value = plaintext
		variable.Configured = false
		ds.Variables[name] = variable
	}
	return nil
}

func prepareDeployment(
	c *gin.Context,
	log *logger.Logger,
	submittedSpec *deployment.AstroDeploymentSpec,
	accountStore *account.AccountStore,
	agentIndex *agentindex.Index,
	cfg *config.Config,
	deployStore *deploymentstore.Store,
	varsStore *accountvars.Store,
	configuredSecrets configuredSecretMode,
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

	applyAccountClusterPlacement(submittedSpec, targetAcct)

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

	agentName := submittedSpec.Source.Name
	if err := spec.ValidateName(agentName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid agent name %q: %v", agentName, err),
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

	// Look up the exact build referenced in the spec (from source account).
	// Signature verification covers tamper detection but the build can still
	// be gone if it was deleted between template generation and deploy.
	if _, err := agentIndex.GetVersion(sourceAcct.ID, agentName, buildID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "agent build not found",
			"details": fmt.Sprintf("no build %q found for agent %q", buildID, agentName),
		})
		return nil, false
	}

	// Resolve identity. The deployment id is the only handle for "redeploy this
	// thing" — it is what binds a row to its K8s namespace forever. Without an
	// explicit Target.DeploymentID, every deploy is a brand-new row with a
	// fresh id and a namespace derived from that id. display_name uniqueness
	// is enforced atomically by the (account_id, display_name) partial unique
	// index at INSERT time — surfaced below as a 409 — so no pre-check is
	// needed here.
	var k8sNamespace, deploymentID string
	var existing *deploymentstore.Deployment
	var isUpdate bool
	if submittedSpec.Target.DeploymentID != "" && deployStore != nil {
		existing, _ = deployStore.GetDeploymentByID(submittedSpec.Target.DeploymentID)
		if existing == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found for given deployment_id"})
			return nil, false
		}
		if existing.Status != deploymentstore.StatusActive && existing.Status != deploymentstore.StatusFailed {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deployment is not active or failed"})
			return nil, false
		}
		if existing.AccountID != targetAcct.ID {
			c.JSON(http.StatusForbidden, gin.H{"error": "deployment does not belong to target account"})
			return nil, false
		}
		deploymentID = existing.ID
		k8sNamespace = existing.Namespace
		isUpdate = true
	}

	// Sanitize and validate the optional display name.
	if dn := submittedSpec.Target.DisplayName; strings.TrimSpace(dn) != "" {
		validated, err := validateAgentDisplayName(dn)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return nil, false
		}
		submittedSpec.Target.DisplayName = validated
	}
	if existing != nil && strings.TrimSpace(submittedSpec.Target.DisplayName) == "" {
		submittedSpec.Target.DisplayName = existing.DisplayName
	}

	if k8sNamespace == "" {
		deploymentID = deployid.New()
		k8sNamespace = deploymentNamespace(deploymentID)
	}

	displayName := submittedSpec.Target.DisplayName

	if err := hydratePreservedInlineSecrets(c.Request.Context(), submittedSpec, existing, deployStore, cfg, configuredSecrets); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "failed to preserve configured secret",
			"details": err.Error(),
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

	// Deploy-time invariant: a slack-enabled deploy must always carry at
	// least one slack grant. Without it the messaging container's signed
	// token has an empty anyone_adapters claim and the bot is unreachable.
	// The template-path seed already covers fresh deploys, but CLI-direct
	// deploys and templates with web-only grants fall through to here.
	ensureSlackAnyoneGrant(submittedSpec)
	ensureSlackAnyoneGrant(resolveResult.Spec)

	if authErrs := validateAuthorizationSpec(submittedSpec); len(authErrs) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "authorization grants invalid",
			"validation_errors": toValidationErrors(authErrs),
		})
		return nil, false
	}
	previousDisplayName := ""
	if existing != nil {
		previousDisplayName = existing.DisplayName
	}

	return &deployContext{
		acct:                targetAcct,
		sourceAccountName:   sourceAcct.Name,
		sourceAccountID:     sourceAcct.ID,
		agentName:           agentName,
		displayName:         displayName,
		previousDisplayName: previousDisplayName,
		deploymentID:        deploymentID,
		buildID:             buildID,
		k8sNS:               k8sNamespace,
		isUpdate:            isUpdate,
		resolveResult:       resolveResult,
		varRefs:             varRefs,
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
// EntitlementChecker is the billing gate: an account is either in good standing
// or suspended. Check returns the gating reason too, so a refusal can name the
// fix instead of a generic billing error. A nil EntitlementChecker skips it.
type EntitlementChecker interface {
	Check(ctx context.Context, accountID string) middleware.Decision
}

// blockedByBilling writes the 402 and reports true when the account is gated.
// Call it after the membership check, because the reason names the account's
// billing state. Call it before any state or precondition check, so a billing
// stop is reported as one rather than as a bad request.
func blockedByBilling(c *gin.Context, entCheck EntitlementChecker, accountID string) bool {
	if entCheck == nil {
		return false
	}
	d := entCheck.Check(c.Request.Context(), accountID)
	if !d.Blocked {
		return false
	}
	c.JSON(http.StatusPaymentRequired, middleware.PaymentRequiredResponse(d.Reason, d.PayLink))
	return true
}

// DeployQueue abstracts job insertion for deploy/undeploy/wakeup operations.
type DeployQueue interface {
	InsertDeployJob(ctx context.Context, deploymentID, clusterID string) error
	InsertUndeployJob(ctx context.Context, deploymentID, clusterID string) error
	InsertWakeUpJob(ctx context.Context, deploymentID, clusterID string) error
}

type deploymentFGAQueue interface {
	InsertDeploymentFGAReconcileJob(ctx context.Context, deploymentID string) error
}

var _ deploymentFGAQueue = (*riverqueue.Queue)(nil)

// EnqueueUndeploy transitions a deployment to "undeploying" and inserts an
// async undeploy job. Used by both UndeployAgent and DeleteAccount.
func EnqueueUndeploy(ctx context.Context, deployStore *deploymentstore.Store, queue DeployQueue, dep *deploymentstore.Deployment) error {
	if dep == nil {
		return fmt.Errorf("nil deployment")
	}
	cid := dep.EffectiveClusterID()
	if err := deployStore.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusUndeploying}); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if err := queue.InsertUndeployJob(ctx, dep.ID, cid); err != nil {
		return fmt.Errorf("insert undeploy job: %w", err)
	}
	return nil
}

func DeployAgent(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, varsStore *accountvars.Store, clusterStore *clusterstore.Store, k8sReg *k8s.Registry, entCheck EntitlementChecker, quotaCheck quota.Checker, queue DeployQueue, avatarStore *avatar.Store, fgaSync *authz.DeploymentFGASyncStore, auditStore *auditlog.Store, ksStore *knowledgestore.Store, authzStore *authorizationstore.Store, imagePreflighter *k8s.ImagePreflighter, tmplCache *TemplateCache, cache k8scache.Cache) gin.HandlerFunc {
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

		// The signed-template flow is the only deploy-time integrity check.
		// Clients must obtain a finalized template from /deployment-template
		// and resubmit it verbatim with the signature header — any tampering
		// breaks the signature, and the server refuses to deploy unsigned
		// specs. /deploy/validate skips this check (no actual deploy occurs).
		if !specsign.Verify(cfg.Deployment.TemplateSigningKey, submittedSpec, c.GetHeader("X-Template-Signature")) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid or missing template signature",
				"details": "deploy requests must carry a signed deployment template; obtain one via POST /agents/:account/:name/deployment-template with finalize=true and resubmit unchanged with the X-Template-Signature header",
			})
			return
		}
		// Authorize signed redeploy attempts before legacy or business validation.
		// Shadow mode records the same attempt without changing its response.
		if submittedSpec.Target.DeploymentID != "" {
			if !middleware.AuthorizeDeploymentAction(c, authz.ActionDeploymentOperate, submittedSpec.Target.DeploymentID) {
				return
			}
		}

		dctx, ok := prepareDeployment(c, log, submittedSpec, accountStore, agentIndex, cfg, deployStore, varsStore, restoreConfiguredSecrets)
		if !ok {
			return
		}
		user, _ := middleware.GetUser(c) // prepareDeployment already requires authentication.

		if !validateDeployTargetCluster(c, log, clusterStore, k8sReg, submittedSpec.Target.ClusterID) {
			return
		}
		// validateDeployTargetCluster warms entryCache via GetEntry. Evict before
		// clustercfg.Resolve so a SQL backfill or boot sync on another replica
		// is visible here (Queen Refresh is not called for station edits).
		if submittedSpec.Target.ClusterID != "" && k8sReg != nil {
			_ = k8sReg.Refresh(c.Request.Context(), submittedSpec.Target.ClusterID)
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
				var bindingErrs []deployment.ValidationError
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

		// Gate deploy (account resolved from spec, not middleware). Resource count
		// (active deployments) is a quota check; compute is a metered-consumption
		// check via the entitlement path. Both fail open on provider error.
		if quotaCheck != nil {
			if res, err := quotaCheck.Check(c.Request.Context(), dctx.acct.ID, quota.ResourceAgentDeployments); err == nil && res.Blocked {
				c.JSON(http.StatusPaymentRequired, quota.LimitResponse(res))
				return
			}
		}
		if blockedByBilling(c, entCheck, dctx.acct.ID) {
			return
		}

		// Persist resolved spec and enqueue async deploy job
		if deployStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment store not configured"})
			return
		}

		stripped := deployment.StripSecretVariableValues(dctx.resolveResult.Spec)
		specJSON, marshalErr := json.Marshal(stripped)
		if marshalErr != nil {
			log.Error("Failed to marshal stripped spec for storage", "error", marshalErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process spec"})
			return
		}

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
			DeployedBy:      user.ID,
			AgentName:       dctx.agentName, DisplayName: dctx.displayName,
			BuildID: dctx.buildID, Namespace: dctx.k8sNS,
			SpecJSON:  string(specJSON),
			ClusterID: submittedSpec.Target.ClusterID,
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
		ingressCfg, ingressErr := clustercfg.Resolve(c.Request.Context(), k8sReg, cfg.Deployment, submittedSpec.Target.ClusterID)
		if ingressErr != nil {
			log.Error("Failed to resolve cluster ingress config", "error", ingressErr, "cluster_id", submittedSpec.Target.ClusterID)
			c.JSON(http.StatusBadRequest, gin.H{"error": ingressErr.Error()})
			return
		}
		var fgaRecorded bool
		txFn := func(tx *sql.Tx, deploymentID string) error {
			nsCfg := &deploymentstore.NormalizedSpecConfig{
				Namespace:              dctx.k8sNS,
				IngressDomain:          ingressCfg.AgentIngressDomain,
				PublicIngressDomain:    ingressCfg.AgentPublicIngressDomain,
				IngestionIngressDomain: ingressCfg.IngestionIngressDomain,
				VarRefs:                dctx.varRefs,
				LocalMode:              cfg.Deployment.K8sClientMode == "local",
			}
			if err := deploymentstore.SaveNormalizedSpec(tx, deploymentID, dctx.resolveResult.Spec, enc, nsCfg); err != nil {
				return err
			}
			if ksStore != nil {
				if err := ksStore.SaveBindings(c.Request.Context(), tx, deploymentID, bindingStoreIDs); err != nil {
					return err
				}
			}
			if applyAuth {
				grants := buildAuthorizationGrants(deploymentID, submittedSpec)
				if err := authorizationstore.ReplaceGrantsTx(tx, deploymentID, grants); err != nil {
					return fmt.Errorf("replace grants: %w", err)
				}
			}
			if !dctx.isUpdate || dctx.previousDisplayName != dctx.displayName {
				var recordErr error
				switch {
				case !dctx.isUpdate:
					fgaRecorded, recordErr = fgaSync.RecordRegistrationTx(c.Request.Context(), tx, deploymentID)
				case dctx.previousDisplayName != dctx.displayName:
					fgaRecorded, recordErr = fgaSync.RecordNameUpdateTx(c.Request.Context(), tx, deploymentID)
				}
				if recordErr != nil {
					return recordErr
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

		tmplCache.DeleteByDeploymentID(dctx.deploymentID)

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
			if copied, copyErr := avatarStore.CopyAgentToDeployment(c.Request.Context(), dctx.sourceAccountName, dctx.agentName, dctx.deploymentID); copyErr != nil {
				log.Warn("Failed to copy blueprint avatar to deployment", "error", copyErr, "deployment_id", dctx.deploymentID)
			} else if copied {
				if _, err := deployStore.TouchAvatarUpdatedAt(dctx.deploymentID); err != nil {
					log.Warn("Failed to stamp deployment avatar_updated_at after copy", "error", err, "deployment_id", dctx.deploymentID)
				}
			}
			if agent, err := agentIndex.Get(dctx.sourceAccountID, dctx.agentName); err == nil && agent.AvatarColors != nil {
				_ = deployStore.SetAvatarColors(dctx.deploymentID, *agent.AvatarColors)
			}
		}

		// Enqueue deploy job (separate from DB transaction; UniqueOpts prevents duplicates)
		if err := queue.InsertDeployJob(c.Request.Context(), dctx.deploymentID, params.ClusterID); err != nil {
			log.Error("Failed to enqueue deploy job", "error", err, "deployment_id", dctx.deploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule deployment"})
			return
		}
		if fgaRecorded {
			if fgaQueue, ok := queue.(deploymentFGAQueue); ok {
				if err := fgaQueue.InsertDeploymentFGAReconcileJob(c.Request.Context(), dctx.deploymentID); err != nil {
					log.Warn("Failed to enqueue deployment FGA reconciliation",
						"deployment_id", dctx.deploymentID,
						"desired_state", authz.DeploymentFGARegistered,
						"error", err,
					)
				}
			}
		}
		// Pre-empt the deploy worker's invalidation: the row is committed and
		// the user may navigate to /agents before the worker picks up the job.
		// Without this bust the cached list still reflects pre-deploy state.
		_ = deploycache.Invalidate(c.Request.Context(), cache, dctx.acct.ID)

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

		dctx, ok := prepareDeployment(c, log, submittedSpec, accountStore, agentIndex, cfg, nil, nil, validateConfiguredSecrets)
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
func UndeployAgent(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, queue DeployQueue, db *sql.DB, auditStore *auditlog.Store) gin.HandlerFunc {
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
		// Authorize parsed undeploy attempts before legacy authorization. Shadow
		// mode records the same attempt without changing its response.
		if !middleware.AuthorizeDeploymentAction(c, authz.ActionDeploymentDelete, req.DeploymentID) {
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
		if err := EnqueueUndeploy(c.Request.Context(), deployStore, queue, dep); err != nil {
			log.Error("Failed to enqueue undeploy job", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule undeploy"})
			return
		}

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
	Name    string `json:"name"`
	URL     string `json:"url"`
	Type    string `json:"type,omitempty"`
	Ready   bool   `json:"ready"`
	Message string `json:"message,omitempty"`
}

// assignEnvToWorkloads fills WorkloadSpec.Env on each workload from the
// deployment_build_env map keyed by role. Env is apply-time intent and
// belongs on the record endpoint, alongside the workload spec.
//
// A workload's component determines which roles apply to it:
//   - "agent"               → ["agent", "messaging"]  (messaging only present when configured)
//   - "collector"           → ["collector"]
//   - "knowledge-<name>"    → ["knowledge:<name>"]
//   - "ingestion-<name>"    → ["ingestion:<name>"]
//
// The agent workload picks up both roles because the messaging sidecar
// shares its pod. Clients project this map down to per-container env using
// their own (component, container_name) → role mapping.
func assignEnvToWorkloads(workloads []WorkloadSpec, envByRole map[string][]deploymentstore.DecryptedEnvVar) {
	if len(envByRole) == 0 {
		return
	}
	for wi := range workloads {
		wl := &workloads[wi]
		for _, role := range rolesForComponent(wl.Component) {
			rows, ok := envByRole[role]
			if !ok || len(rows) == 0 {
				continue
			}
			if wl.Env == nil {
				wl.Env = make(map[string][]EnvVar, 2)
			}
			out := make([]EnvVar, len(rows))
			for i, r := range rows {
				out[i] = EnvVar{
					Name:     r.Name,
					Value:    r.Value,
					Source:   r.Source,
					IsSecret: r.IsSecret,
				}
			}
			wl.Env[role] = out
		}
	}
}

// rolesForComponent lists the deployment_build_env roles that apply to a
// workload of the given component. Returns nil for unknown components
// (e.g. ad-hoc integration sidecars not represented in the unified env
// table).
func rolesForComponent(component string) []string {
	switch {
	case component == "agent":
		// Agent pod hosts both the agent container and the messaging
		// sidecar; surface both roles. The messaging entry is harmless
		// when the deployment isn't configured for messaging (env_by_role
		// just won't have a "messaging" key).
		return []string{string(deployment.RoleAgent), string(deployment.RoleMessaging)}
	case component == "collector":
		return []string{string(deployment.RoleCollector)}
	case strings.HasPrefix(component, "knowledge-"):
		return []string{string(deployment.KnowledgeRole(strings.TrimPrefix(component, "knowledge-")))}
	case strings.HasPrefix(component, "ingestion-"):
		return []string{string(deployment.IngestionRole(strings.TrimPrefix(component, "ingestion-")))}
	}
	return nil
}

// EnvVar represents a single environment variable in a container. Sourced
// from deployment_build_env (the apply-time intent), not from the live pod —
// so the runtime endpoint reflects the deployed spec immediately and doesn't
// hammer the K8s Secret/ConfigMap API on every poll.
type EnvVar struct {
	Name string `json:"name"`
	// Value is plaintext for non-secret entries and RedactedSecretValue
	// ("••••••••") for secrets.
	Value string `json:"value,omitempty"`
	// Source is the categorical provenance from deployment_build_env:
	// 'user_var', 'platform_meta', 'service_url', 'knowledge_cred',
	// 'auth_token', 'adapter_config', or 'derived'.
	Source string `json:"source,omitempty"`
	// IsSecret mirrors deployment_build_env.is_secret. When true, Value is
	// redacted; clients should treat the entry as sensitive.
	IsSecret bool `json:"is_secret,omitempty"`
}

// ContainerStatus represents the live status of a single container in a pod.
// Env is NOT carried here — it's apply-time intent and lives on WorkloadSpec
// (the record endpoint), keyed by role. See WorkloadSpec.Env.
type ContainerStatus struct {
	Name         string `json:"name"`
	State        string `json:"state"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	Message      string `json:"message,omitempty"` // plain-language explanation when unhealthy
}

// WorkloadDetail represents a k8s workload — Deployment, StatefulSet, Job
// (one-shot ingestion / startup), or CronJob (scheduled ingestion).
//
// Field population by Kind:
//   - Deployment / StatefulSet: Phase, PodName, Containers, URLs
//   - Job:                       Status, StartTime, Completions, Containers
//     (from the executing pod, when present)
//   - CronJob:                   Status, Schedule, StartTime (last fire),
//     Runs (child Jobs)
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
	Schedule    string      `json:"schedule,omitempty"`    // cron expression (CronJob only)
	StartTime   string      `json:"start_time,omitempty"`  // Job: pod start. CronJob: last schedule time.
	Completions string      `json:"completions,omitempty"` // "succeeded/desired" (Job only)
	Runs        []JobDetail `json:"runs,omitempty"`        // CronJob children, oldest-first
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
	DeployedBy         string                `json:"deployed_by,omitempty"`
	Components         []string              `json:"components"`
	ManualIngestions   []string              `json:"manual_ingestions,omitempty"`
	ExternalURLs       []ServiceEndpointInfo `json:"external_urls,omitempty"`
	MessagingAvailable bool                  `json:"messaging_available,omitempty"`
	Workloads          []WorkloadDetail      `json:"workloads,omitempty"`
}

// DeploymentRecord is the DB-only view of a deployment — spec, status, URLs,
// metadata, intent-level workload list. No K8s reads. Served by
// GET /deployments/:id; renders instantly even when the cluster is
// unreachable. Live observational state (ready counts, pod state, restart
// counts, age) lives in DeploymentRuntime.
//
// The dividing principle: anything we *wrote* into the deployment at apply
// time (or earlier, at normalization) belongs here. Anything that *the
// cluster reports about itself right now* belongs in DeploymentRuntime.
type DeploymentRecord struct {
	ID                  string                `json:"id"`
	Name                string                `json:"name"`
	DisplayName         string                `json:"display_name,omitempty"`
	AvatarURL           string                `json:"avatar_url,omitempty"`
	AvatarColors        json.RawMessage       `json:"avatar_colors,omitempty"`
	BuildID             string                `json:"build_id"`
	LatestBuildID       string                `json:"latest_build_id,omitempty"`
	SourceAccount       string                `json:"source_account,omitempty"`
	Namespace           string                `json:"namespace"`
	Status              string                `json:"status"`
	ErrorMessage        string                `json:"error_message,omitempty"`
	CreatedAt           string                `json:"created_at"`
	UpdatedAt           string                `json:"updated_at,omitempty"`
	UpdatedBy           string                `json:"updated_by,omitempty"`
	DeployedBy          string                `json:"deployed_by,omitempty"`
	ExternalURLs        []ServiceEndpointInfo `json:"external_urls,omitempty"`
	Replicas            int32                 `json:"replicas"`             // desired (sum of primary workload specs)
	Components          []string              `json:"components"`           // distinct component_kind across workloads + sidecars
	MessagingConfigured bool                  `json:"messaging_configured"` // a messaging sidecar is part of the spec
	Workloads           []WorkloadSpec        `json:"workloads,omitempty"`  // intent — name, kind, image, replicas, etc.
	// ManualIngestions is sourced from a namespace annotation today (live K8s)
	// because the normalized store doesn't persist trigger.type="manual"
	// ingestions (they have no K8s workload to point at). When the spec-side
	// list lands in the DB, move this here. Until then, it lives in
	// DeploymentRuntime.
}

// DeploymentStatus is the coarse, server-derived status of a deployment —
// what the toggle/history badges render. It's the join of the DB status
// enum and the K8s agent-workload readiness, computed once on the server
// so the client doesn't have to reconcile two query results across timing
// windows. Served by GET /deployments/:id/status.
//
// Returned directly (no envelope) — the response body IS this object.
//
// `value` enumerates:
//
//	"active"      — DB active AND observed ready >= desired
//	"deploying"   — DB pending/provisioning, OR DB active but ready<desired
//	"inactive"    — DB stopped (paused) or suspended (billing)
//	"undeploying" — DB undeploying
//	"error"       — DB failed/crashloopbackoff
//
// `reason` is a stable machine-readable label for *why* the value was
// chosen — useful for client branching (e.g. show a tooltip when ready_lag)
// without re-deriving anything. `details` is the human-readable counterpart
// rendered in tooltips / status panels.
//
// Live replica/ready counts and per-workload state live on the runtime
// endpoint; this endpoint intentionally stays narrow. WaitingOn is set only
// while value=="deploying"; FailedOn only while value=="error".
type DeploymentStatus struct {
	Value        string          `json:"value"`
	Reason       string          `json:"reason"`
	Details      string          `json:"details"`
	ErrorMessage string          `json:"error_message,omitempty"`
	WaitingOn    []WorkloadIssue `json:"waiting_on,omitempty"`
	FailedOn     []WorkloadIssue `json:"failed_on,omitempty"`
}

// WaitingPhaseMissing marks a declared workload the controller has not yet
// observed in the cluster; it is not a deploymentstore.WorkloadPhase*.
const WaitingPhaseMissing = "missing"

// WorkloadIssue is one workload keeping a deployment out of "active": still
// settling (WaitingOn — Phase "missing" or an observed phase that isn't ready)
// or terminally broken (FailedOn — Phase "failed"). Message is a plain-language
// explanation; raw K8s reason codes are never surfaced to clients.
type WorkloadIssue struct {
	Workload  string `json:"workload"`
	Component string `json:"component,omitempty"`
	Phase     string `json:"phase"`
	Message   string `json:"message,omitempty"`
	Title     string `json:"title,omitempty"`
	Guidance  string `json:"guidance,omitempty"`
}

// humanizeWorkloadReason maps a K8s reason code to a plain-language explanation.
// Unknown codes return "" so the UI falls back to the phase — raw codes like
// "ImagePullBackOff" are never sent to clients.
func humanizeWorkloadReason(reason string) string {
	switch reason {
	case "ImagePullBackOff", "ErrImagePull":
		return "Couldn't pull the container image"
	case "InvalidImageName":
		return "The container image name is invalid"
	case "CreateContainerError", "RunContainerError":
		return "The container failed to start"
	case "CrashLoopBackOff":
		return "The container keeps crashing on startup"
	case "OOMKilled":
		return "The container ran out of memory"
	case "ProgressDeadlineExceeded":
		return "Timed out waiting to become ready"
	case "BackoffLimitExceeded":
		return "A setup job failed"
	case "DeadlineExceeded":
		return "A setup job timed out"
	case "GenerationLag", "ContainerCreating", "PodInitializing":
		return "Starting up"
	default:
		return ""
	}
}

// Stable reason codes — keep in sync with the client's DeploymentStatusReason
// type. Adding a new reason requires a client type change but never breaks
// existing consumers (they branch on the union and ignore unknown codes).
const (
	StatusReasonPaused       = "paused"       // DB stopped
	StatusReasonSuspended    = "suspended"    // DB suspended (billing)
	StatusReasonUndeploying  = "undeploying"  // DB undeploying
	StatusReasonFailed       = "failed"       // DB failed
	StatusReasonProvisioning = "provisioning" // DB pending/provisioning
	StatusReasonReady        = "ready"        // DB active
)

// DeploymentRuntime is the observed-runtime view — replicas, pods, containers,
// services. Served by GET /deployments/:id/runtime, keyed by workload name so
// the client can stitch this onto DeploymentRecord.Workloads to render the
// detail page. It is projected from the controller-maintained snapshot in
// deployment_runtime_status (not a per-request K8s read); may be empty/zero
// before the controller's first observation, and renderers must tolerate that
// without breaking the record-driven UI.
type DeploymentRuntime struct {
	Ready    int32 `json:"ready"`    // observed ready replicas across the deployment
	Replicas int32 `json:"replicas"` // observed total replicas (may differ from desired during scale events)
	// MessagingReachable is true when the messaging Service object exists AND
	// (when it surfaces in the live pod view) the messaging sidecar container is
	// Ready. It is NOT a bare Service-presence check — a crashed/wedged sidecar
	// reads as unreachable so chat-readiness reflects the sidecar, not just the
	// Service object. Derived from the persisted runtime snapshot; see
	// messagingReachableFromSnapshot.
	MessagingReachable bool `json:"messaging_reachable"`
	// ManualIngestions is currently sourced from a namespace annotation.
	// See DeploymentRecord note — this field should move to the record once
	// the spec-side list is persisted in the DB.
	ManualIngestions []string          `json:"manual_ingestions,omitempty"`
	Workloads        []WorkloadRuntime `json:"workloads,omitempty"`
}

// WorkloadSpec is the DB-sourced intent for a single workload: what we
// asked K8s to run. Mirrors the fields the normalized store persists at
// apply time. URLs come from deployment_ingresses (DB), not from live K8s
// Ingress objects.
type WorkloadSpec struct {
	Name      string                `json:"name"`               // K8s resource name
	Kind      string                `json:"kind"`               // "Deployment", "StatefulSet", "Job", "CronJob"
	Component string                `json:"component"`          // component_kind from deployment_workloads / deployment_sidecars
	Provider  string                `json:"provider,omitempty"` // platform provider (e.g. postgres, qdrant); empty for custom/agent
	Image     string                `json:"image"`
	Replicas  int32                 `json:"replicas"`           // desired
	Schedule  string                `json:"schedule,omitempty"` // cron expression for scheduled ingestion
	URLs      []ServiceEndpointInfo `json:"urls,omitempty"`     // per-workload ingress hostnames
	// Env is the deployment_build_env intent for the role(s) this workload
	// covers, keyed by role. Most workloads have a single role; the agent
	// workload carries both "agent" and "messaging" when a messaging sidecar
	// is configured. Roles: "agent" | "messaging" | "collector" |
	// "knowledge:<name>" | "ingestion:<name>". Clients map a container's
	// (component, container_name) → role to look up its env.
	Env map[string][]EnvVar `json:"env,omitempty"`
}

// WorkloadRuntime is the K8s-sourced live state for a single workload,
// keyed by Name to match the corresponding WorkloadSpec.
type WorkloadRuntime struct {
	Name        string            `json:"name"`               // key into DeploymentRecord.Workloads
	Age         string            `json:"age,omitempty"`      // K8s creationTimestamp
	Phase       string            `json:"phase,omitempty"`    // pod phase for long-running workloads
	PodName     string            `json:"pod_name,omitempty"` // representative pod (for restart UI)
	Containers  []ContainerStatus `json:"containers,omitempty"`
	Status      string            `json:"status,omitempty"`      // Job / CronJob status
	StartTime   string            `json:"start_time,omitempty"`  // Job pod start / CronJob last fire
	Completions string            `json:"completions,omitempty"` // Job "succeeded/desired"
	Runs        []JobDetail       `json:"runs,omitempty"`        // CronJob children

	// Representative pod's data-volume usage; nil when there's no PVC or metrics
	// are unavailable. From Prometheus (see enrichRuntimeStorage), not the snapshot.
	StorageUsedBytes     *int64 `json:"storage_used_bytes,omitempty"`
	StorageCapacityBytes *int64 `json:"storage_capacity_bytes,omitempty"`
}

// AgentDeploymentSummary is the trimmed shape returned by ListDeployments.
// It contains only the fields consumed by the agents-grid card and profile
// page, omitting K8s-derived replica counts, workloads, and other detail-only
// fields that are served by GetDeployment instead.
type AgentDeploymentSummary struct {
	ID                     string                `json:"id"`
	Name                   string                `json:"name"`
	DisplayName            string                `json:"display_name,omitempty"`
	AvatarColors           json.RawMessage       `json:"avatar_colors,omitempty"`
	BuildID                string                `json:"build_id"`
	LatestBuildID          string                `json:"latest_build_id,omitempty"`
	Status                 string                `json:"status,omitempty"`
	Namespace              string                `json:"namespace,omitempty"`
	AccountID              string                `json:"account_id"`
	AccountName            string                `json:"account_name"`
	ExternalURLs           []ServiceEndpointInfo `json:"external_urls,omitempty"`
	MessagingWebConfigured bool                  `json:"messaging_web_configured"`
	CreatedAt              string                `json:"created_at"`
	UpdatedAt              string                `json:"updated_at,omitempty"`
	DeployedBy             string                `json:"deployed_by,omitempty"`
	AccessReady            *bool                 `json:"access_ready,omitempty"`
}

// CountDeployments returns the number of visible deployments for an account.
// It never calls K8s, but FGA-enabled organizations resolve live WorkOS visibility before the DB count.
func CountDeployments(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	discovery deploymentVisibilityResolver,
) gin.HandlerFunc {
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

		visibility, err := resolveDeploymentVisibility(
			c.Request.Context(), discovery, user.ID, singleAccountVisibilityScope(acct),
		)
		if err != nil {
			writeDeploymentVisibilityError(c, log, err)
			return
		}
		var count int
		if visibility.EnforcesAccount(acct.ID) {
			count, err = deployStore.CountReadableDeploymentsForUser(
				c.Request.Context(), user.ID, acct.ID,
				visibility.FGAAccountIDs, visibility.ReadableDeploymentIDs,
			)
		} else {
			count, err = deployStore.CountVisibleDeploymentsByAccount(acct.ID)
		}
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
func ListDeploymentsSummary(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	avatarStore *avatar.Store,
	discovery deploymentVisibilityResolver,
) gin.HandlerFunc {
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
		visibility, err := resolveDeploymentVisibility(c.Request.Context(), discovery, user.ID, accounts)
		if err != nil {
			writeDeploymentVisibilityError(c, log, err)
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
		summaries, err = filterReadableDeployments(
			c.Request.Context(), deployStore.ListReadableDeploymentIDsForUser, user.ID, summaries, visibility,
			func(summary *deploymentstore.DeploymentSummary) string { return summary.ID },
		)
		if err != nil {
			log.Error("Failed to filter deployment summaries", "error", err, "user_id", user.ID)
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
				item.AvatarURL = avatarStore.DeploymentAvatarURL(d.ID, d.AvatarUpdatedAt)
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

type deploymentListDependencies struct {
	log         *logger.Logger
	accounts    *account.AccountStore
	deployments *deploymentstore.Store
	agents      *agentindex.Index
	avatars     *avatar.Store
	audit       *auditlog.Store
}

type deploymentListScope struct {
	id   string
	name string
}

// enrichDeploymentsForAccount loads visible deployments for one account and
// runs the standard enrichment pipeline (messaging URLs, audit timestamps,
// avatars, latest build IDs). Shared between the per-account and cross-
// account paths of ListDeployments so the two paths cannot drift.
func enrichDeploymentsForAccount(
	ctx context.Context,
	dependencies deploymentListDependencies,
	scope deploymentListScope,
	buildIDs []string,
) ([]AgentDeploymentSummary, error) {
	var (
		dbDeps []*deploymentstore.Deployment
		err    error
	)
	if len(buildIDs) > 0 {
		dbDeps, err = dependencies.deployments.GetVisibleDeploymentsByAccountAndBuilds(scope.id, buildIDs)
	} else {
		dbDeps, err = dependencies.deployments.GetVisibleDeploymentsByAccount(scope.id)
	}
	if err != nil {
		return nil, err
	}

	return enrichDeploymentRows(
		ctx,
		dependencies,
		scope,
		dbDeps,
	)
}

func enrichDeploymentRows(
	ctx context.Context,
	dependencies deploymentListDependencies,
	scope deploymentListScope,
	dbDeps []*deploymentstore.Deployment,
) ([]AgentDeploymentSummary, error) {
	allDeployments := make([]AgentDeployment, len(dbDeps))
	depIDs := make([]string, len(dbDeps))
	for i, dbDep := range dbDeps {
		allDeployments[i] = agentDeploymentFromDB(dependencies.log, dbDep)
		depIDs[i] = dbDep.ID
	}

	if messagingURLs, merr := dependencies.deployments.GetMessagingURLsContext(ctx, depIDs); merr != nil {
		dependencies.log.Warn("Failed to load messaging URLs", "error", merr)
	} else {
		for i, d := range allDeployments {
			if url, ok := messagingURLs[d.ID]; ok {
				allDeployments[i].ExternalURLs = []ServiceEndpointInfo{
					{Name: "messaging", Type: "messaging", URL: url, Ready: true},
				}
			}
		}
	}

	messagingWebConfigured := make(map[string]bool)
	if webConfigured, werr := dependencies.deployments.GetMessagingWebConfigured(ctx, depIDs); werr != nil {
		dependencies.log.Warn("Failed to load messaging web configured flags", "error", werr)
	} else {
		messagingWebConfigured = webConfigured
	}

	if dependencies.audit != nil && len(allDeployments) > 0 {
		latestMap, err := dependencies.audit.LatestPerResource(ctx, scope.id, "deployment", depIDs)
		if err != nil {
			dependencies.log.Warn("Failed to load audit timestamps for deployments", "error", err)
		} else {
			for i, d := range allDeployments {
				if latest, ok := latestMap[d.ID]; ok {
					allDeployments[i].UpdatedAt = latest.UpdatedAt.Format(time.RFC3339)
					allDeployments[i].UpdatedBy = latest.ActorID
				}
			}
		}

		deployMap, err := dependencies.audit.LatestPerResourceByAction(
			ctx,
			scope.id,
			auditlog.DeploymentDeploy,
			"deployment",
			depIDs,
		)
		if err != nil {
			dependencies.log.Warn("Failed to load deployment authors", "error", err)
		} else {
			for i, d := range allDeployments {
				if latest, ok := deployMap[d.ID]; ok {
					allDeployments[i].DeployedBy = latest.ActorID
				}
			}
		}
	}

	dbColorsByID := make(map[string]json.RawMessage, len(dbDeps))
	for _, dep := range dbDeps {
		if dep.AvatarColors != nil {
			dbColorsByID[dep.ID] = *dep.AvatarColors
		}
	}

	if dependencies.avatars != nil {
		for i := range allDeployments {
			allDeployments[i].AvatarURL = dependencies.avatars.DeploymentAvatarURL(dbDeps[i].ID, dbDeps[i].AvatarUpdatedAt)
		}
	}
	for i, d := range allDeployments {
		if len(allDeployments[i].AvatarColors) == 0 {
			if colors, ok := dbColorsByID[d.ID]; ok {
				allDeployments[i].AvatarColors = colors
			}
		}
		if dependencies.avatars != nil {
			allDeployments[i].AvatarColors = colorextract.EnsureCurrent(ctx, allDeployments[i].AvatarColors,
				func(ctx context.Context) ([]byte, error) { return dependencies.avatars.ReadDeploymentAvatar(ctx, d.ID) },
				func(ctx context.Context, j []byte) error { return dependencies.deployments.SetAvatarColors(d.ID, j) },
			)
		}
	}

	populateLatestBuildIDs(dependencies.log, dependencies.agents, dependencies.accounts, dbDeps, allDeployments)

	summaries := make([]AgentDeploymentSummary, len(allDeployments))
	for i, d := range allDeployments {
		summaries[i] = AgentDeploymentSummary{
			ID:                     d.ID,
			Name:                   d.Name,
			DisplayName:            d.DisplayName,
			AvatarColors:           d.AvatarColors,
			BuildID:                d.BuildID,
			LatestBuildID:          d.LatestBuildID,
			Status:                 d.Status,
			Namespace:              d.Namespace,
			AccountID:              scope.id,
			AccountName:            scope.name,
			ExternalURLs:           d.ExternalURLs,
			MessagingWebConfigured: messagingWebConfigured[d.ID],
			CreatedAt:              d.CreatedAt,
			UpdatedAt:              d.UpdatedAt,
			DeployedBy:             d.DeployedBy,
		}
	}
	return summaries, nil
}

// maxBuildIDFilter caps the number of build IDs accepted in a single
// request. The blueprint sidebar — the only known caller — passes one
// entry per version on the blueprint, which in practice stays well under
// this limit. The cap exists so a misbehaving caller can't expand
// build_id = ANY($N) into an array large enough to bother the query
// planner or balloon parameter memory.
const maxBuildIDFilter = 200

// parseBuildIDFilter extracts the build_id query filter. Accepts either a
// single comma-separated value (?build_id=a,b,c) or repeated params
// (?build_id=a&build_id=b). Empty/whitespace entries are dropped. Returns
// nil when the filter is absent so callers can branch on len()==0.
//
// The size cap is enforced at the handler (400 on overflow) rather than
// here, so this stays a pure input → output transform.
func parseBuildIDFilter(c *gin.Context) []string {
	raw := c.QueryArray("build_id")
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ListDeployments returns the list of deployments visible to the account.
// When `account` is omitted, returns deployments across every account the
// authenticated user belongs to.
//
// DB-ONLY. Do not add Kubernetes reads to this handler. Live operational
// state (replicas, pods, workload status, events) is exposed via the per-
// deployment runtime endpoint (GET /api/v1/deployments/:id/runtime) so the
// dashboard renders deterministically off the database even when a cluster
// is unreachable. Mixing K8s state in here re-creates the source-of-truth
// drift the record/runtime split was introduced to fix.
//
// The messaging Launch URL comes from deployment_ingresses via
// GetMessagingURLs — that table is the authoritative URL source for both
// local-mode (NodePort, written post-apply) and remote (Ingress hostname,
// written at normalization).
func ListDeployments(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	agentIdx *agentindex.Index,
	avatarStore *avatar.Store,
	auditStore *auditlog.Store,
	cache k8scache.Cache,
	discovery deploymentVisibilityResolver,
) gin.HandlerFunc {
	dependencies := deploymentListDependencies{
		log:         log,
		accounts:    accountStore,
		deployments: deployStore,
		agents:      agentIdx,
		avatars:     avatarStore,
		audit:       auditStore,
	}
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

		if deployStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "deployment store not configured",
			})
			return
		}

		// Optional ?build_id= filter. Accepts a comma-separated list or
		// repeated query params; the SQL layer applies it as build_id =
		// ANY(...). Pushing the filter into SQL keeps the cross-account
		// response bounded by the natural cardinality of "deployments of
		// this blueprint" rather than "all deployments across all of the
		// viewer's accounts".
		buildIDs := parseBuildIDFilter(c)
		if len(buildIDs) > maxBuildIDFilter {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("build_id accepts at most %d values, got %d", maxBuildIDFilter, len(buildIDs)),
			})
			return
		}

		// Cross-account fan-out: when `account` is omitted, aggregate
		// deployments across every account the user belongs to. Lets the
		// blueprint detail page surface the viewer's deployments regardless
		// of which account they live in. Cache is per-account, so this
		// path skips the cache.
		accountName := c.Query("account")

		// Require build_id when account is omitted. The cross-account path
		// exists for the blueprint sidebar, which always knows the blueprint's
		// builds. Allowing an unfiltered cross-account fan-out would return
		// every deployment in every account the user belongs to in a single
		// uncached response — refuse it explicitly rather than silently
		// truncating. If a future caller legitimately needs the unfiltered
		// firehose, add cursor pagination then.
		if accountName == "" && len(buildIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "build_id query parameter is required when account is omitted",
			})
			return
		}

		if accountName == "" {
			accounts, err := accountStore.GetAccountsForUser(user.ID)
			if err != nil {
				log.Error("Failed to load accounts for user", "error", err, "user_id", user.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load accounts"})
				return
			}
			visibility, err := resolveDeploymentVisibility(c.Request.Context(), discovery, user.ID, accounts)
			if err != nil {
				writeDeploymentVisibilityError(c, log, err)
				return
			}

			// Per-account enrichment is independent — run in parallel. A
			// single account's failure is logged and skipped so the rest
			// of the response still renders. errgroup.SetLimit caps
			// concurrent DB/avatar fan-out so a user in many accounts
			// can't stampede the pool.
			perAccount := make([][]AgentDeploymentSummary, len(accounts))
			var g errgroup.Group
			g.SetLimit(8)
			ctx := c.Request.Context()
			for i, a := range accounts {
				g.Go(func() error {
					summaries, err := enrichDeploymentsForAccount(
						ctx,
						dependencies,
						deploymentListScope{id: a.ID, name: a.Name},
						buildIDs,
					)
					if err != nil {
						log.Warn("Failed to load deployments for account in cross-account list", "account_id", a.ID, "error", err)
						return nil
					}
					perAccount[i] = summaries
					return nil
				})
			}
			_ = g.Wait()

			total := 0
			for _, s := range perAccount {
				total += len(s)
			}
			combined := make([]AgentDeploymentSummary, 0, total)
			for _, s := range perAccount {
				combined = append(combined, s...)
			}
			combined, err = filterReadableDeployments(
				c.Request.Context(), deployStore.ListReadableDeploymentIDsForUser, user.ID, combined, visibility,
				func(summary AgentDeploymentSummary) string { return summary.ID },
			)
			if err != nil {
				log.Error("Failed to filter cross-account deployments", "error", err, "user_id", user.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"deployments": combined,
				"count":       len(combined),
			})
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
		visibility, err := resolveDeploymentVisibility(
			c.Request.Context(), discovery, user.ID, singleAccountVisibilityScope(acct),
		)
		if err != nil {
			writeDeploymentVisibilityError(c, log, err)
			return
		}

		log.Info("Listing deployments",
			"account", accountName,
			"user_id", user.ID,
		)

		var response ListDeploymentsResponse
		if len(buildIDs) == 0 && !visibility.EnforcesAccount(acct.ID) {
			if cached, ok := deploycache.Get(c.Request.Context(), cache, acct.ID); ok {
				c.Data(http.StatusOK, "application/json", cached)
				return
			}

			var summaries []AgentDeploymentSummary
			summaries, err = enrichDeploymentsForAccount(
				c.Request.Context(),
				dependencies,
				deploymentListScope{id: acct.ID, name: acct.Name},
				nil,
			)
			response = ListDeploymentsResponse{
				Deployments: summaries,
				Count:       len(summaries),
			}
		} else {
			var summaries []AgentDeploymentSummary
			summaries, err = enrichDeploymentsForAccount(
				c.Request.Context(),
				dependencies,
				deploymentListScope{id: acct.ID, name: acct.Name},
				buildIDs,
			)
			response = ListDeploymentsResponse{
				Deployments: summaries,
				Count:       len(summaries),
			}
		}
		if err != nil {
			log.Error("Failed to load deployments from DB", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to list deployments",
				"details": err.Error(),
			})
			return
		}
		response.Deployments, err = filterReadableDeployments(
			c.Request.Context(), deployStore.ListReadableDeploymentIDsForUser, user.ID, response.Deployments, visibility,
			func(summary AgentDeploymentSummary) string { return summary.ID },
		)
		if err != nil {
			log.Error("Failed to filter deployments", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
			return
		}
		response.Count = len(response.Deployments)
		if len(buildIDs) == 0 && !visibility.EnforcesAccount(acct.ID) {
			body, marshalErr := json.Marshal(response)
			if marshalErr != nil {
				log.Error("Failed to encode deployment list", "account_id", acct.ID, "error", marshalErr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode deployments"})
				return
			}
			if cacheErr := deploycache.Put(c.Request.Context(), cache, acct.ID, body); cacheErr != nil {
				log.Warn("Failed to cache deployment list", "account_id", acct.ID, "error", cacheErr)
			}
			c.Data(http.StatusOK, "application/json", body)
			return
		}
		c.JSON(http.StatusOK, response)
	}
}

// populateLatestBuildIDs fills LatestBuildID on each deployment in `deps` from
// a single batch query against agent_versions. Looks up the lineage agent —
// which is the source account when set, falling back to the owning account —
// so cross-account deploys still see upgrade signals from the publisher.
// `agentIdx` is used both for lineage tuple validation (via LineageValidator)
// and for BatchLatestBuildIDs.
//
// Cross-account refs whose source blueprint is private are suppressed: the
// deploy endpoint refuses to honor a private blueprint across an account
// boundary (canDeploySourceAgent), so advertising an upgrade the user can't
// act on would be a false positive.
//
// Quietly leaves LatestBuildID empty on lookup failure rather than failing the
// whole list response: this is a UX hint, not load-bearing data.
func populateLatestBuildIDs(log *logger.Logger, agentIdx *agentindex.Index, accountStore *account.AccountStore, dbDeps []*deploymentstore.Deployment, deps []AgentDeployment) {
	if agentIdx == nil || len(dbDeps) == 0 || len(deps) == 0 {
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
		pubID, _ := validatedLineagePublisher(log, accountStore, agentIdx, dbDep)
		if pubID == "" || dbDep.AgentName == "" {
			continue
		}
		ref := agentindex.AgentVersionRef{AccountID: pubID, Name: dbDep.AgentName}
		crossAccount := pubID != dbDep.AccountID
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
	latest, err := agentIdx.BatchLatestBuildIDs(refs)
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
		agent, err := agentIdx.Get(ref.AccountID, ref.Name)
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

// GetDeployment returns the DB-sourced view of a single deployment — spec,
// URL, metadata, coarse status.
//
// DB-ONLY. Do not add Kubernetes reads to this handler. The companion
// GetDeploymentRuntime (GET /api/v1/deployments/:id/runtime) exposes live
// K8s state — replicas, pods, workload conditions, MessagingAvailable.
// Keeping the two endpoints disjoint is load-bearing:
//
//   - The detail page renders instantly from the DB and keeps working when
//     the cluster is briefly unreachable.
//   - deployment_ingresses is the single source of truth for the Launch URL
//     (both local NodePort and remote Ingress hostname); the K8s Ingress /
//     Service overlay this code used to do is now considered duplication.
//   - Caching and polling cadences can differ per concern: the DB record
//     changes on apply / status transitions, the runtime view changes on
//     every kubelet heartbeat.
//
// If you find yourself wanting a K8s field here, that field belongs in the
// runtime endpoint instead — extend DeploymentRuntime, not DeploymentRecord.
// GET /api/v1/deployments/:id
func GetDeployment(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, agentIdx *agentindex.Index, avatarStore *avatar.Store, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := middleware.GetUser(c); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dbDep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		sourceAccount := resolveSourceAccountName(log, accountStore, agentIdx, dbDep)
		record := deploymentRecordFromDB(dbDep, sourceAccount)

		// CreatedAt should reflect when the deployment first appeared, not the
		// latest revision's deployed_at. Match the historical behavior of
		// enrichDeployment so the detail page's "created" timestamp doesn't
		// jump on every redeploy.
		if firstEventAt, evErr := deployStore.GetDeploymentFirstEventAt(dbDep.ID); evErr != nil {
			log.Warn("Failed to load first deployment event", "error", evErr, "deployment_id", dbDep.ID)
		} else if firstEventAt != nil {
			record.CreatedAt = firstEventAt.Format(time.RFC3339)
		}

		if auditStore != nil {
			latestMap, auditErr := auditStore.LatestPerResource(c.Request.Context(), dbDep.AccountID, "deployment", []string{dbDep.ID})
			if auditErr != nil {
				log.Warn("Failed to load audit timestamps for deployment", "error", auditErr)
			} else if latest, ok := latestMap[dbDep.ID]; ok {
				record.UpdatedAt = latest.UpdatedAt.Format(time.RFC3339)
				record.UpdatedBy = latest.ActorID
			}

			deployMap, deployAuditErr := auditStore.LatestPerResourceByAction(
				c.Request.Context(),
				dbDep.AccountID,
				auditlog.DeploymentDeploy,
				"deployment",
				[]string{dbDep.ID},
			)
			if deployAuditErr != nil {
				log.Warn("Failed to load deployment author", "error", deployAuditErr)
			} else if latest, ok := deployMap[dbDep.ID]; ok {
				record.DeployedBy = latest.ActorID
			}
		}

		if avatarStore != nil {
			record.AvatarURL = avatarStore.DeploymentAvatarURL(dbDep.ID, dbDep.AvatarUpdatedAt)
			record.AvatarColors = colorextract.EnsureCurrent(c.Request.Context(), record.AvatarColors,
				func(ctx context.Context) ([]byte, error) { return avatarStore.ReadDeploymentAvatar(ctx, dbDep.ID) },
				func(ctx context.Context, j []byte) error { return deployStore.SetAvatarColors(dbDep.ID, j) },
			)
		}

		// LatestBuildID — surface the "new build available" upgrade affordance.
		// Reuses the list endpoint's lineage-aware batch lookup. We pass a
		// throwaway AgentDeployment slice (the helper writes back into it)
		// and copy the resolved ID onto the record.
		tmp := []AgentDeployment{{ID: record.ID, Name: record.Name, BuildID: record.BuildID}}
		populateLatestBuildIDs(log, agentIdx, accountStore, []*deploymentstore.Deployment{dbDep}, tmp)
		record.LatestBuildID = tmp[0].LatestBuildID

		// Intent-shaped fields (workload list, components, desired replicas,
		// messaging-configured) from the normalized store. These describe
		// what the deployment is supposed to be running; live state lives in
		// the runtime endpoint.
		record.Workloads, record.Components, record.Replicas, record.MessagingConfigured = loadRecordIntentFromDB(log, deployStore, dbDep.ID)

		// Env vars are apply-time intent — read from deployment_build_env and
		// attach to each WorkloadSpec by role. This is the only env surface;
		// the runtime endpoint intentionally carries none. Reading from the
		// DB avoids the per-poll K8s Secret/ConfigMap GET-storm and surfaces
		// the deployed spec immediately, with no rolling-update lag.
		kmsKeyARN := ""
		if cfg != nil {
			kmsKeyARN = cfg.Deployment.KMSKeyARN
		}
		envByRole, envErr := deployStore.LoadDecryptedBuildEnv(c.Request.Context(), dbDep, kmsKeyARN)
		if envErr != nil {
			log.Warn("LoadDecryptedBuildEnv failed", "error", envErr, "deployment_id", dbDep.ID)
		}
		assignEnvToWorkloads(record.Workloads, envByRole)

		// Overlay the messaging Launch URL. Resolution order:
		//   1. MessagingURLOverride (config) — wins when set, so a developer
		//      pointing at a local messaging stub can override whatever the
		//      DB has. Also flags messaging_configured=true so the Launch
		//      button shows even on agents without a real messaging sidecar.
		//   2. deployment_ingresses row from GetMessagingURLs — the regular
		//      source of truth for the URL (local NodePort or remote
		//      ingress hostname).
		// Exactly one messaging entry is added to ExternalURLs.
		if override := cfg.Deployment.MessagingURLOverride; override != "" {
			record.MessagingConfigured = true
			record.ExternalURLs = append(record.ExternalURLs, ServiceEndpointInfo{
				Name: "messaging", Type: "messaging", URL: override, Ready: true,
			})
		} else if messagingURLs, merr := deployStore.GetMessagingURLsContext(c.Request.Context(), []string{dbDep.ID}); merr != nil {
			log.Warn("Failed to load messaging URL for deployment", "error", merr, "deployment_id", dbDep.ID)
		} else if url, ok := messagingURLs[dbDep.ID]; ok {
			record.ExternalURLs = append(record.ExternalURLs, ServiceEndpointInfo{
				Name: "messaging", Type: "messaging", URL: url, Ready: true,
			})
		}

		c.JSON(http.StatusOK, gin.H{"deployment": record})
	}
}

// GetDeploymentRuntime returns the observed runtime state for a single
// deployment — replicas, pods, containers, workloads, services. It reads the
// controller-maintained snapshot in deployment_runtime_status rather than
// hitting the K8s API per request, so it is cluster-independent: a disabled or
// unreachable cluster returns the last-observed snapshot (or an empty runtime
// the UI renders as "loading"), never a 503. The controller keeps the snapshot
// current from its informer caches.
// GET /api/v1/deployments/:id/runtime
func GetDeploymentRuntime(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, promClient *promquery.Client, k8sReg *k8s.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := middleware.GetUser(c); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dbDep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		// Read the controller-maintained runtime snapshot instead of hitting the
		// K8s API per request. The snapshot is cluster-independent: a disabled or
		// briefly unreachable cluster returns the last-observed state (or an empty
		// runtime, which the UI renders as "loading"), never a 503.
		snap, _, err := deployStore.GetRuntimeSnapshot(dbDep.ID)
		if err != nil {
			log.Warn("GetRuntimeSnapshot failed", "deployment_id", dbDep.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read runtime"})
			return
		}

		rt := runtimeFromSnapshot(snap, dbDep, cfg)

		// Overlay live volume usage (Prometheus) onto StatefulSet workloads.
		// resolvedProm may differ from promClient when dbDep's cluster has its
		// own prometheus_url override; see k8s.Registry.PrometheusClientFor.
		resolvedProm := k8sReg.PrometheusClientFor(c.Request.Context(), dbDep.EffectiveClusterID(), promClient)
		clusterFilter := ""
		if resolvedProm != nil && k8sReg != nil {
			clusterFilter = k8sReg.PrometheusClusterFilter(c.Request.Context(), dbDep.EffectiveClusterID())
		}
		enrichRuntimeStorage(c.Request.Context(), runtimeStorageCache, resolvedProm, clusterFilter, dbDep.ID, dbDep.Namespace, statefulSetWorkloadNames(snap), &rt)

		c.JSON(http.StatusOK, gin.H{"runtime": rt})
	}
}

// GetDeploymentStatus returns the server-derived coarse status of a single
// deployment — the join of the DB status enum and live K8s agent-workload
// readiness, computed in one place so the client doesn't have to reconcile
// two queries across timing windows. The status badge / pause toggle / tile
// each call this directly and read `status` verbatim.
//
// Lightweight: at most one K8s Get (StatefulSet, or Deployment for legacy
// agents) on the agent workload — skipped entirely for non-active DB statuses.
// Polls cheaply.
// GET /api/v1/deployments/:id/status
func GetDeploymentStatus(log *logger.Logger, accountStore *account.AccountStore, k8sReg *k8s.Registry, deployStore *deploymentstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := middleware.GetUser(c); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dbDep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		status := DeploymentStatus{}
		if dbDep.ErrorMessage != nil {
			status.ErrorMessage = *dbDep.ErrorMessage
		}

		// DB status precedence — these statuses are authoritative regardless of
		// what K8s reports. Pause, undeploy, failed, and explicit transitional
		// states all resolve here without a cluster round-trip.
		switch dbDep.Status {
		case deploymentstore.StatusStopped:
			status.Value, status.Reason = "inactive", StatusReasonPaused
			status.Details = "Deployment is paused"
			c.JSON(http.StatusOK, status)
			return
		// Deliberately off, not broken, so inactive rather than error.
		case deploymentstore.StatusSuspended:
			status.Value, status.Reason = "inactive", StatusReasonSuspended
			status.Details = "Stopped by billing"
			c.JSON(http.StatusOK, status)
			return
		case deploymentstore.StatusUndeploying:
			status.Value, status.Reason = "undeploying", StatusReasonUndeploying
			status.Details = "Deployment is being torn down"
			c.JSON(http.StatusOK, status)
			return
		case deploymentstore.StatusFailed:
			status.Value, status.Reason = "error", StatusReasonFailed
			status.Details = "Deployment failed"
			if status.ErrorMessage != "" {
				status.Details = "Deployment failed: " + status.ErrorMessage
			}
			// Name the workloads that failed and why, rather than a bare reason code.
			if expected, expErr := deployStore.GetWorkloads(dbDep.ID); expErr == nil {
				if observed, obsErr := deployStore.GetWorkloadStatuses(dbDep.ID); obsErr == nil {
					status.FailedOn = failedWorkloads(expected, observed)
					if len(status.FailedOn) > 0 {
						status.Details = failedDetail(status.FailedOn)
					}
				}
			}
			c.JSON(http.StatusOK, status)
			return
		case deploymentstore.StatusPending, deploymentstore.StatusProvisioning:
			status.Value, status.Reason = "deploying", StatusReasonProvisioning
			status.Details = "Pods are being provisioned"
			c.JSON(http.StatusOK, status)
			return
		case deploymentstore.StatusDeploying:
			status.Value, status.Reason = "deploying", StatusReasonProvisioning
			status.Details = "Waiting for workloads to become ready"
			// Name the declared workloads still blocking the transition to active;
			// best-effort, falling back to the static message on a read error.
			if expected, expErr := deployStore.GetWorkloads(dbDep.ID); expErr == nil {
				if observed, obsErr := deployStore.GetWorkloadStatuses(dbDep.ID); obsErr == nil {
					status.WaitingOn = waitingOnFromWorkloads(expected, observed)
					if len(status.WaitingOn) > 0 {
						status.Details = deployingDetail(status.WaitingOn)
					}
				}
			}
			c.JSON(http.StatusOK, status)
			return
		}

		// DB status is active — the deployment controller made that transition
		// only after observing every declared workload ready, and keeps it
		// current via informers. Trust it directly instead of a per-request K8s
		// probe; summarize from the persisted per-workload status.
		status.Value, status.Reason = "active", StatusReasonReady
		status.Details = "Deployment is active"
		if ws, wsErr := deployStore.GetWorkloadStatuses(dbDep.ID); wsErr == nil && len(ws) > 0 {
			ready := 0
			for _, w := range ws {
				if w.Phase == deploymentstore.WorkloadPhaseReady || w.Phase == deploymentstore.WorkloadPhaseComplete {
					ready++
				}
			}
			status.Details = fmt.Sprintf("%d of %d workloads ready", ready, len(ws))
		}
		c.JSON(http.StatusOK, status)
	}
}

// waitingOnFromWorkloads mirrors deploycontroller.aggregateDeploymentPhase:
// every declared workload that is unobserved ("missing") or observed but not yet
// ready/complete. An empty result means the gate would pass.
func waitingOnFromWorkloads(expected []*deploymentstore.Workload, observed []deploymentstore.WorkloadStatus) []WorkloadIssue {
	byName := make(map[string]deploymentstore.WorkloadStatus, len(observed))
	for _, o := range observed {
		byName[o.WorkloadName] = o
	}
	var waiting []WorkloadIssue
	for _, w := range expected {
		o, ok := byName[w.Name]
		if !ok {
			waiting = append(waiting, WorkloadIssue{
				Workload:  w.Name,
				Component: w.ComponentKind,
				Phase:     WaitingPhaseMissing,
			})
			continue
		}
		if o.Phase == deploymentstore.WorkloadPhaseReady || o.Phase == deploymentstore.WorkloadPhaseComplete {
			continue
		}
		waiting = append(waiting, WorkloadIssue{
			Workload:  w.Name,
			Component: w.ComponentKind,
			Phase:     o.Phase,
			Message:   humanizeWorkloadReason(o.Reason),
		})
	}
	return waiting
}

// failedWorkloads returns the observed workloads in phase "failed", carrying the
// failure reason/message and the declared component kind.
func failedWorkloads(expected []*deploymentstore.Workload, observed []deploymentstore.WorkloadStatus) []WorkloadIssue {
	component := make(map[string]string, len(expected))
	for _, w := range expected {
		component[w.Name] = w.ComponentKind
	}
	var failed []WorkloadIssue
	for _, o := range observed {
		if o.Phase != deploymentstore.WorkloadPhaseFailed {
			continue
		}
		title, guidance, _, _ := deploycontroller.HumanizeEvent(o.Reason, o.Message)
		failed = append(failed, WorkloadIssue{
			Workload:  o.WorkloadName,
			Component: component[o.WorkloadName],
			Phase:     o.Phase,
			Message:   humanizeWorkloadReason(o.Reason),
			Title:     title,
			Guidance:  guidance,
		})
	}
	return failed
}

// deployingDetail names up to maxNamed waiting workloads and elides the rest,
// e.g. "Waiting for foo (not yet created), bar (ContainerCreating) and 2 more".
func deployingDetail(waiting []WorkloadIssue) string {
	const maxNamed = 3
	parts := make([]string, 0, maxNamed)
	for i, w := range waiting {
		if i == maxNamed {
			break
		}
		switch {
		case w.Phase == WaitingPhaseMissing:
			parts = append(parts, w.Workload+" (not yet created)")
		case w.Message != "":
			parts = append(parts, fmt.Sprintf("%s (%s)", w.Workload, w.Message))
		default:
			parts = append(parts, fmt.Sprintf("%s (%s)", w.Workload, w.Phase))
		}
	}
	detail := "Waiting for " + strings.Join(parts, ", ")
	if extra := len(waiting) - maxNamed; extra > 0 {
		detail += fmt.Sprintf(" and %d more", extra)
	}
	return detail
}

// failedDetail names up to maxNamed failed workloads with their reason,
// e.g. "Deployment failed: sasbot-agent (ImagePullBackOff)".
func failedDetail(failed []WorkloadIssue) string {
	const maxNamed = 3
	parts := make([]string, 0, maxNamed)
	for i, w := range failed {
		if i == maxNamed {
			break
		}
		if w.Message != "" {
			parts = append(parts, fmt.Sprintf("%s (%s)", w.Workload, w.Message))
		} else {
			parts = append(parts, w.Workload)
		}
	}
	detail := "Deployment failed: " + strings.Join(parts, ", ")
	if extra := len(failed) - maxNamed; extra > 0 {
		detail += fmt.Sprintf(" and %d more", extra)
	}
	return detail
}

// messagingContainerName is the K8s container name of the messaging sidecar.
// It runs as a native sidecar (init container with Always restart policy) in
// the agent pod, so its readiness surfaces in the agent workload's container
// statuses (buildContainerStatuses merges InitContainerStatuses).
const messagingContainerName = "messaging"

// runtimeFromSnapshot projects the controller's persisted RuntimeSnapshot into
// the DeploymentRuntime response. A nil snapshot (the controller hasn't observed
// the deployment yet) yields an empty runtime — the UI renders that as "loading"
// and the next poll picks it up once the controller writes the snapshot.
func runtimeFromSnapshot(snap *deploymentstore.RuntimeSnapshot, dbDep *deploymentstore.Deployment, cfg *config.Config) DeploymentRuntime {
	if snap == nil {
		return DeploymentRuntime{}
	}
	rt := DeploymentRuntime{
		Ready:              snap.Ready,
		Replicas:           snap.Replicas,
		ManualIngestions:   snap.ManualIngestions,
		Workloads:          workloadRuntimesFromSnapshot(snap.Workloads),
		MessagingReachable: messagingReachableFromSnapshot(snap, dbDep.AgentName),
	}
	if cfg.Deployment.MessagingURLOverride != "" {
		rt.MessagingReachable = true
	}
	return rt
}

// messagingReachableFromSnapshot mirrors the old live check against the
// snapshot: the messaging Service must exist AND, when the messaging sidecar
// container surfaces in the pod view, it must be Ready. A present Service with
// no observed sidecar reports reachable — the same as the live path's
// "not found → fall back to Service presence" behavior.
func messagingReachableFromSnapshot(snap *deploymentstore.RuntimeSnapshot, agentName string) bool {
	svcName := deployment.GenerateAgentResourceName(agentName, "messaging")
	serviceExists := false
	for _, s := range snap.Services {
		if s.Name == svcName {
			serviceExists = true
			break
		}
	}
	if !serviceExists {
		return false
	}
	for _, w := range snap.Workloads {
		for _, p := range w.Pods {
			for _, ctr := range p.Containers {
				if ctr.Name == messagingContainerName {
					return ctr.Ready // first observed sidecar decides
				}
			}
		}
	}
	return true
}

// workloadRuntimesFromSnapshot projects each snapshot workload to the slim
// live-state WorkloadRuntime the client stitches onto the record by name. It
// selects one representative pod per workload (the same rule the live path used:
// prefer a Running pod, then the newest) for the pod-detail fields.
func workloadRuntimesFromSnapshot(workloads []deploymentstore.RuntimeWorkload) []WorkloadRuntime {
	if len(workloads) == 0 {
		return nil
	}
	out := make([]WorkloadRuntime, 0, len(workloads))
	for _, w := range workloads {
		wr := WorkloadRuntime{Name: w.Name, Status: w.Status}
		if !w.CreatedAt.IsZero() {
			wr.Age = formatAge(w.CreatedAt)
		}
		if best := representativePod(w.Pods); best != nil {
			wr.PodName = best.Name
			wr.Phase = best.Phase
			wr.Containers = containerStatusesFromSnapshot(best.Containers)
		}
		out = append(out, wr)
	}
	return out
}

// representativePod picks the pod that best represents a workload's live state:
// prefer a Running pod, and among pods of the same phase prefer the newest.
func representativePod(pods []deploymentstore.RuntimePod) *deploymentstore.RuntimePod {
	var best *deploymentstore.RuntimePod
	for i := range pods {
		p := &pods[i]
		switch {
		case best == nil:
			best = p
		case p.Phase == string(corev1.PodRunning) && best.Phase != string(corev1.PodRunning):
			best = p
		case p.Phase == best.Phase && p.CreatedAt.After(best.CreatedAt):
			best = p
		}
	}
	return best
}

func containerStatusesFromSnapshot(cs []deploymentstore.RuntimeContainer) []ContainerStatus {
	if len(cs) == 0 {
		return nil
	}
	out := make([]ContainerStatus, 0, len(cs))
	for _, c := range cs {
		out = append(out, ContainerStatus{
			Name:         c.Name,
			State:        c.State,
			Ready:        c.Ready,
			RestartCount: c.RestartCount,
			Message:      humanizeWorkloadReason(c.Reason),
		})
	}
	return out
}

// loadRecordIntentFromDB pulls the intent-shaped fields (workload specs,
// components, desired replicas, messaging-configured flag) out of
// deployment_workloads + deployment_sidecars. Best-effort: a query failure
// logs and leaves the field at its zero value rather than failing the
// whole record response — the page still renders, just without that
// detail. Returns zero values when no rows exist.
func loadRecordIntentFromDB(log *logger.Logger, deployStore *deploymentstore.Store, deploymentID string) (workloads []WorkloadSpec, components []string, desiredReplicas int32, messagingConfigured bool) {
	summaries, err := deployStore.GetWorkloadSummaries(deploymentID)
	if err != nil {
		log.Warn("GetWorkloadSummaries failed", "deployment_id", deploymentID, "error", err)
	}
	sidecars, err := deployStore.GetSidecars(deploymentID)
	if err != nil {
		log.Warn("GetSidecars failed", "deployment_id", deploymentID, "error", err)
	}
	workloadURLs, err := deployStore.GetWorkloadIngresses(deploymentID)
	if err != nil {
		log.Warn("GetWorkloadIngresses failed", "deployment_id", deploymentID, "error", err)
	}

	seenComp := make(map[string]struct{}, len(summaries)+len(sidecars))
	addComp := func(c string) {
		if c == "" {
			return
		}
		if _, ok := seenComp[c]; ok {
			return
		}
		seenComp[c] = struct{}{}
		components = append(components, c)
	}

	for _, w := range summaries {
		addComp(w.ComponentKind)
		// Sum desired replicas across primary-shape workloads. Job/CronJob have
		// Replicas=0 (one-shot or scheduled) so they contribute nothing — that
		// matches the historical K8s-derived behavior.
		if w.WorkloadType == "deployment" || w.WorkloadType == "statefulset" {
			desiredReplicas += int32(w.Replicas) //nolint:gosec
		}
		// "manual"-trigger ingestion entries don't have a workload row at all
		// (they get filtered out at normalization), so they don't appear here.
		var urls []ServiceEndpointInfo
		for _, u := range workloadURLs[w.Name] {
			urls = append(urls, ServiceEndpointInfo{Name: "http", Type: "http", URL: u, Ready: true})
		}
		workloads = append(workloads, WorkloadSpec{
			Name: w.Name,
			Kind: workloadKindForType(w.WorkloadType),
			// Component mirrors the K8s "app.kubernetes.io/component" label
			// convention ("knowledge-<key>", "ingestion-<key>" when keyed)
			// so it matches WorkloadRuntime.Component on the runtime endpoint
			// and feeds correctly into rolesForComponent below.
			Component: componentLabelFor(w.ComponentKind, w.ComponentKey),
			Provider:  w.Provider,
			Image:     w.Image,
			Replicas:  int32(w.Replicas), //nolint:gosec
			Schedule:  w.TriggerSchedule,
			URLs:      urls,
		})
	}
	for _, sc := range sidecars {
		addComp(sc.ComponentKind)
		if sc.ComponentKind == "messaging" {
			messagingConfigured = true
		}
	}
	return workloads, components, desiredReplicas, messagingConfigured
}

// componentLabelFor reconstructs the "app.kubernetes.io/component" label
// value from a workload's stored (component_kind, component_key) pair. For
// keyed components (knowledge, ingestion) the K8s convention is
// "<kind>-<key>" (e.g. "knowledge-cache"); for unkeyed components (agent,
// collector, messaging) it's just the kind. Keeping this aligned with the
// K8s label avoids a record/runtime divergence on WorkloadSpec.Component
// and lets rolesForComponent work uniformly across both endpoints.
func componentLabelFor(kind, key string) string {
	if key == "" {
		return kind
	}
	return kind + "-" + key
}

// workloadKindForType maps the lowercase workload_type column ("deployment",
// "statefulset", "job", "cronjob") to the Pascal-cased K8s Kind the API
// surface uses. Unknown values pass through capitalized as-is.
var workloadKindByType = map[string]string{
	"deployment":  "Deployment",
	"statefulset": "StatefulSet",
	"job":         "Job",
	"cronjob":     "CronJob",
}

func workloadKindForType(t string) string {
	if k, ok := workloadKindByType[t]; ok {
		return k
	}
	if t == "" {
		return ""
	}
	return strings.ToUpper(t[:1]) + t[1:]
}

// dbStatusToUIStatus maps the canonical DB status enum to the loose status
// string the client renders ("Running", "pending", "Stopped", "undeploying",
// "error"). Single source of truth — both deploymentRecordFromDB and
// agentDeploymentFromDB delegate here, so a new status enum value lights up
// every consumer at once.
func dbStatusToUIStatus(s string) string {
	switch s {
	case deploymentstore.StatusActive:
		return "Running"
	case deploymentstore.StatusPending, deploymentstore.StatusProvisioning, deploymentstore.StatusDeploying:
		return "pending"
	case deploymentstore.StatusStopped:
		return "Stopped"
	case deploymentstore.StatusUndeploying:
		return "undeploying"
	}
	return "error"
}

// deploymentRecordFromDB projects a stored Deployment into the public
// DeploymentRecord view. Mirrors agentDeploymentFromDB but produces the
// thin DB-only shape returned by GET /deployments/:id.
func deploymentRecordFromDB(dep *deploymentstore.Deployment, sourceAccount string) DeploymentRecord {
	r := DeploymentRecord{
		ID:            dep.ID,
		Name:          dep.AgentName,
		DisplayName:   dep.DisplayName,
		BuildID:       dep.BuildID,
		Namespace:     dep.Namespace,
		Status:        dbStatusToUIStatus(dep.Status),
		SourceAccount: sourceAccount,
		CreatedAt:     dep.DeployedAt.Format(time.RFC3339),
	}
	if dep.AvatarColors != nil {
		r.AvatarColors = *dep.AvatarColors
	}
	if dep.ErrorMessage != nil && *dep.ErrorMessage != "" {
		r.ErrorMessage = *dep.ErrorMessage
	}
	return r
}

// agentDeploymentFromDB builds an AgentDeployment entry from a DB record alone,
// used when K8s resources are unavailable (failed, pending, or missing namespace).
func agentDeploymentFromDB(log *logger.Logger, dep *deploymentstore.Deployment) AgentDeployment {
	ad := AgentDeployment{
		ID:          dep.ID,
		Name:        dep.AgentName,
		DisplayName: dep.DisplayName,
		BuildID:     dep.BuildID,
		Namespace:   dep.Namespace,
		Status:      dbStatusToUIStatus(dep.Status),
		Replicas:    0,
		Ready:       0,
		CreatedAt:   dep.DeployedAt.Format(time.RFC3339),
		Components:  []string{},
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
	if dep.Status == deploymentstore.StatusFailed {
		ad.Status = "error"
	}

	return ad
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

// RestartDeployment triggers a rolling restart of all Deployments and StatefulSets for an agent
// by patching spec.template.metadata.annotations with kubectl.kubernetes.io/restartedAt.
// Kubernetes performs a rolling update: new pod starts and becomes ready before the old one is terminated.
// POST /api/v1/deployments/:id/restart
func RestartDeployment(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sReg *k8s.Registry, deployStore *deploymentstore.Store, auditStore *auditlog.Store, entCheck EntitlementChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
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
func RestartPod(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sReg *k8s.Registry, deployStore *deploymentstore.Store, auditStore *auditlog.Store, entCheck EntitlementChecker) gin.HandlerFunc {
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
		if blockedByBilling(c, entCheck, dep.AccountID) {
			return
		}

		k8sClient, ok := clusterClientForDeployment(c, k8sReg, dep)
		if !ok {
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

// GetDeploymentEvents returns the deployment's Kubernetes events from the
// controller-maintained snapshot (deployment_runtime_status) instead of a live
// K8s read: the controller lists, scopes to current objects, humanizes, and
// sorts them newest-first each sync (see deploycontroller.buildDeploymentEvents).
func GetDeploymentEvents(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store) gin.HandlerFunc {
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

		snap, _, err := deployStore.GetRuntimeSnapshot(dep.ID)
		if err != nil {
			log.Error("Failed to read events snapshot", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read deployment events"})
			return
		}

		resp := DeploymentEventsResponse{Events: []K8sEventItem{}}
		if snap != nil {
			resp.Events = make([]K8sEventItem, 0, len(snap.Events))
			for _, e := range snap.Events {
				resp.Events = append(resp.Events, K8sEventItem{
					Type:           e.Type,
					Reason:         e.Reason,
					Message:        e.Message,
					ObjectKind:     e.ObjectKind,
					ObjectName:     e.ObjectName,
					Count:          e.Count,
					FirstTimestamp: e.FirstTimestamp,
					LastTimestamp:  e.LastTimestamp,
					Title:          e.Title,
					Guidance:       e.Guidance,
					Severity:       e.Severity,
				})
			}
		}
		c.JSON(http.StatusOK, resp)
	}
}

func GetDeploymentLogs(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sReg *k8s.Registry, deployStore *deploymentstore.Store, lokiClient *loki.Client) gin.HandlerFunc {
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
		// resolvedLoki may differ from lokiClient when dep's cluster has its
		// own loki_url override; see k8s.Registry.LokiClientFor.
		resolvedLoki := k8sReg.LokiClientFor(c.Request.Context(), dep.EffectiveClusterID(), lokiClient)

		var k8sClient k8s.ClusterClient
		if resolvedLoki == nil {
			var ok bool
			k8sClient, ok = clusterClientForDeployment(c, k8sReg, dep)
			if !ok {
				return
			}
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
			Cluster:   k8sReg.LokiClusterName(c.Request.Context(), dep.EffectiveClusterID()),
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
		if resolvedLoki == nil && podName == "" && workloadName != "" {
			resolved, listErr := resolvePodForStream(c.Request.Context(), k8sClient, dep.Namespace, workloadName, containerName)
			if listErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list pods", "details": listErr.Error()})
				return
			}
			podName = resolved
		}

		if resolvedLoki == nil && podName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pod or workload query parameter is required"})
			return
		}

		logOpts := &corev1.PodLogOptions{TailLines: &tailLines, Timestamps: true}
		if containerName != "" {
			logOpts.Container = containerName
		}

		streamLogs(c, log, resolvedLoki, lokiParams, k8sClient, dep.Namespace, podName, logOpts, loc)
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
func StreamDeploymentLogs(log *logger.Logger, accountStore *account.AccountStore, k8sReg *k8s.Registry, deployStore *deploymentstore.Store, lokiClient *loki.Client, heartbeatInterval ...time.Duration) gin.HandlerFunc {
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
		// resolvedLoki may differ from lokiClient when dep's cluster has its
		// own loki_url override; see k8s.Registry.LokiClientFor.
		resolvedLoki := k8sReg.LokiClientFor(c.Request.Context(), dep.EffectiveClusterID(), lokiClient)

		var k8sClient k8s.ClusterClient
		if resolvedLoki == nil {
			var ok bool
			k8sClient, ok = clusterClientForDeployment(c, k8sReg, dep)
			if !ok {
				return
			}
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
		if resolvedLoki != nil {
			backend = "loki"
		} else if k8sClient != nil {
			backend = "k8s"
		}

		log.Debug("SSE stream requested",
			"deployment", dep.ID, "namespace", dep.Namespace,
			"workload", workloadName, "container", containerName, "pod", podName,
			"backend", backend)

		if resolvedLoki == nil && k8sClient == nil {
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

		if resolvedLoki != nil {
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

				ch, tailErr := resolvedLoki.TailLogs(c.Request.Context(), loki.QueryParams{
					Namespace: dep.Namespace,
					Cluster:   k8sReg.LokiClusterName(c.Request.Context(), dep.EffectiveClusterID()),
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
// column, then the owning account_id. When `v` is wired, each candidate
// publisher must pass ValidateLineage against (agent_name, build_id) or
// attribution is suppressed. Returns "" when neither source is available;
// callers treat that as "same account" and use the URL account.
func resolveSourceAccountName(log *logger.Logger, accountStore *account.AccountStore, v deploymentstore.LineageValidator, d *deploymentstore.Deployment) string {
	_, name := validatedLineagePublisher(log, accountStore, v, d)
	return name
}

// validatedLineagePublisher returns the publisher account UUID and display
// name when lineage can be attributed. When `v` is nil or agent_name/build_id
// are incomplete, tuple checks are skipped so older tests and sparse rows keep
// pre-PR5 semantics.
func validatedLineagePublisher(log *logger.Logger, accountStore *account.AccountStore, v deploymentstore.LineageValidator, dep *deploymentstore.Deployment) (pubID string, pubName string) {
	v = deploymentstore.EffectiveLineageValidator(v)
	if accountStore == nil {
		return "", ""
	}

	lineageChecksActive := func() bool {
		return v != nil && dep.AgentName != "" && dep.BuildID != ""
	}

	tryTuple := func(candidateID string) bool {
		if !lineageChecksActive() {
			return true
		}
		if err := v.ValidateLineage(candidateID, dep.AgentName, dep.BuildID); err != nil {
			log.Warn("Deployment lineage tuple invalid for candidate publisher account",
				"deployment_id", dep.ID,
				"candidate_account_id", candidateID,
				"agent_name", dep.AgentName,
				"build_id", dep.BuildID,
				"error", err,
			)
			return false
		}
		return true
	}

	finalizePublisher := func(candidateID string) (pubID string, pubName string) {
		acct, err := accountStore.GetByID(candidateID)
		if err != nil {
			log.Warn("Failed to resolve lineage publisher account id",
				"deployment_id", dep.ID, "account_id", candidateID, "error", err)
		}
		if acct != nil {
			return acct.ID, acct.Name
		}
		specName := deploymentstore.SourceAccountFromSpec(dep.DeploymentSpecJSON)
		if specName != "" {
			return candidateID, specName
		}
		return candidateID, ""
	}

	// Pair a lineage-validated publisher id with display name once tryTuple succeeds.
	publish := func(candidateID string) (string, string) {
		if candidateID == "" || !tryTuple(candidateID) {
			return "", ""
		}
		return finalizePublisher(candidateID)
	}

	if dep.SourceAccountID != nil && *dep.SourceAccountID != "" {
		if id, name := publish(*dep.SourceAccountID); id != "" {
			return id, name
		}
	}

	if srcName := deploymentstore.SourceAccountFromSpec(dep.DeploymentSpecJSON); srcName != "" {
		acct, err := accountStore.GetByName(srcName)
		if err != nil {
			log.Debug("Legacy spec source.account lookup failed",
				"deployment_id", dep.ID,
				"source_account_name", srcName,
				"error", err,
			)
		}
		if acct != nil {
			candidateID := acct.ID
			if lineageChecksActive() && !tryTuple(candidateID) {
				// already logged in tryTuple — fall through to owning account candidate
			} else if id, name := finalizePublisher(candidateID); id != "" {
				return id, name
			}
		}
	}

	if dep.AccountID != "" {
		return publish(dep.AccountID)
	}
	return "", ""
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
) (*deployment.AstroDeploymentSpec, []deployment.ModelSelection, bool) {
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
		return nil, nil, false
	}

	specBytes, err := json.Marshal(agentVersion.Spec)
	if err != nil {
		log.Error("Failed to marshal stored spec", "error", err, "account", acct.Name, "agent", name)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process spec", "details": err.Error()})
		return nil, nil, false
	}
	var astroSpec spec.AstroSpec
	if err := json.Unmarshal(specBytes, &astroSpec); err != nil {
		log.Error("Failed to unmarshal spec into AstroSpec", "error", err, "account", acct.Name, "agent", name, "raw", string(specBytes))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse spec", "details": err.Error()})
		return nil, nil, false
	}

	template, err := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
		Spec:              &astroSpec,
		AgentName:         agent.Name,
		Account:           acct.Name,
		BuildID:           agentVersion.BuildID,
		RegistryURL:       cfg.Deployment.RegistryURL,
		ProxyRegistryHost: cfg.Deployment.ProxyRegistryHost,
		Environment:       cfg.Deployment.Environment,
		MessagingImage:    cfg.Deployment.MessagingImage,
	})
	if err != nil {
		log.Error("Failed to generate deployment template", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to generate deployment template",
			"details": err.Error(),
		})
		return nil, nil, false
	}

	return template, deployment.GatewayModelSelections(&astroSpec), true
}

// PostDeploymentTemplate returns a handler for the interactive POST deployment-template endpoint.
// POST /api/v1/agents/:account/:name/deployment-template
func PostDeploymentTemplate(log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, cfg *config.Config, deployStore *deploymentstore.Store, ksStore *knowledgestore.Store, authzStore *authorizationstore.Store, cache *TemplateCache) gin.HandlerFunc {

	return func(c *gin.Context) {
		var req deployment.TemplateRequest
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

			targetAcct, err := accountStore.GetByID(existing.AccountID)
			if err != nil {
				log.Error("Failed to look up deployment target account", "error", err, "account_id", existing.AccountID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up account"})
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
			sourceAccountName := resolveSourceAccountName(log, accountStore, agentIndex, prefillExisting)
			lookupAccountName := accountName
			if sourceAccountName != "" {
				lookupAccountName = sourceAccountName
			}

			// Restore knowledge bindings from the stored spec so that the
			// initial template load (before the user sends explicit bindings)
			// correctly shapes bound entries and populates the binding picker.
			deployment.ApplyStoredBindingsToRequest(log, &req, prefillExisting.DeploymentSpecJSON)

			// Storage vars flow into req (not into the cached template) so
			// they apply through ShapeTemplate's variable-filling pass, which
			// runs after ApplyAdapterShaping has injected adapter-owned
			// variables. Fetch on every request (cache hit too) — the cached
			// template doesn't carry stored values any more.
			storedVars, err := deployStore.GetDeploymentVariables(req.DeploymentID)
			if err != nil {
				log.Error("Failed to get deployment variables", "error", err, "deployment_id", req.DeploymentID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get deployment variables"})
				return
			}
			applyStoredVarsToRequest(&req, storedVars)

			// Check cache — skips generateTemplate + merge on hit.
			cacheKey := accountName + ":" + sourceAccountName + ":" + agentName + ":" + buildIDOverride + ":" + req.DeploymentID + ":" + strconv.Itoa(req.Revision)
			if base, models, ok := cache.get(cacheKey); ok {
				shapeOpts = shapeOptsWithConfiguredInlineSecrets(shapeOpts, storedVars)
				applyStoredModelChoices(&req, models, prefillExisting.DeploymentSpecJSON)
				shapeOpts = shapeOptsWithModels(shapeOpts, models)
				resp := deployment.ShapeTemplate(c.Request.Context(), base, &req, shapeOpts)
				// Display name is mutable outside the deploy flow — always
				// apply the current value from the DB, not the cached one.
				resp.Template.Target.DisplayName = prefillExisting.DisplayName
				respondDeploymentTemplate(c, cfg, resp, targetAcct, req.Finalize)
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
			template, models, ok := generateTemplate(c, log, agentIndex, cfg, acct, agent, buildIDOverride)
			if !ok {
				return
			}

			mergeDeploymentPrefill(log, template, prefillExisting, accountStore, deployStore, authzStore)

			// For historical revisions, also override display name from the stored spec.
			if req.Revision > 0 {
				var storedSpec deployment.AstroDeploymentSpec
				if jsonErr := json.Unmarshal([]byte(prefillExisting.DeploymentSpecJSON), &storedSpec); jsonErr == nil {
					if storedSpec.Target.DisplayName != "" {
						template.Target.DisplayName = storedSpec.Target.DisplayName
					}
				}
			}

			applyStoredModelChoices(&req, models, prefillExisting.DeploymentSpecJSON)
			cache.set(cacheKey, template, models)
			shapeOpts = shapeOptsWithConfiguredInlineSecrets(shapeOpts, storedVars)
			shapeOpts = shapeOptsWithModels(shapeOpts, models)
			resp := deployment.ShapeTemplate(c.Request.Context(), template, &req, shapeOpts)
			respondDeploymentTemplate(c, cfg, resp, targetAcct, req.Finalize)
			return
		}

		// No deployment_id — fresh template.
		targetAcct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found", "error_code": "account_not_found"})
			return
		}

		cacheKey := accountName + ":" + agentName + ":" + buildIDOverride
		if base, models, ok := cache.get(cacheKey); ok {
			resp := deployment.ShapeTemplate(c.Request.Context(), base, &req, shapeOptsWithModels(shapeOpts, models))
			respondDeploymentTemplate(c, cfg, resp, targetAcct, req.Finalize)
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
		template, models, ok := generateTemplate(c, log, agentIndex, cfg, acct, agent, buildIDOverride)
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

		cache.set(cacheKey, template, models)
		resp := deployment.ShapeTemplate(c.Request.Context(), template, &req, shapeOptsWithModels(shapeOpts, models))
		respondDeploymentTemplate(c, cfg, resp, targetAcct, req.Finalize)
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
func seedFreshAuthGrants(template *deployment.AstroDeploymentSpec, deployerUserID string) {
	if template.Interfaces == nil {
		return
	}
	if template.Interfaces.Auth == nil {
		template.Interfaces.Auth = &deployment.DeploymentInterfacesAuth{}
	}
	auth := template.Interfaces.Auth
	if (auth.Web != nil && len(auth.Web.Grants) > 0) || (auth.Slack != nil && len(auth.Slack.Grants) > 0) {
		return
	}

	if auth.Web == nil {
		auth.Web = &deployment.DeploymentWebAuth{}
	}
	auth.Web.Grants = []deployment.DeploymentAuthorizationGrant{{UserID: deployerUserID}}

	if slices.Contains(template.Interfaces.Adapters, "slack") {
		auth.Slack = &deployment.DeploymentSlackAuth{
			Grants: []deployment.DeploymentAuthorizationGrant{{Anyone: true}},
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
func ensureSlackAnyoneGrant(ds *deployment.AstroDeploymentSpec) {
	if ds.Interfaces == nil {
		return
	}
	if !slices.Contains(ds.Interfaces.Adapters, "slack") {
		return
	}
	if ds.Interfaces.Auth == nil {
		ds.Interfaces.Auth = &deployment.DeploymentInterfacesAuth{}
	}
	auth := ds.Interfaces.Auth
	if auth.Slack != nil && len(auth.Slack.Grants) > 0 {
		return
	}
	auth.Slack = &deployment.DeploymentSlackAuth{
		Grants: []deployment.DeploymentAuthorizationGrant{{Anyone: true}},
	}
}

// mergeDeploymentPrefill merges stored deployment state into a template:
// target identity, adapter selection, ingestion schedules, and live
// authorization grants. Stored variables are routed through req via
// applyStoredVarsToRequest instead, because adapter-injected variables
// (e.g. SLACK_BOT_TOKEN) only exist on the template after ShapeTemplate's
// ApplyAdapterShaping pass.
func mergeDeploymentPrefill(log *logger.Logger, template *deployment.AstroDeploymentSpec, existing *deploymentstore.Deployment, accountStore *account.AccountStore, deployStore *deploymentstore.Store, authzStore *authorizationstore.Store) {
	template.Target.DeploymentID = existing.ID
	template.Target.DisplayName = existing.DisplayName

	// Resolve account name for target.account
	acct, err := accountStore.GetByID(existing.AccountID)
	if err == nil && acct != nil {
		template.Target.Account = acct.Name
	}

	// Merge adapters and ingestion schedules from stored spec. These are
	// nested / optional and don't have flat columns, so the JSON path is
	// still the simplest source.
	if existing.DeploymentSpecJSON != "" {
		var storedSpec deployment.AstroDeploymentSpec
		if jsonErr := json.Unmarshal([]byte(existing.DeploymentSpecJSON), &storedSpec); jsonErr == nil {
			// Restore stored adapters + auth (incl. web/custom public flags). A
			// custom-interface-only agent's freshly generated base has no
			// interfaces block, so create one rather than dropping the stored
			// auth — otherwise the custom interface's public state is lost on
			// redeploy.
			if storedSpec.Interfaces != nil {
				if template.Interfaces == nil {
					template.Interfaces = &deployment.DeploymentInterfaces{}
				}
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

	// Agent resources + volume come from the normalized columns. Without
	// this, the template generator's StandardResources seed wins and
	// Configure resets every existing deployment back to defaults.
	if prov, err := deployStore.GetAgentProvisioning(existing.ID); err != nil {
		log.Error("Failed to load agent provisioning", "error", err, "deployment_id", existing.ID)
	} else if prov != nil {
		// Empty CPU/Memory means the row predates the resources columns;
		// keep the generated StandardResources rather than zeroing them.
		if prov.Resources.CPU != "" || prov.Resources.Memory != "" {
			template.Agent.Resources = prov.Resources
		}
		template.Agent.Volume = prov.Volume
		template.Agent.Storage = prov.Storage
	}

	// Pull live authorization (default_role + grants) from the DB, not the stored spec.
	// The DB is the source of truth — admin endpoints can mutate it between deploys.
	// Fires regardless of Interfaces: frontend-only agents carry their grants on
	// the exposed endpoint, not under interfaces.auth.
	if authzStore != nil {
		mergeAuthorizationFromStore(log, authzStore, existing.ID, template)
	}
}

// shapeOptsWithModels attaches gateway model selectors to the shape options so
// ShapeTemplate can render them and apply req.Models choices.
func shapeOptsWithModels(opts *deployment.ShapeOptions, models []deployment.ModelSelection) *deployment.ShapeOptions {
	if len(models) == 0 {
		return opts
	}
	if opts == nil {
		opts = &deployment.ShapeOptions{}
	}
	opts.ModelSelections = models
	return opts
}

// applyStoredModelChoices folds the previously-chosen gateway model (persisted
// as an agent env literal, since model selectors are template-only) into
// req.Models so ShapeTemplate re-applies it. An explicit choice already in req
// wins — this preserves edits typed in the configure panel.
func applyStoredModelChoices(req *deployment.TemplateRequest, models []deployment.ModelSelection, storedSpecJSON string) {
	if len(models) == 0 || storedSpecJSON == "" {
		return
	}
	var storedSpec deployment.AstroDeploymentSpec
	if json.Unmarshal([]byte(storedSpecJSON), &storedSpec) != nil {
		return
	}
	for _, sel := range models {
		choice, ok := storedSpec.Agent.Environment[sel.EnvVar]
		if !ok {
			continue
		}
		if req.Models == nil {
			req.Models = map[string]string{}
		}
		if _, set := req.Models[sel.Name]; !set {
			req.Models[sel.Name] = choice
		}
	}
}

// applyStoredVarsToRequest folds each stored deployment variable into req
// as if the user had typed it. Anything req already specifies wins — this
// preserves user edits typed in the configure panel.
//
// Routing through req (instead of mutating template.Variables) is what lets
// adapter-injected variables like SLACK_BOT_TOKEN pick up their stored Ref:
// they don't exist on the template until ShapeTemplate's ApplyAdapterShaping
// runs, but ShapeTemplate's variable-filling pass runs after that, so by then
// req.Variables can populate the freshly-injected entry.
func applyStoredVarsToRequest(
	req *deployment.TemplateRequest,
	storedVars []deploymentstore.Variable,
) {
	for _, sv := range storedVars {
		if _, alreadySet := req.Variables[sv.Name]; alreadySet {
			continue
		}
		var input deployment.VariableInput
		switch {
		case sv.Ref != "":
			// Originally set via an account variable reference — restore the
			// ref so the UI shows which one was selected.
			input.Ref = sv.Ref
		case !sv.Secret:
			// Plaintext value for a non-secret variable.
			input.Value = sv.Value
		default:
			// Inline secret values are never sent to the UI. ShapeOptions marks
			// them configured and finalized templates carry a signed sentinel
			// which /deploy resolves from storage.
			continue
		}
		if req.Variables == nil {
			req.Variables = make(map[string]deployment.VariableInput)
		}
		req.Variables[sv.Name] = input
	}
}

func shapeOptsWithConfiguredInlineSecrets(opts *deployment.ShapeOptions, stored []deploymentstore.Variable) *deployment.ShapeOptions {
	names := deploymentstore.InlineSecretNames(stored)
	if len(names) == 0 {
		return opts
	}
	if opts == nil {
		opts = &deployment.ShapeOptions{}
	}
	opts.ConfiguredInlineSecrets = names
	return opts
}

// buildAuthorizationGrants flattens the spec's web and slack grant blocks
// into a single store-shape list, ready for ReplaceGrantsTx.
//
// This is the only writer-side translator from spec form to store form. The
// deploy handler runs the resulting list through ReplaceGrantsTx inside the
// same transaction that creates the deployment row, so the grants table is
// never out of sync with the deployments table.
func buildAuthorizationGrants(deploymentID string, ds *deployment.AstroDeploymentSpec) []authorizationstore.Grant {
	var grants []authorizationstore.Grant
	if ds == nil {
		return grants
	}
	if ds.Interfaces != nil && ds.Interfaces.Auth != nil {
		auth := ds.Interfaces.Auth
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
		if auth.Custom != nil {
			for _, g := range auth.Custom.Grants {
				grants = append(grants, specGrantToStore(deploymentID, g, authorizationstore.AdapterCustom))
			}
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
func specGrantToStore(deploymentID string, g deployment.DeploymentAuthorizationGrant, adapter string) authorizationstore.Grant {
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
func storeGrantToSpec(g *authorizationstore.Grant) deployment.DeploymentAuthorizationGrant {
	out := deployment.DeploymentAuthorizationGrant{}
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
func validateAuthorizationSpec(ds *deployment.AstroDeploymentSpec) []string {
	if ds == nil {
		return nil
	}
	var errs []string
	seen := map[string]struct{}{}

	// adapter keys the dedup set; path is the spec location used in messages
	// (frontend grants live on the endpoint, not under interfaces.auth).
	check := func(adapter, path string, grants []deployment.DeploymentAuthorizationGrant) {
		for i, g := range grants {
			prefix := fmt.Sprintf("%s[%d]", path, i)

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

	if ds.Interfaces != nil && ds.Interfaces.Auth != nil {
		auth := ds.Interfaces.Auth
		if auth.Web != nil {
			check(authorizationstore.AdapterWeb, "interfaces.auth.web.grants", auth.Web.Grants)
		}
		if auth.Slack != nil {
			check(authorizationstore.AdapterSlack, "interfaces.auth.slack.grants", auth.Slack.Grants)
		}

		// A public web surface routes to the open (no-OIDC) cohort, so the
		// front-door ALB injects no x-amzn-oidc identity. The messaging web
		// adapter then has nobody to authorize org/user grants against, so only
		// an "anyone" grant is enforceable. Reject any other combination — it
		// would lock the deployment out entirely.
		if auth.Web != nil && auth.Web.Public {
			hasAnyone := false
			for i, g := range auth.Web.Grants {
				if g.Anyone {
					hasAnyone = true
					continue
				}
				errs = append(errs, fmt.Sprintf("interfaces.auth.web.grants[%d]: public web allows only an 'anyone' grant (org/user grants need the OIDC identity that public bypasses)", i))
			}
			if !hasAnyone {
				errs = append(errs, "interfaces.auth.web: public requires an 'anyone' grant (the web adapter has no OIDC identity to authorize otherwise)")
			}
		}

		// Custom-interface grants are not enforced by the platform (the agent's
		// own server authorizes), so there's no public-requires-anyone rule —
		// public just skips ALB OIDC. Only the grant shape is validated.
		if auth.Custom != nil {
			check(authorizationstore.AdapterCustom, "interfaces.auth.custom.grants", auth.Custom.Grants)
		}
	}
	return errs
}

// mergeAuthorizationFromStore overlays the deployment's stored grants onto the
// template's interfaces.auth so the UI reflects the live access state. Used by
// the deployment-template prefill path on redeploys. Grants are dispatched into
// auth.web / auth.slack / auth.custom based on each row's adapter.
func mergeAuthorizationFromStore(log *logger.Logger, authzStore *authorizationstore.Store, deploymentID string, template *deployment.AstroDeploymentSpec) {
	grants, err := authzStore.ListGrants(deploymentID)
	if err != nil {
		log.Error("Failed to list authorization grants", "error", err, "deployment_id", deploymentID)
		return
	}

	// Reset the grant lists so the DB is authoritative.
	if template.Interfaces != nil && template.Interfaces.Auth != nil {
		if template.Interfaces.Auth.Web != nil {
			template.Interfaces.Auth.Web.Grants = nil
		}
		if template.Interfaces.Auth.Slack != nil {
			template.Interfaces.Auth.Slack.Grants = nil
		}
		if template.Interfaces.Auth.Custom != nil {
			template.Interfaces.Auth.Custom.Grants = nil
		}
	}

	ensureAuth := func() *deployment.DeploymentInterfacesAuth {
		if template.Interfaces == nil {
			template.Interfaces = &deployment.DeploymentInterfaces{}
		}
		if template.Interfaces.Auth == nil {
			template.Interfaces.Auth = &deployment.DeploymentInterfacesAuth{}
		}
		return template.Interfaces.Auth
	}

	for _, g := range grants {
		sg := storeGrantToSpec(g)
		switch g.Adapter {
		case authorizationstore.AdapterWeb:
			auth := ensureAuth()
			if auth.Web == nil {
				auth.Web = &deployment.DeploymentWebAuth{}
			}
			auth.Web.Grants = append(auth.Web.Grants, sg)
		case authorizationstore.AdapterSlack:
			auth := ensureAuth()
			if auth.Slack == nil {
				auth.Slack = &deployment.DeploymentSlackAuth{}
			}
			auth.Slack.Grants = append(auth.Slack.Grants, sg)
		case authorizationstore.AdapterCustom:
			auth := ensureAuth()
			if auth.Custom == nil {
				auth.Custom = &deployment.DeploymentCustomAuth{}
			}
			auth.Custom.Grants = append(auth.Custom.Grants, sg)
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
		if !middleware.AuthorizeDeploymentAction(c, authz.ActionDeploymentRead, d.ID) {
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
func GetDeploymentHistory(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	discovery deploymentVisibilityResolver,
) gin.HandlerFunc {
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
		if len(history) > 0 {
			visibility, visibilityErr := resolveDeploymentVisibility(
				c.Request.Context(), discovery, user.ID, singleAccountVisibilityScope(acct),
			)
			if visibilityErr != nil {
				writeDeploymentVisibilityError(c, log, visibilityErr)
				return
			}
			history, visibilityErr = filterReadableDeployments(
				c.Request.Context(), deployStore.ListReadableDeploymentHistoryIDsForUser, user.ID, history, visibility,
				func(record deploymentstore.RevisionHistoryRecord) string { return record.DeploymentID },
			)
			if visibilityErr != nil {
				log.Error("Failed to filter deployment history", "error", visibilityErr, "account_id", acct.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get deployment history"})
				return
			}
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
			DeployedBy    string    `json:"deployed_by,omitempty"`
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
				DeployedBy:    r.DeployedBy,
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
func GetConfigMapData(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sReg *k8s.Registry, deployStore *deploymentstore.Store) gin.HandlerFunc {
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

		k8sClient, ok := clusterClientForDeployment(c, k8sReg, dep)
		if !ok {
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
func GetSecretKeys(log *logger.Logger, accountStore *account.AccountStore, cfg *config.Config, k8sReg *k8s.Registry, deployStore *deploymentstore.Store) gin.HandlerFunc {
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

		k8sClient, ok := clusterClientForDeployment(c, k8sReg, dep)
		if !ok {
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

// StopDeployment scales all workloads to zero without deleting resources.
// POST /api/v1/deployments/:id/stop
func StopDeployment(log *logger.Logger, accountStore *account.AccountStore, k8sReg *k8s.Registry, deployStore *deploymentstore.Store, auditStore *auditlog.Store, cache k8scache.Cache) gin.HandlerFunc {
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

		k8sClient, ok := clusterClientForDeployment(c, k8sReg, dep)
		if !ok {
			return
		}

		switch dep.Status {
		case deploymentstore.StatusPending, deploymentstore.StatusProvisioning,
			deploymentstore.StatusDeploying, deploymentstore.StatusUndeploying:
			c.JSON(http.StatusBadRequest, gin.H{"error": "deployment cannot be paused while it is still deploying"})
			return
		}

		if err := k8s.StopNamespaceWorkloads(c.Request.Context(), k8sClient.Clientset(), dep.Namespace); err != nil {
			log.Error("Failed to stop deployment workloads", "error", err, "namespace", dep.Namespace, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stop deployment"})
			return
		}
		k8scache.InvalidateNamespace(c.Request.Context(), cache, dep.Namespace)

		if err := deployStore.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusStopped}); err != nil {
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

// CancelDeployment aborts a deployment that is still coming up (pending/
// provisioning/deploying): the escape hatch for a deploy hung on a bad config
// or unschedulable pod, which otherwise blocks redeploys until the K8s progress
// deadline eventually fails it. It marks the deployment failed and leaves the
// workloads untouched: the main process never drives K8s directly, and tearing
// a workload down could kill a still-healthy serving pod from an earlier
// revision. failed is drivable, not terminal: if the workloads turn out to be
// healthy the controller drives the deployment back to active (and bills it) on
// its next sync, so a cancelled-but-healthy rollout is never left serving
// unbilled; a genuinely stuck deploy stays failed and surfaces the recovery
// banner so the user can fix config and redeploy, or roll back.
func CancelDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, auditStore *auditlog.Store) gin.HandlerFunc {
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

		// Compare-and-set so cancel only lands from a still-cancellable state. If
		// the controller drove the row to active between the read above and this
		// write, an unconditional update would clobber it back to failed, and
		// since failed is drivable the controller would re-activate and call
		// startBilling a second time. The guarded update matches no row instead,
		// so a raced cancel is a no-op (400) rather than a double-bill.
		applied, err := deployStore.UpdateStatusIfCurrent(
			dep.ID,
			deploymentstore.StatusUpdate{Status: deploymentstore.StatusFailed, ErrorMsg: "Deployment cancelled"},
			deploymentstore.StatusPending, deploymentstore.StatusProvisioning, deploymentstore.StatusDeploying,
		)
		if err != nil {
			log.Error("Failed to mark deployment failed after cancel", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update deployment status"})
			return
		}
		if !applied {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deployment is not deploying"})
			return
		}

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.DeploymentCancel
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Cancelled deployment " + dep.AgentName
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusAccepted, gin.H{
			"status":        deploymentstore.StatusFailed,
			"deployment_id": dep.ID,
		})
	}
}

// WakeUpDeployment triggers re-provisioning of a paused (stopped) deployment.
//
// entCheck refuses a wakeup on a suspended account. Without it the request still
// failed, but on the status check below, which reports "deployment is not
// stopped" for a deployment billing stopped: true, useless, and it hides the
// only fact the caller can act on.
func WakeUpDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, queue DeployQueue, auditStore *auditlog.Store, entCheck EntitlementChecker) gin.HandlerFunc {
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

		if blockedByBilling(c, entCheck, dep.AccountID) {
			return
		}
		if dep.Status != deploymentstore.StatusStopped {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deployment is not stopped"})
			return
		}

		if err := deployStore.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusPending}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
			return
		}

		if err := queue.InsertWakeUpJob(c.Request.Context(), dep.ID, dep.EffectiveClusterID()); err != nil {
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
func RollbackDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, queue DeployQueue, auditStore *auditlog.Store, cache k8scache.Cache, entCheck EntitlementChecker) gin.HandlerFunc {
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
		// Membership first: the two checks below report the deployment's status and
		// current revision, which a non-member must not learn.
		if !isAccountMember(c, accountStore, dep.AccountID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		if blockedByBilling(c, entCheck, dep.AccountID) {
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

		// SetCurrentRevision atomically sets revision, status=pending, and records event.
		// Job enqueue happens after commit (store uses database/sql, River uses pgx).
		if err := deployStore.SetCurrentRevision(dep.ID, req.Revision, nil); err != nil {
			log.Error("Failed to set revision", "error", err, "deployment_id", dep.ID, "revision", req.Revision)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := queue.InsertDeployJob(c.Request.Context(), dep.ID, dep.EffectiveClusterID()); err != nil {
			log.Error("Failed to enqueue rollback deploy job", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule rollback"})
			return
		}
		// Status flipped to pending and the new revision is recorded — bust
		// before the deploy worker picks up the job so the page reflects the
		// rollback immediately.
		_ = deploycache.Invalidate(c.Request.Context(), cache, dep.AccountID)

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
