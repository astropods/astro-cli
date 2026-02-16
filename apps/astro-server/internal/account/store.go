package account

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// AccountStore manages account persistence in PostgreSQL
type AccountStore struct {
	db *sql.DB
}

// NewAccountStore creates a new account store with the given database connection
func NewAccountStore(db *sql.DB) *AccountStore {
	return &AccountStore{db: db}
}

// Create creates a new account and adds the owner as a member.
// Returns an error if the name is taken or invalid.
func (s *AccountStore) Create(name, accountType, ownerUserID string) (*Account, error) {
	if err := ValidateAccountName(name); err != nil {
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	var account Account
	err = tx.QueryRow(`
		INSERT INTO accounts (name, type, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, type, created_at, updated_at
	`, name, accountType, now, now).Scan(
		&account.ID, &account.Name, &account.Type, &account.CreatedAt, &account.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// Add owner as member
	_, err = tx.Exec(`
		INSERT INTO account_members (account_id, user_id, role, created_at)
		VALUES ($1, $2, 'owner', $3)
	`, account.ID, ownerUserID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to add owner member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return &account, nil
}

// GetByName retrieves an account by its unique name
func (s *AccountStore) GetByName(name string) (*Account, error) {
	var account Account
	err := s.db.QueryRow(`
		SELECT id, name, type, created_at, updated_at
		FROM accounts
		WHERE name = $1
	`, name).Scan(&account.ID, &account.Name, &account.Type, &account.CreatedAt, &account.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("account not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}

	return &account, nil
}

// GetByID retrieves an account by its UUID
func (s *AccountStore) GetByID(id string) (*Account, error) {
	var account Account
	err := s.db.QueryRow(`
		SELECT id, name, type, created_at, updated_at
		FROM accounts
		WHERE id = $1
	`, id).Scan(&account.ID, &account.Name, &account.Type, &account.CreatedAt, &account.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("account not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}

	return &account, nil
}

// GetAccountsForUser returns all accounts a user is a member of, with their role
func (s *AccountStore) GetAccountsForUser(userID string) ([]AccountWithRole, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.name, a.type, am.role, a.created_at, a.updated_at
		FROM accounts a
		JOIN account_members am ON a.id = am.account_id
		WHERE am.user_id = $1
		ORDER BY a.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []AccountWithRole
	for rows.Next() {
		var a AccountWithRole
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.Role, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts = append(accounts, a)
	}

	return accounts, nil
}

// HasRole checks if a user has one of the specified roles in an account
func (s *AccountStore) HasRole(accountID, userID string, roles ...string) (bool, error) {
	if len(roles) == 0 {
		return false, nil
	}

	// Build IN clause for roles
	query := `
		SELECT COUNT(*) FROM account_members
		WHERE account_id = $1 AND user_id = $2 AND role = ANY($3)
	`

	var count int
	err := s.db.QueryRow(query, accountID, userID, pq.Array(roles)).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check role: %w", err)
	}

	return count > 0, nil
}

// IsMember checks if a user is a member of an account (any role)
func (s *AccountStore) IsMember(accountID, userID string) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM account_members
		WHERE account_id = $1 AND user_id = $2
	`, accountID, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check membership: %w", err)
	}

	return count > 0, nil
}

// Rename changes an account's name
func (s *AccountStore) Rename(accountID, newName string) error {
	if err := ValidateAccountName(newName); err != nil {
		return err
	}

	result, err := s.db.Exec(`
		UPDATE accounts SET name = $1, updated_at = $2
		WHERE id = $3
	`, newName, time.Now(), accountID)
	if err != nil {
		return fmt.Errorf("failed to rename account: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("account not found: %s", accountID)
	}

	return nil
}

// HasPersonalAccount checks if a user already has a personal account
func (s *AccountStore) HasPersonalAccount(userID string) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM accounts a
		JOIN account_members am ON a.id = am.account_id
		WHERE am.user_id = $1 AND a.type = 'personal'
	`, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check personal account: %w", err)
	}
	return count > 0, nil
}
