// Package langfuse provides per-account Langfuse project provisioning
// and a REST client for reading traces. Projects and API keys are created
// by writing directly to Langfuse's Postgres database (the management API
// requires an enterprise license we don't have).
package langfuse

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
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
		db.Close()
		return nil, fmt.Errorf("langfuse provisioner: ping db: %w", err)
	}

	return &Provisioner{langfuseDB: db, salt: salt, orgID: orgID}, nil
}

// Close closes the Langfuse database connection.
func (p *Provisioner) Close() error {
	return p.langfuseDB.Close()
}

// EnsureProject provisions a Langfuse project for the given account if not yet provisioned.
// Returns the project's public key and plaintext secret key.
func (p *Provisioner) EnsureProject(
	ctx context.Context,
	store *Store,
	kmsKeyARN string,
	kmsClient envelope.KMSClient,
	accountID, accountName string,
) (publicKey, secretKey string, err error) {
	// Check if already provisioned
	existing, err := store.Get(accountID)
	if err != nil {
		return "", "", fmt.Errorf("check existing: %w", err)
	}
	if existing != nil {
		// Decrypt and return existing credentials
		sk, err := decryptSecretKey(ctx, kmsClient, existing)
		if err != nil {
			return "", "", fmt.Errorf("decrypt existing key: %w", err)
		}
		return existing.PublicKey, sk, nil
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
	} else if err != sql.ErrNoRows {
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

	// KMS-encrypt the secret key and store in astro's database
	if kmsKeyARN != "" && kmsClient != nil {
		enc, err := envelope.NewEncryptor(ctx, kmsClient, kmsKeyARN)
		if err != nil {
			return "", "", fmt.Errorf("create encryptor: %w", err)
		}
		ciphertext, nonce, err := enc.Encrypt([]byte(sk))
		if err != nil {
			return "", "", fmt.Errorf("encrypt secret key: %w", err)
		}
		err = store.Save(&AccountLangfuse{
			AccountID:         accountID,
			LangfuseProjectID: projectID,
			PublicKey:         pk,
			SecretKey:         base64.StdEncoding.EncodeToString(ciphertext),
			EncryptedDataKey:  enc.EncryptedDataKey,
			Nonce:             nonce,
		})
		if err != nil {
			return "", "", fmt.Errorf("save credentials: %w", err)
		}
	} else {
		// No KMS — store plaintext (dev/test environments)
		err = store.Save(&AccountLangfuse{
			AccountID:         accountID,
			LangfuseProjectID: projectID,
			PublicKey:         pk,
			SecretKey:         sk,
		})
		if err != nil {
			return "", "", fmt.Errorf("save credentials: %w", err)
		}
	}

	return pk, sk, nil
}

// decryptSecretKey decrypts the stored secret key using KMS envelope decryption.
func decryptSecretKey(ctx context.Context, kmsClient envelope.KMSClient, al *AccountLangfuse) (string, error) {
	if len(al.EncryptedDataKey) == 0 || len(al.Nonce) == 0 {
		// No encryption — return as-is (dev/test)
		return al.SecretKey, nil
	}
	if kmsClient == nil {
		return "", fmt.Errorf("KMS client required to decrypt Langfuse secret key")
	}

	dec, err := envelope.NewDecryptor(ctx, kmsClient, al.EncryptedDataKey)
	if err != nil {
		return "", fmt.Errorf("create decryptor: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(al.SecretKey)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	plaintext, err := dec.Decrypt(ciphertext, al.Nonce)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
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
