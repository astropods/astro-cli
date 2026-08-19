package deploymentstore

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// UserDeploymentCursor is the stable keyset boundary for the authenticated
// cross-account deployment list. ID is the unique tiebreaker for deployments
// sharing a deployed_at timestamp.
type UserDeploymentCursor struct {
	DeployedAt time.Time
	ID         string
}

// UserDeployment pairs a deployment with the owning account name selected in
// the same membership-guarded query.
type UserDeployment struct {
	Deployment  *Deployment
	AccountName string
	AccessReady bool
}

const userDeploymentColumns = `d.id, d.account_id, d.source_account_id, d.agent_name, d.build_id,
        d.namespace, d.display_name, d.status, d.error_message, d.deployed_at, d.avatar_colors`

const fgaReadPredicate = `
		  AND (
		    $3::uuid[] IS NULL
		    OR NOT (d.account_id = ANY($3::uuid[]))
		    OR NOT EXISTS (
		      SELECT 1 FROM deployment_fga_sync s
		      WHERE s.deployment_id = d.id
		        AND s.desired_state = 'registered'
		    )
		    OR d.id = ANY($4::varchar[])
		  )`

// ListVisibleDeploymentsForUserPage returns one globally ordered page across
// the selected accounts. The account_members join is deliberately retained
// after the handler resolves requested account names so membership is enforced
// again by the authoritative resource query.
func (s *Store) ListVisibleDeploymentsForUserPage(
	ctx context.Context,
	userID string,
	accountIDs []string,
	search string,
	limit int,
	cursor *UserDeploymentCursor,
	fgaAccountIDs []string,
	readableDeploymentIDs []string,
) ([]UserDeployment, error) {
	if userID == "" || len(accountIDs) == 0 || limit <= 0 {
		return []UserDeployment{}, nil
	}

	var cursorTime any
	var cursorID any
	if cursor != nil {
		// deployed_at is stored as a UTC wall-clock in a timestamp-without-time-zone
		// column. Bind the cursor as a UTC instant; the query converts it back to
		// that wall clock explicitly, independent of the session TimeZone.
		cursorTime = cursor.DeployedAt.UTC()
		cursorID = cursor.ID
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+userDeploymentColumns+`, a.name,
		       CASE
		         WHEN NOT (d.account_id = ANY(COALESCE($3::uuid[], ARRAY[]::uuid[]))) THEN TRUE
		         WHEN fs.desired_state IS DISTINCT FROM 'registered' THEN TRUE
		         ELSE COALESCE(
		           fs.desired_state = 'registered'
		           AND fs.synced_state = fs.desired_state
		           AND fs.synced_version = fs.desired_version
		           AND NOT fs.creator_assignment_pending,
		           FALSE
		         )
		       END AS access_ready
		FROM deployments d
		JOIN account_members am
		  ON am.account_id = d.account_id
		 AND am.user_id = $1
		JOIN accounts a ON a.id = d.account_id AND a.deleted_at IS NULL
		LEFT JOIN deployment_fga_sync fs ON fs.deployment_id = d.id
		WHERE d.account_id = ANY($2::uuid[])
		  AND d.status <> 'undeployed'
	`+fgaReadPredicate+`
		  AND (
		    $5::timestamptz IS NULL
		    OR (d.deployed_at, d.id) < (($5::timestamptz AT TIME ZONE 'UTC'), $6)
		  )
		  AND (
		    $7::text = ''
		    OR strpos(lower(d.agent_name), lower($7)) > 0
		    OR strpos(lower(COALESCE(d.display_name, '')), lower($7)) > 0
		  )
		ORDER BY d.deployed_at DESC, d.id DESC
		LIMIT $8
	`, userID, pq.Array(accountIDs), pq.Array(fgaAccountIDs), pq.Array(readableDeploymentIDs),
		cursorTime, cursorID, search, limit)
	if err != nil {
		return nil, fmt.Errorf("list visible deployments for user: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	deployments := make([]UserDeployment, 0, limit)
	for rows.Next() {
		var deployment Deployment
		var accountName string
		var accessReady bool
		if err := rows.Scan(
			&deployment.ID,
			&deployment.AccountID,
			&deployment.SourceAccountID,
			&deployment.AgentName,
			&deployment.BuildID,
			&deployment.Namespace,
			&deployment.DisplayName,
			&deployment.Status,
			&deployment.ErrorMessage,
			&deployment.DeployedAt,
			&deployment.AvatarColors,
			&accountName,
			&accessReady,
		); err != nil {
			return nil, fmt.Errorf("scan visible deployment for user: %w", err)
		}
		deployments = append(deployments, UserDeployment{
			Deployment:  &deployment,
			AccountName: accountName,
			AccessReady: accessReady,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate visible deployments for user: %w", err)
	}
	return deployments, nil
}

// ListVisibleDeploymentIDsForUser authorizes a bounded set of deployment IDs
// in one query. It powers visible-card projections without loading every
// deployment owned by the selected accounts.
func (s *Store) ListVisibleDeploymentIDsForUser(
	ctx context.Context,
	userID string,
	deploymentIDs []string,
) ([]string, error) {
	return s.ListReadableDeploymentIDsForUser(ctx, userID, deploymentIDs, nil, nil)
}

// ListReadableDeploymentIDsForUser applies membership visibility plus FGA to
// deployment IDs that have entered the lifecycle ledger.
func (s *Store) ListReadableDeploymentIDsForUser(
	ctx context.Context,
	userID string,
	deploymentIDs []string,
	fgaAccountIDs []string,
	readableDeploymentIDs []string,
) ([]string, error) {
	return s.listReadableDeploymentIDsForUser(
		ctx, userID, deploymentIDs, fgaAccountIDs, readableDeploymentIDs, false,
	)
}

// ListReadableDeploymentHistoryIDsForUser also authorizes undeployed instances
// so readable revisions are retained in deployment history.
func (s *Store) ListReadableDeploymentHistoryIDsForUser(
	ctx context.Context,
	userID string,
	deploymentIDs []string,
	fgaAccountIDs []string,
	readableDeploymentIDs []string,
) ([]string, error) {
	return s.listReadableDeploymentIDsForUser(
		ctx, userID, deploymentIDs, fgaAccountIDs, readableDeploymentIDs, true,
	)
}

func (s *Store) listReadableDeploymentIDsForUser(
	ctx context.Context,
	userID string,
	deploymentIDs []string,
	fgaAccountIDs []string,
	readableDeploymentIDs []string,
	includeUndeployed bool,
) ([]string, error) {
	if userID == "" || len(deploymentIDs) == 0 {
		return []string{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id
		FROM deployments d
		JOIN account_members am
		  ON am.account_id = d.account_id
		 AND am.user_id = $1
		JOIN accounts a ON a.id = d.account_id AND a.deleted_at IS NULL
		WHERE d.id = ANY($2::varchar[])
		  AND ($5::boolean OR d.status <> 'undeployed')
	`+fgaReadPredicate+`
	`, userID, pq.Array(deploymentIDs), pq.Array(fgaAccountIDs), pq.Array(readableDeploymentIDs), includeUndeployed)
	if err != nil {
		return nil, fmt.Errorf("list visible deployment ids for user: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	visible := make([]string, 0, len(deploymentIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan visible deployment id for user: %w", err)
		}
		visible = append(visible, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate visible deployment ids for user: %w", err)
	}
	return visible, nil
}

// AccountsWithManagedDeployments returns selected accounts containing at
// least one active deployment that has entered the WorkOS lifecycle.
func (s *Store) AccountsWithManagedDeployments(ctx context.Context, accountIDs []string) ([]string, error) {
	if len(accountIDs) == 0 {
		return []string{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT d.account_id
		FROM deployments d
		JOIN deployment_fga_sync s ON s.deployment_id = d.id
		WHERE d.account_id = ANY($1::uuid[])
		  AND d.status <> 'undeployed'
		  AND s.desired_state = 'registered'
	`, pq.Array(accountIDs))
	if err != nil {
		return nil, fmt.Errorf("list FGA-managed deployment accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make([]string, 0, len(accountIDs))
	for rows.Next() {
		var accountID string
		if err := rows.Scan(&accountID); err != nil {
			return nil, fmt.Errorf("scan FGA-managed deployment account: %w", err)
		}
		result = append(result, accountID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate FGA-managed deployment accounts: %w", err)
	}
	return result, nil
}

// CountReadableDeploymentsForUser counts active deployments after applying
// the same rollout-aware visibility rule used by the list endpoints.
func (s *Store) CountReadableDeploymentsForUser(
	ctx context.Context,
	userID string,
	accountID string,
	fgaAccountIDs []string,
	readableDeploymentIDs []string,
) (int, error) {
	if userID == "" || accountID == "" {
		return 0, nil
	}
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM deployments d
		JOIN account_members am
		  ON am.account_id = d.account_id
		 AND am.user_id = $1
		WHERE d.account_id = $2
		  AND d.status <> 'undeployed'
	`+fgaReadPredicate+`
	`, userID, accountID, pq.Array(fgaAccountIDs), pq.Array(readableDeploymentIDs)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count readable deployments for user: %w", err)
	}
	return count, nil
}
