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

const (
	// SourceOAuth marks a mapping created via the raw Slack OAuth link
	// flow — the user authorized via Pipes and the row carries a
	// workos_user_id.
	SourceOAuth = "oauth"

	// SourceObserved marks a directory-only row written by the live-ingest
	// path on /authorize. The row records that we've seen this
	// (team_id, slack_user_id) pair, but the user hasn't linked a WorkOS
	// account — so workos_user_id stays NULL. Used by Insights to attach
	// team_id to bare-form Langfuse userIds for the Slack deep link.
	SourceObserved = "observed"
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

// UpsertObserved records that the server has seen (team_id, slack_user_id)
// via the /authorize live-ingest path. It writes a directory-only row
// (source='observed', workos_user_id=NULL); if a row already exists for this
// pair — observed OR oauth-linked — the call is a no-op. Idempotent.
//
// Used by Insights to attach team_id to bare-form Langfuse userIds so the
// Slack profile deep link works for every Slack row, not just the ones who
// happen to have linked their identity.
//
// Per-process in-memory dedupe (observedSeen) eliminates the steady-state
// DB write on every authorize call: a chatty workspace only touches Postgres
// once per (team, user) pair per pod lifetime. Across pods or restarts, the
// SQL UPSERT is still ON CONFLICT safe so we never get errors — just one
// extra write per restart per unique user.
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

	// ON CONFLICT: an active row (any source) is left alone; a revoked
	// observed row is revived in-place; a revoked oauth row stays
	// revoked. Revocation of an oauth row is a deliberate user action
	// ("I disconnected my Slack from Astro") — silently reviving it on
	// the next message would undo their choice and re-attribute their
	// messages to their old account. The observed-source guard scopes
	// the revival to the rollback path (ops UPDATE … SET revoked_at =
	// now() WHERE source = 'observed') without touching oauth state.
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO slack_identity_mappings
			(team_id, slack_user_id, workos_user_id, source)
		VALUES ($1, $2, NULL, 'observed')
		ON CONFLICT (team_id, slack_user_id) DO UPDATE
		SET revoked_at = NULL,
		    updated_at = now()
		WHERE slack_identity_mappings.revoked_at IS NOT NULL
		  AND slack_identity_mappings.source     = 'observed'
	`, teamID, slackUserID)
	if err != nil {
		// Roll back the dedupe entry so the next call retries the DB.
		s.observedMu.Lock()
		delete(s.observedSeen, key)
		s.observedMu.Unlock()
		return fmt.Errorf("slackidentity: upsert observed: %w", err)
	}

	// PR 1 dual-write: also populate slack_observed_users. The legacy
	// write above is the read path for Insights today; this new table
	// becomes the read path in PR 2. A failure here doesn't unwind the
	// dedupe — the legacy write succeeded, so this pair won't retry on
	// the next call. Gaps are picked up by the one-shot port worker
	// before PR 2's read switch.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO slack_observed_users (team_id, slack_user_id)
		VALUES ($1, $2)
		ON CONFLICT (team_id, slack_user_id) DO UPDATE
		SET last_seen_at = now()
	`, teamID, slackUserID); err != nil {
		return fmt.Errorf("slackidentity: upsert observed user (new table): %w", err)
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
// Returns Found=false (with no error) when no active LINKED mapping exists;
// that's the common "user hasn't linked yet" case and the caller falls back
// to the existing owning-account candidate.
//
// Observed-only rows (source='observed', workos_user_id IS NULL) are
// excluded — those exist for directory join purposes only, not for
// identity resolution. Treating them as a Found=true with empty WorkOSUserID
// would incorrectly flow through the "linked" branch in /authorize.
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

// AccountTeam pairs an Astro account with one of its connected Slack
// workspaces, derived from any active linked (oauth) mapping for a member
// of that account. Used by the one-shot directory backfill worker to
// know which (team_id) to stamp on the account's bare-form Slack
// userIds.
type AccountTeam struct {
	AccountID string
	TeamID    string
}

// ListLinkedAccountTeams returns one row per (account_id, team_id) pair
// observable in slack_identity_mappings — the workspaces that any
// member of each account has linked via oauth. Used by the directory
// backfill worker to seed observed-only rows for historical Slack users
// in accounts that have at least one linked member to derive team_id
// from. Accounts with zero linked Slack members don't appear and can't
// be backfilled this way — they'd need a separate workspace-discovery
// path.
func (s *Store) ListLinkedAccountTeams() ([]AccountTeam, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT am.account_id, sim.team_id
		FROM slack_identity_mappings sim
		JOIN account_members am ON sim.workos_user_id = am.user_id
		WHERE sim.workos_user_id IS NOT NULL
		  AND sim.revoked_at IS NULL
		ORDER BY am.account_id, sim.team_id
	`)
	if err != nil {
		return nil, fmt.Errorf("slackidentity: list linked account teams: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AccountTeam
	for rows.Next() {
		var at AccountTeam
		if err := rows.Scan(&at.AccountID, &at.TeamID); err != nil {
			return nil, fmt.Errorf("slackidentity: scan account team: %w", err)
		}
		out = append(out, at)
	}
	return out, rows.Err()
}

// IsDirectoryBackfillComplete returns true if the one-shot backfill
// marker row exists. The worker checks this on entry and exits
// immediately if true, guaranteeing the work runs at most once per
// environment regardless of how many times River enqueues the job.
func (s *Store) IsDirectoryBackfillComplete(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM slack_directory_backfill_marker)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("slackidentity: check backfill marker: %w", err)
	}
	return exists, nil
}

// MarkDirectoryBackfillComplete writes the singleton marker row after a
// successful backfill. ON CONFLICT DO NOTHING is paranoia — the CHECK
// constraint (id = 1) plus PRIMARY KEY makes the table physically
// single-row, so concurrent writes are bounded.
func (s *Store) MarkDirectoryBackfillComplete(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO slack_directory_backfill_marker (id, completed_at)
		VALUES (1, now())
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("slackidentity: mark backfill complete: %w", err)
	}
	return nil
}

// IsObservedPortComplete reports whether the one-shot port from
// slack_identity_mappings (observed source) into slack_observed_users
// has completed for this environment. Same gating pattern as the
// directory backfill marker.
func (s *Store) IsObservedPortComplete(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM slack_observed_port_marker)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("slackidentity: check observed port marker: %w", err)
	}
	return exists, nil
}

// MarkObservedPortComplete writes the singleton port marker row after a
// successful one-shot copy. CHECK (id = 1) plus PRIMARY KEY enforces
// single-row physically.
func (s *Store) MarkObservedPortComplete(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO slack_observed_port_marker (id, completed_at)
		VALUES (1, now())
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("slackidentity: mark observed port complete: %w", err)
	}
	return nil
}

// PortObservedRowsToNewTable copies every active observed row out of
// slack_identity_mappings and into slack_observed_users. Idempotent:
// ON CONFLICT DO NOTHING leaves existing rows alone (live-ingest dual-
// writes may have already populated some pairs). Returns the number of
// rows inserted. Intended to be called exactly once per environment from
// the one-shot port worker.
func (s *Store) PortObservedRowsToNewTable(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO slack_observed_users (team_id, slack_user_id, first_seen_at, last_seen_at)
		SELECT team_id, slack_user_id, created_at, updated_at
		FROM slack_identity_mappings
		WHERE source = 'observed' AND revoked_at IS NULL
		ON CONFLICT (team_id, slack_user_id) DO NOTHING
	`)
	if err != nil {
		return 0, fmt.Errorf("slackidentity: port observed rows: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("slackidentity: port observed rows affected: %w", err)
	}
	return n, nil
}

// DirectoryEntry is one resolved directory hit per Slack user. TeamID is
// always set; WorkOSUserID is set only when the Slack user has linked via
// oauth — observed-only rows leave it empty. The Insights users-summary
// handler uses both: TeamID drives the deep link, WorkOSUserID redirects
// historical bare-Slack metrics into the linked user's WorkOS row so Bob's
// pre-link and post-link spend roll up under one Insights row instead of
// splitting.
type DirectoryEntry struct {
	TeamID       string
	WorkOSUserID string // empty for observed-only rows
}

// DirectoryEntriesForSlackUsers returns the directory entry for each
// slack_user_id the directory knows about — observed-only rows
// (workos_user_id IS NULL), linked rows (workos_user_id IS NOT NULL),
// AND revoked rows (team_id only; workos_user_id forced empty).
//
// Revoked rows return their team_id so Insights can still build the
// `slack://user?team=…&id=…` deep link for users who linked + then
// disconnected. workos_user_id is masked to empty for revoked rows so
// the metrics-merge path in mergeLinkedSlackRows doesn't fold their
// new (post-disconnect) spend back into the WorkOS account they
// deliberately unlinked from. Without this, a previously-linked-then-
// disconnected Slack user permanently loses their deep link, because
// the unique constraint (team_id, slack_user_id) prevents live-ingest
// from creating a fresh observed row alongside the revoked oauth one.
//
// DISTINCT ON (slack_user_id) collapses to one row per Slack user.
// ORDER BY prefers non-revoked rows so an active observed/oauth row
// wins over a revoked one when both exist (only possible across
// multiple team_ids — within a team the unique constraint forbids it).
// Within revoked-or-not, most recently created wins.
//
// Multi-workspace caveat unchanged: same `U07ABCDEF` across two
// different Slack workspaces collapses to one entry, the most recent.
func (s *Store) DirectoryEntriesForSlackUsers(slackUserIDs []string) (map[string]DirectoryEntry, error) {
	out := make(map[string]DirectoryEntry)
	if len(slackUserIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(`
		SELECT DISTINCT ON (slack_user_id)
		       slack_user_id,
		       team_id,
		       COALESCE(CASE WHEN revoked_at IS NULL THEN workos_user_id END, '')
		FROM slack_identity_mappings
		WHERE slack_user_id = ANY($1)
		ORDER BY slack_user_id, (revoked_at IS NULL) DESC, created_at DESC
	`, pq.Array(slackUserIDs))
	if err != nil {
		return nil, fmt.Errorf("slackidentity: directory entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var slackUserID, teamID, workosUserID string
		if err := rows.Scan(&slackUserID, &teamID, &workosUserID); err != nil {
			return nil, fmt.Errorf("slackidentity: directory scan: %w", err)
		}
		out[slackUserID] = DirectoryEntry{TeamID: teamID, WorkOSUserID: workosUserID}
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
