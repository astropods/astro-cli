// Package authorizationstore is the database layer for per-deployment
// authorization (the messaging container's access policy).
//
// Data model: a single deployment_authorization_grants table keyed by
// (deployment_id, subject_type, subject_id, adapter). A request is allowed
// iff a matching grant row exists. There is no separate policy table and no
// "default allow" fallback — absence of a grant means deny.
package authorizationstore

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

const (
	// IdentityTypeUser is a WorkOS user ID (web/OIDC). Resolved to the user
	// itself plus all accounts the user is a member of.
	IdentityTypeUser = "user"
	// IdentityTypeSlack is a slack user ID. Resolved to the deployment's
	// owning account (looked up from the deployments row).
	IdentityTypeSlack = "slack"

	// SubjectTypeOrg: any member of the named organization (account) is allowed.
	SubjectTypeOrg = "org"
	// SubjectTypeUser: this specific WorkOS user is allowed. Web-only — the
	// schema enforces user_web_only_check, since slack identity is opaque
	// and resolves to the owning account, never to a WorkOS user.
	SubjectTypeUser = "user"
	// SubjectTypeAnyone: anyone hitting the adapter is allowed. Legal on
	// both web and slack; for slack it collapses to "any caller in the
	// bot's workspace" (slack identity always resolves to the owning
	// account). subject_id is empty for these rows.
	SubjectTypeAnyone = "anyone"

	AdapterWeb   = "web"
	AdapterSlack = "slack"
	// AdapterCustom records grants for the agent's own custom interface (the
	// web UI it serves itself). Not enforced by the platform at the ingress;
	// the agent's server authorizes each request itself by calling the
	// /deployments/authorize callback with adapter=custom (web-shaped: an OIDC
	// WorkOS user identity, resolved and matched exactly like web).
	AdapterCustom = "custom"
)

// Store is the data-access layer for authorization grants.
type Store struct {
	db *sql.DB
}

// NewStore wires a Store onto a *sql.DB. The caller owns the lifetime of db.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Grant is a single row in deployment_authorization_grants.
//
// SubjectID's meaning depends on SubjectType:
//   - SubjectTypeOrg    → accounts.id (uuid as text)
//   - SubjectTypeUser   → workos_user_id
//   - SubjectTypeAnyone → empty string
type Grant struct {
	DeploymentID string `json:"deployment_id"`
	SubjectType  string `json:"subject_type"`
	SubjectID    string `json:"subject_id"`
	Adapter      string `json:"adapter"`
}

// Subject identifies one candidate the principal resolves to. Authorization
// passes if any candidate matches a grant for the deployment + adapter.
type Subject struct {
	Type string // SubjectTypeOrg or SubjectTypeUser
	ID   string
}

// HasAnyGrants reports whether the deployment has at least one row for the
// given adapter in the grants table.
//
// Used by the transitional "no grants → owner-account access" fallback,
// scoped per adapter: a deployment that has never written a grant for this
// adapter gets implicit access for members of its owning account on that
// adapter, until the owner adds any grant for it explicitly. Scoping is
// per-adapter so writing a slack grant doesn't silently flip web into
// deny-by-default — and vice versa.
func (s *Store) HasAnyGrants(deploymentID, adapter string) (bool, error) {
	var found int
	err := s.db.QueryRow(`
		SELECT 1 FROM deployment_authorization_grants
		WHERE deployment_id = $1 AND adapter = $2
		LIMIT 1
	`, deploymentID, adapter).Scan(&found)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("query has-any-grants: %w", err)
}

// HasAnyoneGrant returns true when an `anyone` grant exists for the
// (deployment, adapter) pair. Callers use this as a fast-path short-circuit
// so they can skip principal resolution entirely.
func (s *Store) HasAnyoneGrant(deploymentID, adapter string) (bool, error) {
	var found int
	err := s.db.QueryRow(`
		SELECT 1 FROM deployment_authorization_grants
		WHERE deployment_id = $1 AND adapter = $2 AND subject_type = 'anyone'
		LIMIT 1
	`, deploymentID, adapter).Scan(&found)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("query anyone grant: %w", err)
}

// MatchesGrant returns true when any of the supplied candidate subjects has
// an account- or user-typed grant for the (deployment, adapter) pair. It does
// *not* check the anyone short-circuit — callers are expected to invoke
// HasAnyoneGrant first when relevant.
//
// The query runs as a single round-trip with two parallel ANY() clauses so
// the SQL is fixed regardless of the candidate breakdown. Empty arrays bind
// safely via pq.Array and simply match nothing.
func (s *Store) MatchesGrant(deploymentID string, candidates []Subject, adapter string) (bool, error) {
	if len(candidates) == 0 {
		return false, nil
	}
	var orgIDs, userIDs []string
	for _, c := range candidates {
		switch c.Type {
		case SubjectTypeOrg:
			orgIDs = append(orgIDs, c.ID)
		case SubjectTypeUser:
			userIDs = append(userIDs, c.ID)
		}
	}

	var found int
	err := s.db.QueryRow(`
		SELECT 1 FROM deployment_authorization_grants
		WHERE deployment_id = $1
		  AND adapter = $2
		  AND (
		    (subject_type = 'org' AND subject_id = ANY($3))
		    OR
		    (subject_type = 'user' AND subject_id = ANY($4))
		  )
		LIMIT 1
	`, deploymentID, adapter, pq.Array(orgIDs), pq.Array(userIDs)).Scan(&found)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("query grants: %w", err)
}

// AccountIDsForUser returns every account the WorkOS user is a member of.
// Used during principal resolution for web requests.
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

// DeploymentAccountID returns the owning account of a deployment. Used during
// slack principal resolution: a slack call resolves to the account that owns
// the bot (i.e. the deployment's account). Returns sql.ErrNoRows when the
// deployment doesn't exist.
func (s *Store) DeploymentAccountID(deploymentID string) (string, error) {
	var accountID string
	err := s.db.QueryRow(`
		SELECT account_id FROM deployments WHERE id = $1
	`, deploymentID).Scan(&accountID)
	if err != nil {
		return "", err
	}
	return accountID, nil
}

// AnyoneAdapters returns the list of adapters that have an `anyone` grant for
// the deployment. Used at deploy-token issuance time so the messaging container
// can short-circuit public traffic without calling the server.
func (s *Store) AnyoneAdapters(deploymentID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT adapter FROM deployment_authorization_grants
		WHERE deployment_id = $1 AND subject_type = 'anyone'
		ORDER BY adapter
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query anyone adapters: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var adapters []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		adapters = append(adapters, a)
	}
	return adapters, rows.Err()
}

// ListGrants returns every grant for a deployment, ordered for deterministic
// output. Used by the deployment-template prefill endpoint to surface live
// state in the UI.
func (s *Store) ListGrants(deploymentID string) ([]*Grant, error) {
	rows, err := s.db.Query(`
		SELECT deployment_id, subject_type, subject_id, adapter
		FROM deployment_authorization_grants
		WHERE deployment_id = $1
		ORDER BY subject_type, subject_id, adapter
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("list grants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var grants []*Grant
	for rows.Next() {
		g := &Grant{}
		if err := rows.Scan(&g.DeploymentID, &g.SubjectType, &g.SubjectID, &g.Adapter); err != nil {
			return nil, err
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}

// ReplaceGrantsTx atomically swaps the deployment's grant set for the
// supplied list inside an existing transaction. This is the only writer to
// the grants table — there is no imperative add/remove API. The deploy
// flow folds it into the same transaction that creates the deployment
// row so a grants failure rolls back the deployment instead of leaving
// adapters with no rows (which would silently engage the per-adapter
// owner-fallback and widen access).
func ReplaceGrantsTx(tx *sql.Tx, deploymentID string, grants []Grant) error {
	if _, err := tx.Exec(`
		DELETE FROM deployment_authorization_grants WHERE deployment_id = $1
	`, deploymentID); err != nil {
		return fmt.Errorf("delete existing grants: %w", err)
	}
	for _, g := range grants {
		if _, err := tx.Exec(`
			INSERT INTO deployment_authorization_grants
				(deployment_id, subject_type, subject_id, adapter, updated_at)
			VALUES ($1, $2, $3, $4, now())
		`, deploymentID, g.SubjectType, g.SubjectID, g.Adapter); err != nil {
			return fmt.Errorf("insert grant: %w", err)
		}
	}
	return nil
}
