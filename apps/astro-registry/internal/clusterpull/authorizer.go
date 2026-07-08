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
// cluster it loads clusters.pull_key_hash and requires the row to be enabled.
// Returns (false, nil) on any credential/enabled/not-found mismatch; a non-nil
// error only on an unexpected DB failure.
func (a *Authorizer) Authenticate(ctx context.Context, clusterID, secret string) (bool, error) {
	sum := sha256.Sum256([]byte(secret))

	if clusterID == PrimaryClusterID {
		if len(a.primaryHash) == 0 {
			return false, nil
		}
		return subtle.ConstantTimeCompare(sum[:], a.primaryHash) == 1, nil
	}

	var (
		hash    []byte
		enabled bool
	)
	err := a.db.QueryRowContext(ctx,
		`SELECT pull_key_hash, enabled FROM clusters WHERE id = $1`, clusterID,
	).Scan(&hash, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to load cluster pull key: %w", err)
	}
	if !enabled || len(hash) == 0 {
		return false, nil
	}
	return subtle.ConstantTimeCompare(sum[:], hash) == 1, nil
}

// HomedHere reports whether the account (by UUID) is homed on clusterID. For
// the primary, homed means accounts.cluster_id IS NULL; for an additional
// cluster it must equal clusterID. Soft-deleted or unknown accounts are never
// homed.
//
// accountID is validated as a UUID first so a malformed namespace short-circuits
// to (false, nil); this also lets the query bind id directly (WHERE id = $1) and
// use the accounts primary-key index instead of a sequential scan.
func (a *Authorizer) HomedHere(ctx context.Context, accountID, clusterID string) (bool, error) {
	if err := uuid.Validate(accountID); err != nil {
		return false, nil
	}

	var homeCluster sql.NullString
	err := a.db.QueryRowContext(ctx,
		`SELECT cluster_id FROM accounts WHERE id = $1 AND deleted_at IS NULL`, accountID,
	).Scan(&homeCluster)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to resolve account cluster: %w", err)
	}
	if clusterID == PrimaryClusterID {
		return !homeCluster.Valid, nil
	}
	return homeCluster.Valid && homeCluster.String == clusterID, nil
}
