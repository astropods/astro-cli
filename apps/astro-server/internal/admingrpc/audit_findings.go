package admingrpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/astropods/astro/apps/astro-server/internal/systemaudit"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) ListAuditFindings(ctx context.Context, req *adminv1.ListAuditFindingsRequest) (*adminv1.ListAuditFindingsResponse, error) {
	findings, err := systemaudit.NewStore(s.db).List(ctx, req.IncludeResolved)
	if err != nil {
		return nil, fmt.Errorf("list audit findings: %w", err)
	}

	resp := &adminv1.ListAuditFindingsResponse{Findings: make([]*adminv1.AuditFinding, 0, len(findings))}
	for _, f := range findings {
		if f.ResolvedAt == nil {
			switch f.Severity {
			case systemaudit.SeverityError:
				resp.OpenErrors++
			case systemaudit.SeverityWarning:
				resp.OpenWarnings++
			}
		}
		resp.Findings = append(resp.Findings, &adminv1.AuditFinding{
			CheckName:      f.CheckName,
			Title:          f.Title,
			SubjectID:      f.SubjectID,
			SubjectLabel:   f.SubjectLabel,
			Severity:       f.Severity,
			Detail:         string(f.Detail),
			FirstSeenAt:    f.FirstSeenAt.Format(time.RFC3339),
			LastSeenAt:     f.LastSeenAt.Format(time.RFC3339),
			ResolvedAt:     formatOptionalTime(f.ResolvedAt),
			AcknowledgedAt: formatOptionalTime(f.AcknowledgedAt),
		})
	}
	return resp, nil
}

func (s *Server) AcknowledgeAuditFinding(ctx context.Context, req *adminv1.AcknowledgeAuditFindingRequest) (*adminv1.AcknowledgeAuditFindingResponse, error) {
	if req.CheckName == "" || req.SubjectID == "" {
		return nil, status.Error(codes.InvalidArgument, "check_name and subject_id are required")
	}
	err := systemaudit.NewStore(s.db).Acknowledge(ctx, req.CheckName, req.SubjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "no open finding for that check and subject")
	}
	if err != nil {
		return nil, fmt.Errorf("acknowledge audit finding: %w", err)
	}
	return &adminv1.AcknowledgeAuditFindingResponse{Status: "acknowledged"}, nil
}

func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
