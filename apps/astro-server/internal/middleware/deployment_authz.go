package middleware

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/gin-gonic/gin"
)

const (
	deploymentAuthorizationShadowTimeout       = 2 * time.Second
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
	Warn(msg string, args ...any)
}

const deploymentAuthorizationObservationKey = "deploymentAuthorizationObservation"

type deploymentAuthorizationObservation struct {
	action       authz.Action
	deploymentID string
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

// ObserveDeploymentAuthorization samples mapped deployment routes without
// aborting or changing their existing membership-based behavior.
func ObserveDeploymentAuthorization(log authorizationShadowLogger, checker authz.Checker, catalog *DeploymentRouteCatalog) gin.HandlerFunc {
	shadowSlots := make(chan struct{}, deploymentAuthorizationShadowMaxConcurrent)

	return func(c *gin.Context) {
		route := c.FullPath()
		policy, ok := catalog.policy(c.Request.Method, c.FullPath())
		subject, hasSubject := SubjectFromContext(c)
		deploymentID := c.Param("id")
		if ok && policy.kind == deploymentRouteObserved && hasSubject && deploymentID != "" {
			startDeploymentAuthorizationObservation(
				c.Request.Context(), log, checker, shadowSlots, route, subject,
				policy.action, authz.DeploymentResource(deploymentID),
			)
			c.Next()
			return
		}

		c.Next()
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

	ctx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), deploymentAuthorizationShadowTimeout)
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
