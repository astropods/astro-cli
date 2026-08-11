package middleware_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
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
}

func newRecordingChecker(err error, block <-chan struct{}) *recordingChecker {
	return &recordingChecker{done: make(chan struct{}), block: block, err: err}
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
	return true, c.err
}

type recordingShadowLog struct {
	debug chan string
	warn  chan string
}

func newRecordingShadowLog() *recordingShadowLog {
	return &recordingShadowLog{debug: make(chan string, 1), warn: make(chan string, 1)}
}

func (l *recordingShadowLog) Debug(message string, _ ...any) { l.debug <- message }
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
func (l *panicCaptureLog) Warn(message string, attrs ...any) {
	l.warn <- capturedWarn{message: message, attrs: attrs}
}

type concurrencyCaptureLog struct {
	debug chan capturedWarn
}

func (l *concurrencyCaptureLog) Debug(message string, attrs ...any) {
	l.debug <- capturedWarn{message: message, attrs: attrs}
}
func (l *concurrencyCaptureLog) Warn(string, ...any) {}

func TestObserveDeploymentAuthorizationMapsControlPlaneRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		method     string
		route      string
		request    string
		wantCalls  int32
		wantAction authz.Action
	}{
		{
			name:       "read route",
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
			name:      "unreviewed mutation is excluded",
			method:    http.MethodPost,
			route:     "/api/v1/deployments/:id/restart",
			request:   "/api/v1/deployments/dep_123/restart",
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
			router.Use(middleware.ObserveDeploymentAuthorization(newRecordingShadowLog(), checker))
			router.Handle(test.method, test.route, func(c *gin.Context) { c.Status(http.StatusNoContent) })

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(test.method, test.request, nil))

			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
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
	router.Use(middleware.ObserveDeploymentAuthorization(newRecordingShadowLog(), checker))
	router.PATCH("/api/v1/deployments/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })

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
	router.Use(middleware.ObserveDeploymentAuthorization(log, checker))
	router.GET("/api/v1/deployments/:id", func(c *gin.Context) { c.Status(http.StatusNotFound) })

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
	router.Use(middleware.ObserveDeploymentAuthorization(log, panickingChecker{}))
	router.GET("/api/v1/deployments/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })

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
	router.Use(middleware.ObserveDeploymentAuthorization(log, checker))
	router.GET("/api/v1/deployments/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })

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
