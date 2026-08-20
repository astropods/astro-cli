package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

type accountExperimentGateFunc func(context.Context, string) (bool, error)

func (f accountExperimentGateFunc) Enabled(ctx context.Context, accountID string) (bool, error) {
	return f(ctx, accountID)
}

type accessGroupMembersFunc func(context.Context, string, string) (*account.AccountMember, error)

func (f accessGroupMembersFunc) GetMemberContext(ctx context.Context, accountID, userID string) (*account.AccountMember, error) {
	return f(ctx, accountID, userID)
}

type recordingAccessGroupAuditStore struct {
	events []auditlog.Event
}

func (s *recordingAccessGroupAuditStore) LogAsync(_ *logger.Logger, event auditlog.Event) {
	s.events = append(s.events, event)
}

func TestAccessGroupListUsesOrganizationScopeAndCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groups := &authz.FakeGroups{ListGroupsFunc: func(_ context.Context, organizationID string, page authz.PageRequest) (authz.GroupPage, error) {
		if organizationID != "org_123" || page.After != "cursor_1" || page.Limit != 25 {
			t.Fatalf("organization=%q page=%+v", organizationID, page)
		}
		return authz.GroupPage{Groups: []authz.Group{{ID: "group_123", Name: "Platform"}}, NextCursor: "cursor_2"}, nil
	}}
	handler := NewAccessGroupHandler(logger.New("error", "json"), groups, enabledAccountExperiment, nil, nil, true)
	router := accessGroupTestRouter(handler.List)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/groups?limit=25&cursor=cursor_1", nil))
	if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"id":"group_123"`, `"next_cursor":"cursor_2"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAccessGroupDisabledNeverCallsWorkOS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAccessGroupHandler(logger.New("error", "json"), &authz.FakeGroups{}, enabledAccountExperiment, nil, nil, false)
	response := httptest.NewRecorder()
	accessGroupTestRouter(handler.List).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/groups", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAccessGroupCreateWritesAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groups := &authz.FakeGroups{CreateGroupFunc: func(_ context.Context, organizationID, name, description string) (authz.Group, error) {
		if organizationID != "org_123" || name != "Platform" || description != "Build team" {
			t.Fatalf("create organization/name/description = %q/%q/%q", organizationID, name, description)
		}
		return authz.Group{ID: "group_123", OrganizationID: organizationID, Name: name, Description: description}, nil
	}}
	audit := &recordingAccessGroupAuditStore{}
	handler := NewAccessGroupHandler(logger.New("error", "json"), groups, enabledAccountExperiment, nil, audit, true)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/groups", strings.NewReader(`{"name":" Platform ","description":" Build team "}`))
	request.Header.Set("Content-Type", "application/json")
	accessGroupTestRouter(handler.Create).ServeHTTP(response, request)
	if response.Code != http.StatusCreated || len(audit.events) != 1 {
		t.Fatalf("status=%d audit=%v body=%s", response.Code, audit.events, response.Body.String())
	}
	event := audit.events[0]
	if event.AccountID != "acct_123" || event.Action != auditlog.AccessGroupCreate || event.ResourceType != "access_group" || event.ResourceID != "group_123" {
		t.Fatalf("audit event = %+v", event)
	}
}

func TestAccessGroupMemberMutationsUseMirroredMembershipAndAuditChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, operation := range []struct {
		name, method, action string
		add                  bool
	}{
		{name: "add", method: http.MethodPost, action: auditlog.AccessGroupAddMember, add: true},
		{name: "remove", method: http.MethodDelete, action: auditlog.AccessGroupRemoveMember},
	} {
		t.Run(operation.name, func(t *testing.T) {
			called := false
			groups := &authz.FakeGroups{}
			if operation.add {
				groups.AddGroupMemberFunc = func(_ context.Context, organizationID, groupID, membershipID string) error {
					called = organizationID == "org_123" && groupID == "group_123" && membershipID == "om_123"
					return nil
				}
			} else {
				groups.RemoveGroupMemberFunc = func(_ context.Context, organizationID, groupID, membershipID string) error {
					called = organizationID == "org_123" && groupID == "group_123" && membershipID == "om_123"
					return nil
				}
			}
			members := accessGroupMembersFunc(func(_ context.Context, accountID, userID string) (*account.AccountMember, error) {
				return &account.AccountMember{AccountID: accountID, UserID: userID, WorkOSMembershipID: "om_123"}, nil
			})
			audit := &recordingAccessGroupAuditStore{}
			handler := NewAccessGroupHandler(logger.New("error", "json"), groups, enabledAccountExperiment, members, audit, true)
			body := ""
			if operation.add {
				body = `{"user_id":"user_123"}`
			}
			request := httptest.NewRequest(operation.method, "/groups", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			accessGroupTestRouter(func(c *gin.Context) {
				if operation.add {
					handler.AddMember(c)
				} else {
					handler.RemoveMember(c)
				}
			}).ServeHTTP(response, request)
			if response.Code != http.StatusNoContent || !called || len(audit.events) != 1 || audit.events[0].Action != operation.action {
				t.Fatalf("status=%d called=%t audit=%v body=%s", response.Code, called, audit.events, response.Body.String())
			}
		})
	}
}

func TestAccessGroupMemberMustBeProvisioned(t *testing.T) {
	gin.SetMode(gin.TestMode)
	members := accessGroupMembersFunc(func(_ context.Context, accountID, userID string) (*account.AccountMember, error) {
		return &account.AccountMember{AccountID: accountID, UserID: userID}, nil
	})
	handler := NewAccessGroupHandler(logger.New("error", "json"), &authz.FakeGroups{}, enabledAccountExperiment, members, nil, true)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/groups", strings.NewReader(`{"user_id":"user_123"}`))
	request.Header.Set("Content-Type", "application/json")
	accessGroupTestRouter(handler.AddMember).ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAccessGroupRemoveUnprovisionedMemberIsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	members := accessGroupMembersFunc(func(_ context.Context, accountID, userID string) (*account.AccountMember, error) {
		return &account.AccountMember{AccountID: accountID, UserID: userID}, nil
	})
	handler := NewAccessGroupHandler(logger.New("error", "json"), &authz.FakeGroups{}, enabledAccountExperiment, members, nil, true)
	response := httptest.NewRecorder()
	accessGroupTestRouter(handler.RemoveMember).ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/groups", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAccessGroupRejectsMalformedRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAccessGroupHandler(logger.New("error", "json"), &authz.FakeGroups{}, enabledAccountExperiment, accessGroupMembersFunc(nil), nil, true)
	for _, test := range []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "group", handler: handler.Create},
		{name: "member", handler: handler.AddMember},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/groups", strings.NewReader(`{"invalid"`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			accessGroupTestRouter(test.handler).ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid request body") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

var enabledAccountExperiment accountExperimentGateFunc = func(context.Context, string) (bool, error) { return true, nil }

func accessGroupTestRouter(handler gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{
			ID: "acct_123", Type: "organization", WorkOSOrganizationID: "org_123",
		})
		c.Next()
	})
	router.Any("/groups", func(c *gin.Context) {
		c.Params = append(c.Params,
			gin.Param{Key: "group_id", Value: "group_123"},
			gin.Param{Key: "user_id", Value: "user_123"},
		)
		handler(c)
	})
	return router
}
