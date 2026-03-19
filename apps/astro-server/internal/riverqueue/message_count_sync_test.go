package riverqueue

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

func TestMessageCountSyncArgs_Kind(t *testing.T) {
	args := MessageCountSyncArgs{}
	if kind := args.Kind(); kind != "metrics.message_count_sync" {
		t.Errorf("Kind() = %q, want %q", kind, "metrics.message_count_sync")
	}
}

// promServer returns an httptest server that responds with the given agent-label samples.
func promServer(t *testing.T, agents map[string]float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		results := ""
		i := 0
		for agent, val := range agents {
			if i > 0 {
				results += ","
			}
			results += fmt.Sprintf(`{"metric":{"agent":%q},"value":[1234567890,"%g"]}`, agent, val)
			i++
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[%s]}}`, results)
	}))
}

// accountColumns are the columns returned by AccountStore.GetByName.
var accountColumns = []string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}

func TestWork_ParsesAgentLabel(t *testing.T) {
	srv := promServer(t, map[string]float64{"acct-1.bot": 42})
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery("SELECT a.id").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(accountColumns).
			AddRow("uuid-acct-1", "acct-1", "personal", nil, nil, time.Now(), time.Now()))

	mock.ExpectExec("INSERT INTO agent_message_counts").
		WithArgs("uuid-acct-1", "bot", 42.0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := &MessageCountSyncWorker{
		promClient:   promquery.NewClient(srv.URL),
		accountStore: account.NewAccountStore(db),
		db:           db,
		log:          logger.New("error", "text"),
	}

	err := w.Work(t.Context(), &river.Job[MessageCountSyncArgs]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestWork_SkipsMalformedAgentLabels(t *testing.T) {
	// "no-dot" has no separator, "" is empty — both should be skipped.
	// Only "good-acct.agent" should produce an upsert.
	srv := promServer(t, map[string]float64{
		"no-dot":          10,
		"":                20,
		"good-acct.agent": 30,
	})
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery("SELECT a.id").
		WithArgs("good-acct").
		WillReturnRows(sqlmock.NewRows(accountColumns).
			AddRow("uuid-good-acct", "good-acct", "personal", nil, nil, time.Now(), time.Now()))

	mock.ExpectExec("INSERT INTO agent_message_counts").
		WithArgs("uuid-good-acct", "agent", 30.0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := &MessageCountSyncWorker{
		promClient:   promquery.NewClient(srv.URL),
		accountStore: account.NewAccountStore(db),
		db:           db,
		log:          logger.New("error", "text"),
	}

	err := w.Work(t.Context(), &river.Job[MessageCountSyncArgs]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestWork_SkipsEmptyAccountOrAgent(t *testing.T) {
	// ".agent" has empty account, "acct." has empty agent — both skipped.
	srv := promServer(t, map[string]float64{
		".agent": 10,
		"acct.":  20,
	})
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	defer db.Close()

	// No SQL should be executed
	w := &MessageCountSyncWorker{
		promClient:   promquery.NewClient(srv.URL),
		accountStore: account.NewAccountStore(db),
		db:           db,
		log:          logger.New("error", "text"),
	}

	err := w.Work(t.Context(), &river.Job[MessageCountSyncArgs]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestWork_NilPromClient(t *testing.T) {
	w := &MessageCountSyncWorker{
		promClient: nil,
		log:        logger.New("error", "text"),
	}

	err := w.Work(t.Context(), &river.Job[MessageCountSyncArgs]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpsertMessageCount_Insert(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectExec("INSERT INTO agent_message_counts").
		WithArgs("uuid-1", "bot", 100.0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := upsertMessageCount(t.Context(), db, "uuid-1", "bot", 100.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpsertMessageCount_Update(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	// Simulates an update (ON CONFLICT path) — same SQL, different value
	mock.ExpectExec("INSERT INTO agent_message_counts").
		WithArgs("uuid-1", "bot", 200.0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := upsertMessageCount(t.Context(), db, "uuid-1", "bot", 200.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpsertMessageCount_CounterReset(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	// A lower value than before simulates a counter reset (pod restart)
	mock.ExpectExec("INSERT INTO agent_message_counts").
		WithArgs("uuid-1", "bot", 5.0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := upsertMessageCount(t.Context(), db, "uuid-1", "bot", 5.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
