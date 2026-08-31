package accessgroup

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

var groupScanColumns = []string{
	"id", "account_id", "workos_group_id", "name", "description", "status",
	"created_by_user_id", "archived_by_user_id", "archived_at", "created_at", "updated_at",
}

func TestStoreCreatePersistsCreator(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	now := time.Now().UTC()
	mock.ExpectQuery(`INSERT INTO groups`).
		WithArgs("account-1", "Platform Engineering", "Builds the platform", "user-1").
		WillReturnRows(sqlmock.NewRows(groupScanColumns).AddRow(
			"group-1", "account-1", "", "Platform Engineering", "Builds the platform",
			"active", "user-1", "", nil, now, now,
		))

	group, err := NewStore(db).Create(context.Background(), CreateParams{
		AccountID: "account-1", Name: " Platform Engineering ",
		Description: " Builds the platform ", CreatedByUserID: "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if group.ID != "group-1" || group.Name != "Platform Engineering" || group.CreatedByUserID != "user-1" {
		t.Fatalf("unexpected group: %+v", group)
	}
}

func TestStoreCreateClassifiesDuplicateName(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectQuery(`INSERT INTO groups`).WillReturnError(&pq.Error{Code: "23505"})

	_, err = NewStore(db).Create(context.Background(), CreateParams{
		AccountID: "account-1", Name: "Engineering", CreatedByUserID: "user-1",
	})
	if !errors.Is(err, ErrNameExists) {
		t.Fatalf("expected ErrNameExists, got %v", err)
	}
}

func TestStoreListTreatsSearchMetacharactersLiterally(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectQuery(regexp.QuoteMeta(`name ILIKE '%' || $3 || '%' ESCAPE '\'`)).
		WithArgs("account-1", driver.Value(pq.Array([]string{"active", "archiving", "restoring"})), `100\%\_team\*`, 50, 0).
		WillReturnRows(sqlmock.NewRows(groupScanColumns))

	groups, err := NewStore(db).List(context.Background(), "account-1", ListFilter{Search: " 100%_team* "})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected no groups, got %+v", groups)
	}
}

func TestStoreSetWorkOSGroupIDClassifiesCollision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectExec(`UPDATE groups`).
		WithArgs("account-1", "group-1", "workos-group-1").
		WillReturnError(&pq.Error{Code: "23505"})

	err = NewStore(db).SetWorkOSGroupID(context.Background(), "account-1", "group-1", "workos-group-1")
	if !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("expected ErrProjectionConflict, got %v", err)
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
