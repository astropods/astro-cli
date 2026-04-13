package knowledgestore

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode"
)

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
	db *sql.DB
}

// NewStore creates a new knowledge store with the given database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// KnowledgeStore represents a single managed knowledge store record.
type KnowledgeStore struct {
	ID               string
	AccountID        string
	Name             string
	ARN              string
	Provider         string
	Status           string
	Namespace        string
	Storage          string
	Public           bool
	PublicHost       *string
	EncryptedDataKey []byte
	KMSKeyARN        *string
	Error            *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

const (
	StatusProvisioning = "provisioning"
	StatusReady        = "ready"
	StatusError        = "error"
)

const storeColumns = `id, account_id, name, arn, provider, status, namespace, storage,
       public, public_host, encrypted_data_key, kms_key_arn, error, created_at, updated_at`

func scanStore(row interface{ Scan(dest ...any) error }) (*KnowledgeStore, error) {
	var s KnowledgeStore
	err := row.Scan(
		&s.ID, &s.AccountID, &s.Name, &s.ARN, &s.Provider,
		&s.Status, &s.Namespace, &s.Storage, &s.Public, &s.PublicHost,
		&s.EncryptedDataKey, &s.KMSKeyARN, &s.Error,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// CreateParams holds the parameters for creating a new knowledge store.
type CreateParams struct {
	ID               string
	AccountID        string
	Name             string
	ARN              string
	Provider         string
	Namespace        string
	Storage          string
	Public           bool
	PublicHost       string
	EncryptedDataKey []byte
	KMSKeyARN        string
}

// Create inserts a new knowledge store record and returns it.
func (s *Store) Create(p CreateParams) (*KnowledgeStore, error) {
	var publicHost *string
	if p.PublicHost != "" {
		publicHost = &p.PublicHost
	}
	var encKey []byte
	var kmsARN *string
	if len(p.EncryptedDataKey) > 0 {
		encKey = p.EncryptedDataKey
		kmsARN = &p.KMSKeyARN
	}

	row := s.db.QueryRow(`
		INSERT INTO knowledge_stores
		  (id, account_id, name, arn, provider, namespace, storage, public, public_host, encrypted_data_key, kms_key_arn)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+storeColumns,
		p.ID, p.AccountID, p.Name, p.ARN, p.Provider,
		p.Namespace, p.Storage, p.Public, publicHost, encKey, kmsARN,
	)
	return scanStore(row)
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
func (s *Store) GetByARN(arn string) (*KnowledgeStore, error) {
	row := s.db.QueryRow(`SELECT `+storeColumns+` FROM knowledge_stores WHERE arn = $1`, arn)
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
	_, err := s.db.Exec(
		`UPDATE knowledge_stores SET status = $1, error = NULL, updated_at = now() WHERE id = $2`,
		status, id, // status is validated by callers using package constants
	)
	return err
}

// SetError sets the store status to error with the given message.
func (s *Store) SetError(id, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE knowledge_stores SET status = $1, error = $2, updated_at = now() WHERE id = $3`,
		StatusError, errMsg, id,
	)
	return err
}

// SetPublicHost records the assigned public hostname once the LB is ready.
func (s *Store) SetPublicHost(id, host string) error {
	_, err := s.db.Exec(
		`UPDATE knowledge_stores SET public_host = $1, updated_at = now() WHERE id = $2`,
		host, id,
	)
	return err
}

// Delete removes a store record. Cascades to credentials.
func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM knowledge_stores WHERE id = $1`, id)
	return err
}

// ListProvisioning returns all stores currently in the provisioning state.
// Used by the reconciler to check for readiness.
func (s *Store) ListProvisioning() ([]*KnowledgeStore, error) {
	rows, err := s.db.Query(`SELECT `+storeColumns+` FROM knowledge_stores WHERE status = $1`, StatusProvisioning)
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

// ListReady returns all ready stores. Used by the reconciler to ensure
// K8s secrets exist (recreate after cluster migration or accidental deletion).
func (s *Store) ListReady() ([]*KnowledgeStore, error) {
	rows, err := s.db.Query(`SELECT `+storeColumns+` FROM knowledge_stores WHERE status = $1`, StatusReady)
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
