// Package quota enforces per-account resource limits (max agents, deployments,
// members, knowledge stores/endpoints, and builds per period). It is DB-backed
// and independent of billing: limits resolve from an account_limits override
// row else a system-wide config default, and current usage comes from COUNTs
// over the owning tables. Enforced identically for OSS and hosted.
//
// This is the resource-count half of the former OpenMeter entitlement path;
// metered consumption (compute, knowledge storage) is gated separately by the
// billing provider.
package quota

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

// Resource identifiers. These match the feature keys used by the former
// entitlement path so response bodies are unchanged.
const (
	ResourceAgents             = "agents"
	ResourceAgentBuilds        = "agent_builds"
	ResourceAgentDeployments   = "agent_deployments"
	ResourceMembers            = "members"
	ResourceKnowledgeStores    = "knowledge_stores"
	ResourceKnowledgeEndpoints = "knowledge_endpoints"
)

// Unlimited is the sentinel effective limit that disables the check. A limit of
// 0 disables the feature entirely (FEATURE_NOT_IN_PLAN).
const Unlimited int64 = -1

// AllResources is the full set of quota-managed resources, in display order.
var AllResources = []string{
	ResourceAgents,
	ResourceAgentBuilds,
	ResourceAgentDeployments,
	ResourceMembers,
	ResourceKnowledgeStores,
	ResourceKnowledgeEndpoints,
}

// ResourceUsage is the current usage and effective limit for a resource.
type ResourceUsage struct {
	Used  int64
	Limit int64 // -1 unlimited, 0 disabled
}

// Result reports the outcome of a quota check for the first resource over limit.
type Result struct {
	Blocked  bool
	Resource string // first resource over limit (empty when not blocked)
	Limit    int64  // effective limit for that resource (0 = disabled)
	Used     int64  // current usage
}

// Checker resolves and enforces per-account resource limits.
type Checker interface {
	Check(ctx context.Context, accountID string, resources ...string) (Result, error)
}

// Reporter returns current usage and limits per resource, for the usage UI.
type Reporter interface {
	Report(ctx context.Context, accountID string, resources ...string) (map[string]ResourceUsage, error)
}

// countQueries maps each resource to a COUNT query keyed by account_id ($1).
// These mirror the COUNT/GROUP BY queries the metering emit helpers use, so the
// numbers match what the entitlement path saw. agent_builds is a per-period rate
// limit and counts version publishes in the current calendar month (UTC).
var countQueries = map[string]string{
	ResourceAgents:           `SELECT COUNT(*) FROM agents WHERE account_id = $1 AND archived_at IS NULL`,
	ResourceAgentBuilds:      `SELECT COUNT(*) FROM agent_versions WHERE account_id = $1 AND published_at >= date_trunc('month', now())`,
	ResourceAgentDeployments: `SELECT COUNT(*) FROM deployments WHERE account_id = $1 AND status IN ('pending', 'active')`,
	ResourceMembers:          `SELECT COUNT(*) FROM account_members WHERE account_id = $1`,
	ResourceKnowledgeStores:  `SELECT COUNT(*) FROM knowledge_stores WHERE account_id = $1 AND status != 'error'`,
	ResourceKnowledgeEndpoints: `SELECT COUNT(*) FROM knowledge_store_endpoints kse
		JOIN knowledge_stores ks ON ks.id = kse.knowledge_store_id
		WHERE ks.account_id = $1 AND kse.status != 'error'`,
}

// DBChecker is the database-backed Checker. Effective limits resolve from the
// account_limits table (per-account override) else the config defaults map.
type DBChecker struct {
	db       *sql.DB
	log      *logger.Logger
	defaults map[string]int64
	// enforce mirrors the former entitlement enforcement flag: when false,
	// over-limit is logged but not blocked. A disabled feature (limit 0) always
	// blocks regardless, matching the prior "feature absent from plan" behavior.
	enforce bool
}

// NewDBChecker creates a DBChecker. defaults is the system-wide resource→limit
// map; a resource absent from both the overrides and defaults is treated as
// Unlimited.
func NewDBChecker(db *sql.DB, log *logger.Logger, defaults map[string]int64, enforce bool) *DBChecker {
	return &DBChecker{db: db, log: log, defaults: defaults, enforce: enforce}
}

// Check evaluates each resource in order and returns the first one that blocks.
func (c *DBChecker) Check(ctx context.Context, accountID string, resources ...string) (Result, error) {
	for _, resource := range resources {
		limit, err := c.effectiveLimit(ctx, accountID, resource)
		if err != nil {
			return Result{}, err
		}
		if limit == Unlimited {
			continue // unlimited — never blocks
		}

		// Disabled feature: block regardless of the enforce flag (matches the
		// former FEATURE_NOT_IN_PLAN behavior).
		if limit == 0 {
			return Result{Blocked: true, Resource: resource, Limit: 0, Used: 0}, nil
		}

		used, err := c.count(ctx, accountID, resource)
		if err != nil {
			return Result{}, err
		}
		if used < limit {
			continue
		}

		// Over limit. Only block when enforcing; otherwise log and allow.
		if !c.enforce {
			c.log.Warn("Quota exceeded (not enforcing)",
				"account_id", accountID, "resource", resource, "used", used, "limit", limit)
			continue
		}
		return Result{Blocked: true, Resource: resource, Limit: limit, Used: used}, nil
	}
	return Result{}, nil
}

// Report returns current usage and the effective limit for each resource. Used
// by the usage endpoint to render counts from the DB (the authoritative source)
// instead of a metering backend.
func (c *DBChecker) Report(ctx context.Context, accountID string, resources ...string) (map[string]ResourceUsage, error) {
	out := make(map[string]ResourceUsage, len(resources))
	for _, resource := range resources {
		limit, err := c.effectiveLimit(ctx, accountID, resource)
		if err != nil {
			return nil, err
		}
		used, err := c.count(ctx, accountID, resource)
		if err != nil {
			return nil, err
		}
		out[resource] = ResourceUsage{Used: used, Limit: limit}
	}
	return out, nil
}

// effectiveLimit resolves the override for (account, resource) else the config
// default else Unlimited.
func (c *DBChecker) effectiveLimit(ctx context.Context, accountID, resource string) (int64, error) {
	var limit int64
	err := c.db.QueryRowContext(ctx,
		`SELECT limit_value FROM account_limits WHERE account_id = $1 AND resource = $2`,
		accountID, resource,
	).Scan(&limit)
	if err == nil {
		return limit, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("query account_limits: %w", err)
	}
	if def, ok := c.defaults[resource]; ok {
		return def, nil
	}
	return Unlimited, nil
}

// count returns the current usage for a resource.
func (c *DBChecker) count(ctx context.Context, accountID, resource string) (int64, error) {
	query, ok := countQueries[resource]
	if !ok {
		return 0, fmt.Errorf("quota: no count query for resource %q", resource)
	}
	var n int64
	if err := c.db.QueryRowContext(ctx, query, accountID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count %s: %w", resource, err)
	}
	return n, nil
}

// Wrap guards a handler on the given resources. The account must be in context
// via ResolveAccount middleware. On a provider/DB error the check fails open
// (the handler runs), preserving prior behavior.
func (c *DBChecker) Wrap(handler gin.HandlerFunc, resources ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(ctx)
		if !ok {
			handler(ctx)
			return
		}
		res, err := c.Check(ctx.Request.Context(), acct.ID, resources...)
		if err != nil {
			c.log.Warn("Quota check failed", "error", err, "account_id", acct.ID)
			handler(ctx)
			return
		}
		if res.Blocked {
			ctx.JSON(http.StatusPaymentRequired, LimitResponse(res))
			return
		}
		handler(ctx)
	}
}

// resourceInfo mirrors the human-readable descriptions the entitlement path used
// so 402 bodies are byte-identical.
var resourceInfo = map[string]struct{ name, quotaDesc, planDesc string }{
	ResourceAgents:             {"Agents", "Your account has reached the maximum number of registered agents.", "Agents are not included in your current plan."},
	ResourceAgentBuilds:        {"Agent Builds", "Your account has reached the maximum number of agent builds for this billing period.", "Agent builds are not included in your current plan."},
	ResourceAgentDeployments:   {"Deployments", "Your account has reached the maximum number of active deployments.", "Deployments are not included in your current plan."},
	ResourceMembers:            {"Members", "Your account has reached the maximum number of team members.", "Additional members are not included in your current plan."},
	ResourceKnowledgeStores:    {"Knowledge Stores", "Your account has reached the maximum number of knowledge stores.", "Knowledge stores are not included in your current plan."},
	ResourceKnowledgeEndpoints: {"Knowledge Endpoints", "Your account has reached the maximum number of PrivateLink endpoints.", "PrivateLink endpoints are not included in your current plan."},
}

// LimitResponse builds the 402 body for a blocked quota check. The shape and
// codes match the former entitlement responses: a disabled feature (limit 0)
// yields FEATURE_NOT_IN_PLAN; an over-limit resource yields
// ENTITLEMENT_LIMIT_REACHED.
func LimitResponse(res Result) gin.H {
	info, ok := resourceInfo[res.Resource]
	if !ok {
		info.name = res.Resource
		info.quotaDesc = "Your account has reached its usage limit for this feature."
		info.planDesc = "This feature is not included in your current plan."
	}

	if res.Limit == 0 {
		return gin.H{
			"error":   "Feature not available",
			"code":    "FEATURE_NOT_IN_PLAN",
			"feature": res.Resource,
			"usage":   float64(0),
			"limit":   float64(0),
			"details": fmt.Sprintf("%s To access this feature, contact your account admin about upgrading your plan.", info.planDesc),
		}
	}

	return gin.H{
		"error":   "Limit reached",
		"code":    "ENTITLEMENT_LIMIT_REACHED",
		"feature": res.Resource,
		"usage":   float64(res.Used),
		"limit":   float64(res.Limit),
		"details": fmt.Sprintf("%s limit reached: %s To continue, request a quota increase from Settings > Usage.", info.name, info.quotaDesc),
	}
}
