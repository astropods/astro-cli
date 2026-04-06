package account

import (
	"database/sql"
	"errors"
	"fmt"
)

// MembershipChecker provides account membership queries for the registry
type MembershipChecker struct {
	db *sql.DB
}

// NewMembershipChecker creates a new membership checker with the given database connection
func NewMembershipChecker(db *sql.DB) *MembershipChecker {
	return &MembershipChecker{db: db}
}

// IsMember checks if a user is a member of the account by account name
func (m *MembershipChecker) IsMember(accountName, userID string) (bool, error) {
	var count int
	err := m.db.QueryRow(`
		SELECT COUNT(*) FROM account_members am
		JOIN accounts a ON a.id = am.account_id
		WHERE a.name = $1 AND am.user_id = $2
	`, accountName, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check membership: %w", err)
	}
	return count > 0, nil
}

// IsMemberWithID checks membership and returns the account UUID in a single query.
// Returns (true, accountID, nil) if the user is a member, (false, "", nil) if not.
func (m *MembershipChecker) IsMemberWithID(accountName, userID string) (bool, string, error) {
	var id string
	err := m.db.QueryRow(`
		SELECT a.id FROM account_members am
		JOIN accounts a ON a.id = am.account_id
		WHERE a.name = $1 AND am.user_id = $2 AND a.deleted_at IS NULL
	`, accountName, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("failed to check membership: %w", err)
	}
	return true, id, nil
}
