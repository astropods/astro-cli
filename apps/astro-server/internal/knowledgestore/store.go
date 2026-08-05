package knowledgestore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode"

	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgecache"
)

// Annotations is a free-form string→string map attached to a store, modeled on
// Kubernetes annotations. Persisted as a jsonb column; Value/Scan round-trip it
// transparently. Used to record provider-agnostic origin info, e.g. a Supabase
// import: {"source":"supabase","supabase_project_id":"...","region":"..."}.
type Annotations map[string]string

// Value implements driver.Valuer. Returns a JSON string (not []byte, which lib/pq
// would send as bytea) so Postgres parses it into jsonb; nil/empty → SQL NULL.
func (a Annotations) Value() (driver.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(map[string]string(a))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner for a jsonb column (NULL → nil map).
func (a *Annotations) Scan(src any) error {
	if src == nil {
		*a = nil
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("annotations: unsupported scan type %T", src)
	}
	if len(b) == 0 {
		*a = nil
		return nil
	}
	return json.Unmarshal(b, (*map[string]string)(a))
}

// ValidateStoreName checks that a knowledge store name is safe for use in ARNs
// and public DNS hostnames. Rules:
//   - 1–63 characters (DNS label max)
//   - lowercase alphanumeric and hyphens only
//   - cannot start or end with a hyphen
//   - no consecutive hyphens
func ValidateStoreName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("store name must not be empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("store name must be at most 63 characters")
	}
	if name[0] == '-' {
		return fmt.Errorf("store name must not start with a hyphen")
	}
	if name[len(name)-1] == '-' {
		return fmt.Errorf("store name must not end with a hyphen")
	}
	prevHyphen := false
	for _, ch := range name {
		if ch == '-' {
			if prevHyphen {
				return fmt.Errorf("store name must not contain consecutive hyphens")
			}
			prevHyphen = true
			continue
		}
		prevHyphen = false
		if !unicode.IsLower(ch) && !unicode.IsDigit(ch) {
			return fmt.Errorf("store name must contain only lowercase letters, digits, and hyphens")
		}
	}
	return nil
}

// Store manages knowledge store record persistence in PostgreSQL.
type Store struct {
	db    *sql.DB
	cache k8scache.Cache
}

// NewStore creates a new knowledge store with the given database connection.
func NewStore(db *sql.DB, caches ...k8scache.Cache) *Store {
	store := &Store{db: db}
	if len(caches) > 0 {
		store.cache = caches[0]
	}
	return store
}

func (s *Store) invalidateAccount(accountID string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// Invalidate records the new generation locally before writing Redis. The
	// database mutation has already committed, so returning a Redis error would
	// report a successful write as failed. Other replicas self-heal when their
	// cached page expires, within the 30-second remote TTL.
	_ = knowledgecache.Invalidate(ctx, s.cache, accountID)
}

func (s *Store) invalidateMutationAccount(row *sql.Row) error {
	var accountID string
	if err := row.Scan(&accountID); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	s.invalidateAccount(accountID)
	return nil
}

// KnowledgeStore represents a single connected (external) knowledge store record.
type KnowledgeStore struct {
	ID        string
	AccountID string
	Name      string
	ARN       string
	Provider  string
	// Mode is read-only. Every store created now is ModeExternal; rows left over
	// from the withdrawn platform-provisioned path still read back as "managed".
	Mode             string
	Status           string
	EncryptedDataKey []byte
	KMSKeyARN        *string
	Error            *string
	// Annotations is provider-agnostic origin detail (e.g. Supabase import info).
	Annotations Annotations
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const (
	ModeExternal = "external"

	StatusConnecting        = "connecting"
	StatusPendingAcceptance = "pending-acceptance"
	StatusReady             = "ready"
	StatusError             = "error"
)

const storeColumns = `id, account_id, name, arn, provider, mode, status,
       encrypted_data_key, kms_key_arn, error, annotations, created_at, updated_at`

func storeScanDest(s *KnowledgeStore) []any {
	return []any{
		&s.ID, &s.AccountID, &s.Name, &s.ARN, &s.Provider,
		&s.Mode, &s.Status,
		&s.EncryptedDataKey, &s.KMSKeyARN, &s.Error, &s.Annotations,
		&s.CreatedAt, &s.UpdatedAt,
	}
}

func scanStore(row interface{ Scan(dest ...any) error }) (*KnowledgeStore, error) {
	var s KnowledgeStore
	err := row.Scan(storeScanDest(&s)...)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// CreateParams holds the parameters for connecting a new knowledge store.
type CreateParams struct {
	ID               string
	AccountID        string
	Name             string
	ARN              string
	Provider         string
	Status           string // initial status — defaults to "ready"
	EncryptedDataKey []byte
	KMSKeyARN        string
	Annotations      Annotations // optional origin annotations or nil
}

// Create inserts a new knowledge store record and returns it. Stores are always
// external — the platform brokers credentials for a database the account already
// operates and provisions no infrastructure of its own.
func (s *Store) Create(p CreateParams) (*KnowledgeStore, error) {
	var encKey []byte
	var kmsARN *string
	if len(p.EncryptedDataKey) > 0 {
		encKey = p.EncryptedDataKey
		kmsARN = &p.KMSKeyARN
	}

	status := p.Status
	if status == "" {
		status = StatusReady
	}

	row := s.db.QueryRow(`
		INSERT INTO knowledge_stores
		  (id, account_id, name, arn, provider, mode, status, encrypted_data_key, kms_key_arn, annotations)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING `+storeColumns,
		p.ID, p.AccountID, p.Name, p.ARN, p.Provider,
		ModeExternal, status, encKey, kmsARN, p.Annotations,
	)
	store, err := scanStore(row)
	if err == nil {
		s.invalidateAccount(p.AccountID)
	}
	return store, err
}

// GetByID retrieves a store by its ID. Returns nil, nil if not found.
func (s *Store) GetByID(id string) (*KnowledgeStore, error) {
	row := s.db.QueryRow(`SELECT `+storeColumns+` FROM knowledge_stores WHERE id = $1`, id)
	ks, err := scanStore(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ks, err
}

// GetByARN retrieves a store by its ARN. Returns nil, nil if not found.
func (s *Store) GetByARN(ctx context.Context, arn string) (*KnowledgeStore, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+storeColumns+` FROM knowledge_stores WHERE arn = $1`, arn)
	ks, err := scanStore(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ks, err
}

// GetByName retrieves a store by account ID and name. Returns nil, nil if not found.
func (s *Store) GetByName(accountID, name string) (*KnowledgeStore, error) {
	row := s.db.QueryRow(
		`SELECT `+storeColumns+` FROM knowledge_stores WHERE account_id = $1 AND name = $2`,
		accountID, name,
	)
	ks, err := scanStore(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ks, err
}

// ListByAccount returns all stores for an account, newest first.
func (s *Store) ListByAccount(accountID string) ([]*KnowledgeStore, error) {
	rows, err := s.db.Query(
		`SELECT `+storeColumns+` FROM knowledge_stores WHERE account_id = $1 ORDER BY created_at DESC`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var stores []*KnowledgeStore
	for rows.Next() {
		ks, err := scanStore(rows)
		if err != nil {
			return nil, err
		}
		stores = append(stores, ks)
	}
	return stores, rows.Err()
}

// SetStatus updates the store status and clears any error.
func (s *Store) SetStatus(id, status string) error {
	return s.invalidateMutationAccount(s.db.QueryRow(
		`UPDATE knowledge_stores SET status = $1, error = NULL, updated_at = now() WHERE id = $2 RETURNING account_id`,
		status, id, // status is validated by callers using package constants
	))
}

// SetError sets the store status to error with the given message.
func (s *Store) SetError(id, errMsg string) error {
	return s.invalidateMutationAccount(s.db.QueryRow(
		`UPDATE knowledge_stores SET status = $1, error = $2, updated_at = now() WHERE id = $3 RETURNING account_id`,
		StatusError, errMsg, id,
	))
}

// Delete removes a store record. Cascades to credentials.
func (s *Store) Delete(id string) error {
	return s.invalidateMutationAccount(s.db.QueryRow(
		`DELETE FROM knowledge_stores WHERE id = $1 RETURNING account_id`, id,
	))
}

// Credential is a single encrypted key-value pair for a knowledge store.
type Credential struct {
	Key            string
	ValueEncrypted []byte
	Nonce          []byte
}

// SaveCredentials upserts all credentials for a store in a single transaction.
func (s *Store) SaveCredentials(storeID string, creds []Credential) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, c := range creds {
		_, err := tx.Exec(`
			INSERT INTO knowledge_store_credentials (knowledge_store_id, key, value_encrypted, nonce)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (knowledge_store_id, key) DO UPDATE
			  SET value_encrypted = EXCLUDED.value_encrypted, nonce = EXCLUDED.nonce`,
			storeID, c.Key, c.ValueEncrypted, c.Nonce,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetCredentials returns all encrypted credentials for a store.
func (s *Store) GetCredentials(storeID string) ([]Credential, error) {
	rows, err := s.db.Query(
		`SELECT key, value_encrypted, nonce FROM knowledge_store_credentials WHERE knowledge_store_id = $1`,
		storeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var creds []Credential
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.Key, &c.ValueEncrypted, &c.Nonce); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// ExternalCredentialKeys returns the required credential keys for an external
// store of the given provider. These are the keys the user must supply when
// connecting an existing database.
func ExternalCredentialKeys(provider string) []string {
	switch provider {
	case "postgres":
		return []string{"HOST", "PORT", "DATABASE", "USERNAME", "PASSWORD"}
	case "qdrant":
		return []string{"HOST", "PORT", "API_KEY"}
	case "redis":
		return []string{"HOST", "PORT", "PASSWORD"}
	case "neo4j":
		return []string{"HOST", "PORT", "USERNAME", "PASSWORD"}
	case "pinecone":
		return []string{"HOST", "API_KEY"}
	case "mysql":
		return []string{"HOST", "PORT", "DATABASE", "USERNAME", "PASSWORD"}
	default:
		return []string{"HOST", "PORT"}
	}
}

// ValidateExternalCredentials checks that all required credential keys for the
// given provider are present and non-empty in the supplied map.
func ValidateExternalCredentials(provider string, creds map[string]string) error {
	for _, key := range ExternalCredentialKeys(provider) {
		if creds[key] == "" {
			return fmt.Errorf("missing required credential: %s", key)
		}
	}
	return nil
}
