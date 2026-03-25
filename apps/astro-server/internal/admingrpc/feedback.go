package admingrpc

import (
	"context"
	"fmt"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

// ListFeedback returns all feedback submissions ordered by most recent first.
func (s *Server) ListFeedback(ctx context.Context, _ *adminv1.ListFeedbackRequest) (*adminv1.ListFeedbackResponse, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, COALESCE(user_email, ''), message, COALESCE(page_url, ''), created_at
		 FROM feedback_submissions
		 ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list feedback: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var results []*adminv1.FeedbackSubmission
	for rows.Next() {
		var f adminv1.FeedbackSubmission
		var createdAt time.Time
		if err := rows.Scan(&f.ID, &f.UserID, &f.UserEmail, &f.Message, &f.PageURL, &createdAt); err != nil {
			return nil, fmt.Errorf("scan feedback: %w", err)
		}
		f.CreatedAt = createdAt.Format(time.RFC3339)
		results = append(results, &f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feedback: %w", err)
	}

	return &adminv1.ListFeedbackResponse{
		Submissions: results,
		Count:       int32(len(results)), //nolint:gosec // bounded by DB rows
	}, nil
}
