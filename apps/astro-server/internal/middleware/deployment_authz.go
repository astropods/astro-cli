package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/gin-gonic/gin"
)

const (
	deploymentAuthorizationCheckTimeout        = 2 * time.Second
	deploymentAuthorizationShadowMaxConcurrent = 16
)

type deploymentRoute struct {
	method string
	path   string
}

type deploymentRoutePolicyKind uint8

const (
	deploymentRouteObserved deploymentRoutePolicyKind = iota
	deploymentRouteDeferred
	deploymentRouteModelDeferred
	deploymentRouteDataPlane
)

type deploymentRoutePolicy struct {
	action authz.Action
	kind   deploymentRoutePolicyKind
}

func observedDeploymentRoute(action authz.Action) deploymentRoutePolicy {
	return deploymentRoutePolicy{action: action, kind: deploymentRouteObserved}
}

func deferredDeploymentRoute(action authz.Action) deploymentRoutePolicy {
	return deploymentRoutePolicy{action: action, kind: deploymentRouteDeferred}
}

func modelDeferredDeploymentRoute() deploymentRoutePolicy {
	return deploymentRoutePolicy{kind: deploymentRouteModelDeferred}
}

func deploymentDataPlaneRoute() deploymentRoutePolicy {
	return deploymentRoutePolicy{kind: deploymentRouteDataPlane}
}

type authorizationShadowLogger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

const deploymentAuthorizationObservationKey = "deploymentAuthorizationObservation"
const deploymentAuthorizationEnforcerKey = "deploymentAuthorizationEnforcer"
const deploymentAuthorizationEvaluatedKey = "deploymentAuthorizationEvaluated"

type deploymentAuthorizationObservation struct {
	action       authz.Action
	deploymentID string
}

type deploymentAuthorizationEnforcer struct {
	log     authorizationShadowLogger
	checker authz.Checker
}

// SetDeploymentAuthorizationObservation records a body-addressed deployment
// action after its handler has resolved the deployment.
func SetDeploymentAuthorizationObservation(c *gin.Context, action authz.Action, deploymentID string) {
	if c == nil || action == "" || deploymentID == "" {
		return
	}
	c.Set(deploymentAuthorizationObservationKey, deploymentAuthorizationObservation{
		action:       action,
		deploymentID: deploymentID,
	})
}

// AuthorizeDeploymentAction enforces a body-addressed deployment mutation when
// enforcement is configured; in shadow mode it only records the observation.
func AuthorizeDeploymentAction(c *gin.Context, action authz.Action, deploymentID string) bool {
	SetDeploymentAuthorizationObservation(c, action, deploymentID)
	value, ok := c.Get(deploymentAuthorizationEnforcerKey)
	enforcer, valid := value.(deploymentAuthorizationEnforcer)
	if !ok || !valid {
		return true
	}
	return enforcer.authorize(c, action, authz.DeploymentResource(deploymentID))
}

// EnforceDeploymentAuthorization blocks reviewed deployment control-plane
// routes. Model-owned and data-plane routes retain their separate policies.
func EnforceDeploymentAuthorization(log authorizationShadowLogger, checker authz.Checker, catalog *DeploymentRouteCatalog) gin.HandlerFunc {
	enforcer := deploymentAuthorizationEnforcer{log: log, checker: checker}
	return func(c *gin.Context) {
		ctx := authz.WithRequestCache(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Set(deploymentAuthorizationEnforcerKey, enforcer)

		policy, ok := catalog.policy(c.Request.Method, c.FullPath())
		if !ok || !policy.enforced() || c.Param("id") == "" {
			c.Next()
			return
		}
		if !enforcer.authorize(c, policy.action, authz.DeploymentResource(c.Param("id"))) {
			return
		}
		c.Next()
	}
}

func (p deploymentRoutePolicy) enforced() bool {
	return p.kind == deploymentRouteObserved || p.kind == deploymentRouteDeferred
}

func (e deploymentAuthorizationEnforcer) authorize(c *gin.Context, action authz.Action, resource authz.ResourceRef) bool {
	subject, ok := SubjectFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), deploymentAuthorizationCheckTimeout)
	defer cancel()
	ctx = authz.WithAuthorizationRoute(ctx, c.FullPath())

	allowed, err := e.checker.Authorize(ctx, subject, action, resource)
	// A rollout skip is not an authorization decision. Leave it observable by
	// the broader shadow gate registered after enforcement.
	if !errors.Is(err, authz.ErrFGAResourceNotEnabled) {
		c.Set(deploymentAuthorizationEvaluatedKey, true)
	}
	attrs := []any{
		"route", c.FullPath(),
		"action", action,
		"resource_type", resource.Type,
		"resource_id", resource.ExternalID,
		"user_id", subject.UserID,
		"membership_id", subject.MembershipID,
		"organization_id", subject.OrgID,
	}
	if err != nil {
		attrs = append(attrs, "error", err)
		switch {
		case errors.Is(err, authz.ErrFGAResourceNotEnabled):
			e.log.Debug("FGA enforcement skipped", attrs...)
			return true
		case errors.Is(err, sql.ErrNoRows):
			e.log.Debug("FGA authorization resource unavailable", attrs...)
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		case errors.Is(err, authz.ErrWorkOSMembershipUnavailable):
			e.log.Warn("FGA authorization identity unavailable", attrs...)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "authorization session is unavailable; refresh or sign in again"})
		default:
			e.log.Warn("FGA authorization check failed", attrs...)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "authorization temporarily unavailable"})
		}
		return false
	}
	if !allowed {
		e.log.Info("FGA authorization denied", attrs...)
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return false
	}
	e.log.Debug("FGA authorization allowed", attrs...)
	return true
}

// ObserveDeploymentAuthorization samples mapped deployment routes without
// aborting or changing their existing membership-based behavior.
func ObserveDeploymentAuthorization(log authorizationShadowLogger, checker authz.Checker, catalog *DeploymentRouteCatalog) gin.HandlerFunc {
	shadowSlots := make(chan struct{}, deploymentAuthorizationShadowMaxConcurrent)

	return func(c *gin.Context) {
		route := c.FullPath()
		policy, ok := catalog.policy(c.Request.Method, c.FullPath())
		subject, hasSubject := SubjectFromContext(c)
		deploymentID := c.Param("id")
		_, alreadyEvaluated := c.Get(deploymentAuthorizationEvaluatedKey)
		if alreadyEvaluated {
			c.Next()
			return
		}
		if ok && policy.kind == deploymentRouteObserved && hasSubject && deploymentID != "" {
			startDeploymentAuthorizationObservation(
				c.Request.Context(), log, checker, shadowSlots, route, subject,
				policy.action, authz.DeploymentResource(deploymentID),
			)
			c.Next()
			return
		}

		c.Next()
		if _, alreadyEvaluated := c.Get(deploymentAuthorizationEvaluatedKey); alreadyEvaluated {
			return
		}
		if !hasSubject {
			return
		}
		value, exists := c.Get(deploymentAuthorizationObservationKey)
		observation, valid := value.(deploymentAuthorizationObservation)
		if !exists || !valid {
			return
		}
		startDeploymentAuthorizationObservation(
			c.Request.Context(), log, checker, shadowSlots, route, subject,
			observation.action, authz.DeploymentResource(observation.deploymentID),
		)
	}
}

func startDeploymentAuthorizationObservation(
	requestCtx context.Context,
	log authorizationShadowLogger,
	checker authz.Checker,
	shadowSlots chan struct{},
	route string,
	subject authz.Subject,
	action authz.Action,
	resource authz.ResourceRef,
) {
	select {
	case shadowSlots <- struct{}{}:
		go func() {
			defer func() { <-shadowSlots }()
			observeDeploymentAuthorization(requestCtx, log, checker, route, subject, action, resource)
		}()
	default:
		log.Debug("FGA shadow check skipped: concurrency limit reached",
			"route", route,
			"action", action,
			"resource_type", resource.Type,
			"resource_id", resource.ExternalID,
			"user_id", subject.UserID,
			"concurrency_limit", cap(shadowSlots),
		)
	}
}

func observeDeploymentAuthorization(
	requestCtx context.Context,
	log authorizationShadowLogger,
	checker authz.Checker,
	route string,
	subject authz.Subject,
	action authz.Action,
	resource authz.ResourceRef,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			// Keep recovery best-effort even if the logger caused the panic.
			func() {
				defer func() { _ = recover() }()
				log.Warn("FGA shadow check panic recovered",
					"route", route,
					"resource_id", resource.ExternalID,
					"panic", recovered,
				)
			}()
		}
	}()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), deploymentAuthorizationCheckTimeout)
	defer cancel()
	ctx = authz.WithRequestCache(ctx)
	ctx = authz.WithAuthorizationRoute(ctx, route)

	if _, err := checker.Authorize(ctx, subject, action, resource); err != nil {
		attrs := []any{
			"route", route,
			"action", action,
			"resource_type", resource.Type,
			"resource_id", resource.ExternalID,
			"user_id", subject.UserID,
			"error", err,
		}
		if errors.Is(err, sql.ErrNoRows) {
			log.Debug("FGA shadow membership check failed", attrs...)
			return
		}
		log.Warn("FGA shadow membership check failed", attrs...)
	}
}
