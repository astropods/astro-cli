package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/ingesttoken"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/gin-gonic/gin"
)

// OtelIngestTokenMeta is the non-secret view of an ingest key. ExcludedEmails
// is included so the UI can edit the exclusion list without revealing the key.
type OtelIngestTokenMeta struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	TokenPrefix    string     `json:"token_prefix"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	ExcludedEmails []string   `json:"excluded_emails"`
}

type ListOtelIngestTokensResponse struct {
	Tokens   []OtelIngestTokenMeta `json:"tokens"`
	Endpoint string                `json:"endpoint,omitempty"`
}

type CreateOtelIngestTokenRequest struct {
	Name           string   `json:"name" binding:"required"`
	ExcludedEmails []string `json:"excluded_emails"`
}

// UpdateOtelIngestTokenExclusionsRequest replaces a key's exclusion list.
type UpdateOtelIngestTokenExclusionsRequest struct {
	ExcludedEmails []string `json:"excluded_emails"`
}

// UpdateOtelIngestTokenExclusionsResponse is the normalized list as stored.
type UpdateOtelIngestTokenExclusionsResponse struct {
	ExcludedEmails []string `json:"excluded_emails"`
}

// RenameOtelIngestTokenRequest replaces a key's display name.
type RenameOtelIngestTokenRequest struct {
	Name string `json:"name" binding:"required"`
}

// RenameOtelIngestTokenResponse is the name as stored.
type RenameOtelIngestTokenResponse struct {
	Name string `json:"name"`
}

// maxExcludedEmails caps a key's exclusion list. Generous — a large team can
// still be listed — while bounding the row and the ingest-time set.
const maxExcludedEmails = 500

// maxTokenNameLen bounds a key's display name. The column is unbounded text,
// so this is what keeps a name renderable in the sources table.
const maxTokenNameLen = 200

// ingestKeyKind is how the security notification names this credential. It
// matches the label the settings page uses ("Ingestion key"), so the email and
// the app call the same thing by the same name.
const ingestKeyKind = "ingestion"

// normalizeTokenName trims and length-checks a key name. Shared by create and
// rename so a rename cannot set a name create would have rejected.
func normalizeTokenName(in string) (string, error) {
	name := strings.TrimSpace(in)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if len(name) > maxTokenNameLen {
		return "", fmt.Errorf("name too long (max %d characters)", maxTokenNameLen)
	}
	return name, nil
}

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// normalizeEmails trims, lowercases, dedupes, and validates the exclusion list.
// Ingest matches lowercased user.email, so normalization here is what makes the
// match case-insensitive. Returns a 400-worthy error on any invalid entry.
func normalizeEmails(in []string) ([]string, error) {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		e := strings.ToLower(strings.TrimSpace(raw))
		if e == "" {
			continue
		}
		if !emailRe.MatchString(e) {
			return nil, fmt.Errorf("invalid email address: %q", raw)
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	if len(out) > maxExcludedEmails {
		return nil, fmt.Errorf("too many excluded emails (max %d)", maxExcludedEmails)
	}
	return out, nil
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
	emails := t.ExcludedEmails
	if emails == nil {
		emails = []string{}
	}
	return OtelIngestTokenMeta{
		ID:             t.ID,
		Name:           t.Name,
		TokenPrefix:    t.TokenPrefix,
		CreatedAt:      t.CreatedAt,
		LastUsedAt:     t.LastUsedAt,
		ExcludedEmails: emails,
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
	queue notifyQueue,
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
		name, err := normalizeTokenName(req.Name)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.Name = name

		excluded, err := normalizeEmails(req.ExcludedEmails)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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

		tok, err := store.Create(acct.ID, req.Name, hash, prefix, createdBy, excluded)
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
		emitNotify(c, log, queue, notify.SecurityKeyCreated(acct.ID, ingestKeyKind, req.Name))
		c.JSON(http.StatusCreated, CreateOtelIngestTokenResponse{
			OtelIngestTokenMeta: toMeta(tok),
			Token:               plaintext,
			Endpoint:            cfg.OTelIngestEndpoint,
		})
	}
}

// RevokeOtelIngestToken revokes an ingest key.
// DELETE /api/v1/accounts/:account/otel-keys/:tokenID
func RevokeOtelIngestToken(log *logger.Logger, store *ingesttoken.Store, queue notifyQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		tokenID := c.Param("tokenID")

		keyName, err := store.Revoke(acct.ID, tokenID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ingest key not found"})
				return
			}
			log.Error("Failed to revoke OTel ingest token", "error", err, "account_id", acct.ID, "token_id", tokenID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke ingest key"})
			return
		}

		log.Info("OTel ingest token revoked", "account_id", acct.ID, "token_id", tokenID)
		emitNotify(c, log, queue, notify.SecurityKeyRevoked(acct.ID, ingestKeyKind, keyName))
		c.JSON(http.StatusOK, gin.H{"message": "ingest key revoked"})
	}
}

// RenameOtelIngestToken changes a key's display name. Naming is otherwise
// only possible at creation; the credential itself is untouched, so this
// neither rotates nor re-reveals the key.
// PATCH /api/v1/accounts/:account/otel-keys/:tokenID/name
func RenameOtelIngestToken(log *logger.Logger, store *ingesttoken.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		tokenID := c.Param("tokenID")

		var req RenameOtelIngestTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
			return
		}
		name, err := normalizeTokenName(req.Name)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := store.Rename(acct.ID, tokenID, name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ingest key not found"})
				return
			}
			log.Error("Failed to rename OTel ingest token", "error", err, "account_id", acct.ID, "token_id", tokenID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rename ingest key"})
			return
		}

		log.Info("OTel ingest token renamed", "account_id", acct.ID, "token_id", tokenID)
		c.JSON(http.StatusOK, RenameOtelIngestTokenResponse{Name: name})
	}
}

// UpdateOtelIngestTokenExclusions replaces a key's exclusion list. This edits
// privacy exclusions in place — no key rotation, and the plaintext is never
// re-revealed — so it takes only the key id and the new email set.
// PATCH /api/v1/accounts/:account/otel-keys/:tokenID/exclusions
func UpdateOtelIngestTokenExclusions(log *logger.Logger, store *ingesttoken.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		tokenID := c.Param("tokenID")

		var req UpdateOtelIngestTokenExclusionsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
			return
		}
		excluded, err := normalizeEmails(req.ExcludedEmails)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := store.UpdateExclusions(acct.ID, tokenID, excluded); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ingest key not found"})
				return
			}
			log.Error("Failed to update OTel ingest token exclusions", "error", err, "account_id", acct.ID, "token_id", tokenID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update exclusions"})
			return
		}

		log.Info("OTel ingest token exclusions updated", "account_id", acct.ID, "token_id", tokenID, "excluded_count", len(excluded))
		c.JSON(http.StatusOK, UpdateOtelIngestTokenExclusionsResponse{ExcludedEmails: excluded})
	}
}
