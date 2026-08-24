package systemaudit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Finding struct {
	CheckName      string          `json:"check_name"`
	Title          string          `json:"title"`
	SubjectID      string          `json:"subject_id"`
	SubjectLabel   string          `json:"subject_label"`
	Severity       string          `json:"severity"`
	Detail         json.RawMessage `json:"detail"`
	FirstSeenAt    time.Time       `json:"first_seen_at"`
	LastSeenAt     time.Time       `json:"last_seen_at"`
	ResolvedAt     *time.Time      `json:"resolved_at,omitempty"`
	AcknowledgedAt *time.Time      `json:"acknowledged_at,omitempty"`
}

type Result struct {
	CheckName string
	Open      int
	Resolved  int
	Err       error
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Run(ctx context.Context) []Result {
	results := make([]Result, 0, len(checks))
	for _, c := range checks {
		open, resolved, err := s.runCheck(ctx, c)
		results = append(results, Result{CheckName: c.Name, Open: open, Resolved: resolved, Err: err})
	}
	return results
}

func (s *Store) runCheck(ctx context.Context, c Check) (open, resolved int, err error) {
	startedAt := time.Now()

	rows, err := s.db.QueryContext(ctx, c.Query)
	if err != nil {
		return 0, 0, fmt.Errorf("run check %s: %w", c.Name, err)
	}
	defer rows.Close() //nolint:errcheck

	type row struct {
		subjectID    string
		subjectLabel string
		detail       []byte
	}
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.subjectID, &r.subjectLabel, &r.detail); err != nil {
			return 0, 0, fmt.Errorf("scan check %s: %w", c.Name, err)
		}
		found = append(found, r)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("read check %s: %w", c.Name, err)
	}

	for _, r := range found {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO system_audit_findings
			       (check_name, subject_id, subject_label, severity, detail, first_seen_at, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, now(), now())
			ON CONFLICT (check_name, subject_id) DO UPDATE
			   SET subject_label   = excluded.subject_label,
			       severity        = excluded.severity,
			       detail          = excluded.detail,
			       last_seen_at    = now(),
			       first_seen_at   = CASE WHEN system_audit_findings.resolved_at IS NULL
			                              THEN system_audit_findings.first_seen_at ELSE now() END,
			       acknowledged_at = CASE WHEN system_audit_findings.resolved_at IS NULL
			                              THEN system_audit_findings.acknowledged_at ELSE NULL END,
			       resolved_at     = NULL
		`, c.Name, r.subjectID, r.subjectLabel, c.Severity, r.detail); err != nil {
			return 0, 0, fmt.Errorf("record finding %s/%s: %w", c.Name, r.subjectID, err)
		}
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE system_audit_findings
		   SET resolved_at = now()
		 WHERE check_name = $1 AND resolved_at IS NULL AND last_seen_at < $2
	`, c.Name, startedAt)
	if err != nil {
		return len(found), 0, fmt.Errorf("resolve findings for %s: %w", c.Name, err)
	}
	n, _ := res.RowsAffected()
	return len(found), int(n), nil
}

func (s *Store) Purge(ctx context.Context, olderThan time.Duration) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM system_audit_findings
		 WHERE resolved_at IS NOT NULL AND resolved_at < now() - $1::interval
	`, fmt.Sprintf("%d seconds", int(olderThan.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("purge resolved findings: %w", err)
	}
	return res.RowsAffected()
}

func (s *Store) List(ctx context.Context, includeResolved bool) ([]Finding, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.check_name, f.subject_id, f.subject_label, f.severity, f.detail,
		       f.first_seen_at, f.last_seen_at, f.resolved_at, f.acknowledged_at
		  FROM system_audit_findings f
		 WHERE $1 OR f.resolved_at IS NULL
		 ORDER BY CASE f.severity WHEN 'error' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
		          f.last_seen_at DESC
	`, includeResolved)
	if err != nil {
		return nil, fmt.Errorf("list findings: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	titles := make(map[string]string, len(checks))
	for _, c := range checks {
		titles[c.Name] = c.Title
	}

	var out []Finding
	for rows.Next() {
		var f Finding
		var detail []byte
		if err := rows.Scan(&f.CheckName, &f.SubjectID, &f.SubjectLabel, &f.Severity, &detail,
			&f.FirstSeenAt, &f.LastSeenAt, &f.ResolvedAt, &f.AcknowledgedAt); err != nil {
			return nil, fmt.Errorf("scan finding: %w", err)
		}
		f.Detail = detail
		f.Title = titles[f.CheckName]
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read findings: %w", err)
	}
	return out, nil
}

func (s *Store) Acknowledge(ctx context.Context, checkName, subjectID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE system_audit_findings
		   SET acknowledged_at = now()
		 WHERE check_name = $1 AND subject_id = $2 AND resolved_at IS NULL
	`, checkName, subjectID)
	if err != nil {
		return fmt.Errorf("acknowledge finding: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("acknowledge finding: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
