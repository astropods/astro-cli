-- Migration: Create connected_devices table for ast connect
-- Run this against the astro database before enabling the connect gRPC server.

CREATE TABLE IF NOT EXISTS connected_devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    hostname TEXT,
    os TEXT,
    arch TEXT,
    cli_version TEXT,
    status TEXT NOT NULL DEFAULT 'connected',
    last_heartbeat_at TIMESTAMPTZ,
    connected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disconnected_at TIMESTAMPTZ,
    UNIQUE(account_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_connected_devices_account_status
    ON connected_devices(account_id, status);
