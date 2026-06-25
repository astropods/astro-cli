package account

import (
	"database/sql/driver"
	"time"
)

// SQLMockScanColumns matches SELECT column order for AccountStore lookups that call scanAccount.
var SQLMockScanColumns = []string{
	"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at",
	"display_name", "avatar_colors", "avatar_updated_at", "cluster_id",
	"account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
}

// SQLMockScanRow builds driver values for SQLMockScanColumns (nil profile, no cluster binding).
func SQLMockScanRow(id, name, accountType string, workosOrgID interface{}, deleted interface{}, createdAt, updatedAt time.Time) []driver.Value {
	return SQLMockScanRowWithCluster(id, name, accountType, workosOrgID, deleted, createdAt, updatedAt, "")
}

// SQLMockScanRowWithCluster sets accounts.cluster_id when clusterID is non-empty.
func SQLMockScanRowWithCluster(id, name, accountType string, workosOrgID interface{}, deleted interface{}, createdAt, updatedAt time.Time, clusterID string) []driver.Value {
	var cid any
	if clusterID != "" {
		cid = clusterID
	}
	return []driver.Value{
		id, name, accountType, workosOrgID, deleted, createdAt, updatedAt,
		"", nil, nil, cid,
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
	}
}
