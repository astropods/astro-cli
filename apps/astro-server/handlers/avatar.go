package handlers

import (
	"io"
	"net/http"
	"strconv"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// AvatarResponse is returned after avatar mutations.
type AvatarResponse struct {
	AvatarURL     string `json:"avatar_url"`
	AvatarVersion int    `json:"avatar_version"`
}

// UploadAvatar handles POST /api/v1/accounts/:account/avatar
func UploadAvatar(log *logger.Logger, accountStore *account.AccountStore, avatarStore *avatar.Store, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		file, _, err := c.Request.FormFile("avatar")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing avatar file"})
			return
		}
		defer func() { _ = file.Close() }()

		data, err := io.ReadAll(io.LimitReader(file, avatar.MaxUploadSize+1))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read upload"})
			return
		}

		if err := avatarStore.Upload(c.Request.Context(), acct.Name, data); err != nil {
			log.Error("Failed to upload avatar", "error", err, "account", acct.Name)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		version, err := accountStore.IncrementAvatarVersion(acct.ID)
		if err != nil {
			log.Error("Failed to increment avatar version", "error", err, "account", acct.Name)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update avatar version"})
			return
		}

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AvatarUpload
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Uploaded account avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:     avatarStore.AvatarURL(acct.Name, version),
			AvatarVersion: version,
		})
	}
}

// SetAvatarPreset handles PUT /api/v1/accounts/:account/avatar/preset/:index
func SetAvatarPreset(log *logger.Logger, accountStore *account.AccountStore, avatarStore *avatar.Store, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		index, err := strconv.Atoi(c.Param("index"))
		if err != nil || index < 1 || index > avatar.PresetCount {
			c.JSON(http.StatusBadRequest, gin.H{"error": "preset index must be 1-25"})
			return
		}

		if err := avatarStore.SetPreset(c.Request.Context(), acct.Name, index); err != nil {
			log.Error("Failed to set avatar preset", "error", err, "account", acct.Name)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set preset"})
			return
		}

		version, err := accountStore.IncrementAvatarVersion(acct.ID)
		if err != nil {
			log.Error("Failed to increment avatar version", "error", err, "account", acct.Name)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update avatar version"})
			return
		}

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AvatarUpload
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Set account avatar preset"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:     avatarStore.AvatarURL(acct.Name, version),
			AvatarVersion: version,
		})
	}
}

// ResetAvatar handles DELETE /api/v1/accounts/:account/avatar
func ResetAvatar(log *logger.Logger, accountStore *account.AccountStore, avatarStore *avatar.Store, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		if err := avatarStore.AssignPreset(c.Request.Context(), acct.Name); err != nil {
			log.Error("Failed to reset avatar", "error", err, "account", acct.Name)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset avatar"})
			return
		}

		version, err := accountStore.IncrementAvatarVersion(acct.ID)
		if err != nil {
			log.Error("Failed to increment avatar version", "error", err, "account", acct.Name)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update avatar version"})
			return
		}

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AvatarReset
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Reset account avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:     avatarStore.AvatarURL(acct.Name, version),
			AvatarVersion: version,
		})
	}
}

// readAvatarUpload reads the multipart "avatar" field from the request.
func readAvatarUpload(c *gin.Context) ([]byte, error) {
	file, _, err := c.Request.FormFile("avatar")
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(io.LimitReader(file, avatar.MaxUploadSize+1))
}

// UploadBlueprintAvatar handles POST /api/v1/agents/:account/:name/avatar
func UploadBlueprintAvatar(log *logger.Logger, agentIndex *agentindex.Index, avatarStore *avatar.Store, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		agentName := c.Param("name")

		data, err := readAvatarUpload(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing avatar file"})
			return
		}

		if err := avatarStore.UploadAgent(c.Request.Context(), acct.Name, agentName, data); err != nil {
			log.Error("Failed to upload blueprint avatar", "error", err, "account", acct.Name, "agent", agentName)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		version, err := agentIndex.IncrementAvatarVersion(acct.ID, agentName)
		if err != nil {
			log.Error("Failed to increment agent avatar version", "error", err, "account", acct.Name, "agent", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update avatar version"})
			return
		}

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AvatarUpload
		evt.ResourceType = "agent"
		evt.ResourceID = agentName
		evt.ResourceName = agentName
		evt.Description = "Uploaded agent avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:     avatarStore.AgentAvatarURL(acct.Name, agentName, version),
			AvatarVersion: version,
		})
	}
}

// ResetBlueprintAvatar handles DELETE /api/v1/agents/:account/:name/avatar
func ResetBlueprintAvatar(log *logger.Logger, agentIndex *agentindex.Index, avatarStore *avatar.Store, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		agentName := c.Param("name")

		if err := avatarStore.DeleteAgent(c.Request.Context(), acct.Name, agentName); err != nil {
			log.Error("Failed to reset blueprint avatar", "error", err, "account", acct.Name, "agent", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset avatar"})
			return
		}

		if err := agentIndex.ResetAvatarVersion(acct.ID, agentName); err != nil {
			log.Error("Failed to reset agent avatar version", "error", err, "account", acct.Name, "agent", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update avatar version"})
			return
		}

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AvatarReset
		evt.ResourceType = "agent"
		evt.ResourceID = agentName
		evt.ResourceName = agentName
		evt.Description = "Reset agent avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:     "",
			AvatarVersion: 0,
		})
	}
}

// UploadDeploymentAvatar handles POST /api/v1/deployments/:id/avatar
func UploadDeploymentAvatar(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, avatarStore *avatar.Store, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		data, err := readAvatarUpload(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing avatar file"})
			return
		}

		if err := avatarStore.UploadDeployment(c.Request.Context(), dep.ID, data); err != nil {
			log.Error("Failed to upload deployment avatar", "error", err, "deployment", dep.ID)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		version, err := deployStore.IncrementDeploymentAvatarVersion(dep.ID)
		if err != nil {
			log.Error("Failed to increment deployment avatar version", "error", err, "deployment", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update avatar version"})
			return
		}

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.AvatarUpload
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Uploaded deployment avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:     avatarStore.DeploymentAvatarURL(dep.ID, version),
			AvatarVersion: version,
		})
	}
}

// ResetDeploymentAvatar handles DELETE /api/v1/deployments/:id/avatar
func ResetDeploymentAvatar(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, avatarStore *avatar.Store, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		if err := avatarStore.DeleteDeployment(c.Request.Context(), dep.ID); err != nil {
			log.Error("Failed to reset deployment avatar", "error", err, "deployment", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset avatar"})
			return
		}

		if err := deployStore.ResetDeploymentAvatarVersion(dep.ID); err != nil {
			log.Error("Failed to reset deployment avatar version", "error", err, "deployment", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update avatar version"})
			return
		}

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.AvatarReset
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Reset deployment avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:     "",
			AvatarVersion: 0,
		})
	}
}
