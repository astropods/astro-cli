// Package clusterpull authenticates cluster pull credentials (CPCs) and
// authorizes image pulls by verifying the target tenant is homed on the
// requesting cluster. It backs the CPC issuance path of the /token endpoint.
// See docs/01-spec/registry-pull-through-spec.md.
package clusterpull

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// PrimaryClusterID is the reserved cluster identifier for the primary cluster,
// which has no row in the clusters table. Its hash is configured out-of-band
// (PRIMARY_PULL_KEY_HASH) and its tenants are those with accounts.cluster_id
// IS NULL.
const PrimaryClusterID = "primary"

// Authorizer authenticates CPCs and resolves tenant-to-cluster homing.
type Authorizer struct {
	db          *sql.DB
	primaryHash []byte // sha256 of the primary CPC secret; nil when unconfigured
}

// NewAuthorizer builds an Authorizer. primaryHashHex is the hex-encoded sha256
// of the primary cluster's CPC secret; empty disables the primary CPC path.
func NewAuthorizer(db *sql.DB, primaryHashHex string) *Authorizer {
	var h []byte
	if primaryHashHex != "" {
		if b, err := hex.DecodeString(primaryHashHex); err == nil {
			h = b
		}
	}
	return &Authorizer{db: db, primaryHash: h}
}

// Authenticate reports whether secret matches the stored hash for clusterID.
// For the primary it compares against the configured hash; for an additional
// cluster it loads clusters.pull_key_hash — every row present is usable, there's
// no enabled/disabled gate. Returns (false, nil) on any credential/not-found
// mismatch; a non-nil error only on an unexpected DB failure.
func (a *Authorizer) Authenticate(ctx context.Context, clusterID, secret string) (bool, error) {
	sum := sha256.Sum256([]byte(secret))

	if clusterID == PrimaryClusterID {
		if len(a.primaryHash) == 0 {
			return false, nil
		}
		return subtle.ConstantTimeCompare(sum[:], a.primaryHash) == 1, nil
	}

	var hash []byte
	err := a.db.QueryRowContext(ctx,
		`SELECT pull_key_hash FROM clusters WHERE id = $1`, clusterID,
	).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to load cluster pull key: %w", err)
	}
	if len(hash) == 0 {
		return false, nil
	}
	return subtle.ConstantTimeCompare(sum[:], hash) == 1, nil
}

// ResolveHomedAccount maps an image-namespace segment to its account id and
// reports whether that account is homed on clusterID. The namespace is whatever
// the pod's image reference carries — the account name the developer pushed
// under (the common case), or an account id (deployments rendered before the
// server stopped rewriting). This is the registry's job, mirroring the push
// path's name→id resolution, so astro-server can pass the pushed reference
// through untouched.
//
// Returns the resolved account id (used to rewrite the request to the ECR
// {env}-tenant-{id} repo) and whether it is homed here: for the primary,
// accounts.cluster_id IS NULL; for an additional cluster, it must equal
// clusterID. Unknown or soft-deleted accounts return ("", false, nil).
func (a *Authorizer) ResolveHomedAccount(ctx context.Context, namespace, clusterID string) (accountID string, homed bool, err error) {
	// A uuid-shaped namespace is an account id (transitional refs); otherwise it
	// is an account name. Either way the lookup hits a unique index.
	query := `SELECT id, cluster_id FROM accounts WHERE name = $1 AND deleted_at IS NULL`
	if uuid.Validate(namespace) == nil {
		query = `SELECT id, cluster_id FROM accounts WHERE id = $1 AND deleted_at IS NULL`
	}

	var (
		id          string
		homeCluster sql.NullString
	)
	if err := a.db.QueryRowContext(ctx, query, namespace).Scan(&id, &homeCluster); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to resolve account %q: %w", namespace, err)
	}

	if clusterID == PrimaryClusterID {
		return id, !homeCluster.Valid, nil
	}
	return id, homeCluster.Valid && homeCluster.String == clusterID, nil
}
