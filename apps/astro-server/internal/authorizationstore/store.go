package authorizationstore

import (
	"database/sql"
	"errors"
	"fmt"
)

const (
	IdentityTypeUser  = "user"  // WorkOS user ID (web/OIDC) — resolved to account_id via account_members
	IdentityTypeSlack = "slack" // Slack user ID — resolved to the deployment owner's account_id

	AdapterWeb   = "web"
	AdapterSlack = "slack"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

type Grant struct {
	DeploymentID string `json:"deployment_id"`
	AccountID    string `json:"account_id"`
	Adapter      string `json:"adapter"`
	Role         string `json:"role"`
}

type Policy struct {
	DeploymentID string `json:"deployment_id"`
	DefaultRole  string `json:"default_role"`
}

// IsAllowedByAccount checks if the given account has a non-"none" role for the deployment
// on the specified adapter. Falls back to the deployment's default_role when no explicit grant exists.
func (s *Store) IsAllowedByAccount(deploymentID, accountID, adapter string) (bool, error) {
	var role string
	err := s.db.QueryRow(`
		SELECT role FROM deployment_authorization_grants
		WHERE deployment_id = $1 AND account_id = $2 AND adapter = $3
	`, deploymentID, accountID, adapter).Scan(&role)
	if err == nil {
		return role != "none", nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("query grant: %w", err)
	}

	var defaultRole string
	err = s.db.QueryRow(`
		SELECT default_role FROM deployment_access_policy
		WHERE deployment_id = $1
	`, deploymentID).Scan(&defaultRole)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("query policy: %w", err)
	}
	return defaultRole != "none", nil
}

// AccountIDsForUser returns all account IDs the WorkOS user is a member of.
func (s *Store) AccountIDsForUser(userID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT account_id FROM account_members WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query account members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) GetPolicy(deploymentID string) (*Policy, error) {
	var p Policy
	err := s.db.QueryRow(`
		SELECT deployment_id, default_role FROM deployment_access_policy
		WHERE deployment_id = $1
	`, deploymentID).Scan(&p.DeploymentID, &p.DefaultRole)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query policy: %w", err)
	}
	return &p, nil
}

func (s *Store) SetPolicy(deploymentID, defaultRole string) error {
	_, err := s.db.Exec(`
		INSERT INTO deployment_access_policy (deployment_id, default_role, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (deployment_id) DO UPDATE SET default_role = $2, updated_at = now()
	`, deploymentID, defaultRole)
	return err
}

func (s *Store) ListGrants(deploymentID string) ([]*Grant, error) {
	rows, err := s.db.Query(`
		SELECT deployment_id, account_id, adapter, role
		FROM deployment_authorization_grants
		WHERE deployment_id = $1
		ORDER BY account_id, adapter
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("list grants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var grants []*Grant
	for rows.Next() {
		g := &Grant{}
		if err := rows.Scan(&g.DeploymentID, &g.AccountID, &g.Adapter, &g.Role); err != nil {
			return nil, err
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}

func (s *Store) UpsertGrant(deploymentID, accountID, adapter, role string) error {
	_, err := s.db.Exec(`
		INSERT INTO deployment_authorization_grants (deployment_id, account_id, adapter, role, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (deployment_id, account_id, adapter) DO UPDATE SET role = $4, updated_at = now()
	`, deploymentID, accountID, adapter, role)
	return err
}

func (s *Store) DeleteGrant(deploymentID, accountID, adapter string) error {
	_, err := s.db.Exec(`
		DELETE FROM deployment_authorization_grants
		WHERE deployment_id = $1 AND account_id = $2 AND adapter = $3
	`, deploymentID, accountID, adapter)
	return err
}
