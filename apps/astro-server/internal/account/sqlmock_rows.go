package account

import (
	"database/sql/driver"
	"time"
)

// SQLMockScanColumns matches SELECT column order for AccountStore lookups that call scanAccount.
var SQLMockScanColumns = []string{
	"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at",
	"display_name", "avatar_colors", "avatar_updated_at",
	"account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
}

func SQLMockScanRow(id, name, accountType string, workosOrgID any, deleted any, createdAt, updatedAt time.Time) []driver.Value {
	return []driver.Value{
		id, name, accountType, workosOrgID, deleted, createdAt, updatedAt,
		"", nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil,
	}
}
