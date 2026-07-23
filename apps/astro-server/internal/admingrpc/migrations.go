package admingrpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

const (
	clusterMigrationEventsLimit = 50
	clusterMigrationJobsLimit   = 50
)

// ListClusterMigrations returns recent migration-related deployment events and
// migrate/deploy River jobs. Placement alignment lives on the Deployments page.
func (s *Server) ListClusterMigrations(ctx context.Context, req *adminv1.ListClusterMigrationsRequest) (*adminv1.ListClusterMigrationsResponse, error) {
	if s.deployStore == nil {
		return nil, fmt.Errorf("deployment store not configured")
	}

	mismatchCount, err := s.countPlacementMismatches(ctx)
	if err != nil {
		return nil, fmt.Errorf("count placement mismatches: %w", err)
	}

	events, err := s.listClusterMigrationEvents(ctx, req.MismatchesOnly)
	if err != nil {
		s.log.Warn("list cluster migration events", "error", err)
		events = nil
	}

	jobs, err := s.listClusterMigrationJobs(ctx, req.MismatchesOnly)
	if err != nil {
		s.log.Warn("list cluster migration jobs", "error", err)
		jobs = nil
	}

	return &adminv1.ListClusterMigrationsResponse{
		Events:        events,
		Jobs:          jobs,
		MismatchCount: mismatchCount,
	}, nil
}

func (s *Server) countPlacementMismatches(ctx context.Context) (int32, error) {
	var count int32
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::int
		FROM deployments d
		JOIN accounts a ON a.id = d.account_id
		WHERE d.status <> 'undeployed'
		  AND COALESCE(NULLIF(a.cluster_id, ''), '') IS DISTINCT FROM COALESCE(NULLIF(d.cluster_id, ''), '')
	`).Scan(&count)
	return count, err
}

const mismatchDeploymentsSubquerySQL = `
		SELECT d.id FROM deployments d
		JOIN accounts a ON a.id = d.account_id
		WHERE d.status <> 'undeployed'
		  AND COALESCE(NULLIF(a.cluster_id, ''), '') IS DISTINCT FROM COALESCE(NULLIF(d.cluster_id, ''), '')
	`

// Event filters use fixed prefixes from clusterplacement.*EventMessage helpers (not ILIKE wildcards).
const clusterMigrationEventsWhereSQL = `
		(e.message LIKE 'Account cluster migration:%'
		 OR e.message LIKE 'Cluster placement updated from %'
		 OR e.message LIKE 'Admin re-apply: Cluster placement updated from %')`

const clusterMigrationEventsQuery = `
		SELECT e.deployment_id, a.name, d.agent_name, e.status, COALESCE(e.message, ''), e.created_at
		FROM deployment_events e
		JOIN deployments d ON d.id = e.deployment_id
		JOIN accounts a ON a.id = d.account_id
		WHERE ` + clusterMigrationEventsWhereSQL + `
		ORDER BY e.created_at DESC LIMIT $1`

const clusterMigrationEventsMismatchQuery = `
		SELECT e.deployment_id, a.name, d.agent_name, e.status, COALESCE(e.message, ''), e.created_at
		FROM deployment_events e
		JOIN deployments d ON d.id = e.deployment_id
		JOIN accounts a ON a.id = d.account_id
		WHERE ` + clusterMigrationEventsWhereSQL + `
		  AND e.deployment_id IN (` + mismatchDeploymentsSubquerySQL + `)
		ORDER BY e.created_at DESC LIMIT $1`

func (s *Server) listClusterMigrationEvents(ctx context.Context, mismatchesOnly bool) ([]*adminv1.ClusterMigrationEvent, error) {
	query := clusterMigrationEventsQuery
	if mismatchesOnly {
		query = clusterMigrationEventsMismatchQuery
	}

	rows, err := s.db.QueryContext(ctx, query, clusterMigrationEventsLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var out []*adminv1.ClusterMigrationEvent
	for rows.Next() {
		var ev adminv1.ClusterMigrationEvent
		var createdAt time.Time
		if err := rows.Scan(&ev.DeploymentId, &ev.AccountName, &ev.AgentName, &ev.Status, &ev.Message, &createdAt); err != nil {
			return nil, fmt.Errorf("scan migration event: %w", err)
		}
		ev.CreatedAt = createdAt.Format(time.RFC3339)
		out = append(out, &ev)
	}
	return out, rows.Err()
}

type riverJobArgs struct {
	DeploymentID    string `json:"deployment_id"`
	SourceClusterID string `json:"source_cluster_id"`
	TargetClusterID string `json:"target_cluster_id"`
	ClusterID       string `json:"cluster_id"`
}

func populateMigrationJobFields(j *adminv1.ClusterMigrationJob, kind, argsJSON string) {
	var args riverJobArgs
	if argsJSON != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	if args.DeploymentID != "" && j.DeploymentId == "" {
		j.DeploymentId = args.DeploymentID
	}
	switch kind {
	case "deployment.migrate_cluster":
		j.SourceClusterId = args.SourceClusterID
		j.TargetClusterId = args.TargetClusterID
	case "deployment.deploy":
		j.DeployClusterId = args.ClusterID
	}
}

// Include all migrate jobs; deploy jobs only when enqueued soon after a migrate for the same deployment.
const clusterMigrationJobsWhereSQL = `
		(j.kind = 'deployment.migrate_cluster'
		 OR (j.kind = 'deployment.deploy'
		     AND EXISTS (
		       SELECT 1 FROM river.river_job m
		       WHERE m.kind = 'deployment.migrate_cluster'
		         AND m.args->>'deployment_id' = j.args->>'deployment_id'
		         AND m.created_at <= j.created_at
		         AND j.created_at <= m.created_at + interval '2 hours'
		     )))`

const clusterMigrationJobsQuery = `
		SELECT j.id, j.kind, j.state,
		       COALESCE(j.args->>'deployment_id', ''),
		       j.args::text,
		       j.errors::text, j.created_at, j.finalized_at, j.attempt, j.max_attempts,
		       COALESCE(a.name, ''), COALESCE(d.agent_name, '')
		FROM river.river_job j
		LEFT JOIN deployments d ON d.id = j.args->>'deployment_id'
		LEFT JOIN accounts a ON a.id = d.account_id
		WHERE ` + clusterMigrationJobsWhereSQL + `
		ORDER BY j.created_at DESC LIMIT $1`

const clusterMigrationJobsMismatchQuery = `
		SELECT j.id, j.kind, j.state,
		       COALESCE(j.args->>'deployment_id', ''),
		       j.args::text,
		       j.errors::text, j.created_at, j.finalized_at, j.attempt, j.max_attempts,
		       COALESCE(a.name, ''), COALESCE(d.agent_name, '')
		FROM river.river_job j
		LEFT JOIN deployments d ON d.id = j.args->>'deployment_id'
		LEFT JOIN accounts a ON a.id = d.account_id
		WHERE ` + clusterMigrationJobsWhereSQL + `
		  AND j.args->>'deployment_id' IN (` + mismatchDeploymentsSubquerySQL + `)
		ORDER BY j.created_at DESC LIMIT $1`

func (s *Server) listClusterMigrationJobs(ctx context.Context, mismatchesOnly bool) ([]*adminv1.ClusterMigrationJob, error) {
	query := clusterMigrationJobsQuery
	if mismatchesOnly {
		query = clusterMigrationJobsMismatchQuery
	}

	rows, err := s.db.QueryContext(ctx, query, clusterMigrationJobsLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var out []*adminv1.ClusterMigrationJob
	for rows.Next() {
		var j adminv1.ClusterMigrationJob
		var createdAt time.Time
		var finalizedAt sql.NullTime
		var errorsStr sql.NullString
		if err := rows.Scan(
			&j.JobId, &j.Kind, &j.State, &j.DeploymentId, &j.ArgsJson,
			&errorsStr, &createdAt, &finalizedAt, &j.Attempt, &j.MaxAttempt,
			&j.AccountName, &j.AgentName,
		); err != nil {
			return nil, fmt.Errorf("scan migration job: %w", err)
		}
		j.CreatedAt = createdAt.Format(time.RFC3339)
		if finalizedAt.Valid {
			j.FinalizedAt = finalizedAt.Time.Format(time.RFC3339)
			j.DurationMs = finalizedAt.Time.Sub(createdAt).Milliseconds()
		}
		if errorsStr.Valid {
			j.Errors = errorsStr.String
		}
		populateMigrationJobFields(&j, j.Kind, j.ArgsJson)
		out = append(out, &j)
	}
	return out, rows.Err()
}
