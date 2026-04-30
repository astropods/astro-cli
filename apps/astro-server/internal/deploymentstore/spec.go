package deploymentstore

import (
	"context"
	"encoding/json"
	"fmt"
)

// SourceAccountFromSpec extracts the source.account field from a deployment spec JSON.
//
// Cross-account deployments store the publisher account in
// deployment_spec_json.source.account even when the deployment row itself
// lives under a different (target) account. Any code that needs to look up
// the agent build for a deployment should resolve the source account from
// this field, not from the target account's URL parameter.
//
// Returns empty string when the spec is empty, omits the field, or fails to
// parse; callers should treat that as "legacy / same-account" and fall back
// to their prior behavior.
func SourceAccountFromSpec(specJSON string) string {
	if specJSON == "" || specJSON == "{}" {
		return ""
	}
	var parsed struct {
		Source struct {
			Account string `json:"account"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(specJSON), &parsed); err != nil {
		return ""
	}
	return parsed.Source.Account
}

// BackfillResult summarizes the outcome of BackfillSourceAccountIDs.
type BackfillResult struct {
	FromSpec   int // rows resolved via deployment_spec_json.source.account
	FromSelf   int // rows for which the target account was used as fallback
	SpecMisses int // rows with a source.account that did not match an accounts row
	Scanned    int
}

// BackfillSourceAccountIDs populates deployments.source_account_id for legacy
// rows created before the column existed. It parses deployment_spec_json to
// resolve the publisher account name, falling back to the row's own
// account_id when the spec has no source block. The operation is idempotent:
// rows with source_account_id already set are skipped.
func (s *Store) BackfillSourceAccountIDs(ctx context.Context) (BackfillResult, error) {
	var res BackfillResult

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, deployment_spec_json
		FROM deployments
		WHERE source_account_id IS NULL
	`)
	if err != nil {
		return res, fmt.Errorf("query legacy deployments: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	type pending struct {
		id              string
		accountID       string
		sourceAccountID string
		fromSpec        bool
	}
	var toUpdate []pending

	for rows.Next() {
		res.Scanned++
		var id, accountID, specJSON string
		if err := rows.Scan(&id, &accountID, &specJSON); err != nil {
			return res, fmt.Errorf("scan legacy deployment: %w", err)
		}

		if name := SourceAccountFromSpec(specJSON); name != "" {
			var sourceID string
			lookupErr := s.db.QueryRowContext(ctx, `SELECT id FROM accounts WHERE name = $1`, name).Scan(&sourceID)
			if lookupErr == nil {
				toUpdate = append(toUpdate, pending{id: id, sourceAccountID: sourceID, fromSpec: true})
				continue
			}
			res.SpecMisses++
		}
		toUpdate = append(toUpdate, pending{id: id, accountID: accountID, sourceAccountID: accountID, fromSpec: false})
	}
	if err := rows.Err(); err != nil {
		return res, fmt.Errorf("iterate legacy deployments: %w", err)
	}

	for _, p := range toUpdate {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE deployments SET source_account_id = $2 WHERE id = $1 AND source_account_id IS NULL`,
			p.id, p.sourceAccountID,
		); err != nil {
			return res, fmt.Errorf("update deployment %s: %w", p.id, err)
		}
		if p.fromSpec {
			res.FromSpec++
		} else {
			res.FromSelf++
		}
	}

	return res, nil
}

// StaleRebindResult summarizes the outcome of RebindStaleSourceAccountIDs.
type StaleRebindResult struct {
	Rebound int // rows whose source_account_id was repointed to the unique current owner
}

// RebindStaleSourceAccountIDs repairs deployments whose source_account_id
// references an account that no longer owns the (agent_name, build_id)
// tuple in agent_versions, when there is exactly one other account that
// does. This is the data sweep for the historical bug where
// agentindex.Transfer moved the agents and agent_versions rows between
// accounts but left deployments.source_account_id pointing at the old
// publisher — every cross-account deployment of a transferred agent
// silently lost its upgrade signal until this runs.
//
// Single statement, idempotent: rows whose current source_account_id
// already matches agent_versions are excluded by the `<>` predicate, and
// rows with multiple candidate accounts (n > 1, e.g. an unrelated agent
// in another account that happens to share the same name + build_id) are
// excluded by `c.n = 1` and left for a human to triage.
//
// Excluded by design:
//   - source_account_id IS NULL (publisher-deleted rows; FK SET NULL path)
//   - (name, build_id) absent from agent_versions (build was deleted)
//   - (name, build_id) present in >1 accounts (ambiguous)
func (s *Store) RebindStaleSourceAccountIDs(ctx context.Context) (StaleRebindResult, error) {
	var res StaleRebindResult

	rows, err := s.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT av.name, av.build_id, av.account_id,
			       COUNT(*) OVER (PARTITION BY av.name, av.build_id) AS n
			FROM agent_versions av
		)
		UPDATE deployments d
		SET source_account_id = c.account_id
		FROM candidates c
		WHERE c.name = d.agent_name
		  AND c.build_id = d.build_id
		  AND c.n = 1
		  AND d.source_account_id IS NOT NULL
		  AND d.source_account_id <> c.account_id
		RETURNING d.id
	`)
	if err != nil {
		return res, fmt.Errorf("rebind stale source_account_id: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return res, fmt.Errorf("scan rebound deployment id: %w", err)
		}
		res.Rebound++
	}
	if err := rows.Err(); err != nil {
		return res, fmt.Errorf("iterate rebound deployments: %w", err)
	}
	return res, nil
}
