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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lib/pq"
)

// observedSeenResetInterval bounds how long the in-memory dedupe set
// holds entries before being wiped. Caps memory growth to one day's
// worth of unique (team, user) pairs per pod; after the reset, the next
// call per user re-pays a single DB UPSERT (idempotent, ON CONFLICT
// safe). Trade-off: tighter than 24h would burn more DB writes for the
// same dedupe benefit; longer would let memory grow unbounded across
// long-running pods.
const observedSeenResetInterval = 24 * time.Hour

// Store is the data-access layer for Slack identity mappings.
type Store struct {
	db *sql.DB

	// observedSeen dedupes UpsertObserved within a single process so a
	// chatty Slack workspace doesn't generate one DB write per message —
	// only the first call for each (team_id, slack_user_id) pair touches
	// Postgres. Reset every observedSeenResetInterval to bound memory.
	// The DB UPSERT is still ON CONFLICT safe; this is purely a
	// write-amplification cap.
	observedMu        sync.Mutex
	observedSeen      map[string]struct{}
	observedLastReset time.Time
	// now is the time source — injected in tests to advance past the
	// reset interval deterministically; defaults to time.Now in prod.
	now func() time.Time
}

// NewStore wires a Store onto a *sql.DB. The caller owns the lifetime of db.
func NewStore(db *sql.DB) *Store {
	now := time.Now
	return &Store{
		db:                db,
		observedSeen:      make(map[string]struct{}),
		observedLastReset: now(),
		now:               now,
	}
}

// Mapping is one row in slack_identity_mappings — an oauth-linked
// identity. OrganizationID is stored as SQL NULL when empty; display
// fields (TeamName/TeamDomain/SlackUsername) are stored as "" when
// blank. PR 3 dropped the observed-source variant; every row now
// represents a WorkOS↔Slack link captured at oauth time.
type Mapping struct {
	TeamID         string
	SlackUserID    string
	WorkOSUserID   string
	OrganizationID string // optional; empty means stored NULL
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

// SlackProfile is the best-effort display metadata resolved from Slack's
// directory APIs. Empty values are valid: Slack may return a user without a
// display name/avatar, or the app may be missing the users:read scope.
type SlackProfile struct {
	DisplayName string
	Username    string
	AvatarURL   string
	IsBot       bool
	Deleted     bool
}

// ObservedUser is one row in slack_observed_users.
type ObservedUser struct {
	TeamID      string
	SlackUserID string
	Profile     SlackProfile
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
	_, err := s.db.Exec(`
		INSERT INTO slack_identity_mappings
			(team_id, slack_user_id, workos_user_id, organization_id,
			 team_name, team_domain, team_icon_url, slack_username, updated_at, revoked_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, now(), NULL)
		ON CONFLICT (team_id, slack_user_id) DO UPDATE SET
			workos_user_id   = EXCLUDED.workos_user_id,
			organization_id  = EXCLUDED.organization_id,
			team_name        = EXCLUDED.team_name,
			team_domain      = EXCLUDED.team_domain,
			team_icon_url    = EXCLUDED.team_icon_url,
			slack_username   = EXCLUDED.slack_username,
			updated_at       = now(),
			revoked_at       = NULL
	`, m.TeamID, m.SlackUserID, m.WorkOSUserID, m.OrganizationID,
		m.TeamName, m.TeamDomain, m.TeamIconURL, m.SlackUsername)
	if err != nil {
		return fmt.Errorf("slackidentity: upsert: %w", err)
	}
	return nil
}

// UpsertObserved records that the server has seen (team_id, slack_user_id)
// via the /authorize live-ingest path. Writes the pair into
// slack_observed_users — the post-PR-3 sole home of the observed
// directory. ON CONFLICT bumps last_seen_at so the row stays fresh
// without growing.
//
// Used by Insights to enrich Slack trace rows that already carry team_id with
// profile and workspace metadata, not to infer team_id for unscoped traces.
//
// Per-process in-memory dedupe (observedSeen) eliminates the steady-
// state DB write on every authorize call: a chatty workspace only
// touches Postgres once per (team, user) pair per pod lifetime.
// Across pods or restarts, the SQL UPSERT is still ON CONFLICT safe.
func (s *Store) UpsertObserved(ctx context.Context, teamID, slackUserID string) error {
	if teamID == "" || slackUserID == "" {
		return nil // tolerate empty inputs silently — authorize handler tolerates them too
	}
	key := teamID + ":" + slackUserID
	s.observedMu.Lock()
	// Periodic reset bounds memory growth: after observedSeenResetInterval
	// the map is wiped wholesale so the per-pod working set caps at one
	// interval's worth of unique users. The DB UPSERT downstream is
	// idempotent, so the worst-case fallout of a reset is one extra
	// no-op write per still-active user — far cheaper than letting the
	// map grow unbounded for the lifetime of the pod.
	if s.now().Sub(s.observedLastReset) > observedSeenResetInterval {
		s.observedSeen = make(map[string]struct{})
		s.observedLastReset = s.now()
	}
	if _, seen := s.observedSeen[key]; seen {
		s.observedMu.Unlock()
		return nil
	}
	s.observedSeen[key] = struct{}{}
	s.observedMu.Unlock()

	if err := s.upsertObserved(ctx, teamID, slackUserID); err != nil {
		// Roll back the dedupe entry so the next call retries the DB.
		s.observedMu.Lock()
		delete(s.observedSeen, key)
		s.observedMu.Unlock()
		return err
	}
	return nil
}

// UpsertObservedProfiles refreshes Slack directory profiles in bulk. It is
// used by the account Slack OAuth callback after users.list returns the
// workspace directory, so Insights can render unlinked Slack users with
// avatar/name/workspace/deep link before those users generate new traces.
//
// Existing rows keep their last_seen_at timestamp; live usage remains the
// responsibility of UpsertObserved. New rows receive the table default for
// first_seen_at/last_seen_at because the table stores both directory and live
// observed identities.
func (s *Store) UpsertObservedProfiles(ctx context.Context, observed []ObservedUser) error {
	if len(observed) == 0 {
		return nil
	}

	teamIDs := make([]string, 0, len(observed))
	slackUserIDs := make([]string, 0, len(observed))
	displayNames := make([]string, 0, len(observed))
	usernames := make([]string, 0, len(observed))
	avatarURLs := make([]string, 0, len(observed))
	isBots := make([]bool, 0, len(observed))
	deleted := make([]bool, 0, len(observed))
	seen := make(map[string]struct{}, len(observed))
	for _, user := range observed {
		if user.TeamID == "" || user.SlackUserID == "" {
			continue
		}
		key := slackIdentityKey(user.TeamID, user.SlackUserID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		teamIDs = append(teamIDs, user.TeamID)
		slackUserIDs = append(slackUserIDs, user.SlackUserID)
		displayNames = append(displayNames, user.Profile.DisplayName)
		usernames = append(usernames, user.Profile.Username)
		avatarURLs = append(avatarURLs, user.Profile.AvatarURL)
		isBots = append(isBots, user.Profile.IsBot)
		deleted = append(deleted, user.Profile.Deleted)
	}
	if len(teamIDs) == 0 {
		return nil
	}

	if _, err := s.db.ExecContext(ctx, `
		WITH input AS (
			SELECT *
			FROM unnest(
				$1::text[],
				$2::text[],
				$3::text[],
				$4::text[],
				$5::text[],
				$6::boolean[],
				$7::boolean[]
			) AS t(team_id, slack_user_id, slack_display_name, slack_username, slack_avatar_url, slack_is_bot, slack_deleted)
		)
		INSERT INTO slack_observed_users
			(team_id, slack_user_id, slack_display_name, slack_username,
			 slack_avatar_url, slack_is_bot, slack_deleted, profile_updated_at)
		SELECT team_id, slack_user_id, slack_display_name, slack_username,
		       slack_avatar_url, slack_is_bot, slack_deleted, now()
		FROM input
		ON CONFLICT (team_id, slack_user_id) DO UPDATE
		SET slack_display_name = EXCLUDED.slack_display_name,
		    slack_username     = EXCLUDED.slack_username,
		    slack_avatar_url   = EXCLUDED.slack_avatar_url,
		    slack_is_bot       = EXCLUDED.slack_is_bot,
		    slack_deleted      = EXCLUDED.slack_deleted,
		    profile_updated_at = now()
	`, pq.Array(teamIDs), pq.Array(slackUserIDs), pq.Array(displayNames),
		pq.Array(usernames), pq.Array(avatarURLs), pq.Array(isBots), pq.Array(deleted)); err != nil {
		return fmt.Errorf("slackidentity: upsert observed profiles: %w", err)
	}
	return nil
}

func (s *Store) upsertObserved(ctx context.Context, teamID, slackUserID string) error {
	if teamID == "" || slackUserID == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO slack_observed_users (team_id, slack_user_id)
		VALUES ($1, $2)
		ON CONFLICT (team_id, slack_user_id) DO UPDATE
		SET last_seen_at = now()
	`, teamID, slackUserID); err != nil {
		return fmt.Errorf("slackidentity: upsert observed: %w", err)
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
// Returns Found=false (with no error) when no active mapping exists —
// the common "user hasn't linked yet" case where the caller falls back
// to the owning-account candidate. Revoked rows are excluded (their
// disconnect was deliberate).
//
// The `AND workos_user_id IS NOT NULL` filter is redundant after the
// PR 3 cleanup worker runs + the follow-up schema migration restores
// the NOT NULL constraint. It's kept here to guard the transition
// window: between the binary deploy and the cleanup-worker completion,
// orphaned observed rows (workos_user_id IS NULL) still exist, and
// Scan into a non-nullable string would fail on a NULL match.
func (s *Store) Lookup(teamID, slackUserID string) (LookupResult, error) {
	var workosUserID string
	err := s.db.QueryRow(`
		SELECT workos_user_id
		FROM slack_identity_mappings
		WHERE team_id = $1 AND slack_user_id = $2
		  AND workos_user_id IS NOT NULL
		  AND revoked_at IS NULL
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

// DirectoryEntry is one resolved directory hit for a Slack user.
// TeamID is always set; WorkOSUserID is set only when the Slack user has linked
// via oauth — observed-only rows leave it empty. The Insights users-summary
// handler uses both: TeamID drives the deep link, WorkOSUserID redirects scoped
// Slack metrics into the linked user's WorkOS row.
type DirectoryEntry struct {
	TeamID           string
	WorkOSUserID     string // empty for observed-only rows
	Profile          SlackProfile
	WorkspaceName    string
	WorkspaceDomain  string
	WorkspaceIconURL string
}

func slackIdentityKey(teamID, slackUserID string) string {
	return teamID + "\x00" + slackUserID
}

// DirectoryEntriesForSlackUserIDs returns directory entries for bare Slack
// user IDs only when the directory contains exactly one workspace for that
// user. It is a conservative fallback for legacy Langfuse rows that do not
// carry team_id: one observed workspace can safely provide a deep link/profile;
// multiple workspaces means the trace row is ambiguous and must remain raw.
func (s *Store) DirectoryEntriesForSlackUserIDs(slackUserIDs []string) (map[string]DirectoryEntry, error) {
	out := make(map[string]DirectoryEntry)
	if len(slackUserIDs) == 0 {
		return out, nil
	}
	input := make([]string, 0, len(slackUserIDs))
	seen := make(map[string]struct{}, len(slackUserIDs))
	for _, slackUserID := range slackUserIDs {
		if slackUserID == "" {
			continue
		}
		if _, ok := seen[slackUserID]; ok {
			continue
		}
		seen[slackUserID] = struct{}{}
		input = append(input, slackUserID)
	}
	if len(input) == 0 {
		return out, nil
	}

	rows, err := s.db.Query(`
			WITH input AS (
				SELECT unnest($1::text[]) AS slack_user_id
			),
			combined AS (
				SELECT m.team_id,
				       m.slack_user_id,
				       COALESCE(CASE WHEN m.revoked_at IS NULL THEN m.workos_user_id END, '') AS workos_user_id,
				       COALESCE(observed_profile.slack_display_name, '')                       AS slack_display_name,
				       COALESCE(NULLIF(observed_profile.slack_username, ''), m.slack_username) AS slack_username,
				       COALESCE(observed_profile.slack_avatar_url, '')                         AS slack_avatar_url,
				       COALESCE(observed_profile.slack_is_bot, FALSE)                          AS slack_is_bot,
				       COALESCE(observed_profile.slack_deleted, FALSE)                         AS slack_deleted,
				       m.team_name,
				       m.team_domain,
				       m.team_icon_url,
				       (m.revoked_at IS NULL)                                                  AS active_flag,
				       m.created_at,
				       1                                                                       AS source_priority
				FROM slack_identity_mappings m
				JOIN input USING (slack_user_id)
				LEFT JOIN slack_observed_users observed_profile
				  ON observed_profile.team_id = m.team_id
				 AND observed_profile.slack_user_id = m.slack_user_id
				UNION ALL
				SELECT o.team_id,
				       o.slack_user_id,
				       ''                                                                      AS workos_user_id,
				       o.slack_display_name,
				       o.slack_username,
				       o.slack_avatar_url,
				       o.slack_is_bot,
				       o.slack_deleted,
				       COALESCE(workspace.team_name, '')                                       AS team_name,
				       COALESCE(workspace.team_domain, '')                                     AS team_domain,
				       COALESCE(workspace.team_icon_url, '')                                   AS team_icon_url,
				       TRUE                                                                    AS active_flag,
				       o.last_seen_at                                                          AS created_at,
				       2                                                                       AS source_priority
				FROM slack_observed_users o
				JOIN input USING (slack_user_id)
				LEFT JOIN LATERAL (
					SELECT team_name, team_domain, team_icon_url
					FROM slack_identity_mappings
					WHERE slack_identity_mappings.team_id = o.team_id
					ORDER BY (revoked_at IS NULL) DESC, updated_at DESC, created_at DESC
					LIMIT 1
				) workspace ON TRUE
			),
			ranked AS (
				SELECT *,
				       row_number() OVER (
				           PARTITION BY team_id, slack_user_id
				           ORDER BY source_priority, active_flag DESC, created_at DESC
				       ) AS rn
				FROM combined
			),
			deduped AS (
				SELECT *
				FROM ranked
				WHERE rn = 1
			),
			unambiguous AS (
				SELECT slack_user_id
				FROM deduped
				GROUP BY slack_user_id
				HAVING COUNT(DISTINCT team_id) = 1
			)
			SELECT d.team_id,
			       d.slack_user_id,
			       d.workos_user_id,
			       d.slack_display_name,
			       d.slack_username,
			       d.slack_avatar_url,
			       d.slack_is_bot,
			       d.slack_deleted,
			       d.team_name,
			       d.team_domain,
			       d.team_icon_url
			FROM deduped d
			JOIN unambiguous USING (slack_user_id)
			ORDER BY d.slack_user_id, d.team_id
		`, pq.Array(input))
	if err != nil {
		return nil, fmt.Errorf("slackidentity: unscoped directory entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var slackUserID, teamID, workosUserID, workspaceName, workspaceDomain, workspaceIconURL string
		var profile SlackProfile
		if err := rows.Scan(
			&teamID,
			&slackUserID,
			&workosUserID,
			&profile.DisplayName,
			&profile.Username,
			&profile.AvatarURL,
			&profile.IsBot,
			&profile.Deleted,
			&workspaceName,
			&workspaceDomain,
			&workspaceIconURL,
		); err != nil {
			return nil, fmt.Errorf("slackidentity: unscoped directory scan: %w", err)
		}
		out[slackUserID] = DirectoryEntry{
			TeamID:           teamID,
			WorkOSUserID:     workosUserID,
			Profile:          profile,
			WorkspaceName:    workspaceName,
			WorkspaceDomain:  workspaceDomain,
			WorkspaceIconURL: workspaceIconURL,
		}
	}
	return out, rows.Err()
}

// ListByWorkOSUser returns all active mappings for a WorkOS user. Used by
// the "Connect Slack" settings panel to render the user's linked
// workspaces. Returns an empty slice (no error) when the user has linked
// nothing.
func (s *Store) ListByWorkOSUser(workosUserID string) ([]Mapping, error) {
	rows, err := s.db.Query(`
		SELECT team_id, slack_user_id, workos_user_id,
		       COALESCE(organization_id, ''),
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
			&m.OrganizationID,
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
		       COALESCE(organization_id, ''),
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
			&m.OrganizationID,
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
