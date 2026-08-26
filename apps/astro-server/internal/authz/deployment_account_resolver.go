package authz

import (
	"context"
	"database/sql"
	"fmt"
)

type resourceAccount struct {
	accountID          string
	workOSOrgID        string
	personal           bool
	fgaResourceManaged bool
}

// DeploymentAccountResolver maps a deployment to its account and reports
// whether PR4 has taken responsibility for its WorkOS resource.
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

func (r *DeploymentAccountResolver) OrganizationForResource(ctx context.Context, resource ResourceRef) (string, bool, error) {
	account, err := r.resolve(ctx, resource)
	return account.workOSOrgID, account.personal, err
}

// Enabled reports whether FGA owns this deployment. Historical deployments
// without a PR4 ledger row stay on legacy behavior until backfill. Pending
// resources fail closed instead of becoming temporarily organization-visible.
func (r *DeploymentAccountResolver) Enabled(ctx context.Context, resource ResourceRef) (bool, error) {
	account, err := r.resolve(ctx, resource)
	if err != nil {
		return false, err
	}
	return !account.personal && account.fgaResourceManaged, nil
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
			       COALESCE(ao.workos_org_id, ''),
			       COALESCE(s.desired_state = 'registered', FALSE)
			       OR COALESCE(rs.desired_state = 'registered', FALSE)
			FROM deployments d
			JOIN accounts a ON a.id = d.account_id
			LEFT JOIN account_organizations ao ON ao.account_id = a.id
			LEFT JOIN deployment_fga_sync s ON s.deployment_id = d.id
			LEFT JOIN authorization_resource_sync rs
			  ON rs.resource_type = 'deployment' AND rs.resource_id = d.id
			WHERE d.id = $1 AND a.deleted_at IS NULL
		`, resource.ExternalID).Scan(
			&account.accountID,
			&accountType,
			&account.workOSOrgID,
			&account.fgaResourceManaged,
		)
		if err != nil {
			return resourceAccount{}, fmt.Errorf("resolve account for %s:%s: %w", resource.Type, resource.ExternalID, err)
		}
		account.personal = accountType == "personal"
		return account, nil
	})
}

var _ AccountResolver = (*DeploymentAccountResolver)(nil)
var _ OrganizationResolver = (*DeploymentAccountResolver)(nil)
var _ ResourceGate = (*DeploymentAccountResolver)(nil)
