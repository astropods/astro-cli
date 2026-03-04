package account

import (
	"database/sql"
	"errors"
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
	defer tx.Rollback() //nolint:errcheck

	now := time.Now()
	var acct Account
	err = tx.QueryRow(`
		INSERT INTO accounts (name, type, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, type, created_at, updated_at
	`, name, accountType, now, now).Scan(
		&acct.ID, &acct.Name, &acct.Type, &acct.CreatedAt, &acct.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// Add owner as member
	_, err = tx.Exec(`
		INSERT INTO account_members (account_id, user_id, role, created_at)
		VALUES ($1, $2, 'owner', $3)
	`, acct.ID, ownerUserID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to add owner member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return &acct, nil
}

// scanAccount scans an account row with the workos_org_id column.
func scanAccount(row interface{ Scan(...any) error }) (*Account, error) {
	var acct Account
	var workosOrgID sql.NullString
	err := row.Scan(&acct.ID, &acct.Name, &acct.Type, &workosOrgID, &acct.CreatedAt, &acct.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if workosOrgID.Valid {
		acct.WorkOSOrganizationID = workosOrgID.String
	}
	return &acct, nil
}

// GetByName retrieves an account by its unique name
func (s *AccountStore) GetByName(name string) (*Account, error) {
	acct, err := scanAccount(s.db.QueryRow(`
		SELECT a.id, a.name, a.type, ao.workos_org_id, a.created_at, a.updated_at
		FROM accounts a
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		WHERE a.name = $1
	`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("account not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}
	return acct, nil
}

// GetByID retrieves an account by its UUID
func (s *AccountStore) GetByID(id string) (*Account, error) {
	acct, err := scanAccount(s.db.QueryRow(`
		SELECT a.id, a.name, a.type, ao.workos_org_id, a.created_at, a.updated_at
		FROM accounts a
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		WHERE a.id = $1
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("account not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}
	return acct, nil
}

// GetByWorkOSOrganizationID retrieves an account linked to a WorkOS organization.
func (s *AccountStore) GetByWorkOSOrganizationID(orgID string) (*Account, error) {
	acct, err := scanAccount(s.db.QueryRow(`
		SELECT a.id, a.name, a.type, ao.workos_org_id, a.created_at, a.updated_at
		FROM accounts a
		JOIN account_organizations ao ON ao.account_id = a.id
		WHERE ao.workos_org_id = $1
	`, orgID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("account not found for workos org: %s", orgID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}
	return acct, nil
}

// SetWorkOSOrganizationID links an account to a WorkOS organization.
func (s *AccountStore) SetWorkOSOrganizationID(accountID, orgID string) error {
	_, err := s.db.Exec(`
		INSERT INTO account_organizations (account_id, workos_org_id)
		VALUES ($1, $2)
		ON CONFLICT (account_id) DO UPDATE SET workos_org_id = $2
	`, accountID, orgID)
	if err != nil {
		return fmt.Errorf("failed to set workos_org_id: %w", err)
	}
	return nil
}

// GetAccountsForUser returns all accounts a user is a member of, with their role
func (s *AccountStore) GetAccountsForUser(userID string) ([]AccountWithRole, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.name, a.type, am.role, COALESCE(ao.workos_org_id, ''), a.created_at, a.updated_at
		FROM accounts a
		JOIN account_members am ON a.id = am.account_id
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		WHERE am.user_id = $1
		ORDER BY a.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var accounts []AccountWithRole
	for rows.Next() {
		var a AccountWithRole
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.Role, &a.WorkOSOrganizationID, &a.CreatedAt, &a.UpdatedAt); err != nil {
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

// GetOwnerUserID returns the user ID of the account owner
func (s *AccountStore) GetOwnerUserID(accountID string) (string, error) {
	var userID string
	err := s.db.QueryRow(`
		SELECT user_id FROM account_members
		WHERE account_id = $1 AND role = 'owner'
		LIMIT 1
	`, accountID).Scan(&userID)

	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no owner found for account: %s", accountID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query account owner: %w", err)
	}

	return userID, nil
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

// CountOwners returns the number of members with role 'owner' in an account.
func (s *AccountStore) CountOwners(accountID string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM account_members
		WHERE account_id = $1 AND role = 'owner'
	`, accountID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count owners: %w", err)
	}
	return count, nil
}

// --- Member CRUD ---

// AddMember inserts a new account member with optional WorkOS membership ID.
func (s *AccountStore) AddMember(accountID, userID, role, workosMembershipID string) error {
	_, err := s.db.Exec(`
		INSERT INTO account_members (account_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4)
	`, accountID, userID, role, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add member: %w", err)
	}
	if workosMembershipID != "" {
		_, err = s.db.Exec(`
			INSERT INTO account_member_workos (account_id, user_id, workos_membership_id)
			VALUES ($1, $2, $3)
		`, accountID, userID, workosMembershipID)
		if err != nil {
			return fmt.Errorf("failed to add member workos link: %w", err)
		}
	}
	return nil
}

// UpdateMemberRole updates a member's role.
func (s *AccountStore) UpdateMemberRole(accountID, userID, newRole string) error {
	result, err := s.db.Exec(`
		UPDATE account_members SET role = $1
		WHERE account_id = $2 AND user_id = $3
	`, newRole, accountID, userID)
	if err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("member not found")
	}
	return nil
}

// RemoveMember removes a member from an account.
func (s *AccountStore) RemoveMember(accountID, userID string) error {
	result, err := s.db.Exec(`
		DELETE FROM account_members
		WHERE account_id = $1 AND user_id = $2
	`, accountID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("member not found")
	}
	return nil
}

// GetMember retrieves a single account member.
func (s *AccountStore) GetMember(accountID, userID string) (*AccountMember, error) {
	var m AccountMember
	var wid sql.NullString
	err := s.db.QueryRow(`
		SELECT am.account_id, am.user_id, am.role, mw.workos_membership_id, am.created_at
		FROM account_members am
		LEFT JOIN account_member_workos mw ON mw.account_id = am.account_id AND mw.user_id = am.user_id
		WHERE am.account_id = $1 AND am.user_id = $2
	`, accountID, userID).Scan(&m.AccountID, &m.UserID, &m.Role, &wid, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("member not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query member: %w", err)
	}
	if wid.Valid {
		m.WorkOSMembershipID = wid.String
	}
	return &m, nil
}

// GetMembersForAccount returns all members of an account.
func (s *AccountStore) GetMembersForAccount(accountID string) ([]AccountMember, error) {
	rows, err := s.db.Query(`
		SELECT am.account_id, am.user_id, am.role, mw.workos_membership_id, am.created_at
		FROM account_members am
		LEFT JOIN account_member_workos mw ON mw.account_id = am.account_id AND mw.user_id = am.user_id
		WHERE am.account_id = $1
		ORDER BY am.created_at
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query members: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var members []AccountMember
	for rows.Next() {
		var m AccountMember
		var wid sql.NullString
		if err := rows.Scan(&m.AccountID, &m.UserID, &m.Role, &wid, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		if wid.Valid {
			m.WorkOSMembershipID = wid.String
		}
		members = append(members, m)
	}
	return members, nil
}

// GetMemberByWorkosMembershipID looks up a member by their WorkOS membership ID.
func (s *AccountStore) GetMemberByWorkosMembershipID(membershipID string) (*AccountMember, error) {
	var m AccountMember
	err := s.db.QueryRow(`
		SELECT am.account_id, am.user_id, am.role, mw.workos_membership_id, am.created_at
		FROM account_member_workos mw
		JOIN account_members am ON am.account_id = mw.account_id AND am.user_id = mw.user_id
		WHERE mw.workos_membership_id = $1
	`, membershipID).Scan(&m.AccountID, &m.UserID, &m.Role, &m.WorkOSMembershipID, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("member not found for workos membership: %s", membershipID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query member: %w", err)
	}
	return &m, nil
}

// UpsertMemberByWorkosMembershipID inserts or updates a member keyed by WorkOS membership ID.
// Used by event sync and login-time reconciliation.
func (s *AccountStore) UpsertMemberByWorkosMembershipID(accountID, userID, role, workosMembershipID string) error {
	_, err := s.db.Exec(`
		INSERT INTO account_members (account_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id, user_id) DO UPDATE SET role = $3
	`, accountID, userID, role, time.Now())
	if err != nil {
		return fmt.Errorf("failed to upsert member: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO account_member_workos (account_id, user_id, workos_membership_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (account_id, user_id) DO UPDATE SET workos_membership_id = $3
	`, accountID, userID, workosMembershipID)
	if err != nil {
		return fmt.Errorf("failed to upsert member workos link: %w", err)
	}
	return nil
}

// DeleteByID deletes an account by its UUID (used for cleanup on org creation failure).
func (s *AccountStore) DeleteByID(accountID string) error {
	_, err := s.db.Exec(`DELETE FROM accounts WHERE id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}
	return nil
}
