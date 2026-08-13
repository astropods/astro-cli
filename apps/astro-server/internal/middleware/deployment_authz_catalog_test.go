package middleware

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
	oapispec "github.com/astropods/astro/apps/astro-server/internal/openapi"
	"github.com/gin-gonic/gin"
)

func TestDeploymentRoutesRegisterPolicyAlongsideRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	group := engine.Group("/api/v1")
	catalog := NewDeploymentRouteCatalog()
	routes := NewDeploymentRoutes(oapispec.New("test", "1", ""), group, catalog)
	handler := func(c *gin.Context) { c.Status(http.StatusNoContent) }

	routes.ObservedPOST(authz.ActionDeploymentOperate, "/deployments/:id/restart", "restart", handler)
	routes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/status", "status", handler)
	routes.ModelDeferredPOST("/deployments/:id/dataset/judgments", "judgment", handler)
	routes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/capabilities", "capabilities", handler)
	routes.DataPlaneAny("/deployments/:id/messaging/*proxyPath", handler)

	if err := catalog.Validate(engine.Routes()); err != nil {
		t.Fatal(err)
	}
	policy, ok := catalog.policy(http.MethodPost, "/api/v1/deployments/:id/restart")
	if !ok || policy.kind != deploymentRouteObserved || policy.action != authz.ActionDeploymentOperate {
		t.Fatalf("restart policy = %+v, found=%v", policy, ok)
	}
	if policy, ok := catalog.policy(http.MethodPatch, "/api/v1/deployments/:id/messaging/*proxyPath"); !ok || policy.kind != deploymentRouteDataPlane {
		t.Fatalf("data-plane policy = %+v, found=%v", policy, ok)
	}
}

func TestDeploymentRouteCatalogRejectsUnclassifiedLiveRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/api/v1/deployments/:id/unclassified", func(c *gin.Context) {})

	err := NewDeploymentRouteCatalog().Validate(engine.Routes())
	if err == nil || !strings.Contains(err.Error(), "has no authorization policy") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDeploymentRouteCatalogRejectsPolicyWithoutLiveRoute(t *testing.T) {
	catalog := NewDeploymentRouteCatalog()
	catalog.add(http.MethodGet, "/api/v1/deployments/:id/missing", deferredDeploymentRoute(authz.ActionDeploymentRead))

	err := catalog.Validate(nil)
	if err == nil || !strings.Contains(err.Error(), "have no route") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDeploymentRouteCatalogRejectsUnknownAction(t *testing.T) {
	catalog := NewDeploymentRouteCatalog()
	defer func() {
		if recover() == nil {
			t.Fatal("registering an unknown deployment action did not panic")
		}
	}()
	catalog.add(http.MethodGet, "/api/v1/deployments/:id/test", deferredDeploymentRoute(authz.Action("deployment:not_real")))
}

func TestMainDeploymentRoutesUseCatalogRegistrar(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	mainFile := filepath.Join(filepath.Dir(testFile), "..", "..", "main.go")
	files := token.NewFileSet()
	source, err := parser.ParseFile(files, mainFile, nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	ast.Inspect(source, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		containsDeploymentRoute := false
		for _, argument := range call.Args {
			literal, ok := argument.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}
			value, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr == nil && strings.Contains(value, "/deployments/:id") {
				containsDeploymentRoute = true
				break
			}
		}
		if !containsDeploymentRoute {
			return true
		}

		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			t.Errorf("%s: deployment route must use deploymentRoutes", files.Position(call.Pos()))
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || receiver.Name != "deploymentRoutes" {
			t.Errorf("%s: deployment route must use deploymentRoutes, found %s", files.Position(call.Pos()), selector.Sel.Name)
		}
		return true
	})
}
