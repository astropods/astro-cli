// Package langfuse provides per-account Langfuse project provisioning
// and a REST client for reading traces. Projects and API keys are created
// by writing directly to Langfuse's Postgres database (the management API
// requires an enterprise license we don't have).
package langfuse

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Provisioner creates Langfuse projects and API keys by writing directly
// to Langfuse's Postgres database.
type Provisioner struct {
	langfuseDB *sql.DB
	salt       string // must match Langfuse's SALT env var
	orgID      string // the single org in our Langfuse instance
}

// NewProvisioner opens a connection to Langfuse's Postgres and returns a Provisioner.
func NewProvisioner(dbURL, salt, orgID string) (*Provisioner, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("langfuse provisioner: open db: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("langfuse provisioner: ping db: %w", err)
	}

	return &Provisioner{langfuseDB: db, salt: salt, orgID: orgID}, nil
}

// Close closes the Langfuse database connection.
func (p *Provisioner) Close() error {
	return p.langfuseDB.Close()
}

// EnsureProject provisions a Langfuse project for the given account if not yet
// provisioned. Returns the project's public and secret key.
func (p *Provisioner) EnsureProject(
	ctx context.Context,
	store *Store,
	accountID, accountName string,
) (publicKey, secretKey string, err error) {
	// Check if already provisioned
	existing, err := store.Get(accountID)
	if err != nil {
		return "", "", fmt.Errorf("check existing: %w", err)
	}
	if existing != nil {
		return existing.PublicKey, existing.SecretKey, nil
	}

	// Generate keys
	pk := "pk-lf-" + uuid.New().String()
	sk := "sk-lf-" + uuid.New().String()
	projectID := generateCUID()
	apiKeyID := generateCUID()

	// Compute hashes for Langfuse's api_keys table
	hashedSK, err := bcrypt.GenerateFromPassword([]byte(sk), 11)
	if err != nil {
		return "", "", fmt.Errorf("bcrypt hash: %w", err)
	}
	fastHashedSK := computeFastHash(sk, p.salt)
	displaySK := sk[:6] + "..." + sk[len(sk)-4:]

	projectName := accountName + "-" + accountID
	now := time.Now().UTC()

	// Write to Langfuse's database in a transaction
	tx, err := p.langfuseDB.BeginTx(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Check for existing project with same name (idempotency)
	var existingProjectID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM projects WHERE org_id = $1 AND name = $2 AND deleted_at IS NULL`,
		p.orgID, projectName,
	).Scan(&existingProjectID)
	if err == nil {
		// Project already exists in Langfuse, reuse it
		projectID = existingProjectID
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("check existing project: %w", err)
	} else {
		// Create project
		_, err = tx.ExecContext(ctx, `
			INSERT INTO projects (id, org_id, name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)
		`, projectID, p.orgID, projectName, now, now)
		if err != nil {
			return "", "", fmt.Errorf("insert project: %w", err)
		}
	}

	// Create API key
	_, err = tx.ExecContext(ctx, `
		INSERT INTO api_keys (id, public_key, hashed_secret_key, fast_hashed_secret_key, display_secret_key, project_id, scope, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'PROJECT', $7)
	`, apiKeyID, pk, string(hashedSK), fastHashedSK, displaySK, projectID, now)
	if err != nil {
		return "", "", fmt.Errorf("insert api key: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", "", fmt.Errorf("commit: %w", err)
	}

	if err := store.Save(&AccountLangfuse{
		AccountID:         accountID,
		LangfuseProjectID: projectID,
		PublicKey:         pk,
		SecretKey:         sk,
	}); err != nil {
		return "", "", fmt.Errorf("save credentials: %w", err)
	}

	return pk, sk, nil
}

// DeleteProject soft-deletes a Langfuse project and hard-deletes its API keys.
// Treats already-deleted projects as success (idempotent).
func (p *Provisioner) DeleteProject(ctx context.Context, projectID string) error {
	now := time.Now().UTC()
	tx, err := p.langfuseDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// api_keys carries no deleted_at, and Langfuse's auth path reads the table
	// unfiltered, so removing the rows is what revokes the credentials.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM api_keys WHERE project_id = $1`,
		projectID,
	); err != nil {
		return fmt.Errorf("delete api keys: %w", err)
	}

	// Soft-delete the project
	if _, err := tx.ExecContext(ctx,
		`UPDATE projects SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`,
		now, projectID,
	); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}

	return tx.Commit()
}

// computeFastHash computes Langfuse's fast_hashed_secret_key:
// sha256(secretKey + hex(sha256(salt)))
func computeFastHash(secretKey, salt string) string {
	saltHash := sha256.Sum256([]byte(salt))
	saltHex := hex.EncodeToString(saltHash[:])

	h := sha256.New()
	h.Write([]byte(secretKey))
	h.Write([]byte(saltHex))
	return hex.EncodeToString(h.Sum(nil))
}

// generateCUID generates a CUID-like random string.
// Langfuse uses @paralleldrive/cuid2 which produces 24-char lowercase alphanumeric strings
// starting with a letter. We approximate this with a random UUID stripped of dashes and truncated.
func generateCUID() string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	// Ensure starts with a letter (cuid convention)
	if id[0] >= '0' && id[0] <= '9' {
		id = "c" + id[1:]
	}
	if len(id) > 24 {
		id = id[:24]
	}
	return id
}
