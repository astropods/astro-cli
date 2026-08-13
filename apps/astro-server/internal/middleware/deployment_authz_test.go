package middleware_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	oapispec "github.com/astropods/astro/apps/astro-server/internal/openapi"
	"github.com/gin-gonic/gin"
)

type recordingChecker struct {
	calls       atomic.Int32
	done        chan struct{}
	block       <-chan struct{}
	hasDeadline bool
	subject     authz.Subject
	action      authz.Action
	resource    authz.ResourceRef
	err         error
	allowed     bool
}

func newRecordingChecker(err error, block <-chan struct{}) *recordingChecker {
	return &recordingChecker{done: make(chan struct{}), block: block, err: err, allowed: true}
}

func (c *recordingChecker) Authorize(ctx context.Context, subject authz.Subject, action authz.Action, resource authz.ResourceRef) (bool, error) {
	if c.block != nil {
		select {
		case <-c.block:
		case <-ctx.Done():
		}
	}
	c.subject = subject
	c.action = action
	c.resource = resource
	_, c.hasDeadline = ctx.Deadline()
	c.calls.Add(1)
	close(c.done)
	return c.allowed, c.err
}

type recordingShadowLog struct {
	debug chan string
	warn  chan string
}

func newRecordingShadowLog() *recordingShadowLog {
	return &recordingShadowLog{debug: make(chan string, 1), warn: make(chan string, 1)}
}

func (l *recordingShadowLog) Debug(message string, _ ...any) { l.debug <- message }
func (l *recordingShadowLog) Info(string, ...any)            {}
func (l *recordingShadowLog) Warn(message string, _ ...any)  { l.warn <- message }

type panickingChecker struct{}

func (panickingChecker) Authorize(context.Context, authz.Subject, authz.Action, authz.ResourceRef) (bool, error) {
	panic("checker panic")
}

type blockingChecker struct {
	calls   atomic.Int32
	started chan struct{}
	done    chan struct{}
	release <-chan struct{}
}

func (c *blockingChecker) Authorize(ctx context.Context, _ authz.Subject, _ authz.Action, _ authz.ResourceRef) (bool, error) {
	c.calls.Add(1)
	c.started <- struct{}{}
	defer func() { c.done <- struct{}{} }()
	select {
	case <-c.release:
	case <-ctx.Done():
	}
	return true, nil
}

type capturedWarn struct {
	message string
	attrs   []any
}

type panicCaptureLog struct {
	warn chan capturedWarn
}

func (l *panicCaptureLog) Debug(string, ...any) {}
func (l *panicCaptureLog) Info(string, ...any)  {}
func (l *panicCaptureLog) Warn(message string, attrs ...any) {
	l.warn <- capturedWarn{message: message, attrs: attrs}
}

type concurrencyCaptureLog struct {
	debug chan capturedWarn
}

func (l *concurrencyCaptureLog) Debug(message string, attrs ...any) {
	l.debug <- capturedWarn{message: message, attrs: attrs}
}
func (l *concurrencyCaptureLog) Info(string, ...any) {}
func (l *concurrencyCaptureLog) Warn(string, ...any) {}

func deploymentTestRoutes(router *gin.Engine, catalog *middleware.DeploymentRouteCatalog) *middleware.DeploymentRoutes {
	return middleware.NewDeploymentRoutes(
		oapispec.New("test", "1", ""),
		router.Group("/api/v1"),
		catalog,
	)
}

func registerObservedTestRoute(routes *middleware.DeploymentRoutes, method string, action authz.Action, path string, handler gin.HandlerFunc) {
	switch method {
	case http.MethodGet:
		routes.ObservedGET(action, path, "test", handler)
	case http.MethodPost:
		routes.ObservedPOST(action, path, "test", handler)
	case http.MethodPut:
		routes.ObservedPUT(action, path, "test", handler)
	case http.MethodPatch:
		routes.ObservedPATCH(action, path, "test", handler)
	case http.MethodDelete:
		routes.ObservedDELETE(action, path, "test", handler)
	default:
		panic("unsupported observed test method " + method)
	}
}

func TestObserveDeploymentAuthorizationMapsControlPlaneRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		method        string
		route         string
		request       string
		wantCalls     int32
		wantAction    authz.Action
		handlerAction authz.Action
		handlerStatus int
	}{
		{
			name:       "configuration-bearing detail route",
			method:     http.MethodGet,
			route:      "/api/v1/deployments/:id",
			request:    "/api/v1/deployments/dep_123",
			wantCalls:  1,
			wantAction: authz.ActionDeploymentRead,
		},
		{
			name:       "edit route",
			method:     http.MethodPatch,
			route:      "/api/v1/deployments/:id",
			request:    "/api/v1/deployments/dep_123",
			wantCalls:  1,
			wantAction: authz.ActionDeploymentEdit,
		},
		{
			name:       "operation route",
			method:     http.MethodPost,
			route:      "/api/v1/deployments/:id/restart",
			request:    "/api/v1/deployments/dep_123/restart",
			wantCalls:  1,
			wantAction: authz.ActionDeploymentOperate,
		},
		{
			name:       "file mutation route",
			method:     http.MethodDelete,
			route:      "/api/v1/deployments/:id/files/:fileKey",
			request:    "/api/v1/deployments/dep_123/files/file_123",
			wantCalls:  1,
			wantAction: authz.ActionDeploymentEdit,
		},
		{
			name:          "body-addressed redeploy",
			method:        http.MethodPost,
			route:         "/api/v1/deploy",
			request:       "/api/v1/deploy",
			wantCalls:     1,
			wantAction:    authz.ActionDeploymentOperate,
			handlerAction: authz.ActionDeploymentOperate,
		},
		{
			name:          "body-addressed undeploy",
			method:        http.MethodPost,
			route:         "/api/v1/undeploy",
			request:       "/api/v1/undeploy",
			wantCalls:     1,
			wantAction:    authz.ActionDeploymentDelete,
			handlerAction: authz.ActionDeploymentDelete,
		},
		{
			name:          "rejected body-addressed attempt is still observed",
			method:        http.MethodPost,
			route:         "/api/v1/undeploy",
			request:       "/api/v1/undeploy",
			wantCalls:     1,
			wantAction:    authz.ActionDeploymentDelete,
			handlerAction: authz.ActionDeploymentDelete,
			handlerStatus: http.StatusForbidden,
		},
		{
			name:      "new deployment has no redeploy observation",
			method:    http.MethodPost,
			route:     "/api/v1/deploy",
			request:   "/api/v1/deploy",
			wantCalls: 0,
		},
		{
			name:      "frequently fetched read is cataloged but deferred",
			method:    http.MethodGet,
			route:     "/api/v1/deployments/:id/status",
			request:   "/api/v1/deployments/dep_123/status",
			wantCalls: 0,
		},
		{
			name:      "authorization model is deferred",
			method:    http.MethodPost,
			route:     "/api/v1/deployments/:id/dataset/judgments",
			request:   "/api/v1/deployments/dep_123/dataset/judgments",
			wantCalls: 0,
		},
		{
			name:      "data plane route is excluded",
			method:    http.MethodGet,
			route:     "/api/v1/deployments/:id/chat/conversations",
			request:   "/api/v1/deployments/dep_123/chat/conversations",
			wantCalls: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantStatus := test.handlerStatus
			if wantStatus == 0 {
				wantStatus = http.StatusNoContent
			}
			checker := newRecordingChecker(nil, nil)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
				c.Set(string(auth.SessionContextKey), &auth.Session{
					UserID:             "user_123",
					OrganizationID:     "org_123",
					WorkOSMembershipID: "om_123",
				})
				c.Next()
			})
			catalog := middleware.NewDeploymentRouteCatalog()
			router.Use(middleware.ObserveDeploymentAuthorization(newRecordingShadowLog(), checker, catalog))
			handler := func(c *gin.Context) {
				if test.handlerAction != "" {
					middleware.SetDeploymentAuthorizationObservation(c, test.handlerAction, "dep_123")
				}
				c.Status(wantStatus)
			}
			if strings.Contains(test.route, "/deployments/:id") {
				routes := deploymentTestRoutes(router, catalog)
				path := strings.TrimPrefix(test.route, "/api/v1")
				switch test.name {
				case "frequently fetched read is cataloged but deferred":
					routes.DeferredGET(authz.ActionDeploymentRead, path, "test", handler)
				case "authorization model is deferred":
					routes.ModelDeferredPOST(path, "test", handler)
				case "data plane route is excluded":
					routes.DataPlaneGET(path, "test", handler)
				default:
					registerObservedTestRoute(routes, test.method, test.wantAction, path, handler)
				}
			} else {
				router.Handle(test.method, test.route, handler)
			}

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(test.method, test.request, nil))

			if response.Code != wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, wantStatus)
			}
			if test.wantCalls == 1 {
				select {
				case <-checker.done:
				case <-time.After(time.Second):
					t.Fatal("Authorize() was not called")
				}
			}
			if calls := checker.calls.Load(); calls != test.wantCalls {
				t.Fatalf("Authorize() calls = %d, want %d", calls, test.wantCalls)
			}
			if test.wantCalls == 1 {
				if !checker.hasDeadline {
					t.Fatal("Authorize() context has no timeout")
				}
				if checker.action != test.wantAction || checker.resource != authz.DeploymentResource("dep_123") {
					t.Fatalf("Authorize() action=%q resource=%+v", checker.action, checker.resource)
				}
				if checker.subject.MembershipID != "om_123" || checker.subject.OrgID != "org_123" {
					t.Fatalf("Authorize() subject = %+v", checker.subject)
				}
			}
		})
	}
}

func TestObserveDeploymentAuthorizationDoesNotBlockHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	release := make(chan struct{})
	checker := newRecordingChecker(errors.New("membership lookup failed"), release)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user_123", WorkOSMembershipID: "om_123"})
		c.Next()
	})
	catalog := middleware.NewDeploymentRouteCatalog()
	router.Use(middleware.ObserveDeploymentAuthorization(newRecordingShadowLog(), checker, catalog))
	deploymentTestRoutes(router, catalog).ObservedPATCH(
		authz.ActionDeploymentEdit,
		"/deployments/:id",
		"test",
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	response := httptest.NewRecorder()
	responseDone := make(chan struct{})
	go func() {
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/v1/deployments/dep_123", nil))
		close(responseDone)
	}()

	select {
	case <-responseDone:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("HTTP handler waited for the shadow checker")
	}
	close(release)
	select {
	case <-checker.done:
	case <-time.After(time.Second):
		t.Fatal("Authorize() did not finish")
	}
	if response.Code != http.StatusNoContent || checker.calls.Load() != 1 {
		t.Fatalf("status=%d calls=%d, want status=204 calls=1", response.Code, checker.calls.Load())
	}
}

func TestObserveDeploymentAuthorizationLogsNotFoundAtDebug(t *testing.T) {
	gin.SetMode(gin.TestMode)

	checker := newRecordingChecker(fmt.Errorf("resolve deployment: %w", sql.ErrNoRows), nil)
	log := newRecordingShadowLog()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user_123", WorkOSMembershipID: "om_123"})
		c.Next()
	})
	catalog := middleware.NewDeploymentRouteCatalog()
	router.Use(middleware.ObserveDeploymentAuthorization(log, checker, catalog))
	deploymentTestRoutes(router, catalog).ObservedGET(
		authz.ActionDeploymentRead,
		"/deployments/:id",
		"test",
		func(c *gin.Context) { c.Status(http.StatusNotFound) },
	)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/missing", nil))

	select {
	case message := <-log.debug:
		if message != "FGA shadow membership check failed" {
			t.Fatalf("debug message = %q", message)
		}
	case <-time.After(time.Second):
		t.Fatal("not-found shadow error was not logged at debug")
	}
	select {
	case message := <-log.warn:
		t.Fatalf("unexpected warn log %q", message)
	default:
	}
}

func TestObserveDeploymentAuthorizationRecoversCheckerPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	log := &panicCaptureLog{warn: make(chan capturedWarn, 1)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user_123", WorkOSMembershipID: "om_123"})
		c.Next()
	})
	catalog := middleware.NewDeploymentRouteCatalog()
	router.Use(middleware.ObserveDeploymentAuthorization(log, panickingChecker{}, catalog))
	deploymentTestRoutes(router, catalog).ObservedGET(
		authz.ActionDeploymentRead,
		"/deployments/:id",
		"test",
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep_123", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}

	select {
	case entry := <-log.warn:
		if entry.message != "FGA shadow check panic recovered" {
			t.Fatalf("warn message = %q", entry.message)
		}
		attrs := make(map[string]any, len(entry.attrs)/2)
		for i := 0; i+1 < len(entry.attrs); i += 2 {
			key, ok := entry.attrs[i].(string)
			if ok {
				attrs[key] = entry.attrs[i+1]
			}
		}
		if attrs["route"] != "/api/v1/deployments/:id" || attrs["resource_id"] != "dep_123" || attrs["panic"] != "checker panic" {
			t.Fatalf("warn attrs = %+v", attrs)
		}
	case <-time.After(time.Second):
		t.Fatal("recovered panic was not logged")
	}
}

func TestObserveDeploymentAuthorizationDropsCheckAtConcurrencyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const maxConcurrent = 16
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	checker := &blockingChecker{
		started: make(chan struct{}),
		done:    make(chan struct{}, maxConcurrent),
		release: release,
	}
	log := &concurrencyCaptureLog{debug: make(chan capturedWarn, 1)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user_123", WorkOSMembershipID: "om_123"})
		c.Next()
	})
	catalog := middleware.NewDeploymentRouteCatalog()
	router.Use(middleware.ObserveDeploymentAuthorization(log, checker, catalog))
	deploymentTestRoutes(router, catalog).ObservedGET(
		authz.ActionDeploymentRead,
		"/deployments/:id",
		"test",
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	for i := 0; i < maxConcurrent; i++ {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/deployments/dep_%d", i), nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("request %d status = %d, want %d", i, response.Code, http.StatusNoContent)
		}
		select {
		case <-checker.started:
		case <-time.After(time.Second):
			t.Fatalf("shadow check %d did not start", i)
		}
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dropped", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("dropped request status = %d, want %d", response.Code, http.StatusNoContent)
	}
	select {
	case entry := <-log.debug:
		if entry.message != "FGA shadow check skipped: concurrency limit reached" {
			t.Fatalf("debug message = %q", entry.message)
		}
		attrs := make(map[string]any, len(entry.attrs)/2)
		for i := 0; i+1 < len(entry.attrs); i += 2 {
			key, ok := entry.attrs[i].(string)
			if ok {
				attrs[key] = entry.attrs[i+1]
			}
		}
		if attrs["resource_id"] != "dropped" || attrs["concurrency_limit"] != maxConcurrent {
			t.Fatalf("debug attrs = %+v", attrs)
		}
	case <-time.After(time.Second):
		t.Fatal("saturated shadow check was not logged")
	}
	if calls := checker.calls.Load(); calls != maxConcurrent {
		t.Fatalf("Authorize() calls = %d, want %d", calls, maxConcurrent)
	}

	close(release)
	released = true
	for i := 0; i < maxConcurrent; i++ {
		select {
		case <-checker.done:
		case <-time.After(time.Second):
			t.Fatalf("shadow check %d did not finish", i)
		}
	}
}

func TestEnforceDeploymentAuthorizationStagesMutationsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		method      string
		route       string
		request     string
		action      authz.Action
		allowed     bool
		err         error
		wantStatus  int
		wantHandler bool
		wantCalls   int32
	}{
		{
			name: "allowed mutation", method: http.MethodPost,
			route: "/api/v1/deployments/:id/restart", request: "/api/v1/deployments/dep_123/restart",
			action:  authz.ActionDeploymentOperate,
			allowed: true, wantStatus: http.StatusNoContent, wantHandler: true, wantCalls: 1,
		},
		{
			name: "denied mutation is concealed", method: http.MethodPatch,
			route: "/api/v1/deployments/:id", request: "/api/v1/deployments/dep_123",
			action:     authz.ActionDeploymentEdit,
			wantStatus: http.StatusNotFound, wantCalls: 1,
		},
		{
			name: "resource outside rollout stays legacy", method: http.MethodPost,
			route: "/api/v1/deployments/:id/stop", request: "/api/v1/deployments/dep_123/stop",
			action: authz.ActionDeploymentOperate,
			err:    authz.ErrFGAResourceNotEnabled, wantStatus: http.StatusNoContent, wantHandler: true, wantCalls: 1,
		},
		{
			name: "authorization failure is retryable", method: http.MethodDelete,
			route: "/api/v1/deployments/:id/files/:fileKey", request: "/api/v1/deployments/dep_123/files/file_123",
			action: authz.ActionDeploymentEdit,
			err:    errors.New("workos unavailable"), wantStatus: http.StatusServiceUnavailable, wantCalls: 1,
		},
		{
			name: "read remains unenforced", method: http.MethodGet,
			route: "/api/v1/deployments/:id", request: "/api/v1/deployments/dep_123",
			action:     authz.ActionDeploymentRead,
			wantStatus: http.StatusNoContent, wantHandler: true,
		},
		{
			name: "watch subscription uses read visibility", method: http.MethodPost,
			route: "/api/v1/deployments/:id/watchers/me", request: "/api/v1/deployments/dep_123/watchers/me",
			action:     authz.ActionDeploymentRead,
			wantStatus: http.StatusNotFound, wantCalls: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := newRecordingChecker(test.err, nil)
			checker.allowed = test.allowed
			handlerCalled := false
			router := gin.New()
			catalog := middleware.NewDeploymentRouteCatalog()
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
				c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user_123", WorkOSMembershipID: "om_123"})
				c.Next()
			})
			router.Use(middleware.EnforceDeploymentAuthorization(newRecordingShadowLog(), checker, catalog))
			handler := func(c *gin.Context) {
				handlerCalled = true
				c.Status(http.StatusNoContent)
			}
			routes := deploymentTestRoutes(router, catalog)
			registerObservedTestRoute(routes, test.method, test.action, strings.TrimPrefix(test.route, "/api/v1"), handler)

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(test.method, test.request, nil))
			if response.Code != test.wantStatus || handlerCalled != test.wantHandler || checker.calls.Load() != test.wantCalls {
				t.Fatalf("status=%d handler=%v calls=%d", response.Code, handlerCalled, checker.calls.Load())
			}
			if test.wantCalls == 1 && !checker.hasDeadline {
				t.Fatal("enforcement check context has no deadline")
			}
			if test.wantCalls == 1 && checker.action != test.action {
				t.Fatalf("checked action = %q, want %q", checker.action, test.action)
			}
		})
	}
}

func TestEnforcementAndShadowMiddlewareCoordinate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		enforcementAllowed bool
		enforcementErr     error
		wantStatus         int
		wantHandler        bool
		wantShadow         bool
	}{
		{
			name:               "opted-out organization keeps legacy handler and shadow evidence",
			enforcementAllowed: true,
			enforcementErr:     authz.ErrFGAResourceNotEnabled,
			wantStatus:         http.StatusNoContent,
			wantHandler:        true,
			wantShadow:         true,
		},
		{
			name:               "enforcement allow does not duplicate shadow check",
			enforcementAllowed: true,
			wantStatus:         http.StatusNoContent,
			wantHandler:        true,
		},
		{
			name:       "enforcement denial does not duplicate shadow check",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enforcementChecker := newRecordingChecker(test.enforcementErr, nil)
			enforcementChecker.allowed = test.enforcementAllowed
			shadowChecker := newRecordingChecker(nil, nil)
			handlerCalled := false

			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
				c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user_123", WorkOSMembershipID: "om_123"})
				c.Next()
			})
			catalog := middleware.NewDeploymentRouteCatalog()
			router.Use(middleware.EnforceDeploymentAuthorization(newRecordingShadowLog(), enforcementChecker, catalog))
			router.Use(middleware.ObserveDeploymentAuthorization(newRecordingShadowLog(), shadowChecker, catalog))
			deploymentTestRoutes(router, catalog).ObservedPOST(
				authz.ActionDeploymentOperate,
				"/deployments/:id/restart",
				"test",
				func(c *gin.Context) {
					handlerCalled = true
					c.Status(http.StatusNoContent)
				},
			)

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/deployments/dep_123/restart", nil))
			if response.Code != test.wantStatus || handlerCalled != test.wantHandler || enforcementChecker.calls.Load() != 1 {
				t.Fatalf("status=%d handler=%v enforcement calls=%d", response.Code, handlerCalled, enforcementChecker.calls.Load())
			}

			if test.wantShadow {
				select {
				case <-shadowChecker.done:
				case <-time.After(time.Second):
					t.Fatal("shadow check did not run after enforcement rollout skip")
				}
				if shadowChecker.calls.Load() != 1 {
					t.Fatalf("shadow calls=%d, want 1", shadowChecker.calls.Load())
				}
				return
			}

			select {
			case <-shadowChecker.done:
				t.Fatal("shadow check duplicated a completed enforcement decision")
			case <-time.After(100 * time.Millisecond):
			}
		})
	}
}

func TestEnforceDeploymentAuthorizationLeavesModelDeferredRoutesOnLegacyPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	checker := newRecordingChecker(nil, nil)
	checker.allowed = false
	router := gin.New()
	catalog := middleware.NewDeploymentRouteCatalog()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user_123", WorkOSMembershipID: "om_123"})
		c.Next()
	})
	router.Use(middleware.EnforceDeploymentAuthorization(newRecordingShadowLog(), checker, catalog))
	routes := deploymentTestRoutes(router, catalog)
	routes.ModelDeferredPOST("/deployments/:id/dataset/judgments", "test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/deployments/dep_123/dataset/judgments", nil))
	if response.Code != http.StatusNoContent || checker.calls.Load() != 0 {
		t.Fatalf("status=%d calls=%d, want legacy handler with no deployment FGA check", response.Code, checker.calls.Load())
	}
}

func TestAuthorizeDeploymentActionEnforcesBodyAddressedMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	checker := newRecordingChecker(nil, nil)
	checker.allowed = false
	router := gin.New()
	catalog := middleware.NewDeploymentRouteCatalog()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user_123", WorkOSMembershipID: "om_123"})
		c.Next()
	})
	router.Use(middleware.EnforceDeploymentAuthorization(newRecordingShadowLog(), checker, catalog))
	router.POST("/api/v1/undeploy", func(c *gin.Context) {
		if middleware.AuthorizeDeploymentAction(c, authz.ActionDeploymentDelete, "dep_123") {
			c.Status(http.StatusNoContent)
		}
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/undeploy", nil))
	if response.Code != http.StatusNotFound || checker.calls.Load() != 1 || checker.action != authz.ActionDeploymentDelete {
		t.Fatalf("status=%d calls=%d action=%q", response.Code, checker.calls.Load(), checker.action)
	}
}
