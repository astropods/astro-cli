package authorizationadmin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CreateReset(ctx context.Context, accountID string, dryRun bool, confirmedCount *int) (*Operation, error) {
	if s == nil || s.db == nil {
		return nil, ErrNotConfigured
	}
	if accountID == "" {
		return nil, ErrAccountNotFound
	}
	maintenanceHold := !dryRun
	var operation Operation
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO authorization_admin_operations (
			account_id, kind, dry_run, confirmed_count, maintenance_hold
		) VALUES ($1, 'resource_reset', $2, $3, $4)
		RETURNING `+operationColumns,
		accountID, dryRun, confirmedCount, maintenanceHold,
	).Scan(operationScan(&operation)...)
	if isUniqueViolation(err) && maintenanceHold {
		return nil, ErrMaintenanceActive
	}
	if err != nil {
		return nil, fmt.Errorf("create authorization reset operation: %w", err)
	}
	return &operation, nil
}

func (s *Store) AttachJob(ctx context.Context, operationID string, jobID int64) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE authorization_admin_operations
		SET river_job_id = $2, updated_at = NOW()
		WHERE id = $1
	`, operationID, jobID)
	if err != nil {
		return fmt.Errorf("attach authorization reset job: %w", err)
	}
	return requireChanged(result, "attach authorization reset job")
}

func (s *Store) Get(ctx context.Context, operationID string) (*Operation, error) {
	if s == nil || s.db == nil {
		return nil, ErrNotConfigured
	}
	var operation Operation
	err := s.db.QueryRowContext(ctx, `
		SELECT `+operationColumns+`
		FROM authorization_admin_operations
		WHERE id = $1
	`, operationID).Scan(operationScan(&operation)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOperationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get authorization operation: %w", err)
	}
	return &operation, nil
}

func (s *Store) List(ctx context.Context, limit int) ([]Operation, error) {
	if s == nil || s.db == nil {
		return nil, ErrNotConfigured
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+operationColumns+`
		FROM authorization_admin_operations
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list authorization operations: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	operations := make([]Operation, 0)
	for rows.Next() {
		var operation Operation
		if err := rows.Scan(operationScan(&operation)...); err != nil {
			return nil, fmt.Errorf("scan authorization operation: %w", err)
		}
		operations = append(operations, operation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authorization operations: %w", err)
	}
	return operations, nil
}

func (s *Store) Start(ctx context.Context, operationID string) (*Operation, error) {
	var operation Operation
	err := s.db.QueryRowContext(ctx, `
		UPDATE authorization_admin_operations
		SET status = 'running',
		    target_count = 0,
		    processed_count = 0,
		    succeeded_count = 0,
		    failed_count = 0,
		    attempt_count = attempt_count + 1,
		    last_error = NULL,
		    report = '[]'::jsonb,
		    started_at = NOW(),
		    completed_at = NULL,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING `+operationColumns,
		operationID,
	).Scan(operationScan(&operation)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOperationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("start authorization operation: %w", err)
	}
	return &operation, nil
}

func (s *Store) Progress(ctx context.Context, operationID string, target, processed, succeeded, failed int, report []ReportEntry) error {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode authorization operation report: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE authorization_admin_operations
		SET target_count = $2,
		    processed_count = $3,
		    succeeded_count = $4,
		    failed_count = $5,
		    report = $6,
		    updated_at = NOW()
		WHERE id = $1
	`, operationID, target, processed, succeeded, failed, reportJSON)
	if err != nil {
		return fmt.Errorf("update authorization operation progress: %w", err)
	}
	return nil
}

func (s *Store) Complete(ctx context.Context, operationID string, target, processed, succeeded int, report []ReportEntry) error {
	if err := s.Progress(ctx, operationID, target, processed, succeeded, 0, report); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE authorization_admin_operations
		SET status = 'succeeded', completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, operationID)
	if err != nil {
		return fmt.Errorf("complete authorization operation: %w", err)
	}
	return nil
}

func (s *Store) Fail(ctx context.Context, operationID string, target, processed, succeeded, failed int, report []ReportEntry, cause error) error {
	if err := s.Progress(ctx, operationID, target, processed, succeeded, failed, report); err != nil {
		return err
	}
	message := "authorization operation failed"
	if cause != nil {
		message = cause.Error()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE authorization_admin_operations
		SET status = 'failed', last_error = $2, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, operationID, message)
	if err != nil {
		return fmt.Errorf("fail authorization operation: %w", err)
	}
	return nil
}

func (s *Store) MaintenanceEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrNotConfigured
	}
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM authorization_admin_operations
			WHERE maintenance_hold AND maintenance_released_at IS NULL
		)
	`).Scan(&enabled)
	if err != nil {
		return false, fmt.Errorf("read authorization maintenance state: %w", err)
	}
	return enabled, nil
}

func (s *Store) ReleaseMaintenance(ctx context.Context, operationID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE authorization_admin_operations
		SET maintenance_released_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND maintenance_hold AND maintenance_released_at IS NULL
		  AND status IN ('succeeded', 'failed')
	`, operationID)
	if err != nil {
		return fmt.Errorf("release authorization maintenance: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("release authorization maintenance rows affected: %w", err)
	}
	if rows > 0 {
		return nil
	}
	operation, err := s.Get(ctx, operationID)
	if err != nil {
		return err
	}
	if operation.Status == "queued" || operation.Status == "running" {
		return ErrOperationNotComplete
	}
	return ErrOperationNotFound
}

func (s *Store) RunningFGAJobs(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM river.river_job
		WHERE kind = ANY($1::text[]) AND state = 'running'
	`, pq.Array([]string{"deployment.fga_reconcile", "resource_access.fga_reconcile"})).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count running FGA jobs: %w", err)
	}
	return count, nil
}

const operationColumns = `
	id, account_id, dry_run, status, confirmed_count, target_count, processed_count,
	succeeded_count, failed_count, attempt_count, maintenance_hold,
	maintenance_released_at, COALESCE(last_error, ''), created_at`

func operationScan(operation *Operation) []any {
	return []any{
		&operation.ID,
		&operation.AccountID,
		&operation.DryRun,
		&operation.Status,
		&operation.ConfirmedCount,
		&operation.TargetCount,
		&operation.ProcessedCount,
		&operation.SucceededCount,
		&operation.FailedCount,
		&operation.AttemptCount,
		&operation.MaintenanceHold,
		&operation.MaintenanceReleasedAt,
		&operation.LastError,
		&operation.CreatedAt,
	}
}

func requireChanged(result sql.Result, operation string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", operation, err)
	}
	if rows == 0 {
		return ErrOperationNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
