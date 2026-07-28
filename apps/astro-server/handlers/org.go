package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
	"github.com/gin-gonic/gin"
)

// memberRoleSyncer is the subset of *org.Sync used by role-change and removal
// handlers. Defined as an interface so tests can inject fakes to cover
// permission paths that would otherwise require mocking WorkOS HTTP calls.
type memberRoleSyncer interface {
	ChangeMemberRole(ctx context.Context, accountID, userID, newRole, callerRole string) (string, error)
	RemoveMember(ctx context.Context, accountID, userID, callerRole string) error
}

// callerOrgRole returns the caller's role slug within the resolved org, or
// empty string if there's no org-scoped session. Used to enforce role
// hierarchy when managing existing members.
func callerOrgRole(c *gin.Context) string {
	session, ok := middleware.GetSession(c)
	if !ok {
		return ""
	}
	return session.Role
}

// validRoles is the set of role slugs accepted by member management endpoints.
var validRoles = map[string]bool{
	"owner":  true,
	"admin":  true,
	"member": true,
}

// isValidRole returns true if the role slug is one of the allowed values.
func isValidRole(role string) bool {
	return validRoles[role]
}

// requireOwnerForOwnerRole guards against non-owners assigning the owner role.
// For org accounts, checks session.Role from JWT. For personal accounts (no WorkOS org),
// any member is owner so always allows.
// Returns true if the request should continue, false if it was aborted with 403.
func requireOwnerForOwnerRole(c *gin.Context, requestedRole string) bool {
	if requestedRole != "owner" {
		return true
	}
	session, ok := middleware.GetSession(c)
	if !ok || session.Role != "owner" {
		c.JSON(http.StatusForbidden, gin.H{"error": "only owners can assign the owner role"})
		return false
	}
	return true
}

// --- Member Management ---

// AddMemberRequest represents the request to add a member to an account.
type AddMemberRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required"`
}

// ChangeMemberRoleRequest represents the request to change a member's role.
type ChangeMemberRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// MemberResponse is a member with role and profile information included.
// SlackWorkspaces lists every Slack workspace the member has linked via the
// Slack identity mapping; empty means they haven't connected Slack and
// callers (e.g. the grants UI) can warn that a Slack grant for this user
// won't resolve.
type MemberResponse struct {
	AccountID       string              `json:"account_id"`
	UserID          string              `json:"user_id"`
	Role            string              `json:"role"`
	Status          string              `json:"status"`
	Username        string              `json:"username"`
	DisplayName     string              `json:"display_name"`
	AvatarURL       string              `json:"avatar_url,omitempty"`
	CreatedAt       string              `json:"created_at"`
	SlackWorkspaces []SlackWorkspaceRef `json:"slack_workspaces"`
}

// SlackWorkspaceRef is a compact Slack workspace identifier emitted on
// member listings so the grants UI can render "linked to: …" badges
// without a second round-trip.
type SlackWorkspaceRef struct {
	TeamID     string `json:"team_id"`
	TeamName   string `json:"team_name"`
	TeamDomain string `json:"team_domain"`
	IconURL    string `json:"icon_url"`
}

func toSlackWorkspaceRefs(ms []slackidentity.Mapping) []SlackWorkspaceRef {
	if len(ms) == 0 {
		return []SlackWorkspaceRef{}
	}
	out := make([]SlackWorkspaceRef, 0, len(ms))
	for _, m := range ms {
		out = append(out, SlackWorkspaceRef{
			TeamID:     m.TeamID,
			TeamName:   m.TeamName,
			TeamDomain: m.TeamDomain,
			IconURL:    m.TeamIconURL,
		})
	}
	return out
}

// ListMembers handles GET /api/v1/accounts/:account/members
func ListMembers(log *logger.Logger, accountStore *account.AccountStore, avatarStore *avatar.Store, orgClient *org.Client, slackStore *slackidentity.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		user, ok := middleware.GetUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		isMember, err := accountStore.IsMember(acct.ID, user.ID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "you are not a member of this account"})
			return
		}

		members, err := accountStore.GetMembersForAccount(acct.ID)
		if err != nil {
			log.Error("Failed to list members", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list members"})
			return
		}

		// Build role + status lookups from WorkOS memberships for org accounts
		type memberInfo struct {
			Role      string
			Status    string
			CreatedAt string
		}
		infoByUserID := map[string]memberInfo{}
		if acct.WorkOSOrganizationID != "" && orgClient != nil {
			memberships, err := orgClient.ListMemberships(c.Request.Context(), acct.WorkOSOrganizationID, org.ListOpts{Limit: 100})
			if err != nil {
				log.Error("Failed to fetch WorkOS memberships", "error", err, "account_id", acct.ID)
				c.JSON(http.StatusBadGateway, gin.H{
					"error": "failed to fetch member roles from identity provider",
				})
				return
			}
			for _, m := range memberships {
				infoByUserID[m.UserID] = memberInfo{Role: m.RoleSlug, Status: m.Status, CreatedAt: m.CreatedAt}
			}
		}

		// Batch-fetch personal-account profiles for all members in one query
		memberUserIDs := make([]string, len(members))
		for i, m := range members {
			memberUserIDs[i] = m.UserID
		}
		profileByUserID, err := accountStore.GetPersonalProfiles(memberUserIDs)
		if err != nil {
			log.Error("Failed to fetch member profiles", "error", err, "account_id", acct.ID)
			profileByUserID = map[string]account.PersonalProfile{}
		}

		// Slack mappings keyed by member user_id. Best-effort: on lookup
		// failure we fall through with no workspaces — the warning that
		// would have surfaced is a softer signal than blocking the list.
		var slackByUserID map[string][]slackidentity.Mapping
		if slackStore != nil {
			slackByUserID, err = slackStore.ListByWorkOSUsers(memberUserIDs)
			if err != nil {
				log.Error("Failed to fetch slack identities", "error", err, "account_id", acct.ID)
				slackByUserID = map[string][]slackidentity.Mapping{}
			}
		}

		result := make([]MemberResponse, 0, len(members))
		localUserIDs := map[string]bool{}
		for _, m := range members {
			localUserIDs[m.UserID] = true
			info := infoByUserID[m.UserID]
			role := info.Role
			if role == "" {
				role = "member"
			}
			status := info.Status
			if status == "" {
				status = "active"
			}
			p := profileByUserID[m.UserID]
			mr := MemberResponse{
				AccountID:       m.AccountID,
				UserID:          m.UserID,
				Role:            role,
				Status:          status,
				Username:        p.Name,
				DisplayName:     p.DisplayName,
				CreatedAt:       m.CreatedAt.Format("2006-01-02T15:04:05Z"),
				SlackWorkspaces: toSlackWorkspaceRefs(slackByUserID[m.UserID]),
			}
			if avatarStore != nil && p.Name != "" {
				mr.AvatarURL = avatarStore.AvatarURL(p.Name, p.AvatarUpdatedAt)
			}
			result = append(result, mr)
		}

		// When include_pending is set, append WorkOS memberships that have no
		// local DB entry yet (e.g. freshly invited users before the event
		// consumer syncs them).
		if c.Query("include_pending") == "true" {
			var pendingUIDs []string
			for uid, info := range infoByUserID {
				if !localUserIDs[uid] && info.Status == "pending" {
					pendingUIDs = append(pendingUIDs, uid)
				}
			}
			pendingProfiles, err := accountStore.GetPersonalProfiles(pendingUIDs)
			if err != nil {
				log.Error("Failed to fetch pending member profiles", "error", err, "account_id", acct.ID)
				pendingProfiles = map[string]account.PersonalProfile{}
			}
			for _, uid := range pendingUIDs {
				info := infoByUserID[uid]
				p := pendingProfiles[uid]
				mr := MemberResponse{
					AccountID:       acct.ID,
					UserID:          uid,
					Role:            info.Role,
					Status:          info.Status,
					Username:        p.Name,
					DisplayName:     p.DisplayName,
					CreatedAt:       info.CreatedAt,
					SlackWorkspaces: []SlackWorkspaceRef{},
				}
				if avatarStore != nil && p.Name != "" {
					mr.AvatarURL = avatarStore.AvatarURL(p.Name, p.AvatarUpdatedAt)
				}
				result = append(result, mr)
			}
		}

		c.JSON(http.StatusOK, gin.H{"members": result})
	}
}

// AddMember handles POST /api/v1/accounts/:account/members
func AddMember(log *logger.Logger, syncSvc *org.Sync, accountStore *account.AccountStore, db *sql.DB, auditStore *auditlog.Store, queue notifyQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		var req AddMemberRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		if !isValidRole(req.Role) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role: must be owner, admin, or member"})
			return
		}

		if !requireOwnerForOwnerRole(c, req.Role) {
			return
		}

		member, err := syncSvc.AddMember(c.Request.Context(), acct.ID, req.UserID, req.Role)
		if err != nil {
			log.Error("Failed to add member", "error", err, "account_id", acct.ID, "user_id", req.UserID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "failed to add member",
			})
			return
		}

		log.Info("Member added", "account_id", acct.ID, "user_id", req.UserID, "role", req.Role)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.MemberAdd
		evt.ResourceType = "member"
		evt.ResourceID = req.UserID
		evt.Description = "Added member with role " + req.Role
		evt.Metadata = map[string]any{"role": req.Role}
		auditStore.LogAsync(log, evt)

		emitNotify(c, log, queue, notify.TeamMemberAdded(acct.ID, acct.Name, req.UserID, req.Role))

		c.JSON(http.StatusCreated, gin.H{"member": member})
	}
}

// UpdateMemberRole handles PUT /api/v1/accounts/:account/members/:user_id
func UpdateMemberRole(log *logger.Logger, syncSvc memberRoleSyncer, accountStore *account.AccountStore, auditStore *auditlog.Store, queue notifyQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		userID := c.Param("user_id")

		var req ChangeMemberRoleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		if !isValidRole(req.Role) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role: must be owner, admin, or member"})
			return
		}

		if !requireOwnerForOwnerRole(c, req.Role) {
			return
		}

		previousRole, err := syncSvc.ChangeMemberRole(c.Request.Context(), acct.ID, userID, req.Role, callerOrgRole(c))
		if err != nil {
			if errors.Is(err, org.ErrOwnerManagementForbidden) {
				c.JSON(http.StatusForbidden, gin.H{"error": "only owners can modify or remove other owners"})
				return
			}
			log.Error("Failed to update member role", "error", err, "account_id", acct.ID, "user_id", userID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "failed to update member role",
			})
			return
		}

		log.Info("Member role updated", "account_id", acct.ID, "user_id", userID, "old_role", previousRole, "new_role", req.Role)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.MemberUpdateRole
		evt.ResourceType = "member"
		evt.ResourceID = userID
		evt.Description = "Updated member role from " + previousRole + " to " + req.Role
		evt.Metadata = map[string]any{"old_role": previousRole, "new_role": req.Role}
		auditStore.LogAsync(log, evt)

		emitNotify(c, log, queue, notify.TeamRoleChanged(acct.ID, acct.Name, userID, req.Role))

		c.JSON(http.StatusOK, gin.H{"message": "role updated"})
	}
}

// RemoveMember handles DELETE /api/v1/accounts/:account/members/:user_id
// Self-removal (leaving) is allowed for any member; removing others requires
// org:manage. Additionally, only owners can remove other owners.
func RemoveMember(log *logger.Logger, syncSvc memberRoleSyncer, accountStore *account.AccountStore, db *sql.DB, auditStore *auditlog.Store, queue notifyQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		user, ok := middleware.GetUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		userID := c.Param("user_id")

		// Self-removal only requires membership; removing others requires org:manage.
		if userID != user.ID {
			if !middleware.HasAccountPermission(c, accountStore, acct, user, "org:manage") {
				c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions to remove other members"})
				return
			}
		}

		if err := syncSvc.RemoveMember(c.Request.Context(), acct.ID, userID, callerOrgRole(c)); err != nil {
			if errors.Is(err, org.ErrOwnerManagementForbidden) {
				c.JSON(http.StatusForbidden, gin.H{"error": "only owners can modify or remove other owners"})
				return
			}
			log.Error("Failed to remove member", "error", err, "account_id", acct.ID, "user_id", userID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "failed to remove member",
			})
			return
		}

		log.Info("Member removed", "account_id", acct.ID, "user_id", userID)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.MemberRemove
		evt.ResourceType = "member"
		evt.ResourceID = userID
		evt.Description = "Removed member"
		auditStore.LogAsync(log, evt)

		emitNotify(c, log, queue, notify.TeamMemberRemoved(acct.ID, acct.Name, userID))

		c.JSON(http.StatusOK, gin.H{"message": "member removed"})
	}
}

// --- Invitation Management ---

// BulkInvitationRequest is the request body for bulk invitation creation.
type BulkInvitationRequest struct {
	Invitations []InvitationEntry `json:"invitations"`
}

// InvitationEntry represents a single invitation in a bulk request.
type InvitationEntry struct {
	Value string `json:"value"`
	Kind  string `json:"kind"`
	Role  string `json:"role"`
}

// CreateInvitations handles POST /api/v1/accounts/:account/invitations
// Expects {"invitations":[{value, kind, role}, ...]}.
func CreateInvitations(log *logger.Logger, orgSync *org.Sync, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		if acct.WorkOSOrganizationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invitations are only supported for organization accounts"})
			return
		}

		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		var req BulkInvitationRequest
		if err := c.ShouldBindJSON(&req); err != nil || len(req.Invitations) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: expected {invitations:[...]}"})
			return
		}

		// Validate and check escalation for each entry's role
		for _, e := range req.Invitations {
			if e.Role != "" && !isValidRole(e.Role) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role: must be owner, admin, or member"})
				return
			}
			if !requireOwnerForOwnerRole(c, e.Role) {
				return
			}
		}

		reqs := make([]org.InviteRequest, 0, len(req.Invitations))
		for _, e := range req.Invitations {
			role := e.Role
			if role == "" {
				role = "member"
			}
			reqs = append(reqs, org.InviteRequest{
				Kind:     e.Kind,
				Value:    e.Value,
				RoleSlug: role,
			})
		}

		results := orgSync.SendBulkInvitations(c.Request.Context(), acct.WorkOSOrganizationID, user.ID, reqs)

		for _, r := range results {
			if r.Success {
				log.Info("Invitation sent", "account_id", acct.ID, "value", r.Value)
			} else {
				log.Warn("Invitation failed", "account_id", acct.ID, "value", r.Value, "error", r.Error)
			}
		}

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.InvitationCreate
		evt.ResourceType = "invitation"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Sent invitations"
		evt.Metadata = map[string]any{"count": len(req.Invitations)}
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusCreated, gin.H{"results": results})
	}
}

// ListAccountInvitations handles GET /api/v1/accounts/:account/invitations
func ListAccountInvitations(log *logger.Logger, orgClient *org.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		if acct.WorkOSOrganizationID == "" {
			c.JSON(http.StatusOK, gin.H{"invitations": []any{}})
			return
		}

		invitations, err := orgClient.ListInvitations(c.Request.Context(), acct.WorkOSOrganizationID)
		if err != nil {
			log.Error("Failed to list invitations", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list invitations"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"invitations": invitations})
	}
}

// RevokeInvitation handles DELETE /api/v1/accounts/:account/invitations/:id
func RevokeInvitation(log *logger.Logger, orgClient *org.Client, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		invitationID := c.Param("id")

		// Verify the invitation belongs to this account's WorkOS org to prevent IDOR
		inv, err := orgClient.GetInvitation(c.Request.Context(), invitationID)
		if err != nil {
			log.Error("Failed to get invitation", "error", err, "invitation_id", invitationID)
			c.JSON(http.StatusNotFound, gin.H{"error": "invitation not found"})
			return
		}
		if inv.OrganizationID != acct.WorkOSOrganizationID {
			c.JSON(http.StatusNotFound, gin.H{"error": "invitation not found"})
			return
		}

		if err := orgClient.RevokeInvitation(c.Request.Context(), invitationID); err != nil {
			log.Error("Failed to revoke invitation", "error", err, "invitation_id", invitationID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "failed to revoke invitation",
			})
			return
		}

		log.Info("Invitation revoked", "invitation_id", invitationID, "account_id", acct.ID)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.InvitationRevoke
		evt.ResourceType = "invitation"
		evt.ResourceID = invitationID
		evt.Description = "Revoked invitation"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, gin.H{"message": "invitation revoked"})
	}
}
