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
const PrimaryClusterID = "primary"

// Authorizer authenticates CPCs and resolves tenant-to-cluster homing.
type Authorizer struct {
	db               *sql.DB
	primaryHash      []byte // sha256 of the primary CPC secret; nil when unconfigured
	defaultClusterID string // astro-server's DEFAULT_CLUSTER_ID; empty when boot sync isn't in play
}

// NewAuthorizer builds an Authorizer. primaryHashHex is the hex-encoded sha256
// of the primary cluster's CPC secret; empty disables the primary CPC path.
// defaultClusterID is astro-server's DEFAULT_CLUSTER_ID — the real clusters.id
// unbound accounts route to under cluster-config boot sync; empty when boot
// sync isn't in play.
func NewAuthorizer(db *sql.DB, primaryHashHex, defaultClusterID string) *Authorizer {
	var h []byte
	if primaryHashHex != "" {
		if b, err := hex.DecodeString(primaryHashHex); err == nil {
			h = b
		}
	}
	return &Authorizer{db: db, primaryHash: h, defaultClusterID: defaultClusterID}
}

// canonicalClusterID resolves the "primary" sentinel to the configured default
// cluster's real id, which is what account_clusters records. A CPC issued
// before boot sync carries the sentinel for the same cluster a bound account
// names by id.
func (a *Authorizer) canonicalClusterID(clusterID string) string {
	if clusterID == PrimaryClusterID && a.defaultClusterID != "" {
		return a.defaultClusterID
	}
	return clusterID
}

// isPrimaryRequest reports whether clusterID names the primary cluster, under
// either the reserved sentinel or the configured default cluster's real id.
func (a *Authorizer) isPrimaryRequest(clusterID string) bool {
	return clusterID == PrimaryClusterID || (a.defaultClusterID != "" && clusterID == a.defaultClusterID)
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
func (a *Authorizer) ResolveHomedAccount(ctx context.Context, namespace, clusterID string) (accountID string, homed bool, err error) {
	// A uuid-shaped namespace is an account id (transitional refs); otherwise it
	// is an account name. Either way the lookup hits a unique index.
	column := "name"
	if uuid.Validate(namespace) == nil {
		column = "id"
	}
	query := fmt.Sprintf(`
		SELECT a.id,
		       EXISTS (SELECT 1 FROM account_clusters ac WHERE ac.account_id = a.id AND ac.cluster_id = $2),
		       EXISTS (SELECT 1 FROM account_clusters ac WHERE ac.account_id = a.id)
		FROM accounts a WHERE a.%s = $1 AND a.deleted_at IS NULL`, column)

	var (
		id          string
		bound       bool
		hasBindings bool
	)
	if err := a.db.QueryRowContext(ctx, query, namespace, a.canonicalClusterID(clusterID)).Scan(&id, &bound, &hasBindings); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to resolve account %q: %w", namespace, err)
	}

	if bound {
		return id, true, nil
	}
	if !a.isPrimaryRequest(clusterID) {
		return id, false, nil
	}
	// Nothing can be bound to a primary with no clusters row, so an exhaustive
	// check would demand a binding no operator can create.
	if a.defaultClusterID == "" {
		return id, true, nil
	}
	// An account nothing has bound yet routes to the primary: that is the
	// binding astro-server records the first time it materializes the set.
	// Reading it as unrestricted instead would let any authenticated cluster
	// pull for every account no one has read.
	return id, !hasBindings, nil
}
