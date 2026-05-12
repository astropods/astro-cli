// Package clusterstore manages workload cluster records in PostgreSQL.
//
// astro-server reconciles tenant agent deployments into one of these clusters.
// A row here is the registration record for a Kubernetes cluster the control
// plane can talk to — it does not provision anything, just records that the
// cluster exists and how to authenticate to it.
//
// See `sql/astro-server/schema.sql` (clusters table) for the schema.
package clusterstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/lib/pq"
)

// Errors returned by the store.
var (
	ErrNotFound      = errors.New("cluster not found")
	ErrAlreadyExists = errors.New("cluster already registered")
	ErrInUse         = errors.New("cluster has active deployments")
)

// Postgres SQLSTATE codes we translate to typed errors.
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
)

// idPattern enforces a conservative cluster id format: lowercase letters,
// digits, dashes. This matches Kubernetes-style names and lets the id appear
// safely in DNS labels and IAM role names.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$`)

// Cluster is a managed workload Kubernetes cluster known to astro-server.
type Cluster struct {
	ID                 string
	Region             string
	EKSClusterName     string
	EKSClusterEndpoint string
	Enabled            bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ValidateID returns nil if id is a valid cluster identifier.
func ValidateID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("cluster id %q must match %s", id, idPattern.String())
	}
	return nil
}

// Store provides CRUD access to the clusters table.
type Store struct {
	db *sql.DB
}

// New constructs a Store backed by the given database connection.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Register inserts a new cluster. Returns ErrAlreadyExists if a row with the
// same id is already present.
func (s *Store) Register(ctx context.Context, c *Cluster) error {
	if err := ValidateID(c.ID); err != nil {
		return err
	}
	if c.EKSClusterName == "" {
		return fmt.Errorf("eks_cluster_name is required")
	}
	if c.EKSClusterEndpoint == "" {
		return fmt.Errorf("eks_cluster_endpoint is required")
	}
	if c.Region == "" {
		return fmt.Errorf("region is required")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO clusters (
			id, region, eks_cluster_name, eks_cluster_endpoint, enabled
		) VALUES ($1, $2, $3, $4, $5)`,
		c.ID, c.Region, c.EKSClusterName, c.EKSClusterEndpoint, c.Enabled,
	)
	if err != nil {
		if pgCode(err) == pgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("insert cluster: %w", err)
	}
	return nil
}

// Get returns the cluster with the given id.
func (s *Store) Get(ctx context.Context, id string) (*Cluster, error) {
	row := s.db.QueryRowContext(ctx, baseSelect+` WHERE id = $1`, id)
	c, err := scanCluster(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	return c, nil
}

// List returns all clusters, ordered by region then id. When enabledOnly is
// true, disabled clusters are excluded.
func (s *Store) List(ctx context.Context, enabledOnly bool) ([]*Cluster, error) {
	query := baseSelect
	if enabledOnly {
		query += ` WHERE enabled = true`
	}
	query += ` ORDER BY region ASC, id ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var clusters []*Cluster
	for rows.Next() {
		c, scanErr := scanCluster(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan cluster: %w", scanErr)
		}
		clusters = append(clusters, c)
	}
	return clusters, rows.Err()
}

// SetEnabled flips the enabled flag for a cluster. Returns ErrNotFound if no
// row matches.
func (s *Store) SetEnabled(ctx context.Context, id string, enabled bool) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE clusters SET enabled = $1, updated_at = now()
		WHERE id = $2`,
		enabled, id,
	)
	if err != nil {
		return fmt.Errorf("set enabled: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Deregister deletes a cluster row. Returns ErrInUse if any deployment still
// references it (the FK is ON DELETE RESTRICT). Returns ErrNotFound if no row
// matches.
func (s *Store) Deregister(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM clusters WHERE id = $1`, id)
	if err != nil {
		if pgCode(err) == pgForeignKeyViolation {
			return ErrInUse
		}
		return fmt.Errorf("delete cluster: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// baseSelect is the column projection shared by Get and List.
const baseSelect = `
	SELECT id, region, eks_cluster_name, eks_cluster_endpoint,
	       enabled, created_at, updated_at
	FROM clusters`

// rowScanner is the subset of sql.Row / sql.Rows we need.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanCluster(r rowScanner) (*Cluster, error) {
	var c Cluster
	if err := r.Scan(
		&c.ID, &c.Region, &c.EKSClusterName, &c.EKSClusterEndpoint,
		&c.Enabled, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// pgCode returns the SQLSTATE code from a Postgres error, or "" if err is not
// a *pq.Error. Matches the pattern used by other stores in this codebase
// (see handlers/agents.go, handlers/knowledge.go).
func pgCode(err error) string {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code)
	}
	return ""
}
