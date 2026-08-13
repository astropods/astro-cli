package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

type capabilityEvaluatorFunc func(context.Context, authz.Subject, authz.ResourceRef, []authz.Action) (authz.CapabilitySet, error)

func (f capabilityEvaluatorFunc) Evaluate(ctx context.Context, subject authz.Subject, resource authz.ResourceRef, actions []authz.Action) (authz.CapabilitySet, error) {
	return f(ctx, subject, resource, actions)
}

func TestGetDeploymentCapabilitiesReturnsCompleteEffectiveSet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	wantActions := authz.DeploymentActions()
	handler := GetDeploymentCapabilities(logger.New("error", "json"), capabilityEvaluatorFunc(func(ctx context.Context, subject authz.Subject, resource authz.ResourceRef, actions []authz.Action) (authz.CapabilitySet, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("capability context has no deadline")
		}
		if subject.MembershipID != "om_123" || resource != authz.DeploymentResource("dep_123") || len(actions) != len(wantActions) {
			t.Fatalf("subject=%+v resource=%+v actions=%d", subject, resource, len(actions))
		}
		decisions := make(map[authz.Action]bool, len(actions))
		for _, action := range actions {
			decisions[action] = action == authz.ActionDeploymentRead
		}
		return authz.CapabilitySet{
			Mode:    authz.CapabilityModeFGA,
			Actions: decisions,
		}, nil
	}))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user_123", WorkOSMembershipID: "om_123"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/capabilities", handler)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep_123/capabilities", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if body == "" || !containsAll(body, `"mode":"fga"`, `"deployment:read":true`, `"deployment:edit":false`) {
		t.Fatalf("body = %s", body)
	}
	for _, action := range wantActions {
		if !strings.Contains(body, `"`+string(action)+`":`) {
			t.Fatalf("body missing capability %q: %s", action, body)
		}
	}
}

func TestGetDeploymentCapabilitiesHidesResourcesAndSurfacesRetryableIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "hidden resource", err: authz.ErrResourceNotVisible, wantStatus: http.StatusNotFound},
		{name: "missing membership", err: authz.ErrWorkOSMembershipUnavailable, wantStatus: http.StatusServiceUnavailable},
		{name: "workos failure", err: errors.New("workos unavailable"), wantStatus: http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
				c.Next()
			})
			router.GET("/api/v1/deployments/:id/capabilities", GetDeploymentCapabilities(
				logger.New("error", "json"),
				capabilityEvaluatorFunc(func(context.Context, authz.Subject, authz.ResourceRef, []authz.Action) (authz.CapabilitySet, error) {
					return authz.CapabilitySet{}, test.err
				}),
			))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep_123/capabilities", nil))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}

func TestGetDeploymentCapabilitiesConcealsMissingBaselineRead(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user_123", WorkOSMembershipID: "om_123"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/capabilities", GetDeploymentCapabilities(
		logger.New("error", "json"),
		capabilityEvaluatorFunc(func(context.Context, authz.Subject, authz.ResourceRef, []authz.Action) (authz.CapabilitySet, error) {
			return authz.CapabilitySet{
				Mode:    authz.CapabilityModeFGA,
				Actions: map[authz.Action]bool{authz.ActionDeploymentRead: false},
			}, nil
		}),
	))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep_123/capabilities", nil))
	if response.Code != http.StatusNotFound || !containsAll(response.Body.String(), `"error":"deployment not found"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
