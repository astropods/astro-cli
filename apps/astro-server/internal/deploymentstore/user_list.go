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
}

const userDeploymentColumns = `d.id, d.account_id, d.source_account_id, d.agent_name, d.build_id,
       d.namespace, d.display_name, d.status, d.error_message, d.deployed_at, d.avatar_colors`

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
		SELECT `+userDeploymentColumns+`, a.name
		FROM deployments d
		JOIN account_members am
		  ON am.account_id = d.account_id
		 AND am.user_id = $1
		JOIN accounts a ON a.id = d.account_id AND a.deleted_at IS NULL
		WHERE d.account_id = ANY($2::uuid[])
		  AND d.status <> 'undeployed'
		  AND (
		    $3::timestamptz IS NULL
		    OR (d.deployed_at, d.id) < (($3::timestamptz AT TIME ZONE 'UTC'), $4)
		  )
		  AND (
		    $5::text = ''
		    OR strpos(lower(d.agent_name), lower($5)) > 0
		    OR strpos(lower(COALESCE(d.display_name, '')), lower($5)) > 0
		  )
		ORDER BY d.deployed_at DESC, d.id DESC
		LIMIT $6
	`, userID, pq.Array(accountIDs), cursorTime, cursorID, search, limit)
	if err != nil {
		return nil, fmt.Errorf("list visible deployments for user: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	deployments := make([]UserDeployment, 0, limit)
	for rows.Next() {
		var deployment Deployment
		var accountName string
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
		); err != nil {
			return nil, fmt.Errorf("scan visible deployment for user: %w", err)
		}
		deployments = append(deployments, UserDeployment{
			Deployment:  &deployment,
			AccountName: accountName,
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
		  AND d.status <> 'undeployed'
	`, userID, pq.Array(deploymentIDs))
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
