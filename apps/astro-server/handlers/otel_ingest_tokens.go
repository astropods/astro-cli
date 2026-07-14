package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/ingesttoken"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// OtelIngestTokenMeta is the non-secret view of an ingest key.
type OtelIngestTokenMeta struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

type ListOtelIngestTokensResponse struct {
	Tokens   []OtelIngestTokenMeta `json:"tokens"`
	Endpoint string                `json:"endpoint,omitempty"`
}

type CreateOtelIngestTokenRequest struct {
	Name string `json:"name" binding:"required"`
}

// CreateOtelIngestTokenResponse carries the plaintext token, returned exactly
// once. The endpoint is included so the UI can render a ready-to-paste
// managed-settings block.
type CreateOtelIngestTokenResponse struct {
	OtelIngestTokenMeta
	Token    string `json:"token"`
	Endpoint string `json:"endpoint,omitempty"`
}

func toMeta(t *ingesttoken.Token) OtelIngestTokenMeta {
	return OtelIngestTokenMeta{
		ID:          t.ID,
		Name:        t.Name,
		TokenPrefix: t.TokenPrefix,
		CreatedAt:   t.CreatedAt,
		LastUsedAt:  t.LastUsedAt,
	}
}

// ListOtelIngestTokens returns the account's active ingest keys (metadata only).
// GET /api/v1/accounts/:account/otel-keys
func ListOtelIngestTokens(log *logger.Logger, store *ingesttoken.Store, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		tokens, err := store.ListByAccount(acct.ID)
		if err != nil {
			log.Error("Failed to list OTel ingest tokens", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list ingest keys"})
			return
		}

		metas := make([]OtelIngestTokenMeta, 0, len(tokens))
		for _, t := range tokens {
			metas = append(metas, toMeta(t))
		}
		c.JSON(http.StatusOK, ListOtelIngestTokensResponse{
			Tokens:   metas,
			Endpoint: cfg.OTelIngestEndpoint,
		})
	}
}

// CreateOtelIngestToken mints a new ingest key, returns the plaintext once, and
// best-effort ensures the account's Langfuse project exists so the trace leg
// has somewhere to route once the ingest endpoint is live. Provisioning is off
// the ingest hot path and never blocks key creation.
// POST /api/v1/accounts/:account/otel-keys
func CreateOtelIngestToken(
	log *logger.Logger,
	store *ingesttoken.Store,
	provisioner *langfuse.Provisioner,
	lfStore *langfuse.Store,
	kmsClient envelope.KMSClient,
	cfg *config.Config,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		var req CreateOtelIngestTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
			return
		}

		plaintext, hash, prefix, err := ingesttoken.Generate()
		if err != nil {
			log.Error("Failed to generate OTel ingest token", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate ingest key"})
			return
		}

		createdBy := ""
		if user, ok := middleware.GetUser(c); ok && user != nil {
			createdBy = user.ID
		}

		tok, err := store.Create(acct.ID, req.Name, hash, prefix, createdBy)
		if err != nil {
			log.Error("Failed to create OTel ingest token", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create ingest key"})
			return
		}

		// Ensure the account has a Langfuse project for the trace leg. Idempotent
		// and best-effort: a failure here must not fail key creation.
		if provisioner != nil && lfStore != nil {
			if _, _, lfErr := provisioner.EnsureProject(
				c.Request.Context(), lfStore,
				cfg.Deployment.KMSKeyARN, kmsClient,
				acct.ID, acct.Name,
			); lfErr != nil {
				log.Warn("Langfuse project ensure failed during ingest-key creation; continuing",
					"error", lfErr, "account_id", acct.ID)
			}
		}

		log.Info("OTel ingest token created", "account_id", acct.ID, "token_id", tok.ID)
		c.JSON(http.StatusCreated, CreateOtelIngestTokenResponse{
			OtelIngestTokenMeta: toMeta(tok),
			Token:               plaintext,
			Endpoint:            cfg.OTelIngestEndpoint,
		})
	}
}

// RevokeOtelIngestToken revokes an ingest key.
// DELETE /api/v1/accounts/:account/otel-keys/:tokenID
func RevokeOtelIngestToken(log *logger.Logger, store *ingesttoken.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		tokenID := c.Param("tokenID")

		if err := store.Revoke(acct.ID, tokenID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ingest key not found"})
				return
			}
			log.Error("Failed to revoke OTel ingest token", "error", err, "account_id", acct.ID, "token_id", tokenID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke ingest key"})
			return
		}

		log.Info("OTel ingest token revoked", "account_id", acct.ID, "token_id", tokenID)
		c.JSON(http.StatusOK, gin.H{"message": "ingest key revoked"})
	}
}
