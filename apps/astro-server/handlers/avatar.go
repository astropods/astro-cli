package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/colorextract"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// touchAccountAvatar stamps the account's avatar_updated_at and returns the
// DB-persisted timestamp to embed in the avatar URL's cache-busting token. The
// database clock is used (not the app clock) so the immediate response token
// matches what later reads compute from the column — otherwise the same image
// is cached under two `?v=` keys. On a stamp failure it returns nil: the
// response URL is then unversioned, consistent with the still-NULL column, and
// the uploader still sees their new image via the local blob override. A
// persistent stamp failure therefore leaves the avatar unversioned — served
// from the edge cache for up to its max-age for every viewer but the uploader —
// until a later write succeeds.
func touchAccountAvatar(log *logger.Logger, accountStore *account.AccountStore, accountID, accountName string) *time.Time {
	ts, err := accountStore.TouchAvatarUpdatedAt(accountID)
	if err != nil {
		log.Warn("avatar: stamp account avatar_updated_at failed", "error", err, "account", accountName)
		return nil
	}
	return &ts
}

// touchAgentAvatar stamps the agent's avatar_updated_at and returns the
// DB-persisted token timestamp (nil on failure). See touchAccountAvatar for the
// clock and failure semantics.
func touchAgentAvatar(log *logger.Logger, index *agentindex.Index, accountID, accountName, agentName string) *time.Time {
	ts, err := index.TouchAvatarUpdatedAt(accountID, agentName)
	if err != nil {
		log.Warn("avatar: stamp agent avatar_updated_at failed", "error", err, "account", accountName, "agent", agentName)
		return nil
	}
	return &ts
}

// touchDeploymentAvatar stamps the deployment's avatar_updated_at and returns
// the DB-persisted token timestamp (nil on failure). See touchAccountAvatar for
// the clock and failure semantics.
func touchDeploymentAvatar(log *logger.Logger, deployStore *deploymentstore.Store, deploymentID string) *time.Time {
	ts, err := deployStore.TouchAvatarUpdatedAt(deploymentID)
	if err != nil {
		log.Warn("avatar: stamp deployment avatar_updated_at failed", "error", err, "deployment", deploymentID)
		return nil
	}
	return &ts
}

// extractAndStoreColors reads an avatar via readFn, extracts its color palette,
// persists the result via storeFn, and returns the raw JSON for inclusion in
// API responses. Failures are logged but not propagated; returns nil on error.
func extractAndStoreColors(ctx context.Context, log *logger.Logger,
	readFn func(context.Context) ([]byte, error),
	storeFn func([]byte) error,
	logAttrs ...any,
) json.RawMessage {
	data, err := readFn(ctx)
	if err != nil {
		log.Warn("avatar: read avatar for color extraction failed", append([]any{"error", err}, logAttrs...)...)
		return nil
	}
	colors, err := colorextract.ExtractFromJPEG(data)
	if err != nil {
		log.Warn("avatar: extract avatar colors failed", append([]any{"error", err}, logAttrs...)...)
		return nil
	}
	colorsJSON, err := json.Marshal(colors)
	if err != nil {
		log.Warn("avatar: marshal avatar colors failed", append([]any{"error", err}, logAttrs...)...)
		return nil
	}
	if err := storeFn(colorsJSON); err != nil {
		log.Warn("avatar: store avatar colors failed", append([]any{"error", err}, logAttrs...)...)
	}
	return colorsJSON
}

// AvatarResponse is returned after avatar mutations.
type AvatarResponse struct {
	AvatarURL    string          `json:"avatar_url"`
	AvatarColors json.RawMessage `json:"avatar_colors,omitempty"`
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

		if avatar.IsSVGContent(data) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "SVG uploads are not supported"})
			return
		}

		if err := avatarStore.Upload(c.Request.Context(), acct.Name, data); err != nil {
			log.Error("avatar: upload avatar failed", "error", err, "account", acct.Name)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		avatarTS := touchAccountAvatar(log, accountStore, acct.ID, acct.Name)

		colorsJSON := extractAndStoreColors(c.Request.Context(), log,
			func(ctx context.Context) ([]byte, error) { return avatarStore.ReadAvatar(ctx, acct.Name) },
			func(j []byte) error { return accountStore.SetAvatarColors(acct.ID, j) },
			"account", acct.Name,
		)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AvatarUpload
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Uploaded account avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:    avatarStore.AvatarURL(acct.Name, avatarTS),
			AvatarColors: colorsJSON,
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
			log.Error("avatar: set avatar preset failed", "error", err, "account", acct.Name)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set preset"})
			return
		}
		avatarTS := touchAccountAvatar(log, accountStore, acct.ID, acct.Name)

		colorsJSON := extractAndStoreColors(c.Request.Context(), log,
			func(ctx context.Context) ([]byte, error) { return avatarStore.ReadAvatar(ctx, acct.Name) },
			func(j []byte) error { return accountStore.SetAvatarColors(acct.ID, j) },
			"account", acct.Name,
		)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AvatarPreset
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Set account avatar preset"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:    avatarStore.AvatarURL(acct.Name, avatarTS),
			AvatarColors: colorsJSON,
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
			log.Error("avatar: reset avatar failed", "error", err, "account", acct.Name)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset avatar"})
			return
		}
		avatarTS := touchAccountAvatar(log, accountStore, acct.ID, acct.Name)

		colorsJSON := extractAndStoreColors(c.Request.Context(), log,
			func(ctx context.Context) ([]byte, error) { return avatarStore.ReadAvatar(ctx, acct.Name) },
			func(j []byte) error { return accountStore.SetAvatarColors(acct.ID, j) },
			"account", acct.Name,
		)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AvatarReset
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Reset account avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:    avatarStore.AvatarURL(acct.Name, avatarTS),
			AvatarColors: colorsJSON,
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
func UploadBlueprintAvatar(log *logger.Logger, avatarStore *avatar.Store, index *agentindex.Index, auditStore *auditlog.Store) gin.HandlerFunc {
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

		if avatar.IsSVGContent(data) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "SVG uploads are not supported"})
			return
		}

		if err := avatarStore.UploadAgent(c.Request.Context(), acct.Name, agentName, data); err != nil {
			log.Error("avatar: upload blueprint avatar failed", "error", err, "account", acct.Name, "agent", agentName)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		avatarTS := touchAgentAvatar(log, index, acct.ID, acct.Name, agentName)

		colorsJSON := extractAndStoreColors(c.Request.Context(), log,
			func(ctx context.Context) ([]byte, error) {
				return avatarStore.ReadAgentAvatar(ctx, acct.Name, agentName)
			},
			func(j []byte) error { return index.SetAvatarColors(acct.ID, agentName, j) },
			"account", acct.Name, "agent", agentName,
		)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AvatarUpload
		evt.ResourceType = "agent"
		evt.ResourceID = agentName
		evt.ResourceName = agentName
		evt.Description = "Uploaded agent avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:    avatarStore.AgentAvatarURL(acct.Name, agentName, avatarTS),
			AvatarColors: colorsJSON,
		})
	}
}

// ResetBlueprintAvatar handles DELETE /api/v1/agents/:account/:name/avatar
func ResetBlueprintAvatar(log *logger.Logger, avatarStore *avatar.Store, index *agentindex.Index, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		agentName := c.Param("name")

		if err := avatarStore.DeleteAgent(c.Request.Context(), acct.Name, agentName); err != nil {
			log.Error("avatar: reset blueprint avatar failed", "error", err, "account", acct.Name, "agent", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset avatar"})
			return
		}
		_ = touchAgentAvatar(log, index, acct.ID, acct.Name, agentName)

		// Clear colors — the backfill worker will regenerate the placeholder and re-extract.
		_ = index.SetAvatarColors(acct.ID, agentName, nil)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AvatarReset
		evt.ResourceType = "agent"
		evt.ResourceID = agentName
		evt.ResourceName = agentName
		evt.Description = "Reset agent avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL: "",
		})
	}
}

// UploadDeploymentAvatar handles POST /api/v1/deployments/:id/avatar
func UploadDeploymentAvatar(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, avatarStore *avatar.Store, auditStore *auditlog.Store, cache k8scache.Cache) gin.HandlerFunc {
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
			log.Error("avatar: upload deployment avatar failed", "error", err, "deployment", dep.ID)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		avatarTS := touchDeploymentAvatar(log, deployStore, dep.ID)

		colorsJSON := extractAndStoreColors(c.Request.Context(), log,
			func(ctx context.Context) ([]byte, error) { return avatarStore.ReadDeploymentAvatar(ctx, dep.ID) },
			func(j []byte) error { return deployStore.SetAvatarColors(dep.ID, j) },
			"deployment", dep.ID,
		)
		_ = deploycache.Invalidate(c.Request.Context(), cache, dep.AccountID)

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.AvatarUpload
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Uploaded deployment avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL:    avatarStore.DeploymentAvatarURL(dep.ID, avatarTS),
			AvatarColors: colorsJSON,
		})
	}
}

// ResetDeploymentAvatar handles DELETE /api/v1/deployments/:id/avatar
func ResetDeploymentAvatar(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, avatarStore *avatar.Store, auditStore *auditlog.Store, cache k8scache.Cache) gin.HandlerFunc {
	return func(c *gin.Context) {
		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		if err := avatarStore.DeleteDeployment(c.Request.Context(), dep.ID); err != nil {
			log.Error("avatar: reset deployment avatar failed", "error", err, "deployment", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset avatar"})
			return
		}
		_ = touchDeploymentAvatar(log, deployStore, dep.ID)

		// Clear colors — the backfill worker will re-copy from the blueprint and re-extract.
		_ = deployStore.SetAvatarColors(dep.ID, nil)
		_ = deploycache.Invalidate(c.Request.Context(), cache, dep.AccountID)

		evt := auditlog.FromGinContext(c, dep.AccountID)
		evt.Action = auditlog.AvatarReset
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Reset deployment avatar"
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, AvatarResponse{
			AvatarURL: "",
		})
	}
}
