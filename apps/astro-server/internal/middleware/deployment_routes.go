package middleware

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
	oapispec "github.com/astropods/astro/apps/astro-server/internal/openapi"
	"github.com/gin-gonic/gin"
)

// DeploymentRouteCatalog is populated by the same calls that register
// deployment routes, keeping the route and its authorization policy together.
type DeploymentRouteCatalog struct {
	policies map[deploymentRoute]deploymentRoutePolicy
}

func NewDeploymentRouteCatalog() *DeploymentRouteCatalog {
	return &DeploymentRouteCatalog{policies: make(map[deploymentRoute]deploymentRoutePolicy)}
}

func (c *DeploymentRouteCatalog) policy(method, path string) (deploymentRoutePolicy, bool) {
	if c == nil {
		return deploymentRoutePolicy{}, false
	}
	policy, ok := c.policies[deploymentRoute{method: method, path: path}]
	if !ok {
		policy, ok = c.policies[deploymentRoute{method: "*", path: path}]
	}
	return policy, ok
}

func (c *DeploymentRouteCatalog) add(method, path string, policy deploymentRoutePolicy) {
	if c == nil {
		panic("deployment route catalog is not configured")
	}
	if policy.kind == deploymentRouteDataPlane || policy.kind == deploymentRouteModelDeferred {
		if policy.action != "" {
			panic(fmt.Sprintf("deployment special route %s %s has action %q", method, path, policy.action))
		}
	} else if !deploymentActionExists(policy.action) {
		panic(fmt.Sprintf("deployment route %s %s uses unknown action %q", method, path, policy.action))
	}
	route := deploymentRoute{method: method, path: path}
	if _, exists := c.policies[route]; exists {
		panic(fmt.Sprintf("deployment route policy already registered for %s %s", method, path))
	}
	c.policies[route] = policy
}

func deploymentActionExists(action authz.Action) bool {
	for _, candidate := range authz.DeploymentActions() {
		if candidate == action {
			return true
		}
	}
	return false
}

// Validate confirms that every live deployment-ID route has exactly one
// explicitly registered authorization classification.
func (c *DeploymentRouteCatalog) Validate(routes gin.RoutesInfo) error {
	registered := make(map[deploymentRoute]struct{})
	for _, route := range routes {
		if !strings.Contains(route.Path, "/deployments/:id") {
			continue
		}
		key := deploymentRoute{method: route.Method, path: route.Path}
		registered[key] = struct{}{}
		if _, ok := c.policy(route.Method, route.Path); !ok {
			return fmt.Errorf("deployment route %s %s has no authorization policy", route.Method, route.Path)
		}
	}

	missing := make([]string, 0)
	for route := range c.policies {
		if route.method == "*" {
			found := false
			for registeredRoute := range registered {
				if registeredRoute.path == route.path {
					found = true
					break
				}
			}
			if found {
				continue
			}
		} else if _, ok := registered[route]; ok {
			continue
		}
		missing = append(missing, fmt.Sprintf("%s %s", route.method, route.path))
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("deployment authorization policies have no route: %s", strings.Join(missing, ", "))
	}
	return nil
}

// DeploymentRoutes registers each deployment route and its policy in one call.
type DeploymentRoutes struct {
	api     *oapispec.Spec
	group   *gin.RouterGroup
	catalog *DeploymentRouteCatalog
}

func NewDeploymentRoutes(api *oapispec.Spec, group *gin.RouterGroup, catalog *DeploymentRouteCatalog) *DeploymentRoutes {
	return &DeploymentRoutes{api: api, group: group, catalog: catalog}
}

func (r *DeploymentRoutes) ObservedGET(action authz.Action, path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodGet, r.group.BasePath()+path, observedDeploymentRoute(action))
	r.api.GET(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) DeferredGET(action authz.Action, path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodGet, r.group.BasePath()+path, deferredDeploymentRoute(action))
	r.api.GET(r.group, path, summary, handler, opts...)
}

// ModelDeferred routes are deployment-addressed, but their authorization
// resource and permissions have not been established yet.
func (r *DeploymentRoutes) ModelDeferredGET(path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodGet, r.group.BasePath()+path, modelDeferredDeploymentRoute())
	r.api.GET(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) ModelDeferredPOST(path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodPost, r.group.BasePath()+path, modelDeferredDeploymentRoute())
	r.api.POST(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) ModelDeferredPUT(path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodPut, r.group.BasePath()+path, modelDeferredDeploymentRoute())
	r.api.PUT(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) ModelDeferredPATCH(path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodPatch, r.group.BasePath()+path, modelDeferredDeploymentRoute())
	r.api.PATCH(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) ModelDeferredDELETE(path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodDelete, r.group.BasePath()+path, modelDeferredDeploymentRoute())
	r.api.DELETE(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) ObservedPOST(action authz.Action, path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodPost, r.group.BasePath()+path, observedDeploymentRoute(action))
	r.api.POST(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) ObservedPUT(action authz.Action, path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodPut, r.group.BasePath()+path, observedDeploymentRoute(action))
	r.api.PUT(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) ObservedPATCH(action authz.Action, path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodPatch, r.group.BasePath()+path, observedDeploymentRoute(action))
	r.api.PATCH(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) ObservedDELETE(action authz.Action, path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodDelete, r.group.BasePath()+path, observedDeploymentRoute(action))
	r.api.DELETE(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) DataPlaneGET(path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodGet, r.group.BasePath()+path, deploymentDataPlaneRoute())
	r.api.GET(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) DataPlanePUT(path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodPut, r.group.BasePath()+path, deploymentDataPlaneRoute())
	r.api.PUT(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) DataPlaneDELETE(path, summary string, handler gin.HandlerFunc, opts ...oapispec.Option) {
	r.catalog.add(http.MethodDelete, r.group.BasePath()+path, deploymentDataPlaneRoute())
	r.api.DELETE(r.group, path, summary, handler, opts...)
}

func (r *DeploymentRoutes) DataPlaneAny(path string, handler gin.HandlerFunc) {
	r.catalog.add("*", r.group.BasePath()+path, deploymentDataPlaneRoute())
	r.group.Any(path, handler)
}
