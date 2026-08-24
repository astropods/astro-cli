package admingrpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountlifecycle"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DeleteAccount soft-deletes an account on an operator's behalf, for accounts
// whose owner can no longer reach the product delete. It runs the same sequence
// the owner-initiated delete does, so billing archiving and resource teardown
// cannot be skipped by going through the admin console.
func (s *Server) DeleteAccount(ctx context.Context, req *adminv1.DeleteAccountRequest) (*adminv1.DeleteAccountResponse, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if s.accountDeleter == nil {
		return nil, status.Error(codes.FailedPrecondition, "account deletion is not configured")
	}

	acct, err := s.accounts.GetByID(req.AccountID)
	if errors.Is(err, account.ErrAccountNotFound) {
		return nil, status.Errorf(codes.NotFound, "account not found: %s", req.AccountID)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get account: %v", err)
	}

	// The typed name is the only guard against deleting the wrong account: the
	// caller sends a UUID, and a UUID transposed from another row looks exactly
	// as valid as the intended one.
	if req.ConfirmName != acct.Name {
		return nil, status.Errorf(codes.InvalidArgument, "confirm_name must equal the account name %q", acct.Name)
	}

	result, err := s.accountDeleter.Delete(ctx, acct)
	if errors.Is(err, account.ErrAlreadyDeleted) {
		return nil, status.Errorf(codes.NotFound, "account not found: %s", req.AccountID)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete account: %v", err)
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(acct.ID, "grpc")
		evt.Action = auditlog.AccountDelete
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Admin deleted account " + acct.Name
		evt.Metadata = map[string]any{"deployments_undeploying": result.DeploymentsUndeploying}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.DeleteAccountResponse{
		Status:                 "deleted",
		DeploymentsUndeploying: int32(result.DeploymentsUndeploying), //nolint:gosec // bounded by the account's deployment count
	}, nil
}

// PurgeAccount hard-deletes an already soft-deleted account without waiting out
// the retention window, for an operator who wants a defunct account gone now
// (its name is reserved until the row goes). It runs the same sequence the
// periodic sweep runs, including the refusal while teardown is outstanding.
func (s *Server) PurgeAccount(ctx context.Context, req *adminv1.PurgeAccountRequest) (*adminv1.PurgeAccountResponse, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if s.accountPurger == nil {
		return nil, status.Error(codes.FailedPrecondition, "account purge is not configured")
	}

	// GetByID hides soft-deleted rows, which is exactly the set this operates
	// on, so read the two columns directly.
	var name string
	var deletedAt sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT name, deleted_at FROM accounts WHERE id = $1`, req.AccountID,
	).Scan(&name, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "account not found: %s", req.AccountID)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get account: %v", err)
	}

	// Purging a live account would skip the billing archive and the WorkOS
	// organization delete that only the soft-delete performs.
	if !deletedAt.Valid {
		return nil, status.Error(codes.FailedPrecondition, "account is not deleted; delete it first")
	}
	if req.ConfirmName != name {
		return nil, status.Errorf(codes.InvalidArgument, "confirm_name must equal the account name %q", name)
	}

	if err := s.accountPurger.Purge(ctx, req.AccountID); errors.Is(err, accountlifecycle.ErrTeardownPending) {
		return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "purge account: %v", err)
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.AccountPurge
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.ResourceName = name
		evt.Description = "Admin purged account " + name
		evt.Metadata = map[string]any{"deleted_at": deletedAt.Time}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.PurgeAccountResponse{Status: "purged"}, nil
}

// ListAccountClusters returns the clusters an account may deploy to.
func (s *Server) ListAccountClusters(ctx context.Context, req *adminv1.ListAccountClustersRequest) (*adminv1.AccountClusterList, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}
	return s.accountClusterList(req.AccountID)
}

// AddAccountCluster allows a cluster for an account, optionally as its default.
func (s *Server) AddAccountCluster(ctx context.Context, req *adminv1.AddAccountClusterRequest) (*adminv1.AccountClusterList, error) {
	if req.AccountID == "" || req.ClusterID == "" {
		return nil, fmt.Errorf("account_id and cluster_id are required")
	}

	if err := s.bindings.Add(req.AccountID, req.ClusterID, req.SetDefault); err != nil {
		return nil, err
	}

	s.logAccountClusterAudit(req.AccountID, auditlog.AccountSetCluster,
		"Admin allowed cluster "+req.ClusterID+" for account",
		map[string]any{"cluster_id": req.ClusterID, "set_default": req.SetDefault})

	return s.accountClusterList(req.AccountID)
}

// RemoveAccountCluster refuses while the account still has deployments there.
func (s *Server) RemoveAccountCluster(ctx context.Context, req *adminv1.RemoveAccountClusterRequest) (*adminv1.AccountClusterList, error) {
	if req.AccountID == "" || req.ClusterID == "" {
		return nil, fmt.Errorf("account_id and cluster_id are required")
	}

	if err := s.bindings.Remove(req.AccountID, req.ClusterID); err != nil {
		if errors.Is(err, account.ErrClusterInUse) {
			return nil, fmt.Errorf("%w: move or undeploy them before unbinding it", err)
		}
		return nil, err
	}

	s.logAccountClusterAudit(req.AccountID, auditlog.AccountSetCluster,
		"Admin disallowed cluster "+req.ClusterID+" for account",
		map[string]any{"cluster_id": req.ClusterID})

	return s.accountClusterList(req.AccountID)
}

// SetAccountDefaultCluster moves the default flag to an already-allowed cluster.
func (s *Server) SetAccountDefaultCluster(ctx context.Context, req *adminv1.SetAccountDefaultClusterRequest) (*adminv1.AccountClusterList, error) {
	if req.AccountID == "" || req.ClusterID == "" {
		return nil, fmt.Errorf("account_id and cluster_id are required")
	}

	if err := s.bindings.SetDefault(req.AccountID, req.ClusterID); err != nil {
		return nil, err
	}

	s.logAccountClusterAudit(req.AccountID, auditlog.AccountSetCluster,
		"Admin set default cluster to "+req.ClusterID,
		map[string]any{"cluster_id": req.ClusterID})

	return s.accountClusterList(req.AccountID)
}

func (s *Server) accountClusterList(accountID string) (*adminv1.AccountClusterList, error) {
	allowed, err := s.bindings.List(accountID)
	if err != nil {
		return nil, err
	}
	out := &adminv1.AccountClusterList{}
	for _, b := range allowed {
		out.Clusters = append(out.Clusters, &adminv1.AccountCluster{
			ClusterID:   b.ClusterID,
			Region:      b.Region,
			RegionLabel: b.RegionLabel,
			RegionFlag:  b.RegionFlag,
			IsDefault:   b.IsDefault,
		})
	}
	return out, nil
}

func (s *Server) logAccountClusterAudit(accountID, action, description string, metadata map[string]any) {
	if s.auditStore == nil {
		return
	}
	evt := auditlog.ForAdmin(accountID, "grpc")
	evt.Action = action
	evt.ResourceType = "account"
	evt.ResourceID = accountID
	evt.Description = description
	evt.Metadata = metadata
	s.auditStore.LogAsync(s.log, evt)
}
