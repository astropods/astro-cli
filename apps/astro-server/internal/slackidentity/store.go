// Package slackidentity is the database layer for Slack ↔ WorkOS user
// mappings. Each row records that a WorkOS user has linked a specific Slack
// account (team_id + slack_user_id) via WorkOS Pipes, so the messaging
// container's authorization path can resolve a slack request to a real
// WorkOS user identity and apply per-user grants.
//
// Lifecycle:
//   - Linked at /api/v1/users/me/slack/callback after a successful Pipes
//     OAuth round-trip. The handler calls Slack auth.test with the
//     Pipes-issued token to get team_id + slack_user_id, then Upserts.
//   - Revoked at /api/v1/users/me/slack DELETE. Soft-delete via revoked_at;
//     the row is preserved for audit and easy re-link.
//
// Uniqueness: at most one active mapping per (team_id, slack_user_id). A
// revoked mapping for the same key may coexist; an Upsert resurrects it by
// clearing revoked_at and refreshing the workos_user_id / metadata.
package slackidentity

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

const (
	// SourceOAuth marks a mapping created via the raw Slack OAuth link
	// flow. Reserved as a discriminator for future sources (e.g. SCIM-
	// driven, manual admin import).
	SourceOAuth = "oauth"
)

// Store is the data-access layer for Slack identity mappings.
type Store struct {
	db *sql.DB
}

// NewStore wires a Store onto a *sql.DB. The caller owns the lifetime of db.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Mapping is one row in slack_identity_mappings. OrganizationID is
// stored as SQL NULL when empty; display fields
// (TeamName/TeamDomain/SlackUsername) are stored as ” when blank.
type Mapping struct {
	TeamID         string
	SlackUserID    string
	WorkOSUserID   string
	OrganizationID string // optional; empty means stored NULL
	Source         string
	// Display fields captured at link time from oauth.v2.access + team.info.
	// Used by the settings UI to render "Connected as @alice in Acme"
	// with the workspace icon, no fresh round-trip per render.
	TeamName      string
	TeamDomain    string
	TeamIconURL   string
	SlackUsername string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	RevokedAt     *time.Time // nil for active mappings
}

// Upsert writes or refreshes a mapping. Used by the link handler after
// auth.test resolves the slack identity for a freshly-connected Pipes
// account. Re-linking the same (team_id, slack_user_id) — including after a
// revoke — overwrites the existing row, clearing revoked_at and refreshing
// the metadata so the mapping is active again.
func (s *Store) Upsert(m Mapping) error {
	if m.TeamID == "" || m.SlackUserID == "" || m.WorkOSUserID == "" {
		return errors.New("slackidentity: team_id, slack_user_id, and workos_user_id are required")
	}
	source := m.Source
	if source == "" {
		source = SourceOAuth
	}
	_, err := s.db.Exec(`
		INSERT INTO slack_identity_mappings
			(team_id, slack_user_id, workos_user_id, organization_id, source,
			 team_name, team_domain, team_icon_url, slack_username, updated_at, revoked_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, now(), NULL)
		ON CONFLICT (team_id, slack_user_id) DO UPDATE SET
			workos_user_id   = EXCLUDED.workos_user_id,
			organization_id  = EXCLUDED.organization_id,
			source           = EXCLUDED.source,
			team_name        = EXCLUDED.team_name,
			team_domain      = EXCLUDED.team_domain,
			team_icon_url    = EXCLUDED.team_icon_url,
			slack_username   = EXCLUDED.slack_username,
			updated_at       = now(),
			revoked_at       = NULL
	`, m.TeamID, m.SlackUserID, m.WorkOSUserID, m.OrganizationID, source,
		m.TeamName, m.TeamDomain, m.TeamIconURL, m.SlackUsername)
	if err != nil {
		return fmt.Errorf("slackidentity: upsert: %w", err)
	}
	return nil
}

// LookupResult holds what the messaging-container resolver needs from a
// single Slack→WorkOS lookup. Found is false when no active mapping exists.
type LookupResult struct {
	Found        bool
	WorkOSUserID string
}

// Lookup resolves an active (team_id, slack_user_id) to its WorkOS user.
// Returns Found=false (with no error) when no active mapping exists; that's
// the common "user hasn't linked yet" case and the caller falls back to the
// existing owning-account candidate.
func (s *Store) Lookup(teamID, slackUserID string) (LookupResult, error) {
	var workosUserID string
	err := s.db.QueryRow(`
		SELECT workos_user_id
		FROM slack_identity_mappings
		WHERE team_id = $1 AND slack_user_id = $2 AND revoked_at IS NULL
		LIMIT 1
	`, teamID, slackUserID).Scan(&workosUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LookupResult{Found: false}, nil
		}
		return LookupResult{}, fmt.Errorf("slackidentity: lookup: %w", err)
	}
	return LookupResult{Found: true, WorkOSUserID: workosUserID}, nil
}

// ListByWorkOSUser returns all active mappings for a WorkOS user. Used by
// the "Connect Slack" settings panel to render the user's linked
// workspaces. Returns an empty slice (no error) when the user has linked
// nothing.
func (s *Store) ListByWorkOSUser(workosUserID string) ([]Mapping, error) {
	rows, err := s.db.Query(`
		SELECT team_id, slack_user_id, workos_user_id,
		       COALESCE(organization_id, ''), source,
		       team_name, team_domain, team_icon_url, slack_username,
		       created_at, updated_at, revoked_at
		FROM slack_identity_mappings
		WHERE workos_user_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, workosUserID)
	if err != nil {
		return nil, fmt.Errorf("slackidentity: list by workos user: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Mapping
	for rows.Next() {
		var m Mapping
		if err := rows.Scan(
			&m.TeamID, &m.SlackUserID, &m.WorkOSUserID,
			&m.OrganizationID, &m.Source,
			&m.TeamName, &m.TeamDomain, &m.TeamIconURL, &m.SlackUsername,
			&m.CreatedAt, &m.UpdatedAt, &m.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("slackidentity: scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListByWorkOSUsers returns active mappings for many WorkOS users in a
// single query, grouped by user. Used by the members listing endpoint to
// render which Slack workspaces each member has linked without N+1
// round-trips. Users with no active mappings are absent from the result map
// (callers should treat a missing key as "not connected").
func (s *Store) ListByWorkOSUsers(workosUserIDs []string) (map[string][]Mapping, error) {
	out := make(map[string][]Mapping)
	if len(workosUserIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(`
		SELECT team_id, slack_user_id, workos_user_id,
		       COALESCE(organization_id, ''), source,
		       team_name, team_domain, team_icon_url, slack_username,
		       created_at, updated_at, revoked_at
		FROM slack_identity_mappings
		WHERE workos_user_id = ANY($1) AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, pq.Array(workosUserIDs))
	if err != nil {
		return nil, fmt.Errorf("slackidentity: list by workos users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var m Mapping
		if err := rows.Scan(
			&m.TeamID, &m.SlackUserID, &m.WorkOSUserID,
			&m.OrganizationID, &m.Source,
			&m.TeamName, &m.TeamDomain, &m.TeamIconURL, &m.SlackUsername,
			&m.CreatedAt, &m.UpdatedAt, &m.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("slackidentity: scan: %w", err)
		}
		out[m.WorkOSUserID] = append(out[m.WorkOSUserID], m)
	}
	return out, rows.Err()
}

// Revoke soft-deletes every active mapping for a WorkOS user. Used by the
// "Disconnect Slack entirely" path. Returns the number of rows affected.
func (s *Store) Revoke(workosUserID string) (int64, error) {
	res, err := s.db.Exec(`
		UPDATE slack_identity_mappings
		SET revoked_at = now(), updated_at = now()
		WHERE workos_user_id = $1 AND revoked_at IS NULL
	`, workosUserID)
	if err != nil {
		return 0, fmt.Errorf("slackidentity: revoke: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("slackidentity: revoke rows affected: %w", err)
	}
	return n, nil
}

// RevokeOne soft-deletes a single (workos_user_id, team_id) mapping. Used
// by per-workspace disconnect — a user can have many slack workspaces
// linked and may want to drop one without losing the rest. Returns the
// number of rows affected (0 or 1).
func (s *Store) RevokeOne(workosUserID, teamID string) (int64, error) {
	res, err := s.db.Exec(`
		UPDATE slack_identity_mappings
		SET revoked_at = now(), updated_at = now()
		WHERE workos_user_id = $1 AND team_id = $2 AND revoked_at IS NULL
	`, workosUserID, teamID)
	if err != nil {
		return 0, fmt.Errorf("slackidentity: revoke one: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("slackidentity: revoke one rows affected: %w", err)
	}
	return n, nil
}
