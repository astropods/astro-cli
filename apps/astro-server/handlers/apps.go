package handlers

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/appstore"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/connectapps"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const appTimeout = 10 * time.Second

type appAuditStore interface {
	LogAsync(*logger.Logger, auditlog.Event)
}

type AppHandler struct {
	log    *logger.Logger
	store  *appstore.Store
	workos connectapps.Client
	audit  appAuditStore
}

func NewAppHandler(log *logger.Logger, store *appstore.Store, workos connectapps.Client, auditStore appAuditStore) *AppHandler {
	return &AppHandler{log: log, store: store, workos: workos, audit: auditStore}
}

type AppResponse struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	ClientID    string               `json:"client_id"`
	Scopes      []string             `json:"scopes"`
	Secrets     []connectapps.Secret `json:"secrets"`
	CreatedAt   time.Time            `json:"created_at"`
}

type AppListResponse struct {
	Apps []AppResponse `json:"apps"`
}

type AppScopesResponse struct {
	Scopes []connectapps.Permission `json:"scopes"`
}

type CreateAppRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Scopes      []string `json:"scopes"`
}

type UpdateAppScopesRequest struct {
	Scopes []string `json:"scopes"`
}

type CreateAppResponse struct {
	App    AppResponse           `json:"app"`
	Secret connectapps.NewSecret `json:"secret"`
}

func (h *AppHandler) List(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()

	apps, err := h.store.ListByAccount(ctx, acct.ID)
	if err != nil {
		h.fail(c, "apps: list failed", err)
		return
	}
	out := make([]AppResponse, 0, len(apps))
	for _, app := range apps {
		secrets, err := h.workos.ListSecrets(ctx, app.WorkOSApplicationID)
		if err != nil {
			h.log.Warn("apps: list secrets failed", "app_id", app.ID, "error", err)
			secrets = nil
		}
		out = append(out, appResponse(app, secrets))
	}
	c.JSON(http.StatusOK, AppListResponse{Apps: out})
}

func (h *AppHandler) ListScopes(c *gin.Context) {
	_, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()

	scopes, err := h.workos.ListPermissions(ctx)
	if err != nil {
		h.fail(c, "apps: list permissions failed", err)
		return
	}
	c.JSON(http.StatusOK, AppScopesResponse{Scopes: scopes})
}

func (h *AppHandler) Create(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()

	var req CreateAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if err := appstore.ValidateName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.scopesAreGrantable(c, ctx, req.Scopes) {
		return
	}

	application, err := h.workos.CreateApplication(ctx, acct.WorkOSOrganizationID, req.Name, req.Description, req.Scopes)
	if err != nil {
		h.fail(c, "apps: create WorkOS application failed", err)
		return
	}

	app, err := h.store.Create(ctx, appstore.CreateParams{
		AccountID:           acct.ID,
		Name:                req.Name,
		Description:         req.Description,
		WorkOSApplicationID: application.ID,
		ClientID:            application.ClientID,
		Scopes:              req.Scopes,
		CreatedBy:           actorUserID(c),
	})
	if err != nil {
		if delErr := h.workos.DeleteApplication(ctx, application.ID); delErr != nil {
			h.log.Warn("apps: roll back WorkOS application failed",
				"workos_application_id", application.ID, "error", delErr)
		}
		if errors.Is(err, appstore.ErrNameTaken) {
			c.JSON(http.StatusConflict, gin.H{"error": "an app with that name already exists"})
			return
		}
		h.fail(c, "apps: create failed", err)
		return
	}

	secret, err := h.workos.CreateSecret(ctx, application.ID)
	if err != nil {
		h.fail(c, "apps: create client secret failed", err)
		return
	}

	h.logAudit(c, auditlog.AppCreate, acct.ID, app.ID, map[string]any{
		"name":   app.Name,
		"scopes": app.Scopes,
	})
	c.JSON(http.StatusCreated, CreateAppResponse{App: appResponse(app, nil), Secret: *secret})
}

func (h *AppHandler) UpdateScopes(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()

	app, ok := h.resolve(c, ctx, acct)
	if !ok {
		return
	}
	var req UpdateAppScopesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if !h.scopesAreGrantable(c, ctx, req.Scopes) {
		return
	}

	if err := h.workos.UpdateApplicationScopes(ctx, app.WorkOSApplicationID, req.Scopes); err != nil {
		h.fail(c, "apps: update WorkOS application scopes failed", err)
		return
	}
	updated, err := h.store.UpdateScopes(ctx, app.ID, req.Scopes)
	if err != nil {
		h.fail(c, "apps: update scopes failed", err)
		return
	}
	if updated == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "app not found"})
		return
	}
	h.logAudit(c, auditlog.AppUpdateScopes, acct.ID, app.ID, map[string]any{
		"name":   app.Name,
		"scopes": req.Scopes,
	})
	c.JSON(http.StatusOK, appResponse(updated, nil))
}

// scopesAreGrantable checks the requested scopes against the WorkOS registry so
// creation and update cannot diverge on what is allowed.
func (h *AppHandler) scopesAreGrantable(c *gin.Context, ctx context.Context, scopes []string) bool {
	if len(scopes) == 0 {
		return true
	}
	permitted, err := h.workos.ListPermissions(ctx)
	if err != nil {
		h.fail(c, "apps: list permissions failed", err)
		return false
	}
	known := make([]string, 0, len(permitted))
	for _, p := range permitted {
		known = append(known, p.Slug)
	}
	for _, s := range scopes {
		if !slices.Contains(known, s) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown scope: " + s})
			return false
		}
	}
	return true
}

func (h *AppHandler) Delete(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()

	app, ok := h.resolve(c, ctx, acct)
	if !ok {
		return
	}
	if err := h.workos.DeleteApplication(ctx, app.WorkOSApplicationID); err != nil {
		h.fail(c, "apps: delete WorkOS application failed", err)
		return
	}
	if err := h.store.Delete(ctx, app.ID); err != nil {
		h.fail(c, "apps: delete failed", err)
		return
	}
	h.logAudit(c, auditlog.AppDelete, acct.ID, app.ID, map[string]any{"name": app.Name})
	c.Status(http.StatusNoContent)
}

func (h *AppHandler) CreateSecret(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()

	app, ok := h.resolve(c, ctx, acct)
	if !ok {
		return
	}
	secret, err := h.workos.CreateSecret(ctx, app.WorkOSApplicationID)
	if errors.Is(err, connectapps.ErrSecretLimit) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		h.fail(c, "apps: create client secret failed", err)
		return
	}
	h.logAudit(c, auditlog.AppCreateSecret, acct.ID, app.ID, map[string]any{
		"name":      app.Name,
		"secret_id": secret.ID,
	})
	c.JSON(http.StatusCreated, secret)
}

func (h *AppHandler) DeleteSecret(c *gin.Context) {
	acct, ctx, cancel, ok := h.scope(c)
	if !ok {
		return
	}
	defer cancel()

	app, ok := h.resolve(c, ctx, acct)
	if !ok {
		return
	}
	secretID := c.Param("secret_id")

	secrets, err := h.workos.ListSecrets(ctx, app.WorkOSApplicationID)
	if err != nil {
		h.fail(c, "apps: list secrets failed", err)
		return
	}
	if !slices.ContainsFunc(secrets, func(s connectapps.Secret) bool { return s.ID == secretID }) {
		c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
		return
	}
	if len(secrets) == 1 {
		c.JSON(http.StatusConflict, gin.H{
			"error": "an app must keep one secret; add another before revoking this one, or delete the app",
		})
		return
	}
	if err := h.workos.DeleteSecret(ctx, secretID); err != nil {
		h.fail(c, "apps: delete client secret failed", err)
		return
	}
	h.logAudit(c, auditlog.AppDeleteSecret, acct.ID, app.ID, map[string]any{
		"name":      app.Name,
		"secret_id": secretID,
	})
	c.Status(http.StatusNoContent)
}

func (h *AppHandler) scope(c *gin.Context) (*account.Account, context.Context, context.CancelFunc, bool) {
	if h.store == nil || h.workos == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "apps are unavailable"})
		return nil, nil, nil, false
	}
	acct, ok := middleware.GetAccountFromContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
		return nil, nil, nil, false
	}
	if acct.WorkOSOrganizationID == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "this account is still being set up, try again in a moment"})
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), appTimeout)
	return acct, ctx, cancel, true
}

func (h *AppHandler) resolve(c *gin.Context, ctx context.Context, acct *account.Account) (*appstore.App, bool) {
	app, err := h.store.GetByID(ctx, c.Param("app_id"))
	if err != nil {
		h.fail(c, "apps: get failed", err)
		return nil, false
	}
	if app == nil || app.AccountID != acct.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "app not found"})
		return nil, false
	}
	return app, true
}

func (h *AppHandler) fail(c *gin.Context, message string, err error) {
	h.log.Error(message, "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "app operation failed"})
}

func (h *AppHandler) logAudit(c *gin.Context, action, accountID, appID string, metadata map[string]any) {
	if h.audit == nil {
		return
	}
	event := auditlog.FromGinContext(c, accountID)
	event.Action = action
	event.ResourceType = "app"
	event.ResourceID = appID
	event.Metadata = metadata
	h.audit.LogAsync(h.log, event)
}

func actorUserID(c *gin.Context) string {
	if user, ok := middleware.GetUser(c); ok && user != nil {
		return user.ID
	}
	return ""
}

func appResponse(a *appstore.App, secrets []connectapps.Secret) AppResponse {
	if secrets == nil {
		secrets = []connectapps.Secret{}
	}
	return AppResponse{
		ID:          a.ID,
		Name:        a.Name,
		Description: a.Description,
		ClientID:    a.ClientID,
		Scopes:      a.Scopes,
		Secrets:     secrets,
		CreatedAt:   a.CreatedAt,
	}
}
