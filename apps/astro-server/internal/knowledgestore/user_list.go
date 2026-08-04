package knowledgestore

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
)

const userStoreColumns = `ks.id, ks.account_id, ks.name, ks.arn, ks.provider, ks.mode, ks.status, ks.storage, ks.storage_class,
       ks.public, ks.public_host, ks.error, ks.annotations, ks.created_at, ks.updated_at`

func userStoreScanDest(store *KnowledgeStore) []any {
	return []any{
		&store.ID, &store.AccountID, &store.Name, &store.ARN, &store.Provider,
		&store.Mode, &store.Status, &store.Storage, &store.StorageClass, &store.Public,
		&store.PublicHost, &store.Error, &store.Annotations, &store.CreatedAt, &store.UpdatedAt,
	}
}

type UserKnowledgeCursor struct {
	CreatedAt time.Time
	ID        string
}

type UserKnowledgeStore struct {
	Store       *KnowledgeStore
	AccountName string
}

// ListVisibleForUserPage returns one membership-guarded keyset page across the
// selected accounts. All list fields come from PostgreSQL; no cluster reads are
// performed.
func (s *Store) ListVisibleForUserPage(
	ctx context.Context,
	userID string,
	accountIDs []string,
	search string,
	limit int,
	cursor *UserKnowledgeCursor,
) ([]UserKnowledgeStore, error) {
	if len(accountIDs) == 0 {
		return []UserKnowledgeStore{}, nil
	}
	args := []any{userID, pq.Array(accountIDs)}
	where := `am.user_id = $1 AND ks.account_id = ANY($2::uuid[]) AND a.deleted_at IS NULL`
	if search != "" {
		where += ` AND strpos(lower(ks.name), lower($` + fmt.Sprint(len(args)+1) + `)) > 0`
		args = append(args, search)
	}
	if cursor != nil {
		createdAtArg := len(args) + 1
		idArg := createdAtArg + 1
		where += ` AND (ks.created_at, ks.id) < ($` + fmt.Sprint(createdAtArg) + `, $` + fmt.Sprint(idArg) + `)`
		args = append(args, cursor.CreatedAt, cursor.ID)
	}
	args = append(args, limit)
	query := `
		SELECT ` + userStoreColumns + `, a.name
		FROM knowledge_stores ks
		JOIN account_members am ON am.account_id = ks.account_id
		JOIN accounts a ON a.id = ks.account_id
		WHERE ` + where + `
		ORDER BY ks.created_at DESC, ks.id DESC
		LIMIT $` + fmt.Sprint(len(args)) // #nosec G202 -- limit placeholder number is server-generated
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query user knowledge stores: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make([]UserKnowledgeStore, 0)
	for rows.Next() {
		var store KnowledgeStore
		var accountName string
		if err := rows.Scan(append(userStoreScanDest(&store), &accountName)...); err != nil {
			return nil, fmt.Errorf("scan user knowledge store: %w", err)
		}
		result = append(result, UserKnowledgeStore{Store: &store, AccountName: accountName})
	}
	return result, rows.Err()
}
