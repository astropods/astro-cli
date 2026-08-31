package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/eventstream"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

// eventsTestRouter mounts the handler behind a stand-in for the middleware that
// resolves and authorizes the account, which is where the real gate lives.
func eventsTestRouter(t *testing.T, hub *eventstream.Hub, mockSetup func(sqlmock.Sqlmock)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	t.Cleanup(func() { _ = db.Close() })
	mock.MatchExpectationsInOrder(false)
	if mockSetup != nil {
		mockSetup(mock)
	}

	router := gin.New()
	router.GET("/api/v1/accounts/:account/events",
		func(c *gin.Context) {
			c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "acme"})
		},
		StreamAccountEvents(logger.New("error", "json"), hub, eventstream.NewStore(db)))
	return router
}

// The recorder is read only after the handler exits, so its buffer has no
// concurrent access.
func streamUntilClose(t *testing.T, router *gin.Engine, hub *eventstream.Hub, req *http.Request, publish func()) string {
	t.Helper()
	ctx, cancel := context.WithCancel(req.Context())
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req.WithContext(ctx))
		close(done)
	}()

	// Publishing before the handler subscribes would test the wrong thing.
	deadline := time.Now().Add(2 * time.Second)
	for hub.Subscribers("acct-1") == 0 {
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if publish != nil {
		publish()
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	return rec.Body.String()
}

func eventsRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/api/v1/accounts/acme/events", nil)
}

func TestStreamAccountEventsWritesReadyThenTheEvent(t *testing.T) {
	hub := eventstream.New()
	router := eventsTestRouter(t, hub, nil)

	body := streamUntilClose(t, router, hub, eventsRequest(), func() {
		hub.Publish(eventstream.Event{
			ID: "12", AccountID: "acct-1",
			Type: "agent.build", Agent: "reviewer", BuildID: "b1", Status: "registered",
		})
	})

	if !strings.Contains(body, "event: ready") {
		t.Fatalf("missing ready handshake:\n%s", body)
	}
	for _, want := range []string{"id: 12", "event: agent.build", `"agent":"reviewer"`, `"status":"registered"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestStreamAccountEventsIgnoresOtherAccounts(t *testing.T) {
	hub := eventstream.New()
	router := eventsTestRouter(t, hub, nil)

	body := streamUntilClose(t, router, hub, eventsRequest(), func() {
		hub.Publish(eventstream.Event{ID: "1", AccountID: "acct-2", Type: "agent.build", Agent: "secret"})
	})

	if strings.Contains(body, "secret") {
		t.Fatalf("leaked another account's event:\n%s", body)
	}
}

func TestStreamAccountEventsRefusesWithoutAResolvedAccount(t *testing.T) {
	// The handler must not fall back to reading :account off the path. Doing so
	// would stream any account's agents and builds to any signed-in user.
	gin.SetMode(gin.TestMode)
	db, _, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	router := gin.New()
	router.GET("/api/v1/accounts/:account/events",
		StreamAccountEvents(logger.New("error", "json"), eventstream.New(), eventstream.NewStore(db)))

	// Bounded: an unauthorized build streams instead of refusing, and this test
	// should report that rather than hang on an open connection.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, eventsRequest().WithContext(ctx))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler streamed instead of refusing an unresolved account")
	}

	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "event: ready") {
		t.Fatalf("opened a stream without authorization:\n%s", rec.Body.String())
	}
}

func TestStreamAccountEventsUnsubscribesOnDisconnect(t *testing.T) {
	hub := eventstream.New()
	router := eventsTestRouter(t, hub, nil)

	streamUntilClose(t, router, hub, eventsRequest(), nil)

	if n := hub.Subscribers("acct-1"); n != 0 {
		t.Fatalf("Subscribers after disconnect = %d, want 0", n)
	}
}

func TestStreamAccountEventsReplaysFromLastEventID(t *testing.T) {
	hub := eventstream.New()
	router := eventsTestRouter(t, hub, func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(`(?s)SELECT e\.id.+FROM agent_events e`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "agent_name", "type", "build_id", "status"}).
				AddRow(int64(12), "reviewer", "agent.build", "b9", "registered"))
	})

	req := eventsRequest()
	req.Header.Set("Last-Event-ID", "11")
	body := streamUntilClose(t, router, hub, req, nil)

	// The missed event must precede ready, so a client reading ready as "you are
	// caught up" is not lied to.
	replayAt := strings.Index(body, "id: 12")
	readyAt := strings.Index(body, "event: ready")
	if replayAt == -1 || readyAt == -1 {
		t.Fatalf("missing replay or ready:\n%s", body)
	}
	if replayAt > readyAt {
		t.Fatalf("ready preceded the replayed event:\n%s", body)
	}
}

func TestStreamAccountEventsSkipsReplayWithoutACursor(t *testing.T) {
	hub := eventstream.New()
	// No agent_events expectation registered: a fresh connection querying it
	// would be a wasted read on every page load, and sqlmock would fail here.
	router := eventsTestRouter(t, hub, nil)

	body := streamUntilClose(t, router, hub, eventsRequest(), nil)

	if !strings.Contains(body, "event: ready") {
		t.Fatalf("missing ready:\n%s", body)
	}
}

var accountRowCols = []string{
	"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at",
	"display_name", "avatar_colors", "avatar_updated_at", "account_number", "bio",
	"location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
}

// The handler cannot tell an authorized account from a merely resolved one, so
// this chain, mirroring main.go, is where cross-account access is refused.
func gatedEventsRouter(t *testing.T, member bool) (*gin.Engine, *eventstream.Hub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	t.Cleanup(func() { _ = db.Close() })
	store := account.NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("acme").
		WillReturnRows(sqlmock.NewRows(accountRowCols).AddRow(
			"acct-1", "acme", "organization", "wos-1", nil, time.Now(), time.Now(),
			"Acme", nil, nil, nil, "", "", "", "", "", "{}", "{}"))
	count := 0
	if member {
		count = 1
	}
	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-outsider").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))

	hub := eventstream.New()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-outsider"})
		c.Set(string(auth.SessionContextKey), &auth.Session{OrganizationID: ""})
		c.Next()
	})
	group := router.Group("/api/v1/accounts/:account")
	group.Use(middleware.ResolveAccount(store), middleware.RequireAccountMember(store))
	group.GET("/events", StreamAccountEvents(logger.New("error", "json"), hub, eventstream.NewStore(db)))
	return router, hub
}

func TestStreamAccountEventsRefusesANonMemberAtTheRoute(t *testing.T) {
	router, hub := gatedEventsRouter(t, false)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, eventsRequest().WithContext(ctx))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("streamed to a non-member instead of refusing")
	}

	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
	if hub.Subscribers("acct-1") != 0 {
		t.Fatal("a non-member was subscribed to the account's events")
	}
	if strings.Contains(rec.Body.String(), "event: ready") {
		t.Fatalf("opened a stream for a non-member:\n%s", rec.Body.String())
	}
}

func TestStreamAccountEventsServesAMemberThroughTheSameChain(t *testing.T) {
	// The refusal above has to come from membership, not from the chain being
	// broken for everyone.
	router, hub := gatedEventsRouter(t, true)

	body := streamUntilClose(t, router, hub, eventsRequest(), func() {
		hub.Publish(eventstream.Event{ID: "3", AccountID: "acct-1", Type: "agent.build", Agent: "reviewer"})
	})

	if !strings.Contains(body, "event: ready") || !strings.Contains(body, `"agent":"reviewer"`) {
		t.Fatalf("member was not served:\n%s", body)
	}
}
