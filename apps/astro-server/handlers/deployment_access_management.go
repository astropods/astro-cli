package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

const deploymentAccessTimeout = 5 * time.Second

type deploymentAccessService interface {
	ListAccess(context.Context, authz.ResourceRef) ([]authz.AccessAssignment, []authz.AccessIntent, error)
	Assign(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string, authz.AccessLevel) (authz.AccessIntent, bool, error)
	Remove(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string) (authz.AccessIntent, bool, error)
}

type deploymentAccessQueue interface {
	InsertResourceAccessFGAReconcileJob(context.Context, authz.AccessIntentKey) error
}

type deploymentAccessAuditStore interface {
	LogAsync(*logger.Logger, auditlog.Event)
}

type DeploymentAccessRoleResponse struct {
	Role        string   `json:"role"`
	RoleSlug    string   `json:"role_slug"`
	Permissions []string `json:"permissions"`
}

type DeploymentAccessAssignmentResponse struct {
	ID                    string `json:"id"`
	SubjectType           string `json:"subject_type"`
	SubjectID             string `json:"subject_id"`
	Role                  string `json:"role,omitempty"`
	RoleSlug              string `json:"role_slug,omitempty"`
	DesiredRole           string `json:"desired_role,omitempty"`
	DesiredRoleSlug       string `json:"desired_role_slug,omitempty"`
	SyncStatus            string `json:"sync_status,omitempty"`
	SyncError             string `json:"sync_error,omitempty"`
	DesiredVersion        int64  `json:"desired_version,omitempty"`
	Source                string `json:"source"`
	GroupRoleAssignmentID string `json:"group_role_assignment_id,omitempty"`
}

type DeploymentAccessMutationResponse struct {
	DeploymentID   string `json:"deployment_id"`
	SubjectType    string `json:"subject_type"`
	SubjectID      string `json:"subject_id"`
	DesiredRole    string `json:"desired_role,omitempty"`
	DesiredVersion int64  `json:"desired_version"`
	Status         string `json:"status"`
}

type DeploymentAccessResponse struct {
	DeploymentID string                               `json:"deployment_id"`
	Roles        []DeploymentAccessRoleResponse       `json:"roles"`
	Assignments  []DeploymentAccessAssignmentResponse `json:"assignments"`
}

type SetDeploymentAccessRequest struct {
	SubjectType string `json:"subject_type" binding:"required"`
	SubjectID   string `json:"subject_id" binding:"required"`
	Role        string `json:"role" binding:"required"`
}

func ListDeploymentAccess(log *logger.Logger, service deploymentAccessService) gin.HandlerFunc {
	return func(c *gin.Context) {
		deploymentID := c.Param("id")
		ctx, cancel := deploymentAccessContext(c)
		defer cancel()
		assignments, intents, err := service.ListAccess(ctx, authz.DeploymentResource(deploymentID))
		if err != nil {
			writeDeploymentAccessError(c, log, deploymentID, err)
			return
		}
		c.JSON(http.StatusOK, DeploymentAccessResponse{
			DeploymentID: deploymentID,
			Roles:        deploymentAccessRoles(authz.ResourceRoles(authz.ResourceDeployment)),
			Assignments:  deploymentAccessAssignments(assignments, intents),
		})
	}
}

func SetDeploymentAccess(log *logger.Logger, service deploymentAccessService, queue deploymentAccessQueue, auditStore deploymentAccessAuditStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request SetDeploymentAccessRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subject_type, subject_id, and role are required"})
			return
		}
		subjectType, ok := deploymentAccessSubjectType(request.SubjectType)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subject_type must be member or group"})
			return
		}
		level := authz.AccessLevel(request.Role)
		if _, err := authz.RoleForAccessLevel(authz.ResourceDeployment, level); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "role must be viewer, builder, or admin"})
			return
		}

		deploymentID := c.Param("id")
		ctx, cancel := deploymentAccessContext(c)
		defer cancel()
		intent, changed, err := service.Assign(ctx, authz.DeploymentResource(deploymentID), subjectType, request.SubjectID, level)
		if err != nil {
			writeDeploymentAccessError(c, log, deploymentID, err)
			return
		}
		if changed {
			logDeploymentAccessAudit(c, log, auditStore, auditlog.DeploymentGrantAccess, intent, request.Role)
		}
		enqueueDeploymentAccessReconcile(c, log, queue, intent)
		writeDeploymentAccessMutation(c, intent, request.SubjectType, request.SubjectID, request.Role)
	}
}

func RemoveDeploymentAccess(log *logger.Logger, service deploymentAccessService, queue deploymentAccessQueue, auditStore deploymentAccessAuditStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		subjectType, ok := deploymentAccessSubjectType(c.Param("subject_type"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subject_type must be member or group"})
			return
		}
		deploymentID := c.Param("id")
		subjectID := c.Param("subject_id")
		ctx, cancel := deploymentAccessContext(c)
		defer cancel()
		intent, changed, err := service.Remove(ctx, authz.DeploymentResource(deploymentID), subjectType, subjectID)
		if err != nil {
			writeDeploymentAccessError(c, log, deploymentID, err)
			return
		}
		if changed {
			logDeploymentAccessAudit(c, log, auditStore, auditlog.DeploymentRevokeAccess, intent, "")
		}
		enqueueDeploymentAccessReconcile(c, log, queue, intent)
		writeDeploymentAccessMutation(c, intent, c.Param("subject_type"), subjectID, "")
	}
}

func deploymentAccessContext(c *gin.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), deploymentAccessTimeout)
	ctx = authz.WithRequestCache(ctx)
	ctx = authz.WithAuthorizationRoute(ctx, c.FullPath())
	return ctx, cancel
}

func deploymentAccessSubjectType(value string) (authz.AssignmentSubjectType, bool) {
	switch value {
	case "member":
		return authz.AssignmentSubjectMembership, true
	case "group":
		return authz.AssignmentSubjectGroup, true
	default:
		return "", false
	}
}

func deploymentAccessRoles(roles []authz.ResourceRole) []DeploymentAccessRoleResponse {
	response := make([]DeploymentAccessRoleResponse, 0, len(roles))
	for _, role := range roles {
		permissions := make([]string, 0, len(role.Actions))
		for _, action := range role.Actions {
			permissions = append(permissions, string(action))
		}
		response = append(response, DeploymentAccessRoleResponse{
			Role: string(role.Level), RoleSlug: string(role.Slug), Permissions: permissions,
		})
	}
	return response
}

func deploymentAccessAssignments(assignments []authz.AccessAssignment, intents []authz.AccessIntent) []DeploymentAccessAssignmentResponse {
	response := make([]DeploymentAccessAssignmentResponse, 0, len(assignments))
	directByUser := make(map[string]int)
	for _, assignment := range assignments {
		response = append(response, DeploymentAccessAssignmentResponse{
			ID: assignment.ID, SubjectType: "member", SubjectID: assignment.UserID,
			Role: string(assignment.Level), RoleSlug: string(assignment.Role),
			Source: string(assignment.Source), GroupRoleAssignmentID: assignment.GroupRoleAssignmentID,
		})
		if assignment.Source == authz.AssignmentSourceDirect {
			directByUser[assignment.UserID] = len(response) - 1
		}
	}
	for _, intent := range intents {
		if intent.Subject.Type != authz.AssignmentSubjectMembership {
			continue
		}
		status := intent.Status()
		index, exists := directByUser[intent.SubjectID]
		if !exists {
			if status == authz.AccessSyncSynced && intent.DesiredRole == "" {
				continue
			}
			response = append(response, DeploymentAccessAssignmentResponse{
				SubjectType: "member", SubjectID: intent.SubjectID, Source: string(authz.AssignmentSourceDirect),
			})
			index = len(response) - 1
		}
		current := &response[index]
		current.DesiredRoleSlug = string(intent.DesiredRole)
		if level, ok := authz.AccessLevelForRole(intent.Resource.Type, intent.DesiredRole); ok {
			current.DesiredRole = string(level)
		}
		current.SyncStatus = string(status)
		current.SyncError = intent.LastError
		current.DesiredVersion = intent.DesiredVersion
	}
	return response
}

func enqueueDeploymentAccessReconcile(c *gin.Context, log *logger.Logger, queue deploymentAccessQueue, intent authz.AccessIntent) {
	if intent.Status() == authz.AccessSyncSynced {
		return
	}
	if queue == nil {
		log.Warn("Deployment access reconciliation enqueue skipped; periodic sweep will retry",
			"deployment_id", intent.Resource.ExternalID, "subject_id", intent.SubjectID)
		return
	}
	if err := queue.InsertResourceAccessFGAReconcileJob(c.Request.Context(), intent.Key()); err != nil {
		log.Warn("Deployment access reconciliation enqueue failed; periodic sweep will retry",
			"deployment_id", intent.Resource.ExternalID, "subject_id", intent.SubjectID, "error", err)
	}
}

func writeDeploymentAccessMutation(c *gin.Context, intent authz.AccessIntent, subjectType, subjectID, role string) {
	c.JSON(http.StatusAccepted, DeploymentAccessMutationResponse{
		DeploymentID: intent.Resource.ExternalID, SubjectType: subjectType, SubjectID: subjectID,
		DesiredRole: role, DesiredVersion: intent.DesiredVersion, Status: string(intent.Status()),
	})
}

func logDeploymentAccessAudit(
	c *gin.Context,
	log *logger.Logger,
	store deploymentAccessAuditStore,
	action string,
	intent authz.AccessIntent,
	role string,
) {
	if store == nil {
		return
	}
	event := auditlog.FromGinContext(c, intent.AccountID)
	event.Action = action
	event.ResourceType = "deployment"
	event.ResourceID = intent.Resource.ExternalID
	metadata := map[string]any{
		"subject_type": deploymentAccessSubjectName(intent.Subject.Type), "subject_id": intent.SubjectID,
		"sync_status": intent.Status(), "desired_version": intent.DesiredVersion,
	}
	if role != "" {
		metadata["role"] = role
	}
	event.Metadata = metadata
	store.LogAsync(log, event)
}

func deploymentAccessSubjectName(subjectType authz.AssignmentSubjectType) string {
	if subjectType == authz.AssignmentSubjectMembership {
		return "member"
	}
	return string(subjectType)
}

func writeDeploymentAccessError(c *gin.Context, log *logger.Logger, deploymentID string, err error) {
	attrs := []any{"deployment_id", deploymentID, "error", err}
	switch {
	case errors.Is(err, authz.ErrResourceNotVisible), errors.Is(err, sql.ErrNoRows):
		log.Debug("Deployment access resource unavailable", attrs...)
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
	case errors.Is(err, authz.ErrAccessManagementUnavailable), errors.Is(err, authz.ErrFGAResourceNotEnabled):
		log.Debug("Deployment access management disabled", attrs...)
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment access management is unavailable"})
	case errors.Is(err, authz.ErrWorkOSMembershipUnavailable):
		log.Warn("Deployment access identity unavailable", attrs...)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorization session is unavailable; refresh or sign in again"})
	case errors.Is(err, authz.ErrAccessSubjectNotProvisioned):
		log.Debug("Deployment access target is not provisioned", attrs...)
		c.JSON(http.StatusConflict, gin.H{"error": "selected member is not yet provisioned for fine-grained access"})
	default:
		log.Warn("Deployment access operation failed", attrs...)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorization temporarily unavailable"})
	}
}
