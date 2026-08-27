package authz

import (
	"context"
	"database/sql"
	"fmt"
)

type resourceAccount struct {
	accountID   string
	workOSOrgID string
	personal    bool
}

// DeploymentAccountResolver maps a deployment to its account.
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

// Enabled reports whether the deployment belongs to an organization account.
// The global and account experiment gates decide whether live FGA is active.
func (r *DeploymentAccountResolver) Enabled(ctx context.Context, resource ResourceRef) (bool, error) {
	account, err := r.resolve(ctx, resource)
	if err != nil {
		return false, err
	}
	return !account.personal, nil
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
			       COALESCE(ao.workos_org_id, '')
			FROM deployments d
			JOIN accounts a ON a.id = d.account_id
			LEFT JOIN account_organizations ao ON ao.account_id = a.id
			WHERE d.id = $1 AND a.deleted_at IS NULL
		`, resource.ExternalID).Scan(
			&account.accountID,
			&accountType,
			&account.workOSOrgID,
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
