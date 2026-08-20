package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const (
	accessGroupTimeout    = 5 * time.Second
	defaultGroupPageLimit = 50
)

type accessGroupMemberStore interface {
	GetMemberContext(context.Context, string, string) (*account.AccountMember, error)
}

type accessGroupAuditStore interface {
	LogAsync(*logger.Logger, auditlog.Event)
}

type AccessGroupHandler struct {
	log         *logger.Logger
	groups      authz.Groups
	experiments authz.AccountExperimentGate
	members     accessGroupMemberStore
	audit       accessGroupAuditStore
	active      bool
}

func NewAccessGroupHandler(
	log *logger.Logger,
	groups authz.Groups,
	experiments authz.AccountExperimentGate,
	members accessGroupMemberStore,
	auditStore accessGroupAuditStore,
	active bool,
) *AccessGroupHandler {
	return &AccessGroupHandler{log: log, groups: groups, experiments: experiments, members: members, audit: auditStore, active: active}
}

type AccessGroupResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type AccessGroupPageResponse struct {
	Groups     []AccessGroupResponse `json:"groups"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

type AccessGroupMemberResponse struct {
	UserID string `json:"user_id"`
}

type AccessGroupMemberPageResponse struct {
	Members    []AccessGroupMemberResponse `json:"members"`
	NextCursor string                      `json:"next_cursor,omitempty"`
}

type AccessGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type AddAccessGroupMemberRequest struct {
	UserID string `json:"user_id"`
}

func (h *AccessGroupHandler) List(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()
	page, ok := accessGroupPage(c)
	if !ok {
		return
	}
	result, err := h.groups.ListGroups(ctx, acct.WorkOSOrganizationID, page)
	if err != nil {
		h.writeError(c, err)
		return
	}
	groups := make([]AccessGroupResponse, 0, len(result.Groups))
	for _, group := range result.Groups {
		groups = append(groups, accessGroupResponse(group))
	}
	c.JSON(http.StatusOK, AccessGroupPageResponse{Groups: groups, NextCursor: result.NextCursor})
}

func (h *AccessGroupHandler) Create(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()
	request, ok := accessGroupRequest(c)
	if !ok {
		return
	}
	group, err := h.groups.CreateGroup(ctx, acct.WorkOSOrganizationID, request.Name, request.Description)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.logAudit(c, auditlog.AccessGroupCreate, acct.ID, group.ID, nil)
	c.JSON(http.StatusCreated, accessGroupResponse(group))
}

func (h *AccessGroupHandler) Update(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()
	request, ok := accessGroupRequest(c)
	if !ok {
		return
	}
	group, err := h.groups.UpdateGroup(ctx, acct.WorkOSOrganizationID, c.Param("group_id"), request.Name, request.Description)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.logAudit(c, auditlog.AccessGroupUpdate, acct.ID, group.ID, nil)
	c.JSON(http.StatusOK, accessGroupResponse(group))
}

func (h *AccessGroupHandler) Delete(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()
	groupID := c.Param("group_id")
	err := h.groups.DeleteGroup(ctx, acct.WorkOSOrganizationID, groupID)
	if errors.Is(err, authz.ErrGroupNotFound) {
		c.Status(http.StatusNoContent)
		return
	}
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.logAudit(c, auditlog.AccessGroupDelete, acct.ID, groupID, nil)
	c.Status(http.StatusNoContent)
}

func (h *AccessGroupHandler) ListMembers(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()
	page, ok := accessGroupPage(c)
	if !ok {
		return
	}
	result, err := h.groups.ListGroupMembers(ctx, acct.WorkOSOrganizationID, c.Param("group_id"), page)
	if err != nil {
		h.writeError(c, err)
		return
	}
	members := make([]AccessGroupMemberResponse, 0, len(result.Members))
	for _, member := range result.Members {
		members = append(members, AccessGroupMemberResponse{UserID: member.UserID})
	}
	c.JSON(http.StatusOK, AccessGroupMemberPageResponse{Members: members, NextCursor: result.NextCursor})
}

func (h *AccessGroupHandler) AddMember(c *gin.Context) {
	h.mutateMember(c, true)
}

func (h *AccessGroupHandler) RemoveMember(c *gin.Context) {
	h.mutateMember(c, false)
}

func (h *AccessGroupHandler) mutateMember(c *gin.Context, add bool) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()
	if h.members == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "organization member lookup is unavailable"})
		return
	}
	userID := c.Param("user_id")
	if add {
		var request AddAccessGroupMemberRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		userID = strings.TrimSpace(request.UserID)
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
			return
		}
	}
	member, err := h.members.GetMemberContext(ctx, acct.ID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "organization member not found"})
		return
	}
	if err != nil {
		h.log.Warn("Resolve access-group member", "account_id", acct.ID, "user_id", userID, "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorization temporarily unavailable"})
		return
	}
	if member == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "organization member not found"})
		return
	}
	if member.WorkOSMembershipID == "" {
		if !add {
			c.Status(http.StatusNoContent)
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "selected member is not yet provisioned for fine-grained access"})
		return
	}

	if add {
		err = h.groups.AddGroupMember(ctx, acct.WorkOSOrganizationID, c.Param("group_id"), member.WorkOSMembershipID)
	} else {
		err = h.groups.RemoveGroupMember(ctx, acct.WorkOSOrganizationID, c.Param("group_id"), member.WorkOSMembershipID)
	}
	if errors.Is(err, authz.ErrGroupMemberExists) || errors.Is(err, authz.ErrGroupMemberNotFound) {
		c.Status(http.StatusNoContent)
		return
	}
	if err != nil {
		h.writeError(c, err)
		return
	}
	action := auditlog.AccessGroupAddMember
	if !add {
		action = auditlog.AccessGroupRemoveMember
	}
	h.logAudit(c, action, acct.ID, c.Param("group_id"), map[string]any{"user_id": userID})
	c.Status(http.StatusNoContent)
}

func (h *AccessGroupHandler) scope(c *gin.Context) (*account.Account, context.Context, context.CancelFunc, bool) {
	if !h.active || h.groups == nil || h.experiments == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "access groups are unavailable"})
		return nil, nil, nil, false
	}
	acct, ok := middleware.GetAccountFromContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
		return nil, nil, nil, false
	}
	if acct.Type != "organization" || acct.WorkOSOrganizationID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "access groups are unavailable"})
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), accessGroupTimeout)
	enabled, err := h.experiments.Enabled(ctx, acct.ID)
	if err != nil {
		cancel()
		h.log.Warn("Resolve access-group experiment", "account_id", acct.ID, "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorization temporarily unavailable"})
		return nil, nil, nil, false
	}
	if !enabled {
		cancel()
		c.JSON(http.StatusNotFound, gin.H{"error": "access groups are unavailable"})
		return nil, nil, nil, false
	}
	return acct, ctx, cancel, true
}

func accessGroupPage(c *gin.Context) (authz.PageRequest, bool) {
	limit := defaultGroupPageLimit
	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
			return authz.PageRequest{}, false
		}
		limit = parsed
	}
	return authz.PageRequest{After: c.Query("cursor"), Limit: limit}, true
}

func accessGroupRequest(c *gin.Context) (AccessGroupRequest, bool) {
	var request AccessGroupRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return AccessGroupRequest{}, false
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	if request.Name == "" || len(request.Name) > 100 || len(request.Description) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must be 1-100 characters and description at most 500 characters"})
		return AccessGroupRequest{}, false
	}
	return request, true
}

func accessGroupResponse(group authz.Group) AccessGroupResponse {
	return AccessGroupResponse{ID: group.ID, Name: group.Name, Description: group.Description}
}

func (h *AccessGroupHandler) logAudit(c *gin.Context, action, accountID, groupID string, metadata map[string]any) {
	if h.audit == nil {
		return
	}
	event := auditlog.FromGinContext(c, accountID)
	event.Action = action
	event.ResourceType = "access_group"
	event.ResourceID = groupID
	event.Metadata = metadata
	h.audit.LogAsync(h.log, event)
}

func (h *AccessGroupHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, authz.ErrInvalidPageCursor):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page cursor"})
	case errors.Is(err, authz.ErrGroupNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "access group not found"})
	case errors.Is(err, authz.ErrGroupExists):
		c.JSON(http.StatusConflict, gin.H{"error": "access group already exists"})
	default:
		h.log.Warn("Access group operation failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorization temporarily unavailable"})
	}
}
