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
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/clusterfields"
	"github.com/astropods/astro/apps/astro-server/internal/commalist"

	"github.com/lib/pq"
)

// Errors returned by the store.
var (
	ErrNotFound      = errors.New("cluster not found")
	ErrAlreadyExists = errors.New("cluster already registered")
	// ErrInUse is a catch-all for a foreign-key violation on delete when the
	// violated constraint isn't one of the two known ones below. Deregister
	// should normally return one of the more specific errors instead.
	ErrInUse = errors.New("cluster is still referenced by accounts or deployments")
	// ErrInUseByAccounts means an account still has this cluster set as its
	// cluster_id (accounts_cluster_id_fkey) — independent of whether that
	// account has any deployments here.
	ErrInUseByAccounts = errors.New("cluster still has accounts pinned to it")
	// ErrInUseByDeployments means a deployment row still references this
	// cluster (deployments_cluster_id_fkey), regardless of deployment status.
	ErrInUseByDeployments = errors.New("cluster has active deployments")
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
//
// Every ingress field is required: Register and Update reject
// empty values via clusterfields validation, and clustercfg.Resolve fails
// the deploy if a stored row carries an empty field. The primary cluster
// has no row in this table — it reads env vars directly (INGRESS_DOMAIN,
// INGESTION_INGRESS_DOMAIN, ...) and is the only place
// those env defaults apply. TLS termination and DNS for the tenant-router
// data plane are owned by the front-door ALB in astro-infra, so per-
// cluster ACM cert ARNs and per-tenant ALB group names are not stored here.
type Cluster struct {
	ID                     string
	Region                 string
	EKSClusterName         string
	EKSClusterEndpoint     string
	EKSClusterCA           []byte // PEM CA bytes; supplied at registration so per-cluster client builds skip cross-account DescribeCluster
	Enabled                bool
	AgentIngressDomain     string
	IngestionIngressDomain string
	LangfuseBaseURLExt     string // collector LANGFUSE_BASE_URL (http://langfuse.platform...:3000)
	LangfuseVPCEIPs        string // comma-separated VPCE ENI /32 targets for netpol egress
	PodSubnetCIDRs         string // comma-separated pod subnet CIDRs for netpol except list
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// ValidateID returns nil if id is a valid cluster identifier.
func ValidateID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("cluster id %q must match %s", id, idPattern.String())
	}
	return nil
}

func deployConfigFromCluster(c *Cluster) clusterfields.DeployConfig {
	return clusterfields.DeployConfig{
		AgentIngressDomain:     c.AgentIngressDomain,
		IngestionIngressDomain: c.IngestionIngressDomain,
		LangfuseBaseURLExt:     c.LangfuseBaseURLExt,
		LangfuseVPCEIPs:        c.LangfuseVPCEIPs,
		PodSubnetCIDRs:         c.PodSubnetCIDRs,
	}
}

// validateRequiredFields enforces that every additional cluster carries the
// full EKS / ingress / cert configuration. Falling back to env
// defaults is reserved for the primary cluster (which has no row); registered
// clusters must declare these values explicitly.
func validateRequiredFields(c *Cluster) error {
	if err := clusterfields.ValidateRegistrationNonEmpty(clusterfields.Registration{
		Region:             c.Region,
		EKSClusterName:     c.EKSClusterName,
		EKSClusterEndpoint: c.EKSClusterEndpoint,
		Deploy:             deployConfigFromCluster(c),
	}); err != nil {
		return err
	}
	if len(c.EKSClusterCA) == 0 {
		return fmt.Errorf("eks_cluster_ca is required (PEM bytes captured at registration via `aws eks describe-cluster`)")
	}
	if err := validateLangfuseBaseURL(c.LangfuseBaseURLExt); err != nil {
		return err
	}
	if err := validateLangfuseVPCEIPs(c.LangfuseVPCEIPs); err != nil {
		return err
	}
	if err := validatePodSubnetCIDRs(c.PodSubnetCIDRs); err != nil {
		return err
	}
	return nil
}

func validateLangfuseBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("langfuse_base_url_ext must be a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("langfuse_base_url_ext must use http or https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("langfuse_base_url_ext must include a host")
	}
	return nil
}

// validateLangfuseVPCEIPs ensures each token is a bare IP. The deploy applier
// appends /32 when building NetworkPolicy rules; CIDR notation here would
// produce an invalid CIDR like 10.0.1.10/32/32.
func validateLangfuseVPCEIPs(raw string) error {
	ips := commalist.Parse(raw)
	if len(ips) == 0 {
		return fmt.Errorf("langfuse_vpce_ips must contain at least one bare IP address (comma-separated)")
	}
	for _, ip := range ips {
		if strings.Contains(ip, "/") {
			return fmt.Errorf(
				"langfuse_vpce_ips values must be bare IP addresses without prefix length (e.g. 10.0.1.10), got %q",
				ip,
			)
		}
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("langfuse_vpce_ips value %q is not a valid IP address", ip)
		}
	}
	return nil
}

func validatePodSubnetCIDRs(raw string) error {
	cidrs := commalist.Parse(raw)
	if len(cidrs) == 0 {
		return fmt.Errorf("pod_subnet_cidrs must contain at least one CIDR (comma-separated)")
	}
	for _, cidr := range cidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("pod_subnet_cidrs value %q is not a valid CIDR: %w", cidr, err)
		}
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
// same id is already present. All EKS / ingress / cert fields
// are mandatory for additional clusters — empty values are rejected so
// deploys can never silently fall through to the server-wide env defaults
// that exist only for the primary cluster.
func (s *Store) Register(ctx context.Context, c *Cluster) error {
	if err := ValidateID(c.ID); err != nil {
		return err
	}
	if err := validateRequiredFields(c); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO clusters (
			id, region, eks_cluster_name, eks_cluster_endpoint, eks_cluster_ca, enabled,
			agent_ingress_domain, ingestion_ingress_domain,
			langfuse_base_url_ext, langfuse_vpce_ips, pod_subnet_cidrs
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		c.ID, c.Region, c.EKSClusterName, c.EKSClusterEndpoint, c.EKSClusterCA, c.Enabled,
		c.AgentIngressDomain, c.IngestionIngressDomain,
		c.LangfuseBaseURLExt, c.LangfuseVPCEIPs, c.PodSubnetCIDRs,
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

// Update changes mutable fields on an additional cluster row. All required
// fields must be non-empty. Pass current values unchanged to leave them as-is.
// The primary cluster has no row and reads env vars directly.
func (s *Store) Update(ctx context.Context, c *Cluster) error {
	if c == nil {
		return fmt.Errorf("cluster is required")
	}
	if err := validateRequiredFields(c); err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE clusters SET
			region = $1,
			eks_cluster_name = $2,
			eks_cluster_endpoint = $3,
			eks_cluster_ca = $4,
			agent_ingress_domain = $5,
			ingestion_ingress_domain = $6,
			langfuse_base_url_ext = $7,
			langfuse_vpce_ips = $8,
			pod_subnet_cidrs = $9,
			updated_at = now()
		WHERE id = $10`,
		c.Region, c.EKSClusterName, c.EKSClusterEndpoint, c.EKSClusterCA,
		c.AgentIngressDomain, c.IngestionIngressDomain,
		c.LangfuseBaseURLExt, c.LangfuseVPCEIPs, c.PodSubnetCIDRs,
		c.ID,
	)
	if err != nil {
		return fmt.Errorf("update cluster: %w", err)
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

// Deregister deletes a cluster row. Both accounts.cluster_id and
// deployments.cluster_id are ON DELETE RESTRICT, so it returns
// ErrInUseByAccounts or ErrInUseByDeployments depending on which FK actually
// blocked the delete (ErrInUse if Postgres doesn't report the constraint
// name). Returns ErrNotFound if no row matches.
//
// If the delete is blocked by a deployments row, it self-heals once: rows
// that are already undeployed don't need cluster_id anymore (updateStatusTx
// clears it going forward; this catches rows undeployed before that fix)
// and are safe to clear here too, then the delete is retried. Accounts are
// never auto-cleared — a cluster_id pin there is a live routing decision,
// not stale history.
func (s *Store) Deregister(ctx context.Context, id string) error {
	err := s.tryDeleteCluster(ctx, id)
	if !errors.Is(err, ErrInUseByDeployments) {
		return err
	}
	if _, healErr := s.db.ExecContext(ctx,
		`UPDATE deployments SET cluster_id = NULL WHERE cluster_id = $1 AND status = 'undeployed'`, id,
	); healErr != nil {
		return fmt.Errorf("clear stale cluster_id on undeployed deployments: %w", healErr)
	}
	return s.tryDeleteCluster(ctx, id)
}

func (s *Store) tryDeleteCluster(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM clusters WHERE id = $1`, id)
	if err != nil {
		if pgCode(err) == pgForeignKeyViolation {
			switch pgConstraint(err) {
			case "accounts_cluster_id_fkey":
				return ErrInUseByAccounts
			case "deployments_cluster_id_fkey":
				return ErrInUseByDeployments
			default:
				return ErrInUse
			}
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

// Blocker is a single account or deployment row still referencing a cluster,
// as surfaced to admins deciding what to move before deregistering.
type Blocker struct {
	ID     string
	Name   string
	Status string // empty for accounts
}

// Blockers lists (up to a cap) the accounts and deployments that currently
// reference a cluster and would fail Deregister, plus their total counts.
func (s *Store) Blockers(ctx context.Context, id string) (accounts []Blocker, accountCount int, deployments []Blocker, deploymentCount int, err error) {
	const limit = 25

	if err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE cluster_id = $1`, id).Scan(&accountCount); err != nil {
		return nil, 0, nil, 0, fmt.Errorf("count blocking accounts: %w", err)
	}
	accountRows, err := s.db.QueryContext(ctx,
		`SELECT id, name FROM accounts WHERE cluster_id = $1 ORDER BY name LIMIT $2`, id, limit)
	if err != nil {
		return nil, 0, nil, 0, fmt.Errorf("list blocking accounts: %w", err)
	}
	for accountRows.Next() {
		var b Blocker
		if err = accountRows.Scan(&b.ID, &b.Name); err != nil {
			_ = accountRows.Close()
			return nil, 0, nil, 0, fmt.Errorf("scan blocking account: %w", err)
		}
		accounts = append(accounts, b)
	}
	if err = accountRows.Err(); err != nil {
		_ = accountRows.Close()
		return nil, 0, nil, 0, fmt.Errorf("list blocking accounts: %w", err)
	}
	if err = accountRows.Close(); err != nil {
		return nil, 0, nil, 0, fmt.Errorf("list blocking accounts: %w", err)
	}

	if err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM deployments WHERE cluster_id = $1`, id).Scan(&deploymentCount); err != nil {
		return nil, 0, nil, 0, fmt.Errorf("count blocking deployments: %w", err)
	}
	deploymentRows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_name, status FROM deployments WHERE cluster_id = $1 ORDER BY deployed_at DESC LIMIT $2`, id, limit)
	if err != nil {
		return nil, 0, nil, 0, fmt.Errorf("list blocking deployments: %w", err)
	}
	for deploymentRows.Next() {
		var b Blocker
		if err = deploymentRows.Scan(&b.ID, &b.Name, &b.Status); err != nil {
			_ = deploymentRows.Close()
			return nil, 0, nil, 0, fmt.Errorf("scan blocking deployment: %w", err)
		}
		deployments = append(deployments, b)
	}
	if err = deploymentRows.Err(); err != nil {
		_ = deploymentRows.Close()
		return nil, 0, nil, 0, fmt.Errorf("list blocking deployments: %w", err)
	}
	if err = deploymentRows.Close(); err != nil {
		return nil, 0, nil, 0, fmt.Errorf("list blocking deployments: %w", err)
	}
	return accounts, accountCount, deployments, deploymentCount, nil
}

// baseSelect is the column projection shared by Get and List.
const baseSelect = `
	SELECT id, region, eks_cluster_name, eks_cluster_endpoint, eks_cluster_ca, enabled,
	       agent_ingress_domain, ingestion_ingress_domain,
	       langfuse_base_url_ext, langfuse_vpce_ips, pod_subnet_cidrs,
	       created_at, updated_at
	FROM clusters`

// rowScanner is the subset of sql.Row / sql.Rows we need.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanCluster(r rowScanner) (*Cluster, error) {
	var c Cluster
	if err := r.Scan(
		&c.ID, &c.Region, &c.EKSClusterName, &c.EKSClusterEndpoint, &c.EKSClusterCA, &c.Enabled,
		&c.AgentIngressDomain, &c.IngestionIngressDomain,
		&c.LangfuseBaseURLExt, &c.LangfuseVPCEIPs, &c.PodSubnetCIDRs,
		&c.CreatedAt, &c.UpdatedAt,
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

// pgConstraint returns the violated constraint name from a Postgres error
// (e.g. "accounts_cluster_id_fkey"), or "" if err is not a *pq.Error.
func pgConstraint(err error) string {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Constraint
	}
	return ""
}
