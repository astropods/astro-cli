package agentindex

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func accountListRowColumns(paginated bool) []string {
	cols := []string{
		"account_id", "name", "registry", "visibility", "avatar_colors", "created_at", "updated_at",
		"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at",
		"version_count",
	}
	if paginated {
		cols = append(cols, "list_total")
	}
	return cols
}

func publicListRowColumns(paginated bool) []string {
	cols := []string{
		"account_id", "name", "registry", "visibility", "avatar_colors", "created_at", "updated_at",
		"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "published_at", "updated_at",
	}
	if paginated {
		cols = append(cols, "list_total")
	}
	return cols
}

func TestListForAccount_WithQueryFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()

	mock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("acct-1", "%review%").
		WillReturnRows(sqlmock.NewRows(accountListRowColumns(false)).
			AddRow("acct-1", "code-reviewer", "reg", "public", nil, now, now,
				"build-1", "ns", `{"name":"test"}`, "", "", "[]", now, now, 1))

	page, err := idx.ListForAccount("acct-1", BlueprintListOptions{Query: "review", Sort: "name"})
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if page.Total != 1 || len(page.Agents) != 1 || page.Agents[0].Name != "code-reviewer" {
		t.Fatalf("unexpected page: total=%d agents=%+v", page.Total, page.Agents)
	}
	if page.Agents[0].VersionCount != 1 || len(page.Agents[0].Versions) != 1 {
		t.Fatalf("expected latest version only: version_count=%d versions=%d", page.Agents[0].VersionCount, len(page.Agents[0].Versions))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestListForAccount_QueryDoesNotMatchSiblingAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()

	mock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("acct-1", "%retail%").
		WillReturnRows(sqlmock.NewRows(accountListRowColumns(false)).
			AddRow("acct-1", "northstar-sales", "reg", "private", nil, now, now,
				"build-1", "ns", `{"name":"test"}`, "", "", "[]", now, now, 2))

	page, err := idx.ListForAccount("acct-1", BlueprintListOptions{Query: "retail", Sort: "name"})
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if len(page.Agents) != 1 || page.Agents[0].Name != "northstar-sales" {
		t.Fatalf("unexpected agents: %+v", page.Agents)
	}
	if page.Agents[0].VersionCount != 2 {
		t.Fatalf("expected version_count 2, got %d", page.Agents[0].VersionCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestListForAccount_QueryMatchesTags(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()

	mock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("acct-1", "%northstar%").
		WillReturnRows(sqlmock.NewRows(accountListRowColumns(false)).
			AddRow("acct-1", "northstar-support", "reg", "private", nil, now, now,
				"build-1", "ns", `{"name":"test"}`, "", "", "[]", now, now, 1))

	page, err := idx.ListForAccount("acct-1", BlueprintListOptions{Query: "northstar", Sort: "name"})
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if len(page.Agents) != 1 || page.Agents[0].Name != "northstar-support" {
		t.Fatalf("unexpected agents: %+v", page.Agents)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestListForAccount_VisibilityFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	mock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("acct-1", "private").
		WillReturnRows(sqlmock.NewRows(accountListRowColumns(false)))

	_, err = idx.ListForAccount("acct-1", BlueprintListOptions{Visibility: "private", Sort: "name"})
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestListForAccount_Pagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()

	mock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("acct-1", 2, 1).
		WillReturnRows(sqlmock.NewRows(accountListRowColumns(true)).
			AddRow("acct-1", "agent-b", "reg", "public", nil, now, now,
				"build-1", "ns", `{"name":"test"}`, "", "", "[]", now, now, 3, 5))

	page, err := idx.ListForAccount("acct-1", BlueprintListOptions{Sort: "name", Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if page.Total != 5 || len(page.Agents) != 1 {
		t.Fatalf("unexpected page: total=%d len=%d", page.Total, len(page.Agents))
	}
	if page.Agents[0].VersionCount != 3 {
		t.Fatalf("expected version_count 3, got %d", page.Agents[0].VersionCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestListForAccount_DraftWithNoVersions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()

	mock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(accountListRowColumns(false)).
			AddRow("acct-1", "draft-agent", "reg", "private", nil, now, now,
				nil, nil, nil, nil, nil, nil, nil, nil, 0))

	page, err := idx.ListForAccount("acct-1", BlueprintListOptions{Sort: "name"})
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if len(page.Agents) != 1 || len(page.Agents[0].Versions) != 0 {
		t.Fatalf("expected draft agent with no versions, got %+v", page.Agents)
	}
	if page.Total != 1 {
		t.Fatalf("expected total 1 from len(agents), got %d", page.Total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestListPublicAgents_WithQueryFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()

	mock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("%analytics%").
		WillReturnRows(sqlmock.NewRows(publicListRowColumns(false)).
			AddRow("acct-1", "analytics-bot", "reg", "public", nil, now, now,
				"build-1", "ns", `{"name":"test"}`, "", "", now, now))

	page, err := idx.ListPublicAgents(BlueprintListOptions{Query: "analytics", Sort: "name"})
	if err != nil {
		t.Fatalf("ListPublicAgents: %v", err)
	}
	if page.Total != 1 || len(page.Agents) != 1 || page.Agents[0].Name != "analytics-bot" {
		t.Fatalf("unexpected page: total=%d agents=%+v", page.Total, page.Agents)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestListPublicAgents_Pagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()

	mock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("%bot%", 10, 0).
		WillReturnRows(sqlmock.NewRows(publicListRowColumns(true)).
			AddRow("acct-1", "bot-one", "reg", "public", nil, now, now,
				"build-1", "ns", `{"name":"test"}`, "", "", now, now, 12))

	page, err := idx.ListPublicAgents(BlueprintListOptions{Query: "bot", Sort: "name", Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListPublicAgents: %v", err)
	}
	if page.Total != 12 || len(page.Agents) != 1 {
		t.Fatalf("unexpected page: total=%d len=%d", page.Total, len(page.Agents))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
