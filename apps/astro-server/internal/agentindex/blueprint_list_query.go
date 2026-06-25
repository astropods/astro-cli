package agentindex

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const accountBlueprintLatestVersionJoin = `
	LEFT JOIN agent_versions v ON v.account_id = a.account_id AND v.name = a.name
		AND v.published_at = (
			SELECT MAX(v2.published_at) FROM agent_versions v2
			WHERE v2.account_id = a.account_id AND v2.name = a.name
		)`

// accountBlueprintLatestCommitJoin pulls the commit metadata of the github build
// that produced the latest version, when one exists. build_id is not unique in
// github_builds (retries reuse it), so a LATERAL ... LIMIT 1 keeps this to one row
// per agent rather than fanning out the list. repo_full_name comes from the build's
// connection. Direct CLI pushes have no matching build and yield NULL.
const accountBlueprintLatestCommitJoin = `
	LEFT JOIN LATERAL (
		SELECT gb.commit_message, gb.commit_sha, gc.repo_full_name
		FROM github_builds gb
		LEFT JOIN github_connections gc ON gc.id = gb.connection_id
		WHERE gb.account_id = a.account_id AND gb.agent_name = a.name AND gb.build_id = v.build_id
		ORDER BY gb.enqueued_at DESC
		LIMIT 1
	) gbinfo ON true`

const blueprintListTotalColumn = `, COUNT(*) OVER() AS list_total`

func blueprintListPaginated(opts BlueprintListOptions) bool {
	return opts.Limit > 0
}

func appendBlueprintPagination(query *string, args *[]any, argN *int, opts BlueprintListOptions) {
	if opts.Limit <= 0 {
		return
	}
	*query += " LIMIT $" + strconv.Itoa(*argN) + " OFFSET $" + strconv.Itoa(*argN+1)
	*args = append(*args, opts.Limit, opts.Offset)
	*argN += 2
}

func (idx *Index) buildAccountBlueprintWhere(accountID string, opts BlueprintListOptions) ([]string, []any, int) {
	where := []string{"a.account_id = $1", "a.archived_at IS NULL"}
	args := []any{accountID}
	argN := 2

	if opts.Visibility != "" {
		where = append(where, "a.visibility = $"+strconv.Itoa(argN))
		args = append(args, opts.Visibility)
		argN++
	}
	appendBlueprintTextFilter(&where, &args, &argN, "a", opts.Query)
	appendBlueprintTagFilter(&where, &args, &argN, "a", opts.Tag)
	return where, args, argN
}

func scanAccountBlueprintListRow(
	agent *Agent,
	buildID, ecrNamespace, specJSON, readme, agentCard, warningsJSON, commitMessage, commitSHA, repoFullName sql.NullString,
	publishedAt, versionUpdated sql.NullTime,
) error {
	if buildID.Valid {
		v := AgentVersion{
			BuildID:       buildID.String,
			ECRNamespace:  ecrNamespace.String,
			Readme:        readme.String,
			AgentCardJSON: agentCard.String,
			PublishedAt:   publishedAt.Time,
			UpdatedAt:     versionUpdated.Time,
			CommitMessage: commitMessage.String,
			CommitSHA:     commitSHA.String,
			RepoFullName:  repoFullName.String,
		}
		if specJSON.Valid {
			if err := json.Unmarshal([]byte(specJSON.String), &v.Spec); err != nil {
				return fmt.Errorf("failed to unmarshal spec: %w", err)
			}
		}
		if warningsJSON.Valid {
			v.ValidationWarnings = parseValidationWarnings(warningsJSON.String)
		}
		agent.Versions = []*AgentVersion{&v}
	}
	return nil
}

// ListForAccount returns agents for an account matching filters, with optional pagination.
// Each agent includes its latest published version (if any) in a single query.
// When Limit > 0, total matches come from COUNT(*) OVER() in the same query as the page rows.
// When Limit <= 0, no count query runs and Total is len(Agents) after the scan.
func (idx *Index) ListForAccount(accountID string, opts BlueprintListOptions) (*BlueprintListPage, error) {
	where, args, argN := idx.buildAccountBlueprintWhere(accountID, opts)
	paginated := blueprintListPaginated(opts)

	order := blueprintOrderClause(opts.Sort, "a", "v")
	listTotalSQL := ""
	if paginated {
		listTotalSQL = blueprintListTotalColumn
	}
	query := `
		SELECT a.account_id, a.name, a.registry, a.visibility, a.avatar_colors, a.avatar_updated_at, a.created_at, a.updated_at,
		       v.build_id, v.ecr_namespace, v.spec_json, v.readme, v.agent_card_json, v.validation_warnings, v.published_at, v.updated_at,
		       gbinfo.commit_message, gbinfo.commit_sha, gbinfo.repo_full_name,
		       (SELECT COUNT(*) FROM agent_versions av WHERE av.account_id = a.account_id AND av.name = a.name) AS version_count` + listTotalSQL + `
		FROM agents a` + accountBlueprintLatestVersionJoin + accountBlueprintLatestCommitJoin + `
		WHERE ` + strings.Join(where, " AND ") + `
		` + order // #nosec G202 -- WHERE fragments use $N placeholders; values are parameterized
	appendBlueprintPagination(&query, &args, &argN, opts)

	rows, err := idx.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query agents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	page := &BlueprintListPage{}
	for rows.Next() {
		var agent Agent
		var buildID, ecrNamespace, specJSON, readme, agentCard, warningsJSON, commitMessage, commitSHA, repoFullName sql.NullString
		var publishedAt, versionUpdated sql.NullTime
		var listTotal int

		scanDest := []any{
			&agent.AccountID, &agent.Name, &agent.Registry, &agent.Visibility, &agent.AvatarColors, &agent.AvatarUpdatedAt, &agent.CreatedAt, &agent.UpdatedAt,
			&buildID, &ecrNamespace, &specJSON, &readme, &agentCard, &warningsJSON, &publishedAt, &versionUpdated,
			&commitMessage, &commitSHA, &repoFullName,
			&agent.VersionCount,
		}
		if paginated {
			scanDest = append(scanDest, &listTotal)
		}
		if err := rows.Scan(scanDest...); err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}
		if paginated {
			page.Total = listTotal
		}
		if err := scanAccountBlueprintListRow(&agent, buildID, ecrNamespace, specJSON, readme, agentCard, warningsJSON, commitMessage, commitSHA, repoFullName, publishedAt, versionUpdated); err != nil {
			return nil, err
		}
		page.Agents = append(page.Agents, &agent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating agents: %w", err)
	}
	if !paginated {
		page.Total = len(page.Agents)
	}
	return page, nil
}

func (idx *Index) buildPublicBlueprintWhere(opts BlueprintListOptions) ([]string, []any, int) {
	where := []string{
		"a.visibility = 'public'",
		"a.archived_at IS NULL",
		`v.published_at = (
			SELECT MAX(v2.published_at) FROM agent_versions v2
			WHERE v2.account_id = a.account_id AND v2.name = a.name
		)`,
	}
	args := []any{}
	argN := 1

	appendBlueprintTextFilter(&where, &args, &argN, "a", opts.Query)
	appendBlueprintTagFilter(&where, &args, &argN, "a", opts.Tag)
	return where, args, argN
}

// ListPublicAgents returns public agents with their latest version, with optional pagination.
func (idx *Index) ListPublicAgents(opts BlueprintListOptions) (*BlueprintListPage, error) {
	where, args, argN := idx.buildPublicBlueprintWhere(opts)
	paginated := blueprintListPaginated(opts)

	order := blueprintOrderClause(opts.Sort, "a", "v")
	listTotalSQL := ""
	if paginated {
		listTotalSQL = blueprintListTotalColumn
	}
	query := `
		SELECT a.account_id, a.name, a.registry, a.visibility, a.avatar_colors, a.avatar_updated_at, a.created_at, a.updated_at,
		       v.build_id, v.ecr_namespace, v.spec_json, v.readme, v.agent_card_json, v.published_at, v.updated_at` + listTotalSQL + `
		FROM agents a
		INNER JOIN agent_versions v ON a.account_id = v.account_id AND a.name = v.name
		WHERE ` + strings.Join(where, " AND ") + `
		` + order // #nosec G202 -- WHERE fragments use $N placeholders; values are parameterized
	appendBlueprintPagination(&query, &args, &argN, opts)

	rows, err := idx.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query public agents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	page := &BlueprintListPage{}
	for rows.Next() {
		var agent Agent
		var v AgentVersion
		var specJSON string
		var listTotal int

		scanDest := []any{
			&agent.AccountID, &agent.Name, &agent.Registry, &agent.Visibility, &agent.AvatarColors, &agent.AvatarUpdatedAt, &agent.CreatedAt, &agent.UpdatedAt,
			&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &v.AgentCardJSON, &v.PublishedAt, &v.UpdatedAt,
		}
		if paginated {
			scanDest = append(scanDest, &listTotal)
		}
		if err := rows.Scan(scanDest...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		if paginated {
			page.Total = listTotal
		}

		if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
		}

		agent.Versions = []*AgentVersion{&v}
		page.Agents = append(page.Agents, &agent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating public agents: %w", err)
	}
	if !paginated {
		page.Total = len(page.Agents)
	}
	return page, nil
}
