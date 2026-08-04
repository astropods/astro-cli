package agentindex

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListVisibleBlueprintsForUserPageBoundsNameSortBeforeEnrichment(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
		_ = db.Close()
	})

	mock.ExpectQuery(`(?s)WITH page AS .*ORDER BY a\.name ASC, a\.account_id ASC.*LIMIT \$3.*FROM page a.*LEFT JOIN LATERAL`).
		WithArgs("user-1", sqlmock.AnyArg(), 51).
		WillReturnRows(sqlmock.NewRows([]string{"unused"}))

	rows, err := NewIndexWithDB(db).ListVisibleBlueprintsForUserPage(
		context.Background(),
		"user-1",
		[]string{"00000000-0000-0000-0000-000000000001"},
		BlueprintListOptions{Sort: "name", Limit: 51},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}

func TestListVisibleBlueprintsForUserPageNewestSortEnrichesBeforeLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
		_ = db.Close()
	})

	mock.ExpectQuery(`(?s)FROM agents a.*LEFT JOIN LATERAL.*ORDER BY v\.published_at DESC NULLS LAST.*LIMIT \$3`).
		WithArgs("user-1", sqlmock.AnyArg(), 51).
		WillReturnRows(sqlmock.NewRows([]string{"unused"}))

	rows, err := NewIndexWithDB(db).ListVisibleBlueprintsForUserPage(
		context.Background(),
		"user-1",
		[]string{"00000000-0000-0000-0000-000000000001"},
		BlueprintListOptions{Sort: "newest", Limit: 51},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}
