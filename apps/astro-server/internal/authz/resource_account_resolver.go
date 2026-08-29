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

// ResourceAccountResolver maps an authorization resource to the Astro account
// that owns it. Every resource type resolves to the same three facts, so the
// per-type difference is one query.
type ResourceAccountResolver struct {
	db *sql.DB
}

func NewResourceAccountResolver(db *sql.DB) *ResourceAccountResolver {
	return &ResourceAccountResolver{db: db}
}

func (r *ResourceAccountResolver) AccountForResource(ctx context.Context, resource ResourceRef) (string, bool, error) {
	account, err := r.resolve(ctx, resource)
	return account.accountID, account.personal, err
}

func (r *ResourceAccountResolver) OrganizationForResource(ctx context.Context, resource ResourceRef) (string, bool, error) {
	account, err := r.resolve(ctx, resource)
	return account.workOSOrgID, account.personal, err
}

// Enabled reports whether the resource belongs to an organization account.
// The global and account experiment gates decide whether live FGA is active.
func (r *ResourceAccountResolver) Enabled(ctx context.Context, resource ResourceRef) (bool, error) {
	account, err := r.resolve(ctx, resource)
	if err != nil {
		return false, err
	}
	return !account.personal, nil
}

// ownerQuery selects the owning account id, its type, and its WorkOS
// organization for one resource. A resource whose account is soft-deleted
// resolves to no rows, which callers treat as not found.
func ownerQuery(resourceType ResourceType) (string, error) {
	switch resourceType {
	case ResourceAccount:
		return `
			SELECT a.id, a.type, COALESCE(ao.workos_org_id, '')
			FROM accounts a
			LEFT JOIN account_organizations ao ON ao.account_id = a.id
			WHERE a.id = $1::uuid AND a.deleted_at IS NULL`, nil
	case ResourceBlueprint:
		return `
			SELECT a.id, a.type, COALESCE(ao.workos_org_id, '')
			FROM agents g
			JOIN accounts a ON a.id = g.account_id
			LEFT JOIN account_organizations ao ON ao.account_id = a.id
			WHERE g.uid = $1::uuid AND a.deleted_at IS NULL`, nil
	case ResourceDeployment:
		return `
			SELECT a.id, a.type, COALESCE(ao.workos_org_id, '')
			FROM deployments d
			JOIN accounts a ON a.id = d.account_id
			LEFT JOIN account_organizations ao ON ao.account_id = a.id
			WHERE d.id = $1 AND a.deleted_at IS NULL`, nil
	default:
		return "", fmt.Errorf("unsupported resource type %q", resourceType)
	}
}

func (r *ResourceAccountResolver) resolve(ctx context.Context, resource ResourceRef) (resourceAccount, error) {
	query, err := ownerQuery(resource.Type)
	if err != nil {
		return resourceAccount{}, err
	}
	if resource.ExternalID == "" {
		return resourceAccount{}, fmt.Errorf("resource external id is required")
	}

	return cachedAccount(ctx, resource, func() (resourceAccount, error) {
		var account resourceAccount
		var accountType string
		err := r.db.QueryRowContext(ctx, query, resource.ExternalID).Scan(
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

var _ AccountResolver = (*ResourceAccountResolver)(nil)
var _ OrganizationResolver = (*ResourceAccountResolver)(nil)
var _ ResourceGate = (*ResourceAccountResolver)(nil)
