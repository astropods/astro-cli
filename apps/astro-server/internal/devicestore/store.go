// Package devicestore manages connected device records in PostgreSQL.
//
// Required migration:
//
//	CREATE TABLE connected_devices (
//	    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
//	    account_id UUID NOT NULL REFERENCES accounts(id),
//	    user_id TEXT NOT NULL,
//	    device_id TEXT NOT NULL,
//	    hostname TEXT,
//	    os TEXT,
//	    arch TEXT,
//	    cli_version TEXT,
//	    status TEXT NOT NULL DEFAULT 'connected',
//	    last_heartbeat_at TIMESTAMPTZ,
//	    connected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
//	    disconnected_at TIMESTAMPTZ,
//	    UNIQUE(account_id, device_id)
//	);
//	CREATE INDEX idx_connected_devices_account_status ON connected_devices(account_id, status);
package devicestore

import (
	"context"
	"database/sql"
	"time"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

type Device struct {
	ID              string
	AccountID       string
	UserID          string
	DeviceID        string
	Hostname        string
	OS              string
	Arch            string
	CLIVersion      string
	Status          string
	LastHeartbeatAt *time.Time
	ConnectedAt     time.Time
	DisconnectedAt  *time.Time
}

// Upsert registers or reconnects a device. Returns the database row ID.
func (s *Store) Upsert(ctx context.Context, accountID, userID string, d *Device) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO connected_devices (account_id, user_id, device_id, hostname, os, arch, cli_version, status, last_heartbeat_at, connected_at, disconnected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'connected', NOW(), NOW(), NULL)
		ON CONFLICT (account_id, device_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			hostname = EXCLUDED.hostname,
			os = EXCLUDED.os,
			arch = EXCLUDED.arch,
			cli_version = EXCLUDED.cli_version,
			status = 'connected',
			last_heartbeat_at = NOW(),
			connected_at = NOW(),
			disconnected_at = NULL
		RETURNING id`,
		accountID, userID, d.DeviceID, d.Hostname, d.OS, d.Arch, d.CLIVersion,
	).Scan(&id)
	return id, err
}

// Heartbeat updates the last heartbeat timestamp for a device.
func (s *Store) Heartbeat(ctx context.Context, accountID, deviceID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE connected_devices
		SET last_heartbeat_at = NOW()
		WHERE account_id = $1 AND device_id = $2 AND status = 'connected'`,
		accountID, deviceID,
	)
	return err
}

// Disconnect marks a device as disconnected.
func (s *Store) Disconnect(ctx context.Context, accountID, deviceID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE connected_devices
		SET status = 'disconnected', disconnected_at = NOW()
		WHERE account_id = $1 AND device_id = $2 AND status = 'connected'`,
		accountID, deviceID,
	)
	return err
}

// ReapStale marks devices as disconnected if their last heartbeat is older than maxAge.
func (s *Store) ReapStale(ctx context.Context, maxAge time.Duration) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE connected_devices
		SET status = 'disconnected', disconnected_at = NOW()
		WHERE status = 'connected'
		  AND last_heartbeat_at < NOW() - $1::interval`,
		maxAge.String(),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
