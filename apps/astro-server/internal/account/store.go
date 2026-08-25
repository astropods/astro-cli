package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
)

// ErrAlreadyDeleted is returned when MarkDeleted targets an account that is
// already soft-deleted (or does not exist).
var ErrAlreadyDeleted = errors.New("account not found or already deleted")

// ErrAccountNotFound is returned by GetByID and by provider-ID reverse lookups
// when no live account matches. Callers use errors.Is to distinguish a deleted
// account, which no retry can resurrect, from a transient DB error.
var ErrAccountNotFound = errors.New("account not found")

// AccountStore manages account persistence in PostgreSQL
type AccountStore struct {
	db       *sql.DB
	clusters clusterid.Resolver
}

// NewAccountStore creates a new account store with the given database connection
func NewAccountStore(db *sql.DB) *AccountStore {
	return &AccountStore{db: db}
}

// NewAccountStoreWithClusters returns a store that binds every account it
// creates to the primary cluster, so the binding set exists before anything
// reads it. The zero Resolver carries no primary, which is the local mode with
// no cluster config, and binds nothing.
func NewAccountStoreWithClusters(db *sql.DB, clusters clusterid.Resolver) *AccountStore {
	return &AccountStore{db: db, clusters: clusters}
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
		INSERT INTO accounts (name, type, display_name, owner_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, type, display_name, created_at, updated_at
	`, name, accountType, displayName, ownerUserID, now, now).Scan(
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

	// Seed the profile row so account_number is assigned at registration time
	_, err = tx.Exec(`
		INSERT INTO account_profile (account_id, social_links)
		VALUES ($1, '{}')
		ON CONFLICT (account_id) DO NOTHING
	`, acct.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to seed account profile: %w", err)
	}

	if err := BindPrimary(tx, acct.ID, s.clusters.Primary()); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return &acct, nil
}

func (s *AccountStore) OwnerUserID(accountID string) (string, error) {
	var owner string
	err := s.db.QueryRow(`SELECT owner_user_id FROM accounts WHERE id = $1`, accountID).Scan(&owner)
	if err != nil {
		return "", fmt.Errorf("failed to get account owner: %w", err)
	}
	return owner, nil
}

func (s *AccountStore) SetOwner(accountID, userID string) error {
	_, err := s.db.Exec(`
		UPDATE accounts SET owner_user_id = $1, updated_at = now() WHERE id = $2
	`, userID, accountID)
	if err != nil {
		return fmt.Errorf("failed to set account owner: %w", err)
	}
	return nil
}

func (s *AccountStore) ReplaceOwner(accountID, previousUserID, userID string) error {
	_, err := s.db.Exec(`
		UPDATE accounts SET owner_user_id = $1, updated_at = now()
		 WHERE id = $2 AND owner_user_id = $3
	`, userID, accountID, previousUserID)
	if err != nil {
		return fmt.Errorf("failed to replace account owner: %w", err)
	}
	return nil
}

// scanAccount scans an account row with the workos_org_id and deleted_at columns.
func scanAccount(row interface{ Scan(...any) error }) (*Account, error) {
	var acct Account
	var workosOrgID sql.NullString
	var deletedAt sql.NullTime
	var avatarUpdatedAt sql.NullTime
	var accountNumber sql.NullInt32
	var bio, location, localTimezone, pronouns, website sql.NullString
	var socialLinks, blueprintOrder pq.StringArray
	err := row.Scan(
		&acct.ID, &acct.Name, &acct.Type, &workosOrgID, &deletedAt,
		&acct.CreatedAt, &acct.UpdatedAt, &acct.DisplayName, &acct.AvatarColors, &avatarUpdatedAt,
		&accountNumber, &bio, &location, &localTimezone, &pronouns, &website, &socialLinks, &blueprintOrder,
	)
	if err != nil {
		return nil, err
	}
	if avatarUpdatedAt.Valid {
		acct.AvatarUpdatedAt = &avatarUpdatedAt.Time
	}
	if workosOrgID.Valid {
		acct.WorkOSOrganizationID = workosOrgID.String
	}
	if deletedAt.Valid {
		acct.DeletedAt = &deletedAt.Time
	}
	if accountNumber.Valid {
		n := int(accountNumber.Int32)
		acct.AccountNumber = &n
	}
	acct.Bio = bio.String
	acct.Location = location.String
	acct.LocalTimezone = localTimezone.String
	acct.Pronouns = pronouns.String
	acct.Website = website.String
	if socialLinks != nil {
		acct.SocialLinks = []string(socialLinks)
	} else {
		acct.SocialLinks = []string{}
	}
	if blueprintOrder != nil {
		acct.BlueprintOrder = []string(blueprintOrder)
	} else {
		acct.BlueprintOrder = []string{}
	}
	return &acct, nil
}

// GetByName retrieves an account by its unique name
func (s *AccountStore) GetByName(name string) (*Account, error) {
	acct, err := scanAccount(s.db.QueryRow(`
		SELECT a.id, a.name, a.type, ao.workos_org_id, a.deleted_at, a.created_at, a.updated_at, a.display_name, a.avatar_colors, a.avatar_updated_at,
		       ap.account_number, ap.bio, ap.location, ap.local_timezone, ap.pronouns, ap.website, COALESCE(ap.social_links, '{}'), COALESCE(ap.blueprint_order, '{}')
		FROM accounts a
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		LEFT JOIN account_profile ap ON ap.account_id = a.id
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
		SELECT a.id, a.name, a.type, ao.workos_org_id, a.deleted_at, a.created_at, a.updated_at, a.display_name, a.avatar_colors, a.avatar_updated_at,
		       ap.account_number, ap.bio, ap.location, ap.local_timezone, ap.pronouns, ap.website, COALESCE(ap.social_links, '{}'), COALESCE(ap.blueprint_order, '{}')
		FROM accounts a
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		LEFT JOIN account_profile ap ON ap.account_id = a.id
		WHERE a.id = $1 AND a.deleted_at IS NULL
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrAccountNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}
	return acct, nil
}

// GetByWorkOSOrganizationID retrieves an account linked to a WorkOS organization.
func (s *AccountStore) GetByWorkOSOrganizationID(orgID string) (*Account, error) {
	acct, err := scanAccount(s.db.QueryRow(`
		SELECT a.id, a.name, a.type, ao.workos_org_id, a.deleted_at, a.created_at, a.updated_at, a.display_name, a.avatar_colors, a.avatar_updated_at,
		       ap.account_number, ap.bio, ap.location, ap.local_timezone, ap.pronouns, ap.website, COALESCE(ap.social_links, '{}'), COALESCE(ap.blueprint_order, '{}')
		FROM accounts a
		JOIN account_organizations ao ON ao.account_id = a.id
		LEFT JOIN account_profile ap ON ap.account_id = a.id
		WHERE ao.workos_org_id = $1
	`, orgID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("account not found for workos org %q: %w", orgID, ErrAccountNotFound)
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
	return s.GetAccountsForUserContext(context.Background(), userID)
}

// GetAccountsForUserContext is the cancellable form used by request hot paths.
func (s *AccountStore) GetAccountsForUserContext(ctx context.Context, userID string) ([]AccountWithRole, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.name, a.type, COALESCE(ao.workos_org_id, ''), a.created_at, a.updated_at, a.display_name, a.avatar_updated_at
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
		var avatarUpdatedAt sql.NullTime
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.WorkOSOrganizationID, &a.CreatedAt, &a.UpdatedAt, &a.DisplayName, &avatarUpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		if avatarUpdatedAt.Valid {
			a.AvatarUpdatedAt = &avatarUpdatedAt.Time
		}
		accounts = append(accounts, a)
	}

	return accounts, nil
}

// PersonalProfile holds the name, display name, and avatar version stamp from a
// user's personal account.
type PersonalProfile struct {
	UserID          string
	Name            string
	DisplayName     string
	AvatarUpdatedAt *time.Time
}

// GetPersonalProfiles returns the personal-account name, display name, and
// avatar version stamp for each of the given user IDs in a single query.
func (s *AccountStore) GetPersonalProfiles(userIDs []string) (map[string]PersonalProfile, error) {
	if len(userIDs) == 0 {
		return map[string]PersonalProfile{}, nil
	}
	rows, err := s.db.Query(`
		SELECT am.user_id, a.name, a.display_name, a.avatar_updated_at
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
		var (
			p               PersonalProfile
			avatarUpdatedAt sql.NullTime
		)
		if err := rows.Scan(&p.UserID, &p.Name, &p.DisplayName, &avatarUpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan personal profile: %w", err)
		}
		if avatarUpdatedAt.Valid {
			p.AvatarUpdatedAt = &avatarUpdatedAt.Time
		}
		profiles[p.UserID] = p
	}
	return profiles, nil
}

// DisplayNamesForUsers returns user_id → personal display name for the given
// users, for populating notification subscriber names. It prefers the personal
// display name and falls back to the account handle; users with neither (or no
// personal account) are omitted.
func (s *AccountStore) DisplayNamesForUsers(userIDs []string) (map[string]string, error) {
	profiles, err := s.GetPersonalProfiles(userIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(profiles))
	for uid, p := range profiles {
		if name := p.DisplayName; name != "" {
			out[uid] = name
		} else if p.Name != "" {
			out[uid] = p.Name
		}
	}
	return out, nil
}

// IsMember checks if a user is a member of an account
func (s *AccountStore) IsMember(accountID, userID string) (bool, error) {
	return s.IsMemberContext(context.Background(), accountID, userID)
}

// IsMemberContext checks membership while honoring caller cancellation.
func (s *AccountStore) IsMemberContext(ctx context.Context, accountID, userID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
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

// UpdateProfile upserts extended public profile fields for an account atomically.
// If displayName is non-empty it is also applied to the accounts table; an empty
// string leaves the existing display_name unchanged.
// UpdateProfile updates the account's display name and profile fields.
// Pointer fields use PATCH semantics: nil = leave unchanged, non-nil = set (empty string clears the field).
func (s *AccountStore) UpdateProfile(accountID, displayName string, bio, location, localTimezone, pronouns, website *string, socialLinks *[]string, blueprintOrder *[]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if displayName != "" {
		result, err := tx.Exec(`
			UPDATE accounts SET display_name = $1, updated_at = $2 WHERE id = $3
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
	} else {
		// Verify account exists even when display_name is not being changed.
		var cnt int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM accounts WHERE id = $1 AND deleted_at IS NULL`, accountID,
		).Scan(&cnt); err != nil {
			return fmt.Errorf("failed to verify account: %w", err)
		}
		if cnt == 0 {
			return fmt.Errorf("account not found: %s", accountID)
		}
	}

	// Skip profile upsert entirely if no profile fields were provided.
	if bio == nil && location == nil && localTimezone == nil && pronouns == nil && website == nil && socialLinks == nil && blueprintOrder == nil {
		return tx.Commit()
	}

	sl := []string{}
	if socialLinks != nil {
		sl = *socialLinks
	}
	bo := []string{}
	if blueprintOrder != nil {
		bo = *blueprintOrder
	}

	// CASE WHEN flags ensure only explicitly provided fields overwrite existing values.
	if _, err := tx.Exec(`
		INSERT INTO account_profile (account_id, bio, location, local_timezone, pronouns, website, social_links, blueprint_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (account_id) DO UPDATE SET
			bio             = CASE WHEN $9  THEN EXCLUDED.bio             ELSE account_profile.bio             END,
			location        = CASE WHEN $10 THEN EXCLUDED.location        ELSE account_profile.location        END,
			local_timezone  = CASE WHEN $11 THEN EXCLUDED.local_timezone  ELSE account_profile.local_timezone  END,
			pronouns        = CASE WHEN $12 THEN EXCLUDED.pronouns        ELSE account_profile.pronouns        END,
			website         = CASE WHEN $13 THEN EXCLUDED.website         ELSE account_profile.website         END,
			social_links    = CASE WHEN $14 THEN EXCLUDED.social_links    ELSE account_profile.social_links    END,
			blueprint_order = CASE WHEN $15 THEN EXCLUDED.blueprint_order ELSE account_profile.blueprint_order END
	`, accountID,
		nullablePtrStr(bio), nullablePtrStr(location), nullablePtrStr(localTimezone), nullablePtrStr(pronouns), nullablePtrStr(website),
		pq.Array(sl), pq.Array(bo),
		bio != nil, location != nil, localTimezone != nil, pronouns != nil, website != nil, socialLinks != nil, blueprintOrder != nil,
	); err != nil {
		return fmt.Errorf("failed to upsert profile: %w", err)
	}

	return tx.Commit()
}

// nullableStr converts an empty string to a SQL NULL.
func nullableStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// nullablePtrStr converts a *string to a SQL NullString.
// nil → SQL NULL; non-nil empty string → SQL NULL; non-nil non-empty → the value.
func nullablePtrStr(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{Valid: false}
	}
	return nullableStr(*s)
}

// GetOrgAccountsForUser returns all organization accounts the given user belongs to.
func (s *AccountStore) GetOrgAccountsForUser(userID string) ([]Account, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.name, a.type, COALESCE(ao.workos_org_id, ''), NULL, a.created_at, a.updated_at, a.display_name, a.avatar_colors, a.avatar_updated_at,
		       ap.account_number, ap.bio, ap.location, ap.local_timezone, ap.pronouns, ap.website, COALESCE(ap.social_links, '{}'), COALESCE(ap.blueprint_order, '{}')
		FROM accounts a
		JOIN account_members am ON am.account_id = a.id
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		LEFT JOIN account_profile ap ON ap.account_id = a.id
		WHERE am.user_id = $1 AND a.type = 'organization' AND a.deleted_at IS NULL
		ORDER BY a.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query org accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var orgs []Account
	for rows.Next() {
		acct, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan org account: %w", err)
		}
		orgs = append(orgs, *acct)
	}
	return orgs, rows.Err()
}

// SetAvatarColors stores the extracted avatar color scheme for an account.
func (s *AccountStore) SetAvatarColors(accountID string, colorsJSON []byte) error {
	_, err := s.db.Exec(`
		UPDATE accounts SET avatar_colors = $1, updated_at = now()
		WHERE id = $2
	`, colorsJSON, accountID)
	return err
}

// TouchAvatarUpdatedAt stamps avatar_updated_at = now() for an account, bumping
// the cache-busting token on its avatar URL, and returns the persisted value so
// callers embed the DB clock (not the app clock) in the immediate response.
// Called after every avatar write.
func (s *AccountStore) TouchAvatarUpdatedAt(accountID string) (time.Time, error) {
	var ts time.Time
	err := s.db.QueryRow(`
		UPDATE accounts SET avatar_updated_at = now()
		WHERE id = $1
		RETURNING avatar_updated_at
	`, accountID).Scan(&ts)
	return ts, err
}

// TouchAvatarUpdatedAtByName stamps avatar_updated_at = now() for an account by
// name and returns the persisted value. Used by avatar writes that only carry
// the account handle.
func (s *AccountStore) TouchAvatarUpdatedAtByName(name string) (time.Time, error) {
	var ts time.Time
	err := s.db.QueryRow(`
		UPDATE accounts SET avatar_updated_at = now()
		WHERE name = $1
		RETURNING avatar_updated_at
	`, name).Scan(&ts)
	return ts, err
}

// AccountScope returns an account's name and type. It reads two columns instead
// of reusing GetByID, which joins the profile and organization tables to answer
// a question about neither.
func (s *AccountStore) AccountScope(accountID string) (name, accountType string, err error) {
	err = s.db.QueryRow(`
		SELECT name, type FROM accounts
		WHERE id = $1 AND deleted_at IS NULL
	`, accountID).Scan(&name, &accountType)

	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("account not found: %s", accountID)
	}
	if err != nil {
		return "", "", fmt.Errorf("failed to query account scope: %w", err)
	}
	return name, accountType, nil
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
	return s.GetMemberContext(context.Background(), accountID, userID)
}

// GetMemberContext retrieves a single account member with request cancellation.
func (s *AccountStore) GetMemberContext(ctx context.Context, accountID, userID string) (*AccountMember, error) {
	var m AccountMember
	var wid sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT am.account_id, am.user_id, mw.workos_membership_id, am.created_at
		FROM account_members am
		LEFT JOIN account_member_workos mw ON mw.account_id = am.account_id AND mw.user_id = am.user_id
		WHERE am.account_id = $1 AND am.user_id = $2
	`, accountID, userID).Scan(&m.AccountID, &m.UserID, &wid, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("member not found for account %s and user %s: %w", accountID, userID, sql.ErrNoRows)
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

// GetMembersByWorkosMembershipIDsContext resolves WorkOS memberships in one query.
func (s *AccountStore) GetMembersByWorkosMembershipIDsContext(ctx context.Context, membershipIDs []string) (map[string]*AccountMember, error) {
	members := make(map[string]*AccountMember, len(membershipIDs))
	if len(membershipIDs) == 0 {
		return members, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT am.account_id, am.user_id, mw.workos_membership_id, am.created_at
		FROM account_member_workos mw
		JOIN account_members am ON am.account_id = mw.account_id AND am.user_id = mw.user_id
		WHERE mw.workos_membership_id = ANY($1)
	`, pq.Array(membershipIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query members: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	for rows.Next() {
		var member AccountMember
		if err := rows.Scan(&member.AccountID, &member.UserID, &member.WorkOSMembershipID, &member.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members[member.WorkOSMembershipID] = &member
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate members: %w", err)
	}
	return members, nil
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
			SELECT a.id, a.name, a.type, a.created_at, a.updated_at, a.avatar_updated_at
			FROM accounts a
			WHERE a.name LIKE $1 AND a.type = $2
			ORDER BY a.name
			LIMIT $3
		`, pattern, accountType, limit)
	} else {
		rows, err = s.db.Query(`
			SELECT a.id, a.name, a.type, a.created_at, a.updated_at, a.avatar_updated_at
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
		var (
			a               Account
			avatarUpdatedAt sql.NullTime
		)
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.CreatedAt, &a.UpdatedAt, &avatarUpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		if avatarUpdatedAt.Valid {
			a.AvatarUpdatedAt = &avatarUpdatedAt.Time
		}
		accounts = append(accounts, a)
	}

	return accounts, nil
}

// GetBifrostCustomerID returns the linked Bifrost (AI Gateway) customer ID for
// an account, or "" if unset. The account is the Bifrost customer; per-account
// budget lives on that customer and its virtual keys inherit it.
func (s *AccountStore) GetBifrostCustomerID(accountID string) (string, error) {
	var customerID sql.NullString
	err := s.db.QueryRow(`
		SELECT bifrost_customer_id FROM accounts WHERE id = $1
	`, accountID).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("account not found: %s", accountID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get bifrost_customer_id: %w", err)
	}
	if !customerID.Valid {
		return "", nil
	}
	return customerID.String, nil
}

// SetBifrostCustomerID stores the Bifrost customer ID for an account.
func (s *AccountStore) SetBifrostCustomerID(accountID, customerID string) error {
	_, err := s.db.Exec(`
		UPDATE accounts SET bifrost_customer_id = $1, updated_at = $2
		WHERE id = $3
	`, customerID, time.Now(), accountID)
	if err != nil {
		return fmt.Errorf("failed to set bifrost_customer_id: %w", err)
	}
	return nil
}

// ListStaleGatewayBudgetAccounts returns the accounts whose AI gateway ceiling
// was applied longest ago, never-swept first.
//
// Ordering by staleness is what makes a bounded sweep cover everything: an
// account left out of one tick becomes staler and sorts earlier in the next.
// Ordering by id instead would mean the accounts past the bound are swept never
// rather than late, because every tick would restart from the same end.
func (s *AccountStore) ListStaleGatewayBudgetAccounts(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM accounts
		 WHERE bifrost_customer_id IS NOT NULL AND bifrost_customer_id <> ''
		   AND deleted_at IS NULL
		 ORDER BY gateway_budget_swept_at ASC NULLS FIRST
		 LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query stale gateway budget accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan account id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MarkGatewayBudgetSwept stamps an attempt to apply the account's gateway
// ceiling. Called after a failure as well, so one unreachable account cannot
// hold the front of the sweep's worklist and starve the rest.
func (s *AccountStore) MarkGatewayBudgetSwept(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET gateway_budget_swept_at = now() WHERE id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("failed to stamp gateway_budget_swept_at: %w", err)
	}
	return nil
}

// billingCustomerColumns whitelists the DB column holding the provider customer
// ID for each billing backend. Backends without customer records (e.g. noop) are
// absent, so the generic accessors below no-op for them.
var billingCustomerColumns = map[string]string{
	"metronome": "metronome_customer_id",
}

// GetBillingCustomerID returns the provider customer ID for an account under the
// given billing backend, or "" if unset or the backend keeps no customers.
func (s *AccountStore) GetBillingCustomerID(accountID, backend string) (string, error) {
	col, ok := billingCustomerColumns[backend]
	if !ok {
		return "", nil
	}
	var customerID sql.NullString
	// #nosec G201 -- col is from the billingCustomerColumns whitelist, not user input.
	err := s.db.QueryRow("SELECT "+col+" FROM accounts WHERE id = $1", accountID).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("account not found: %s", accountID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get %s: %w", col, err)
	}
	if !customerID.Valid {
		return "", nil
	}
	return customerID.String, nil
}

// SetBillingCustomerID stores the provider customer ID for an account under the
// given billing backend. No-ops for backends without customer records.
func (s *AccountStore) SetBillingCustomerID(accountID, backend, customerID string) error {
	col, ok := billingCustomerColumns[backend]
	if !ok {
		return nil
	}
	// #nosec G201 -- col is from the billingCustomerColumns whitelist, not user input.
	_, err := s.db.Exec("UPDATE accounts SET "+col+" = $1, updated_at = $2 WHERE id = $3", customerID, time.Now(), accountID)
	if err != nil {
		return fmt.Errorf("failed to set %s: %w", col, err)
	}
	return nil
}

// GetAccountsMissingBillingCustomer returns accounts without a provider customer
// under the given backend. Returns nil for backends without customer records.
func (s *AccountStore) GetAccountsMissingBillingCustomer(backend string, limit int) ([]Account, error) {
	col, ok := billingCustomerColumns[backend]
	if !ok {
		return nil, nil
	}
	// #nosec G201 -- col is from the billingCustomerColumns whitelist, not user input.
	rows, err := s.db.Query("SELECT id, name, type, created_at, updated_at FROM accounts WHERE "+col+" IS NULL ORDER BY created_at LIMIT $1", limit)
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

// GetAccountsPendingBillingProvision returns live accounts not yet put on the
// rate card, oldest first.
func (s *AccountStore) GetAccountsPendingBillingProvision(limit int) ([]Account, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, created_at, updated_at
		FROM accounts
		WHERE billing_provisioned_at IS NULL AND deleted_at IS NULL
		ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query accounts pending billing provision: %w", err)
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
	return accounts, rows.Err()
}

// IsBillingProvisioned reports whether the account has already been stamped.
func (s *AccountStore) IsBillingProvisioned(accountID string) (bool, error) {
	var at sql.NullTime
	err := s.db.QueryRow(`SELECT billing_provisioned_at FROM accounts WHERE id = $1`, accountID).Scan(&at)
	if err != nil {
		return false, fmt.Errorf("failed to read billing provisioned stamp: %w", err)
	}
	return at.Valid, nil
}

// MarkBillingProvisioned stamps an account as provisioned.
func (s *AccountStore) MarkBillingProvisioned(accountID string) error {
	_, err := s.db.Exec(`UPDATE accounts SET billing_provisioned_at = now() WHERE id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("failed to mark billing provisioned: %w", err)
	}
	return nil
}

// GetMetronomeCustomerID returns the linked Metronome customer ID for an account, or "" if unset.
func (s *AccountStore) GetMetronomeCustomerID(accountID string) (string, error) {
	var customerID sql.NullString
	err := s.db.QueryRow(`
		SELECT metronome_customer_id FROM accounts WHERE id = $1
	`, accountID).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("account not found: %s", accountID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get metronome_customer_id: %w", err)
	}
	if !customerID.Valid {
		return "", nil
	}
	return customerID.String, nil
}

// GetByMetronomeCustomerID resolves the account linked to a Metronome customer
// ID. Used by the Metronome webhook to map an inbound event back to an account.
func (s *AccountStore) GetByMetronomeCustomerID(customerID string) (*Account, error) {
	var a Account
	err := s.db.QueryRow(`
		SELECT id, name, type, created_at, updated_at
		FROM accounts WHERE metronome_customer_id = $1 AND deleted_at IS NULL
	`, customerID).Scan(&a.ID, &a.Name, &a.Type, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("metronome customer %s: %w", customerID, ErrAccountNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account by metronome_customer_id: %w", err)
	}
	return &a, nil
}

// SetMetronomeCustomerID stores the Metronome customer ID for an account.
func (s *AccountStore) SetMetronomeCustomerID(accountID, customerID string) error {
	_, err := s.db.Exec(`
		UPDATE accounts SET metronome_customer_id = $1, updated_at = $2
		WHERE id = $3
	`, customerID, time.Now(), accountID)
	if err != nil {
		return fmt.Errorf("failed to set metronome_customer_id: %w", err)
	}
	return nil
}

// GetAccountsMissingMetronomeCustomer returns accounts that don't have a Metronome customer yet.
func (s *AccountStore) GetAccountsMissingMetronomeCustomer(limit int) ([]Account, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, created_at, updated_at
		FROM accounts
		WHERE metronome_customer_id IS NULL
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

// GetStripeCustomerID returns the account's Stripe customer ID, or "" if unset.
func (s *AccountStore) GetStripeCustomerID(accountID string) (string, error) {
	var customerID sql.NullString
	err := s.db.QueryRow(`SELECT stripe_customer_id FROM accounts WHERE id = $1`, accountID).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("account not found: %s", accountID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get stripe_customer_id: %w", err)
	}
	if !customerID.Valid {
		return "", nil
	}
	return customerID.String, nil
}

// GetByStripeCustomerID resolves the account linked to a Stripe customer ID.
// Used by the Stripe webhook to map an inbound payment-collection event back to
// an account.
func (s *AccountStore) GetByStripeCustomerID(customerID string) (*Account, error) {
	var a Account
	err := s.db.QueryRow(`
		SELECT id, name, type, created_at, updated_at
		FROM accounts WHERE stripe_customer_id = $1 AND deleted_at IS NULL
	`, customerID).Scan(&a.ID, &a.Name, &a.Type, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("stripe customer %s: %w", customerID, ErrAccountNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account by stripe_customer_id: %w", err)
	}
	return &a, nil
}

// GetOwnerEmail returns the account owner's WorkOS-mirrored email (from
// account_member_emails), i.e. the verified identity email we persist — not a
// request's session user. The owner is the earliest member; a verified address
// wins ties. Returns "" when no mirrored email exists yet (e.g. reconcile
// pending), which callers treat as "unknown".
func (s *AccountStore) GetOwnerEmail(accountID string) (string, error) {
	var email string
	err := s.db.QueryRow(`
		SELECT me.email
		FROM account_members am
		JOIN account_member_emails me ON me.user_id = am.user_id
		WHERE am.account_id = $1 AND me.source = 'workos' AND me.email <> ''
		ORDER BY am.created_at ASC, me.verified DESC
		LIMIT 1
	`, accountID).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get account owner email: %w", err)
	}
	return email, nil
}


// SetStripeCustomerID stores the Stripe customer ID for an account.
func (s *AccountStore) SetStripeCustomerID(accountID, customerID string) error {
	_, err := s.db.Exec(`
		UPDATE accounts SET stripe_customer_id = $1, updated_at = $2 WHERE id = $3
	`, customerID, time.Now(), accountID)
	if err != nil {
		return fmt.Errorf("failed to set stripe_customer_id: %w", err)
	}
	return nil
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

// ClaimSignupCredit records that a user has taken their one signup credit and
// reports whether this account is the one that holds the claim.
//
// The claim is keyed on the person, because nothing caps how many accounts one
// user creates and every account would otherwise carry its own grant. It is
// idempotent for the claiming account: a provisioning retry that already won the
// claim still reads true, so a job that failed after claiming and before
// creating the contract does not leave the account without its credit.
func (s *AccountStore) ClaimSignupCredit(userID, accountID string) (bool, error) {
	var holder string
	err := s.db.QueryRow(`
		INSERT INTO billing_credit_grants (user_id, account_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET user_id = billing_credit_grants.user_id
		RETURNING account_id
	`, userID, accountID).Scan(&holder)
	if err != nil {
		return false, fmt.Errorf("failed to claim signup credit: %w", err)
	}
	return holder == accountID, nil
}

// GetCreatorUserID returns the account's earliest member, which is the user who
// created it. Create inserts the creator as the first member, and later members
// are added with a later timestamp.
func (s *AccountStore) GetCreatorUserID(accountID string) (string, error) {
	var userID string
	err := s.db.QueryRow(`
		SELECT user_id FROM account_members
		WHERE account_id = $1
		ORDER BY created_at
		LIMIT 1
	`, accountID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no member found for account: %s", accountID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query account creator: %w", err)
	}
	return userID, nil
}
