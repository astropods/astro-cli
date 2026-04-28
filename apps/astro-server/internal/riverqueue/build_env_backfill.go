package riverqueue

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// BuildEnvBackfillArgs are the job arguments for the build_env backfill worker.
type BuildEnvBackfillArgs struct{}

func (BuildEnvBackfillArgs) Kind() string { return "buildenv.backfill" }

// BuildEnvBackfillWorker copies user-variable rows from the legacy
// deployment_variables table into deployment_build_env, fanning out per
// (variable, target_role). Idempotent — deployments that already have
// rows in deployment_build_env are skipped, so the job is safe to re-run.
//
// The follow-up PR that drops deployment_variables relies on these rows
// existing for every deployment that had user variables. Run this worker
// at least once across all environments before merging that PR.
//
// Encryption: deployment_variables stores secret values as base64-encoded
// ciphertext + nonce; non-secret values as plaintext. deployment_build_env
// expects raw bytea — secret rows carry ciphertext+nonce as-is, non-secret
// rows carry plaintext bytes with nil nonce. No KMS round-trip needed.
type BuildEnvBackfillWorker struct {
	river.WorkerDefaults[BuildEnvBackfillArgs]
	db  *sql.DB
	log *logger.Logger
}

func (w *BuildEnvBackfillWorker) Work(ctx context.Context, _ *river.Job[BuildEnvBackfillArgs]) error {
	const batchSize = 100
	var lastID string
	var processed, skipped, failed int

	for {
		rows, err := w.db.QueryContext(ctx, `
			SELECT id, deployment_spec_json
			FROM deployments
			WHERE ($1 = '' OR id > $1)
			ORDER BY id
			LIMIT $2
		`, lastID, batchSize)
		if err != nil {
			w.log.Error("BuildEnv backfill: query deployments", "error", err)
			return nil
		}

		type pending struct {
			id       string
			specJSON string
		}
		var batch []pending
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.id, &p.specJSON); err != nil {
				w.log.Error("BuildEnv backfill: scan deployment", "error", err)
				continue
			}
			batch = append(batch, p)
		}
		_ = rows.Close()

		if len(batch) == 0 {
			break
		}
		lastID = batch[len(batch)-1].id

		for _, p := range batch {
			done, err := w.backfillOne(ctx, p.id, p.specJSON)
			switch {
			case err != nil:
				w.log.Error("BuildEnv backfill: deployment failed", "deployment_id", p.id, "error", err)
				failed++
			case done:
				processed++
			default:
				skipped++
			}
		}
	}

	w.log.Info("BuildEnv backfill completed",
		"processed", processed, "skipped", skipped, "failed", failed)
	return nil
}

// backfillOne returns (true, nil) if rows were written for this deployment,
// (false, nil) if it was skipped (already migrated or no variables), or
// (false, err) on a non-skippable failure.
func (w *BuildEnvBackfillWorker) backfillOne(ctx context.Context, deploymentID, specJSON string) (bool, error) {
	// Skip deployments that already have rows. The unique key on
	// (deployment_id, role, env_name) means a re-run after a failed
	// partial write would conflict, so we only proceed when the table
	// is empty for this deployment.
	var hasRows bool
	if err := w.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM deployment_build_env WHERE deployment_id = $1)`,
		deploymentID,
	).Scan(&hasRows); err != nil {
		return false, fmt.Errorf("check existing rows: %w", err)
	}
	if hasRows {
		return false, nil
	}

	// Pull legacy variables.
	rows, err := w.db.QueryContext(ctx, `
		SELECT name, value, ref, secret, optional, targets, nonce
		FROM deployment_variables
		WHERE deployment_id = $1
	`, deploymentID)
	if err != nil {
		return false, fmt.Errorf("query deployment_variables: %w", err)
	}
	type legacyVar struct {
		name     string
		value    string
		ref      string
		secret   bool
		optional bool
		targets  []string
		nonce    []byte
	}
	var vars []legacyVar
	for rows.Next() {
		var v legacyVar
		if err := rows.Scan(&v.name, &v.value, &v.ref, &v.secret, &v.optional,
			pq.Array(&v.targets), &v.nonce); err != nil {
			_ = rows.Close()
			return false, fmt.Errorf("scan: %w", err)
		}
		vars = append(vars, v)
	}
	_ = rows.Close()
	if len(vars) == 0 {
		return false, nil
	}

	// Pull declared ingestion names from the stored spec — needed to
	// fan out the bare "ingestion" target to one row per ingestion.
	ingestionNames, err := ingestionNamesFromSpec(specJSON)
	if err != nil {
		w.log.Warn("BuildEnv backfill: parse spec for ingestions", "deployment_id", deploymentID, "error", err)
		// Continue with empty list; bare "ingestion" targets just produce
		// no rows for that deployment, which mirrors legacy behaviour
		// where ingestion containers envFrom the agent's bundle anyway.
		ingestionNames = nil
	}

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	written := 0
	for _, v := range vars {
		roles := rolesForLegacyTargets(v.targets, ingestionNames)
		for _, role := range roles {
			valueEncrypted, nonce, err := decodeLegacyValue(v.value, v.nonce, v.secret)
			if err != nil {
				return false, fmt.Errorf("decode %s/%s: %w", role, v.name, err)
			}
			var userVarName, accountVarRef sql.NullString
			var optional sql.NullBool
			userVarName = sql.NullString{String: v.name, Valid: true}
			if v.ref != "" {
				accountVarRef = sql.NullString{String: v.ref, Valid: true}
			}
			optional = sql.NullBool{Bool: v.optional, Valid: true}

			if _, err := tx.ExecContext(ctx, `
				INSERT INTO deployment_build_env
					(deployment_id, role, env_name, value_encrypted, nonce,
					 is_secret, source, user_var_name, account_var_ref, optional)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			`, deploymentID, role, v.name, valueEncrypted, nonce,
				v.secret, "user_var", userVarName, accountVarRef, optional); err != nil {
				return false, fmt.Errorf("insert %s/%s: %w", role, v.name, err)
			}
			written++
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	w.log.Info("BuildEnv backfill: deployment migrated",
		"deployment_id", deploymentID, "rows_written", written)
	return true, nil
}

// rolesForLegacyTargets converts a deployment_variables Targets list into
// concrete role strings for deployment_build_env. Bare "ingestion" fans
// out across every ingestion declared in the deployment's spec; the
// qualified "ingestion.<name>" form narrows to one. "interface.*"
// collapses to a single 'messaging' row regardless of adapter count.
func rolesForLegacyTargets(targets []string, ingestionNames []string) []string {
	seen := map[string]bool{}
	add := func(role string) {
		if !seen[role] {
			seen[role] = true
		}
	}
	for _, t := range targets {
		switch {
		case t == "agent":
			add("agent")
		case strings.HasPrefix(t, "interface."):
			add("messaging")
		case t == "ingestion":
			for _, n := range ingestionNames {
				add("ingestion:" + n)
			}
		case strings.HasPrefix(t, "ingestion."):
			name := strings.TrimPrefix(t, "ingestion.")
			add("ingestion:" + name)
		}
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	return out
}

// ingestionNamesFromSpec pulls the keys of ds.Ingestion from a stored
// deployment_spec_json blob without depending on the full spec types.
// Returns nil when the spec doesn't declare any.
func ingestionNamesFromSpec(specJSON string) ([]string, error) {
	if specJSON == "" {
		return nil, nil
	}
	var doc struct {
		Ingestion map[string]json.RawMessage `json:"ingestion"`
	}
	if err := json.Unmarshal([]byte(specJSON), &doc); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(doc.Ingestion))
	for name := range doc.Ingestion {
		out = append(out, name)
	}
	return out, nil
}

// decodeLegacyValue converts a deployment_variables (value, nonce, secret)
// triple into the (value_encrypted, nonce) bytea pair stored on
// deployment_build_env. Three legacy shapes:
//
//   - Secret with KMS configured: value=base64(ciphertext), nonce set.
//     → base64-decode value to raw ciphertext; pass nonce through.
//   - Secret with no KMS (local dev / test): value=plaintext, nonce nil.
//     → store plaintext bytes, nil nonce.
//   - Non-secret: value=plaintext, nonce nil.
//     → store plaintext bytes, nil nonce.
//
// The presence of a nonce is the discriminator.
func decodeLegacyValue(value string, nonce []byte, isSecret bool) ([]byte, []byte, error) {
	if isSecret && len(nonce) > 0 {
		if value == "" {
			// Stripped/optional secret with no rehydrated value — write
			// empty bytea, keep the nonce so downstream readers know
			// the row was meant to be encrypted.
			return []byte{}, nonce, nil
		}
		ct, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, nil, fmt.Errorf("base64 decode: %w", err)
		}
		return ct, nonce, nil
	}
	return []byte(value), nil, nil
}
