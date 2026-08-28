package authzbackfill

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) BackfillBlueprintIDs(ctx context.Context, batchSize int, dryRun bool) (int, error) {
	if dryRun {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM agents WHERE uid IS NULL`).Scan(&count); err != nil {
			return 0, fmt.Errorf("count missing blueprint ids: %w", err)
		}
		return count, nil
	}

	var total int
	for {
		result, err := s.db.ExecContext(ctx, `
			WITH batch AS (
				SELECT account_id, name
				FROM agents
				WHERE uid IS NULL
				ORDER BY account_id, name
				LIMIT $1
				FOR UPDATE SKIP LOCKED
			)
			UPDATE agents a
			SET uid = gen_random_uuid()
			FROM batch
			WHERE a.account_id = batch.account_id AND a.name = batch.name
		`, batchSize)
		if err != nil {
			return total, fmt.Errorf("fill missing blueprint ids: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("count filled blueprint ids: %w", err)
		}
		total += int(changed)
		if changed < int64(batchSize) {
			return total, nil
		}
	}
}

func (s *Store) ListAccounts(ctx context.Context, after string, limit int) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id::text,
		       ao.workos_org_id,
		       COALESCE(NULLIF(a.display_name, ''), a.name),
		       COALESCE(amw.workos_membership_id, '')
		FROM accounts a
		JOIN account_organizations ao ON ao.account_id = a.id
		LEFT JOIN account_member_workos amw
		  ON amw.account_id = a.id AND amw.user_id = a.owner_user_id
		WHERE a.type = 'organization'
		  AND a.deleted_at IS NULL
		  AND a.id > COALESCE(NULLIF($1, '')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
		ORDER BY a.id
		LIMIT $2
	`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("query authorization backfill accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	accounts := make([]Account, 0, limit)
	for rows.Next() {
		var account Account
		if err := rows.Scan(&account.ID, &account.OrganizationID, &account.Name, &account.OwnerMembershipID); err != nil {
			return nil, fmt.Errorf("scan authorization backfill account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authorization backfill accounts: %w", err)
	}
	return accounts, nil
}

func (s *Store) ListResources(ctx context.Context, accountIDs []string) (map[string][]Resource, error) {
	if len(accountIDs) == 0 {
		return map[string][]Resource{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT account_id::text, resource_type, external_id, name
		FROM (
			SELECT a.id AS account_id, 'insights'::text AS resource_type, a.id::text AS external_id, 'Insights'::text AS name
			FROM accounts a
			WHERE a.id = ANY($1::uuid[])
			UNION ALL
			SELECT account_id, 'blueprint', uid::text, name
			FROM agents
			WHERE account_id = ANY($1::uuid[]) AND archived_at IS NULL AND uid IS NOT NULL
			UNION ALL
			SELECT account_id, 'deployment', id, COALESCE(NULLIF(display_name, ''), agent_name)
			FROM deployments
			WHERE account_id = ANY($1::uuid[]) AND status <> 'undeployed'
			UNION ALL
			SELECT account_id, 'variable', account_id::text || ':' || name, name
			FROM account_variables
			WHERE account_id = ANY($1::uuid[])
			UNION ALL
			SELECT account_id, 'knowledge_store', id, name
			FROM knowledge_stores
			WHERE account_id = ANY($1::uuid[])
		) resources
		ORDER BY account_id, resource_type, external_id
	`, pq.Array(accountIDs))
	if err != nil {
		return nil, fmt.Errorf("query authorization backfill resources: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	resources := make(map[string][]Resource, len(accountIDs))
	for rows.Next() {
		var resource Resource
		if err := rows.Scan(&resource.AccountID, &resource.Ref.Type, &resource.Ref.ExternalID, &resource.Name); err != nil {
			return nil, fmt.Errorf("scan authorization backfill resource: %w", err)
		}
		resources[resource.AccountID] = append(resources[resource.AccountID], resource)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authorization backfill resources: %w", err)
	}
	return resources, nil
}
