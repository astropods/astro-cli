package accessgroup

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
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

func TestStoreCreateRejectsNonObjectClassificationMetadata(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	_, err = NewStore(db).Create(context.Background(), CreateParams{
		AccountID:              "account-1",
		Name:                   "Engineering",
		CreatedByUserID:        "user-1",
		ClassificationMetadata: json.RawMessage(`["technical"]`),
	})
	if err == nil || err.Error() != "classification metadata must be a JSON object" {
		t.Fatalf("expected JSON object error, got %v", err)
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

func TestStoreListTreatsSearchMetacharactersLiterally(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	columns := append(append([]string(nil), groupScanColumns...), "member_count", "preview_user_ids")
	mock.ExpectQuery(regexp.QuoteMeta(`name ILIKE '%' || $3 || '%' ESCAPE '\'`)).
		WithArgs("account-1", driver.Value(pq.Array([]string{"active", "archiving", "restoring"})), `100\%\_team\*`, 50, 0).
		WillReturnRows(sqlmock.NewRows(columns))

	groups, err := NewStore(db).List(context.Background(), "account-1", ListFilter{Search: " 100%_team* "})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected no groups, got %+v", groups)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreUpdateRejectsInvalidClassificationMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata json.RawMessage
		want     string
	}{
		{name: "invalid JSON", metadata: json.RawMessage(`{"schema_version":`), want: "classification metadata must be valid JSON"},
		{name: "array", metadata: json.RawMessage(`["technical"]`), want: "classification metadata must be a JSON object"},
		{name: "null", metadata: json.RawMessage(`null`), want: "classification metadata must be a JSON object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close() //nolint:errcheck

			_, err = NewStore(db).Update(context.Background(), "account-1", "group-1", "Engineering", "", tt.metadata)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}

func TestStoreSetStatusClassifiesRestoreNameCollision(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectExec(`UPDATE access_groups`).
		WithArgs("account-1", "group-1", StatusRestoring, "user-1").
		WillReturnError(&pq.Error{Code: "23505"})

	err = NewStore(db).SetStatus(context.Background(), "account-1", "group-1", "user-1", StatusRestoring)
	if !errors.Is(err, ErrNameExists) {
		t.Fatalf("expected ErrNameExists, got %v", err)
	}
}

func TestStoreUpsertMembershipPreservesActiveMembership(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	addedAt := time.Now().UTC().Add(-time.Hour)
	updatedAt := addedAt.Add(time.Minute)
	mock.ExpectQuery(`INSERT INTO access_group_memberships[\s\S]+access_group_memberships\.removed_at IS NOT NULL THEN EXCLUDED\.role`).
		WithArgs("group-1", "account-1", "user-1", MembershipRoleMember, "user-2").
		WillReturnRows(sqlmock.NewRows([]string{
			"group_id", "account_id", "user_id", "role", "added_by_user_id",
			"removed_by_user_id", "added_at", "removed_at", "sync_status", "sync_error", "updated_at",
		}).AddRow(
			"group-1", "account-1", "user-1", MembershipRoleAdmin, "original-admin",
			"", addedAt, nil, SyncSynced, "", updatedAt,
		))

	membership, err := NewStore(db).UpsertMembership(context.Background(), Membership{
		GroupID:       "group-1",
		AccountID:     "account-1",
		UserID:        "user-1",
		AddedByUserID: "user-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if membership.Role != MembershipRoleAdmin || membership.AddedByUserID != "original-admin" || !membership.AddedAt.Equal(addedAt) {
		t.Fatalf("active membership attribution changed: %+v", membership)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreMembershipMethodsRejectUnconfiguredStore(t *testing.T) {
	stores := []struct {
		name  string
		store *Store
	}{
		{name: "nil store"},
		{name: "nil database", store: NewStore(nil)},
	}
	methods := []struct {
		name string
		call func(*Store) error
	}{
		{name: "set role", call: func(store *Store) error {
			return store.SetMembershipRole(context.Background(), "account-1", "group-1", "user-1", MembershipRoleMember)
		}},
		{name: "remove", call: func(store *Store) error {
			return store.RemoveMembership(context.Background(), "account-1", "group-1", "user-1", "actor-1")
		}},
		{name: "list", call: func(store *Store) error {
			_, err := store.ListMemberships(context.Background(), "account-1", "group-1", false)
			return err
		}},
		{name: "count admins", call: func(store *Store) error {
			_, err := store.ActiveAdminCount(context.Background(), "account-1", "group-1")
			return err
		}},
	}
	for _, configured := range stores {
		for _, method := range methods {
			t.Run(configured.name+"/"+method.name, func(t *testing.T) {
				if err := method.call(configured.store); err == nil || err.Error() != "access group store is not configured" {
					t.Fatalf("expected store configuration error, got %v", err)
				}
			})
		}
	}
}
