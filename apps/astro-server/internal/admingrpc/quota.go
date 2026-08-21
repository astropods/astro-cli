package admingrpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

// ListQuotaIncreaseRequests returns all quota increase requests, optionally filtered by status.
func (s *Server) ListQuotaIncreaseRequests(ctx context.Context, req *adminv1.ListQuotaIncreaseRequestsRequest) (*adminv1.ListQuotaIncreaseRequestsResponse, error) {
	query := `
		SELECT q.id, q.account_id, COALESCE(a.name, ''), q.feature_key,
		       q.current_usage, COALESCE(q.current_quota, 0), COALESCE(q.requested_amount, 0),
		       q.reason, q.status, q.requested_by,
		       COALESCE(q.resolved_by, ''), q.resolved_at, COALESCE(q.resolution_note, ''),
		       COALESCE(q.grant_amount, 0), q.created_at
		FROM quota_increase_requests q
		LEFT JOIN accounts a ON a.id = q.account_id
	`
	args := []any{}
	if req.Status != "" {
		query += " WHERE q.status = $1"
		args = append(args, req.Status)
	}
	query += " ORDER BY q.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list quota requests: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var results []*adminv1.QuotaIncreaseRequestProto
	for rows.Next() {
		var r adminv1.QuotaIncreaseRequestProto
		var resolvedAt sql.NullTime
		var createdAt time.Time
		if err := rows.Scan(
			&r.ID, &r.AccountID, &r.AccountName, &r.FeatureKey,
			&r.CurrentUsage, &r.CurrentQuota, &r.RequestedAmount,
			&r.Reason, &r.Status, &r.RequestedBy,
			&r.ResolvedBy, &resolvedAt, &r.ResolutionNote,
			&r.GrantAmount, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan quota request: %w", err)
		}
		r.CreatedAt = createdAt.Format(time.RFC3339)
		if resolvedAt.Valid {
			r.ResolvedAt = resolvedAt.Time.Format(time.RFC3339)
		}
		results = append(results, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quota requests: %w", err)
	}

	return &adminv1.ListQuotaIncreaseRequestsResponse{
		Requests: results,
		Count:    int32(len(results)), //nolint:gosec // bounded by DB rows
	}, nil
}

// ApproveQuotaIncrease marks a pending request as approved and applies the
// granted amount as the account's new absolute limit for the requested
// resource. The request update and the account_limits write happen in one
// transaction so an approved request is never left without an applied grant.
func (s *Server) ApproveQuotaIncrease(ctx context.Context, req *adminv1.ApproveQuotaIncreaseRequest) (*adminv1.ApproveQuotaIncreaseResponse, error) {
	if req.RequestID == "" || req.GrantAmount <= 0 {
		return nil, fmt.Errorf("request_id and positive grant_amount are required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin approval tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit

	// Lock the pending request and read the account + resource it targets.
	var accountID, featureKey string
	err = tx.QueryRowContext(ctx,
		`SELECT account_id, feature_key FROM quota_increase_requests
		 WHERE id = $1 AND status = 'pending' FOR UPDATE`,
		req.RequestID,
	).Scan(&accountID, &featureKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("request not found or already resolved")
	}
	if err != nil {
		return nil, fmt.Errorf("load quota request: %w", err)
	}

	// A grant only applies to a count-enforced resource; metered features are
	// gated by billing and have no account_limits row to raise. Such requests
	// should never exist (rejected at request time) but guard defensively.
	if !quota.IsResource(featureKey) {
		return nil, fmt.Errorf("cannot grant quota for non-managed feature %q; deny it instead", featureKey)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE quota_increase_requests
		 SET status = 'approved', grant_amount = $1, resolved_by = 'astro-team', resolved_at = NOW(), resolution_note = $2
		 WHERE id = $3`,
		req.GrantAmount, req.Note, req.RequestID,
	); err != nil {
		return nil, fmt.Errorf("update quota request: %w", err)
	}

	// Apply the grant as the new absolute limit (count resources are integers).
	limit := int64(math.Round(req.GrantAmount))
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO account_limits (account_id, resource, limit_value)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (account_id, resource) DO UPDATE SET limit_value = EXCLUDED.limit_value`,
		accountID, featureKey, limit,
	); err != nil {
		return nil, fmt.Errorf("apply account limit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit quota approval: %w", err)
	}

	s.log.Info("quota: increase approved",
		"request_id", req.RequestID, "account_id", accountID, "resource", featureKey, "limit", limit)

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(accountID, "grpc")
		evt.Action = auditlog.QuotaApprove
		evt.ResourceType = "quota_request"
		evt.ResourceID = req.RequestID
		evt.Description = "Admin approved quota increase"
		evt.Metadata = map[string]any{"resource": featureKey, "grant_amount": req.GrantAmount, "limit": limit, "note": req.Note}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.ApproveQuotaIncreaseResponse{Status: "approved"}, nil
}

// DenyQuotaIncrease denies a pending request.
func (s *Server) DenyQuotaIncrease(ctx context.Context, req *adminv1.DenyQuotaIncreaseRequest) (*adminv1.DenyQuotaIncreaseResponse, error) {
	if req.RequestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE quota_increase_requests
		 SET status = 'denied', resolved_by = 'astro-team', resolved_at = NOW(), resolution_note = $1
		 WHERE id = $2 AND status = 'pending'`,
		req.Note, req.RequestID,
	)
	if err != nil {
		return nil, fmt.Errorf("update quota request: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, fmt.Errorf("request not found or already resolved")
	}

	s.log.Info("quota: increase denied", "request_id", req.RequestID)

	if s.auditStore != nil {
		accountID := s.lookupQuotaRequestAccountID(ctx, req.RequestID)
		evt := auditlog.ForAdmin(accountID, "grpc")
		evt.Action = auditlog.QuotaDeny
		evt.ResourceType = "quota_request"
		evt.ResourceID = req.RequestID
		evt.Description = "Admin denied quota increase"
		evt.Metadata = map[string]any{"note": req.Note}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.DenyQuotaIncreaseResponse{Status: "denied"}, nil
}

// lookupQuotaRequestAccountID resolves the account_id for a quota request.
// Returns empty string if the lookup fails (audit log will still record the event).
func (s *Server) lookupQuotaRequestAccountID(ctx context.Context, requestID string) string {
	var accountID string
	_ = s.db.QueryRowContext(ctx, "SELECT account_id FROM quota_increase_requests WHERE id = $1", requestID).Scan(&accountID)
	return accountID
}
