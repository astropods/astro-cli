package accessgroup

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

var groupScanColumns = []string{
	"id", "account_id", "workos_group_id", "name", "description", "status",
	"created_by_user_id", "archived_by_user_id", "archived_at", "sync_status",
	"sync_error", "created_at", "updated_at",
}

func TestStoreCreatePersistsCreatorAsAdmin(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO groups`).
		WithArgs("account-1", "Platform Engineering", "Builds the platform", "user-1").
		WillReturnRows(sqlmock.NewRows(groupScanColumns).AddRow(
			"group-1", "account-1", "", "Platform Engineering", "Builds the platform",
			"active", "user-1", "", nil, "pending", "", now, now,
		))
	mock.ExpectExec(`INSERT INTO group_memberships`).
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
	mock.ExpectQuery(`INSERT INTO groups`).
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

func TestStoreTextLimitsCountCharacters(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	store := NewStore(db)
	validName := strings.Repeat("界", 100)
	invalidName := validName + "界"

	mock.ExpectBegin().WillReturnError(errors.New("reached database"))
	if _, err := store.Create(context.Background(), CreateParams{AccountID: "account-1", Name: validName, CreatedByUserID: "user-1"}); err == nil || !strings.Contains(err.Error(), "reached database") {
		t.Fatalf("100-character Create should pass validation, got %v", err)
	}
	if _, err := store.Create(context.Background(), CreateParams{AccountID: "account-1", Name: invalidName, CreatedByUserID: "user-1"}); err == nil || !strings.Contains(err.Error(), "at most 100 characters") {
		t.Fatalf("101-character Create should fail validation, got %v", err)
	}

	mock.ExpectQuery(`UPDATE groups`).WillReturnError(errors.New("reached database"))
	if _, err := store.Update(context.Background(), "account-1", "group-1", validName, ""); err == nil || !strings.Contains(err.Error(), "reached database") {
		t.Fatalf("100-character Update should pass validation, got %v", err)
	}
	if _, err := store.Update(context.Background(), "account-1", "group-1", invalidName, ""); err == nil || !strings.Contains(err.Error(), "1-100 characters") {
		t.Fatalf("101-character Update should fail validation, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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
	mock.ExpectQuery(`SELECT[\s\S]+FROM groups`).
		WithArgs("account-1", driver.Value(pq.Array([]string{"active", "archiving", "restoring"})), "", 50, 0).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			"group-1", "account-1", "workos-group-1", "Engineering", "",
			"active", "user-1", "", nil, "synced", "", now, now,
			4, pq.Array([]string{"user-1", "user-2", "user-3"}),
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

func TestStoreSetProjectionClassifiesWorkOSIDCollision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectExec(`UPDATE groups`).
		WithArgs("account-1", "group-1", "workos-group-1", SyncSynced, "").
		WillReturnError(&pq.Error{Code: "23505"})

	err = NewStore(db).SetProjection(context.Background(), "account-1", "group-1", "workos-group-1", SyncSynced, "")
	if !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("expected ErrProjectionConflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreSetStatusClassifiesRestoreNameCollision(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectExec(`UPDATE groups`).
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
	mock.ExpectQuery(`INSERT INTO group_memberships[\s\S]+group_memberships\.removed_at IS NOT NULL THEN EXCLUDED\.role`).
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
