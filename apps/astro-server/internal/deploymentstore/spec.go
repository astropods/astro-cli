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
	FromSpec     int // rows resolved via deployment_spec_json.source.account
	FromSelf     int // rows for which the target account was used as fallback
	SpecMisses   int // rows with a source.account that did not match an accounts row
	Scanned      int
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
