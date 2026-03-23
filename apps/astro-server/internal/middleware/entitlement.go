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
			continue // feature not in entitlements, fail open
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
	name string
	desc string
}{
	"compute":           {name: "Compute", desc: "Your account has consumed its allocated compute-unit-hours for this billing period."},
	"agents":            {name: "Agents", desc: "Your account has reached the maximum number of registered agents."},
	"agent_builds":      {name: "Agent Builds", desc: "Your account has reached the maximum number of agent builds for this billing period."},
	"agent_deployments": {name: "Deployments", desc: "Your account has reached the maximum number of active deployments."},
	"members":           {name: "Members", desc: "Your account has reached the maximum number of team members."},
}

// LimitResponse builds the JSON response body returned when an entitlement
// limit is reached. It includes actionable detail so the client can display
// a meaningful upgrade prompt.
func LimitResponse(feature string, ent *openmeter.EntitlementValue) gin.H {
	info, ok := featureInfo[feature]
	if !ok {
		info.name = feature
		info.desc = "Your account has reached its usage limit for this feature."
	}

	var usage, limit float64
	if ent != nil && ent.Usage != nil {
		usage = *ent.Usage
	}
	if ent != nil && ent.TotalAvailableGrantAmount != nil {
		limit = *ent.TotalAvailableGrantAmount
	}

	return gin.H{
		"error":   fmt.Sprintf("%s limit reached: %s To continue, request a quota increase from Settings > Usage.", info.name, info.desc),
		"code":    "ENTITLEMENT_LIMIT_REACHED",
		"feature": feature,
		"usage":   usage,
		"limit":   limit,
		"details": gin.H{
			"feature_name": info.name,
			"description":  info.desc,
			"action":       "Request a quota increase from Settings > Usage.",
		},
	}
}
