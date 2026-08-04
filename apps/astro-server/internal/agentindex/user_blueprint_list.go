package agentindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

type UserBlueprintCursor struct {
	Sort        string
	PublishedAt *time.Time
	Name        string
	AccountID   string
}

type UserBlueprint struct {
	Agent       *Agent
	AccountName string
	PublishedAt *time.Time
}

type UserBlueprintPublisher struct {
	Name    string `json:"name"`
	Account string `json:"account,omitempty"`
}

type UserBlueprintMetadata struct {
	HeartCount       int
	LifetimeMessages int64
	DeployCount      int64
	Publishers       []UserBlueprintPublisher
}

func buildUserBlueprintWhere(
	userID string,
	accountIDs []string,
	opts BlueprintListOptions,
	cursor *UserBlueprintCursor,
) ([]string, []any, int) {
	where := []string{
		"am.user_id = $1",
		"a.account_id = ANY($2::uuid[])",
		"acct.deleted_at IS NULL",
		"a.archived_at IS NULL",
	}
	args := []any{userID, pq.Array(accountIDs)}
	argN := 3
	if opts.Visibility != "" {
		where = append(where, "a.visibility = $"+strconv.Itoa(argN))
		args = append(args, opts.Visibility)
		argN++
	}
	appendBlueprintTextFilter(&where, &args, &argN, "a", opts.Query)
	appendBlueprintTagFilter(&where, &args, &argN, "a", opts.Tag)
	if cursor == nil {
		return where, args, argN
	}
	if opts.Sort == "newest" && cursor.PublishedAt != nil {
		where = append(where, fmt.Sprintf(`(
			v.published_at < $%d OR v.published_at IS NULL OR
			(v.published_at = $%d AND (a.name, a.account_id) > ($%d, $%d::uuid))
		)`, argN, argN, argN+1, argN+2))
		args = append(args, *cursor.PublishedAt, cursor.Name, cursor.AccountID)
		argN += 3
	} else if opts.Sort == "newest" {
		where = append(where, fmt.Sprintf("v.published_at IS NULL AND (a.name, a.account_id) > ($%d, $%d::uuid)", argN, argN+1))
		args = append(args, cursor.Name, cursor.AccountID)
		argN += 2
	} else {
		where = append(where, fmt.Sprintf("(a.name, a.account_id) > ($%d, $%d::uuid)", argN, argN+1))
		args = append(args, cursor.Name, cursor.AccountID)
		argN += 2
	}
	return where, args, argN
}

// ListVisibleBlueprintsForUserPage returns one globally ordered, membership-
// guarded keyset page across the selected accounts.
func (idx *Index) ListVisibleBlueprintsForUserPage(
	ctx context.Context,
	userID string,
	accountIDs []string,
	opts BlueprintListOptions,
	cursor *UserBlueprintCursor,
) ([]UserBlueprint, error) {
	if len(accountIDs) == 0 {
		return []UserBlueprint{}, nil
	}
	where, args, argN := buildUserBlueprintWhere(userID, accountIDs, opts, cursor)
	order := "ORDER BY a.name ASC, a.account_id ASC"
	if opts.Sort == "newest" {
		order = "ORDER BY v.published_at DESC NULLS LAST, a.name ASC, a.account_id ASC"
	}
	latestVersionAndCommit := `
		LEFT JOIN LATERAL (
			SELECT av.build_id, av.spec_json, av.readme, av.agent_card_json,
			       av.validation_warnings, av.published_at
			FROM agent_versions av
			WHERE av.account_id = a.account_id AND av.name = a.name
			ORDER BY av.published_at DESC, av.build_id DESC
			LIMIT 1
		) v ON true` + accountBlueprintLatestCommitJoin
	resultColumns := `a.account_id, a.name, a.registry, a.visibility, a.name_reserved,
		       a.avatar_colors, a.avatar_updated_at, %s,
		       v.build_id, v.spec_json, v.readme, v.agent_card_json,
		       v.validation_warnings, v.published_at,
		       gbinfo.commit_message, gbinfo.commit_sha, gbinfo.repo_full_name`

	var query string
	if opts.Sort == "newest" {
		// Newest ordering depends on the lateral version row, so this single-account
		// path must resolve versions before applying the page limit.
		// #nosec G202 -- fragments contain only server-owned SQL and placeholders.
		query = fmt.Sprintf(`
		SELECT `+resultColumns+`
		FROM agents a
		JOIN account_members am ON am.account_id = a.account_id
		JOIN accounts acct ON acct.id = a.account_id
		`+latestVersionAndCommit+`
		WHERE `+strings.Join(where, " AND ")+`
		`+order+`
		LIMIT $`+strconv.Itoa(argN), "acct.name")
	} else {
		// Select the globally ordered page before resolving versions and commit
		// metadata. This bounds both lateral lookups to at most opts.Limit rows.
		// #nosec G202 -- fragments contain only server-owned SQL and placeholders.
		query = `
		WITH page AS (
			SELECT a.account_id, a.name, a.registry, a.visibility, a.name_reserved,
			       a.avatar_colors, a.avatar_updated_at, acct.name AS account_name
			FROM agents a
			JOIN account_members am ON am.account_id = a.account_id
			JOIN accounts acct ON acct.id = a.account_id
			WHERE ` + strings.Join(where, " AND ") + `
			` + order + `
			LIMIT $` + strconv.Itoa(argN) + `
		)
		SELECT a.account_id, a.name, a.registry, a.visibility, a.name_reserved,
		       a.avatar_colors, a.avatar_updated_at, a.account_name,
		       v.build_id, v.spec_json, v.readme, v.agent_card_json,
		       v.validation_warnings, v.published_at,
		       gbinfo.commit_message, gbinfo.commit_sha, gbinfo.repo_full_name
		FROM page a
		` + latestVersionAndCommit + `
		ORDER BY a.name ASC, a.account_id ASC`
	}
	args = append(args, opts.Limit)

	rows, err := idx.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query user blueprints: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make([]UserBlueprint, 0)
	for rows.Next() {
		var agent Agent
		var accountName string
		var buildID, specJSON, readme, agentCard, warningsJSON sql.NullString
		var commitMessage, commitSHA, repoFullName sql.NullString
		var publishedAt sql.NullTime
		if err := rows.Scan(
			&agent.AccountID, &agent.Name, &agent.Registry, &agent.Visibility, &agent.NameReserved,
			&agent.AvatarColors, &agent.AvatarUpdatedAt, &accountName,
			&buildID, &specJSON, &readme, &agentCard, &warningsJSON, &publishedAt,
			&commitMessage, &commitSHA, &repoFullName,
		); err != nil {
			return nil, fmt.Errorf("scan user blueprint: %w", err)
		}
		if err := scanAccountBlueprintListRow(
			&agent, buildID, sql.NullString{}, specJSON, readme, agentCard, warningsJSON,
			commitMessage, commitSHA, repoFullName, publishedAt, sql.NullTime{},
		); err != nil {
			return nil, err
		}
		var cursorPublishedAt *time.Time
		if publishedAt.Valid {
			value := publishedAt.Time
			cursorPublishedAt = &value
		}
		result = append(result, UserBlueprint{Agent: &agent, AccountName: accountName, PublishedAt: cursorPublishedAt})
	}
	return result, rows.Err()
}

// BatchUserBlueprintMetadata enriches a bounded page in one database call.
// It deliberately uses DB-owned identity data so list latency never depends on
// WorkOS or avatar object storage. Hearts, lifetime messages, and deployment
// counts are intentionally eventually consistent within the list cache's
// 30-second TTL; invalidating every shared page on those high-write paths would
// trade a small card-counter delay for sustained generation/cache churn.
func (idx *Index) BatchUserBlueprintMetadata(
	ctx context.Context,
	refs []AgentVersionRef,
) (map[string]UserBlueprintMetadata, error) {
	result := make(map[string]UserBlueprintMetadata, len(refs))
	if len(refs) == 0 {
		return result, nil
	}
	accountIDs := make([]string, 0, len(refs))
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		accountIDs = append(accountIDs, ref.AccountID)
		names = append(names, ref.Name)
	}
	rows, err := idx.db.QueryContext(ctx, `
		WITH refs AS (
			SELECT account_id, name
			FROM unnest($1::uuid[], $2::text[]) AS r(account_id, name)
		), hearts AS (
			SELECT h.account_id, h.agent_name AS name, COUNT(*)::bigint AS count
			FROM agent_hearts h JOIN refs r ON r.account_id = h.account_id AND r.name = h.agent_name
			GROUP BY h.account_id, h.agent_name
		), messages AS (
			SELECT m.account_id, m.agent_name AS name, m.lifetime_total
			FROM agent_message_counts m JOIN refs r ON r.account_id = m.account_id AND r.name = m.agent_name
		), deploys AS (
			SELECT d.account_id, d.agent_name AS name, COUNT(*)::bigint AS count
			FROM deployments d JOIN refs r ON r.account_id = d.account_id AND r.name = d.agent_name
			GROUP BY d.account_id, d.agent_name
		), publisher_actors AS (
			SELECT r.account_id, r.name, al.actor_id, MIN(al.created_at) AS first_seen
			FROM refs r JOIN audit_logs al
			  ON al.account_id = r.account_id AND al.resource_id = r.name
			 AND al.action = 'agent.register' AND al.resource_type = 'agent'
			GROUP BY r.account_id, r.name, al.actor_id
		), publishers AS (
			SELECT p.account_id, p.name,
			       jsonb_agg(jsonb_build_object(
			           'name', COALESCE(NULLIF(personal.display_name, ''), personal.name),
			           'account', personal.name
			       ) ORDER BY p.first_seen) FILTER (WHERE personal.name IS NOT NULL) AS data
			FROM publisher_actors p
			LEFT JOIN LATERAL (
				SELECT a.name, a.display_name
				FROM account_members am JOIN accounts a ON a.id = am.account_id
				WHERE am.user_id = p.actor_id AND a.type = 'personal' AND a.deleted_at IS NULL
				ORDER BY a.created_at ASC
				LIMIT 1
			) personal ON true
			GROUP BY p.account_id, p.name
		)
		SELECT r.account_id, r.name, COALESCE(h.count, 0), COALESCE(m.lifetime_total, 0),
		       COALESCE(d.count, 0), COALESCE(p.data, '[]'::jsonb)
		FROM refs r
		LEFT JOIN hearts h USING (account_id, name)
		LEFT JOIN messages m USING (account_id, name)
		LEFT JOIN deploys d USING (account_id, name)
		LEFT JOIN publishers p USING (account_id, name)
	`, pq.Array(accountIDs), pq.Array(names))
	if err != nil {
		return result, fmt.Errorf("query user blueprint metadata: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	for rows.Next() {
		var accountID, name string
		var metadata UserBlueprintMetadata
		var publishersJSON []byte
		if err := rows.Scan(
			&accountID, &name, &metadata.HeartCount, &metadata.LifetimeMessages,
			&metadata.DeployCount, &publishersJSON,
		); err != nil {
			return result, fmt.Errorf("scan user blueprint metadata: %w", err)
		}
		if err := json.Unmarshal(publishersJSON, &metadata.Publishers); err != nil {
			return result, fmt.Errorf("decode user blueprint publishers: %w", err)
		}
		result[accountID+"/"+name] = metadata
	}
	return result, rows.Err()
}
