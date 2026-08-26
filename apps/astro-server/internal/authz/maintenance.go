package authz

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrAuthorizationMaintenance means Queen temporarily owns FGA lifecycle state.
var ErrAuthorizationMaintenance = errors.New("authorization maintenance is active")

type maintenanceQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func authorizationMaintenanceEnabled(ctx context.Context, db maintenanceQueryer) (bool, error) {
	var enabled bool
	err := db.QueryRowContext(ctx, `
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

func (s *DeploymentFGASyncStore) MaintenanceEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil || !s.maintenanceAware {
		return false, nil
	}
	return authorizationMaintenanceEnabled(ctx, s.db)
}

func (s *ResourceAccessSyncStore) MaintenanceEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil || !s.maintenanceAware {
		return false, nil
	}
	return authorizationMaintenanceEnabled(ctx, s.db)
}
