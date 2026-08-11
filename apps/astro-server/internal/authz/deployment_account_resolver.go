package authz

import (
	"context"
	"database/sql"
	"fmt"
)

type resourceAccount struct {
	accountID        string
	personal         bool
	fgaResourceReady bool
}

// DeploymentAccountResolver maps a deployment to its account and reports
// whether PR4 finished registering its WorkOS resource.
type DeploymentAccountResolver struct {
	db *sql.DB
}

func NewDeploymentAccountResolver(db *sql.DB) *DeploymentAccountResolver {
	return &DeploymentAccountResolver{db: db}
}

func (r *DeploymentAccountResolver) AccountForResource(ctx context.Context, resource ResourceRef) (string, bool, error) {
	account, err := r.resolve(ctx, resource)
	return account.accountID, account.personal, err
}

// Enabled reports whether shadow checks may run for this deployment. Requiring
// a converged PR4 ledger row excludes personal and historical deployments.
func (r *DeploymentAccountResolver) Enabled(ctx context.Context, resource ResourceRef) (bool, error) {
	account, err := r.resolve(ctx, resource)
	if err != nil {
		return false, err
	}
	return !account.personal && account.fgaResourceReady, nil
}

func (r *DeploymentAccountResolver) resolve(ctx context.Context, resource ResourceRef) (resourceAccount, error) {
	if resource.Type != ResourceDeployment {
		return resourceAccount{}, fmt.Errorf("unsupported resource type %q", resource.Type)
	}
	if resource.ExternalID == "" {
		return resourceAccount{}, fmt.Errorf("resource external id is required")
	}

	return cachedAccount(ctx, resource, func() (resourceAccount, error) {
		var account resourceAccount
		var accountType string
		err := r.db.QueryRowContext(ctx, `
			SELECT d.account_id,
			       a.type,
			       COALESCE(
			           s.desired_state = 'registered'
			           AND s.synced_state = 'registered'
			           AND s.synced_version = s.desired_version,
			           FALSE
			       )
			FROM deployments d
			JOIN accounts a ON a.id = d.account_id
			LEFT JOIN deployment_fga_sync s ON s.deployment_id = d.id
			WHERE d.id = $1 AND a.deleted_at IS NULL
		`, resource.ExternalID).Scan(
			&account.accountID,
			&accountType,
			&account.fgaResourceReady,
		)
		if err != nil {
			return resourceAccount{}, fmt.Errorf("resolve account for %s:%s: %w", resource.Type, resource.ExternalID, err)
		}
		account.personal = accountType == "personal"
		return account, nil
	})
}

var _ AccountResolver = (*DeploymentAccountResolver)(nil)
var _ ResourceGate = (*DeploymentAccountResolver)(nil)
