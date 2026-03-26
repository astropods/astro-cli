package admingrpc

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
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

// ApproveQuotaIncrease marks a pending request as approved.
// The caller (queen) is responsible for creating the OpenMeter grant separately.
func (s *Server) ApproveQuotaIncrease(ctx context.Context, req *adminv1.ApproveQuotaIncreaseRequest) (*adminv1.ApproveQuotaIncreaseResponse, error) {
	if req.RequestID == "" || req.GrantAmount <= 0 {
		return nil, fmt.Errorf("request_id and positive grant_amount are required")
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE quota_increase_requests
		 SET status = 'approved', grant_amount = $1, resolved_by = 'admin', resolved_at = NOW(), resolution_note = $2
		 WHERE id = $3 AND status = 'pending'`,
		req.GrantAmount, req.Note, req.RequestID,
	)
	if err != nil {
		return nil, fmt.Errorf("update quota request: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, fmt.Errorf("request not found or already resolved")
	}

	s.log.Info("Quota increase approved", "request_id", req.RequestID, "grant_amount", req.GrantAmount)

	if s.auditStore != nil {
		s.auditStore.LogAsync(s.log, auditlog.Event{
			ActorID:      "admin:grpc",
			ActorType:    auditlog.ActorAdmin,
			Action:       auditlog.QuotaApprove,
			ResourceType: "quota_request",
			ResourceID:   req.RequestID,
			Description:  "Admin approved quota increase",
			Metadata:     map[string]any{"grant_amount": req.GrantAmount, "note": req.Note},
		})
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
		 SET status = 'denied', resolved_by = 'admin', resolved_at = NOW(), resolution_note = $1
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

	s.log.Info("Quota increase denied", "request_id", req.RequestID)

	if s.auditStore != nil {
		s.auditStore.LogAsync(s.log, auditlog.Event{
			ActorID:      "admin:grpc",
			ActorType:    auditlog.ActorAdmin,
			Action:       auditlog.QuotaDeny,
			ResourceType: "quota_request",
			ResourceID:   req.RequestID,
			Description:  "Admin denied quota increase",
			Metadata:     map[string]any{"note": req.Note},
		})
	}

	return &adminv1.DenyQuotaIncreaseResponse{Status: "denied"}, nil
}
