package account

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ErrAlreadyDeleted is returned when MarkDeleted targets an account that is
// already soft-deleted (or does not exist).
var ErrAlreadyDeleted = errors.New("account not found or already deleted")

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
func (s *AccountStore) Create(name, accountType, ownerUserID, displayName string) (*Account, error) {
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
		INSERT INTO accounts (name, type, display_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, type, display_name, created_at, updated_at
	`, name, accountType, displayName, now, now).Scan(
		&acct.ID, &acct.Name, &acct.Type, &acct.DisplayName, &acct.CreatedAt, &acct.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// Add owner as member
	_, err = tx.Exec(`
		INSERT INTO account_members (account_id, user_id, created_at)
		VALUES ($1, $2, $3)
	`, acct.ID, ownerUserID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to add owner member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return &acct, nil
}

// CreateWithoutOwner creates a new account with no initial member.
// Used when syncing externally-created WorkOS organizations.
func (s *AccountStore) CreateWithoutOwner(name, accountType string) (*Account, error) {
	if err := ValidateAccountName(name); err != nil {
		return nil, err
	}

	now := time.Now()
	var acct Account
	err := s.db.QueryRow(`
		INSERT INTO accounts (name, type, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, type, created_at, updated_at
	`, name, accountType, now, now).Scan(
		&acct.ID, &acct.Name, &acct.Type, &acct.CreatedAt, &acct.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}
	return &acct, nil
}

// scanAccount scans an account row with the workos_org_id and deleted_at columns.
func scanAccount(row interface{ Scan(...any) error }) (*Account, error) {
	var acct Account
	var workosOrgID sql.NullString
	var deletedAt sql.NullTime
	err := row.Scan(&acct.ID, &acct.Name, &acct.Type, &workosOrgID, &deletedAt, &acct.CreatedAt, &acct.UpdatedAt, &acct.DisplayName)
	if err != nil {
		return nil, err
	}
	if workosOrgID.Valid {
		acct.WorkOSOrganizationID = workosOrgID.String
	}
	if deletedAt.Valid {
		acct.DeletedAt = &deletedAt.Time
	}
	return &acct, nil
}

// GetByName retrieves an account by its unique name
func (s *AccountStore) GetByName(name string) (*Account, error) {
	acct, err := scanAccount(s.db.QueryRow(`
		SELECT a.id, a.name, a.type, ao.workos_org_id, a.deleted_at, a.created_at, a.updated_at, a.display_name
		FROM accounts a
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		WHERE a.name = $1 AND a.deleted_at IS NULL
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
		SELECT a.id, a.name, a.type, ao.workos_org_id, a.deleted_at, a.created_at, a.updated_at, a.display_name
		FROM accounts a
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		WHERE a.id = $1 AND a.deleted_at IS NULL
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
		SELECT a.id, a.name, a.type, ao.workos_org_id, a.deleted_at, a.created_at, a.updated_at, a.display_name
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

// GetAccountsForUser returns all accounts a user is a member of
func (s *AccountStore) GetAccountsForUser(userID string) ([]AccountWithRole, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.name, a.type, COALESCE(ao.workos_org_id, ''), a.created_at, a.updated_at, a.display_name
		FROM accounts a
		JOIN account_members am ON a.id = am.account_id
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		WHERE am.user_id = $1 AND a.deleted_at IS NULL
		ORDER BY a.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var accounts []AccountWithRole
	for rows.Next() {
		var a AccountWithRole
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.WorkOSOrganizationID, &a.CreatedAt, &a.UpdatedAt, &a.DisplayName); err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts = append(accounts, a)
	}

	return accounts, nil
}

// PersonalProfile holds the name and display name from a user's personal account.
type PersonalProfile struct {
	UserID      string
	Name        string
	DisplayName string
}

// GetPersonalProfiles returns the personal-account name and display name for
// each of the given user IDs in a single query.
func (s *AccountStore) GetPersonalProfiles(userIDs []string) (map[string]PersonalProfile, error) {
	if len(userIDs) == 0 {
		return map[string]PersonalProfile{}, nil
	}
	rows, err := s.db.Query(`
		SELECT am.user_id, a.name, a.display_name
		FROM accounts a
		JOIN account_members am ON a.id = am.account_id
		WHERE am.user_id = ANY($1) AND a.type = 'personal' AND a.deleted_at IS NULL
	`, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query personal profiles: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	profiles := make(map[string]PersonalProfile, len(userIDs))
	for rows.Next() {
		var p PersonalProfile
		if err := rows.Scan(&p.UserID, &p.Name, &p.DisplayName); err != nil {
			return nil, fmt.Errorf("failed to scan personal profile: %w", err)
		}
		profiles[p.UserID] = p
	}
	return profiles, nil
}

// IsMember checks if a user is a member of an account
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

// UpdateDisplayName sets the display name for an account.
func (s *AccountStore) UpdateDisplayName(accountID, displayName string) error {
	result, err := s.db.Exec(`
		UPDATE accounts SET display_name = $1, updated_at = $2
		WHERE id = $3
	`, displayName, time.Now(), accountID)
	if err != nil {
		return fmt.Errorf("failed to update display name: %w", err)
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

// GetFirstMemberUserID returns the user ID of the first member of an account.
// For personal accounts this is the sole owner.
func (s *AccountStore) GetFirstMemberUserID(accountID string) (string, error) {
	var userID string
	err := s.db.QueryRow(`
		SELECT user_id FROM account_members
		WHERE account_id = $1
		LIMIT 1
	`, accountID).Scan(&userID)

	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no member found for account: %s", accountID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query account member: %w", err)
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

// --- Member CRUD ---

// AddMember inserts a new account member with optional WorkOS membership ID.
func (s *AccountStore) AddMember(accountID, userID, workosMembershipID string) error {
	_, err := s.db.Exec(`
		INSERT INTO account_members (account_id, user_id, created_at)
		VALUES ($1, $2, $3)
	`, accountID, userID, time.Now())
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
		SELECT am.account_id, am.user_id, mw.workos_membership_id, am.created_at
		FROM account_members am
		LEFT JOIN account_member_workos mw ON mw.account_id = am.account_id AND mw.user_id = am.user_id
		WHERE am.account_id = $1 AND am.user_id = $2
	`, accountID, userID).Scan(&m.AccountID, &m.UserID, &wid, &m.CreatedAt)
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
		SELECT am.account_id, am.user_id, mw.workos_membership_id, am.created_at
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
		if err := rows.Scan(&m.AccountID, &m.UserID, &wid, &m.CreatedAt); err != nil {
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
		SELECT am.account_id, am.user_id, mw.workos_membership_id, am.created_at
		FROM account_member_workos mw
		JOIN account_members am ON am.account_id = mw.account_id AND am.user_id = mw.user_id
		WHERE mw.workos_membership_id = $1
	`, membershipID).Scan(&m.AccountID, &m.UserID, &m.WorkOSMembershipID, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("member not found for workos membership: %s", membershipID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query member: %w", err)
	}
	return &m, nil
}

// UpsertMemberByWorkosMembershipID inserts or updates a member keyed by WorkOS membership ID.
func (s *AccountStore) UpsertMemberByWorkosMembershipID(accountID, userID, workosMembershipID string) error {
	_, err := s.db.Exec(`
		INSERT INTO account_members (account_id, user_id, created_at)
		VALUES ($1, $2, now())
		ON CONFLICT (account_id, user_id) DO NOTHING
	`, accountID, userID)
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

// Search finds accounts whose name starts with the given prefix.
// If accountType is non-empty, results are filtered to that type.
// Limit is capped at 10.
func (s *AccountStore) Search(query string, accountType string, limit int) ([]Account, error) {
	if limit <= 0 || limit > 10 {
		limit = 10
	}

	// Escape LIKE metacharacters before appending the prefix wildcard
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	pattern := escaped + "%"

	var rows *sql.Rows
	var err error

	if accountType != "" {
		rows, err = s.db.Query(`
			SELECT a.id, a.name, a.type, a.created_at, a.updated_at
			FROM accounts a
			WHERE a.name LIKE $1 AND a.type = $2
			ORDER BY a.name
			LIMIT $3
		`, pattern, accountType, limit)
	} else {
		rows, err = s.db.Query(`
			SELECT a.id, a.name, a.type, a.created_at, a.updated_at
			FROM accounts a
			WHERE a.name LIKE $1
			ORDER BY a.name
			LIMIT $2
		`, pattern, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to search accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var accounts []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts = append(accounts, a)
	}

	return accounts, nil
}

// SetOpenMeterCustomerID stores the OpenMeter customer ID for an account.
func (s *AccountStore) SetOpenMeterCustomerID(accountID, customerID string) error {
	_, err := s.db.Exec(`
		UPDATE accounts SET openmeter_customer_id = $1, updated_at = $2
		WHERE id = $3
	`, customerID, time.Now(), accountID)
	if err != nil {
		return fmt.Errorf("failed to set openmeter_customer_id: %w", err)
	}
	return nil
}

// GetAccountsMissingOpenMeterCustomer returns accounts that don't have an OpenMeter customer yet.
func (s *AccountStore) GetAccountsMissingOpenMeterCustomer(limit int) ([]Account, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, created_at, updated_at
		FROM accounts
		WHERE openmeter_customer_id IS NULL
		ORDER BY created_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var accounts []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// RemoveUserFromAllAccounts removes a user from every account they belong to.
func (s *AccountStore) RemoveUserFromAllAccounts(userID string) (int64, error) {
	result, err := s.db.Exec(`DELETE FROM account_members WHERE user_id = $1`, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to remove user from all accounts: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return n, nil
}

// MarkDeleted soft-deletes an account by setting deleted_at.
func (s *AccountStore) MarkDeleted(accountID string) error {
	result, err := s.db.Exec(`
		UPDATE accounts SET deleted_at = $1, updated_at = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, time.Now(), time.Now(), accountID)
	if err != nil {
		return fmt.Errorf("failed to mark account deleted: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return ErrAlreadyDeleted
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
