package account

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

var (
	ErrClusterInUse = errors.New("account still has deployments on this cluster")

	ErrClusterNotAllowed = errors.New("cluster is not allowed for this account")
)

type ClusterBinding struct {
	ClusterID   string `json:"cluster_id"`
	Region      string `json:"region"`
	RegionLabel string `json:"region_label"`
	RegionFlag  string `json:"region_flag,omitempty"`
	IsDefault   bool   `json:"is_default"`
}

var inUseStatuses = []string{
	deploymentstore.StatusActive,
	deploymentstore.StatusFailed,
	deploymentstore.StatusPending,
}

type ClusterBindings struct {
	db       *sql.DB
	clusters clusterid.Resolver
}

func NewClusterBindings(db *sql.DB, clusters clusterid.Resolver) *ClusterBindings {
	return &ClusterBindings{db: db, clusters: clusters}
}

func (b *ClusterBindings) Clusters() clusterid.Resolver { return b.clusters }

func (s *AccountStore) Clusters(clusters clusterid.Resolver) *ClusterBindings {
	return NewClusterBindings(s.db, clusters)
}

func (b *ClusterBindings) List(accountID string) ([]ClusterBinding, error) {
	out, err := b.read(accountID)
	if err != nil {
		return nil, err
	}
	if len(out) > 0 || b.clusters.Primary() == "" {
		return out, nil
	}
	if err := b.materializePrimary(b.db, accountID); err != nil {
		return nil, err
	}
	return b.read(accountID)
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (b *ClusterBindings) materializePrimary(db execer, accountID string) error {
	_, err := db.Exec(`
		INSERT INTO account_clusters (account_id, cluster_id, is_default)
		SELECT $1::uuid, $2::varchar, true
		WHERE EXISTS (SELECT 1 FROM clusters WHERE id = $2::varchar)
		  AND EXISTS (SELECT 1 FROM accounts WHERE id = $1::uuid AND deleted_at IS NULL)
		  AND NOT EXISTS (SELECT 1 FROM account_clusters WHERE account_id = $1::uuid)
		ON CONFLICT (account_id, cluster_id) DO NOTHING
	`, accountID, b.clusters.Primary())
	if err != nil {
		return fmt.Errorf("bind primary cluster %q: %w", b.clusters.Primary(), err)
	}
	return nil
}

func (b *ClusterBindings) read(accountID string) ([]ClusterBinding, error) {
	rows, err := b.db.Query(`
		SELECT ac.cluster_id, c.region, ac.is_default
		FROM account_clusters ac
		JOIN clusters c ON c.id = ac.cluster_id
		WHERE ac.account_id = $1
		ORDER BY ac.is_default DESC, ac.cluster_id
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list account clusters: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []ClusterBinding
	for rows.Next() {
		var b ClusterBinding
		if err := rows.Scan(&b.ClusterID, &b.Region, &b.IsDefault); err != nil {
			return nil, fmt.Errorf("scan account cluster: %w", err)
		}
		b.RegionLabel = RegionLabel(b.Region)
		b.RegionFlag = RegionFlag(b.Region)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account clusters: %w", err)
	}
	return out, nil
}

func DefaultClusterID(allowed []ClusterBinding) string {
	for _, b := range allowed {
		if b.IsDefault {
			return b.ClusterID
		}
	}
	if len(allowed) == 0 {
		return ""
	}
	return slices.MinFunc(allowed, func(a, b ClusterBinding) int {
		return cmp.Compare(a.ClusterID, b.ClusterID)
	}).ClusterID
}

func IsAllowed(clusterID string, allowed []ClusterBinding) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, b := range allowed {
		if b.ClusterID == clusterID {
			return true
		}
	}
	return false
}

func (b *ClusterBindings) Add(accountID, clusterID string, setDefault bool) error {
	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("begin add account cluster: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var exists bool
	err = tx.QueryRow(`SELECT true FROM clusters WHERE id = $1`, clusterID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("cluster not found: %s", clusterID)
	}
	if err != nil {
		return fmt.Errorf("look up cluster: %w", err)
	}

	if b.clusters.Primary() != "" {
		if err := b.materializePrimary(tx, accountID); err != nil {
			return err
		}
	}

	var accountHasDefault, clusterIsDefault bool
	err = tx.QueryRow(`
		SELECT
			COALESCE(bool_or(is_default), false),
			COALESCE(bool_or(is_default AND cluster_id = $2), false)
		FROM account_clusters WHERE account_id = $1
	`, accountID, clusterID).Scan(&accountHasDefault, &clusterIsDefault)
	if err != nil {
		return fmt.Errorf("read account default cluster: %w", err)
	}
	isDefault := setDefault || clusterIsDefault || !accountHasDefault

	if isDefault {
		if _, err := tx.Exec(`
			UPDATE account_clusters SET is_default = false
			WHERE account_id = $1 AND is_default
		`, accountID); err != nil {
			return fmt.Errorf("clear existing default cluster: %w", err)
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO account_clusters (account_id, cluster_id, is_default)
		VALUES ($1, $2, $3)
		ON CONFLICT (account_id, cluster_id) DO UPDATE SET is_default = EXCLUDED.is_default
	`, accountID, clusterID, isDefault); err != nil {
		return fmt.Errorf("insert account cluster: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add account cluster: %w", err)
	}
	return nil
}

func (b *ClusterBindings) Remove(accountID, clusterID string) error {
	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("begin remove account cluster: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var inUse int
	err = tx.QueryRow(`
		SELECT count(*) FROM deployments
		WHERE account_id = $1 AND cluster_id = $2 AND status = ANY($3)
	`, accountID, clusterID, pq.Array(inUseStatuses)).Scan(&inUse)
	if err != nil {
		return fmt.Errorf("count deployments on cluster: %w", err)
	}
	if inUse > 0 {
		return fmt.Errorf("%d deployment(s) still on cluster %s: %w", inUse, clusterID, ErrClusterInUse)
	}

	res, err := tx.Exec(`
		DELETE FROM account_clusters WHERE account_id = $1 AND cluster_id = $2
	`, accountID, clusterID)
	if err != nil {
		return fmt.Errorf("delete account cluster: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete account cluster rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("cluster %s not allowed for account %s: %w", clusterID, accountID, ErrClusterNotAllowed)
	}

	if _, err := tx.Exec(`
		UPDATE account_clusters SET is_default = true
		WHERE account_id = $1
		  AND cluster_id = (
		        SELECT cluster_id FROM account_clusters
		        WHERE account_id = $1 ORDER BY cluster_id LIMIT 1
		      )
		  AND NOT EXISTS (
		        SELECT 1 FROM account_clusters WHERE account_id = $1 AND is_default
		      )
	`, accountID); err != nil {
		return fmt.Errorf("promote replacement default cluster: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove account cluster: %w", err)
	}
	return nil
}

func (b *ClusterBindings) SetDefault(accountID, clusterID string) error {
	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("begin set default cluster: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`
		UPDATE account_clusters SET is_default = false
		WHERE account_id = $1 AND is_default
	`, accountID); err != nil {
		return fmt.Errorf("clear existing default cluster: %w", err)
	}

	res, err := tx.Exec(`
		UPDATE account_clusters SET is_default = true
		WHERE account_id = $1 AND cluster_id = $2
	`, accountID, clusterID)
	if err != nil {
		return fmt.Errorf("set default cluster: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set default cluster rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("cluster %s not allowed for account %s: %w", clusterID, accountID, ErrClusterNotAllowed)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set default cluster: %w", err)
	}
	return nil
}
