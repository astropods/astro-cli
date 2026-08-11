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
	deploymentAuthorizationShadowTimeout       = 2 * time.Second
	deploymentAuthorizationShadowMaxConcurrent = 16
)

type deploymentRoute struct {
	method string
	path   string
}

// PR5 intentionally samples one read and one edit route. The complete route
// catalog is a security review artifact, not something inferred by middleware.
var deploymentRouteActions = map[deploymentRoute]authz.Action{
	{http.MethodGet, "/api/v1/deployments/:id"}:   authz.ActionDeploymentRead,
	{http.MethodPatch, "/api/v1/deployments/:id"}: authz.ActionDeploymentEdit,
}

type authorizationShadowLogger interface {
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}

// ObserveDeploymentAuthorization samples mapped deployment routes without
// aborting or changing their existing membership-based behavior.
func ObserveDeploymentAuthorization(log authorizationShadowLogger, checker authz.Checker) gin.HandlerFunc {
	shadowSlots := make(chan struct{}, deploymentAuthorizationShadowMaxConcurrent)

	return func(c *gin.Context) {
		action, ok := deploymentRouteActions[deploymentRoute{method: c.Request.Method, path: c.FullPath()}]
		if !ok {
			c.Next()
			return
		}

		subject, ok := SubjectFromContext(c)
		deploymentID := c.Param("id")
		if !ok || deploymentID == "" {
			c.Next()
			return
		}

		route := c.FullPath()
		resource := authz.DeploymentResource(deploymentID)
		select {
		case shadowSlots <- struct{}{}:
			requestCtx := c.Request.Context()
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
		c.Next()
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
