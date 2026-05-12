package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

// Entitlements provides entitlement checking for routes. Create one at startup
// and use Wrap() to guard individual handlers or Check() for inline checks.
//
//	ent := middleware.NewEntitlements(log, omClient, cfg.OpenMeterEnforce)
//	api.POST(g, "/register", "Register", ent.Wrap(handler, "agents", "agent_builds"), ...)
type Entitlements struct {
	log     *logger.Logger
	client  *openmeter.Client
	enforce bool
}

// NewEntitlements creates an Entitlements checker. If client is nil or enforce
// is false, checks become no-ops or log-only respectively.
func NewEntitlements(log *logger.Logger, client *openmeter.Client, enforce bool) *Entitlements {
	return &Entitlements{log: log, client: client, enforce: enforce}
}

// Wrap returns a gin.HandlerFunc that checks the given features before calling
// the handler. The account must be in context via ResolveAccount middleware.
func (e *Entitlements) Wrap(handler gin.HandlerFunc, features ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if e.client == nil {
			handler(c)
			return
		}

		acct, ok := GetAccountFromContext(c)
		if !ok {
			handler(c)
			return
		}

		if blocked, feature, ent := e.check(c.Request.Context(), acct.ID, features); blocked {
			c.JSON(http.StatusPaymentRequired, LimitResponse(feature, ent))
			return
		}

		handler(c)
	}
}

// Check performs an entitlement check for the given account ID and features.
// Use this for handlers that resolve the account outside of middleware (e.g. DeployAgent).
// Returns true if the request should be blocked.
func (e *Entitlements) Check(ctx context.Context, accountID string, features ...string) (blocked bool, feature string, ent *openmeter.EntitlementValue) {
	if e.client == nil {
		return false, "", nil
	}
	return e.check(ctx, accountID, features)
}

func (e *Entitlements) check(ctx context.Context, accountID string, features []string) (blocked bool, feature string, ent *openmeter.EntitlementValue) {
	access, err := e.client.GetCustomerAccess(ctx, accountID)
	if err != nil {
		e.log.Warn("Customer access check failed", "error", err, "account_id", accountID)
		return false, "", nil // fail open
	}

	for _, f := range features {
		result, ok := access.Entitlements[f]
		if !ok {
			// Feature absent from plan entirely — always block.
			// The enforce flag only governs quota overage, not plan structure.
			return true, f, nil
		}

		if !result.HasAccess {
			if e.enforce {
				return true, f, &result
			}
			e.log.Warn("Entitlement exceeded (not enforcing)",
				"account_id", accountID, "feature", f,
				"usage", result.Usage, "limit", result.TotalAvailableGrantAmount,
			)
		}
	}
	return false, "", nil
}

// featureInfo maps feature keys to human-readable descriptions used in error messages.
var featureInfo = map[string]struct {
	name      string
	quotaDesc string // shown when quota is exceeded
	planDesc  string // shown when feature is absent from plan
}{
	"compute":             {name: "Compute", quotaDesc: "Your account has consumed its allocated compute-unit-hours for this billing period.", planDesc: "Compute is not included in your current plan."},
	"agents":              {name: "Agents", quotaDesc: "Your account has reached the maximum number of registered agents.", planDesc: "Agents are not included in your current plan."},
	"agent_builds":        {name: "Agent Builds", quotaDesc: "Your account has reached the maximum number of agent builds for this billing period.", planDesc: "Agent builds are not included in your current plan."},
	"agent_deployments":   {name: "Deployments", quotaDesc: "Your account has reached the maximum number of active deployments.", planDesc: "Deployments are not included in your current plan."},
	"members":             {name: "Members", quotaDesc: "Your account has reached the maximum number of team members.", planDesc: "Additional members are not included in your current plan."},
	"knowledge_stores":    {name: "Knowledge Stores", quotaDesc: "Your account has reached the maximum number of knowledge stores.", planDesc: "Knowledge stores are not included in your current plan."},
	"knowledge_storage":   {name: "Knowledge Storage", quotaDesc: "Your account has reached the maximum provisioned storage for knowledge stores.", planDesc: "Knowledge storage is not included in your current plan."},
	"knowledge_compute":   {name: "Knowledge Compute", quotaDesc: "Your account has consumed its allocated compute for knowledge stores this billing period.", planDesc: "Knowledge compute is not included in your current plan."},
	"knowledge_endpoints": {name: "Knowledge Endpoints", quotaDesc: "Your account has reached the maximum number of PrivateLink endpoints.", planDesc: "PrivateLink endpoints are not included in your current plan."},
}

// LimitResponse builds the JSON response body returned when an entitlement
// limit is reached or a feature is absent from the plan. It includes actionable
// detail so the client can display a meaningful upgrade prompt.
func LimitResponse(feature string, ent *openmeter.EntitlementValue) gin.H {
	info, ok := featureInfo[feature]
	if !ok {
		info.name = feature
		info.quotaDesc = "Your account has reached its usage limit for this feature."
		info.planDesc = "This feature is not included in your current plan."
	}

	// Feature absent from plan entirely — different message and no usage/limit to report.
	if ent == nil {
		return gin.H{
			"error":   "Feature not available",
			"code":    "FEATURE_NOT_IN_PLAN",
			"feature": feature,
			"usage":   float64(0),
			"limit":   float64(0),
			"details": fmt.Sprintf("%s To access this feature, contact your account admin about upgrading your plan.", info.planDesc),
		}
	}

	var usage, limit float64
	if ent.Usage != nil {
		usage = *ent.Usage
	}
	if ent.TotalAvailableGrantAmount != nil {
		limit = *ent.TotalAvailableGrantAmount
	}

	return gin.H{
		"error":   "Limit reached",
		"code":    "ENTITLEMENT_LIMIT_REACHED",
		"feature": feature,
		"usage":   usage,
		"limit":   limit,
		"details": fmt.Sprintf("%s limit reached: %s To continue, request a quota increase from Settings > Usage.", info.name, info.quotaDesc),
	}
}
