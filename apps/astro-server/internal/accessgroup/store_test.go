package accessgroup

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

var groupScanColumns = []string{
	"id", "account_id", "workos_group_id", "name", "description", "status",
	"management_source", "created_by_user_id", "archived_by_user_id", "archived_at",
	"classification_metadata", "sync_status", "sync_error", "created_at", "updated_at",
}

func TestStoreCreatePersistsCreatorAsAdmin(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO access_groups`).
		WithArgs("account-1", "Platform Engineering", "Builds the platform", "user-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(groupScanColumns).AddRow(
			"group-1", "account-1", "", "Platform Engineering", "Builds the platform",
			"active", "astro", "user-1", "", nil, []byte(`{"schema_version":1}`),
			"pending", "", now, now,
		))
	mock.ExpectExec(`INSERT INTO access_group_memberships`).
		WithArgs("group-1", "account-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	group, err := NewStore(db).Create(context.Background(), CreateParams{
		AccountID:       "account-1",
		Name:            " Platform Engineering ",
		Description:     " Builds the platform ",
		CreatedByUserID: "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if group.ID != "group-1" || group.Name != "Platform Engineering" || group.SyncStatus != SyncPending {
		t.Fatalf("unexpected group: %+v", group)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreCreateClassifiesDuplicateName(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO access_groups`).
		WillReturnError(&pq.Error{Code: "23505"})
	mock.ExpectRollback()

	_, err = NewStore(db).Create(context.Background(), CreateParams{
		AccountID:       "account-1",
		Name:            "Engineering",
		CreatedByUserID: "user-1",
	})
	if !errors.Is(err, ErrNameExists) {
		t.Fatalf("expected ErrNameExists, got %v", err)
	}
}

func TestStoreListReturnsCountsAndPreviewMembers(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	now := time.Now().UTC()
	columns := append(append([]string(nil), groupScanColumns...), "member_count", "preview_user_ids")
	mock.ExpectQuery(`SELECT[\s\S]+FROM access_groups`).
		WithArgs("account-1", driver.Value(pq.Array([]string{"active", "archiving", "restoring"})), "", 50, 0).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			"group-1", "account-1", "workos-group-1", "Engineering", "",
			"active", "astro", "user-1", "", nil, []byte(`{"schema_version":1}`),
			"synced", "", now, now, 4, pq.Array([]string{"user-1", "user-2", "user-3"}),
		))

	groups, err := NewStore(db).List(context.Background(), "account-1", ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].MemberCount != 4 || len(groups[0].PreviewUserIDs) != 3 {
		t.Fatalf("unexpected groups: %+v", groups)
	}
}
