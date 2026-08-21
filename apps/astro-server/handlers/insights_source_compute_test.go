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
	"github.com/astropods/astro/apps/astro-server/internal/classification"
	"github.com/astropods/astro/apps/astro-server/internal/experiment"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

const aggregateCols = `SELECT day, axis, label, actor_kind, actor_key, traces, cost_usd`

func computeMocks(t *testing.T) (*classification.Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	mock.MatchExpectationsInOrder(false)
	mock.ExpectQuery(`FROM classification_state`).
		WillReturnRows(sqlmock.NewRows([]string{
			"classified_through", "backfilled_from", "backfill_complete",
			"last_run_at", "last_error", "consecutive_errors",
		}).AddRow(nil, nil, false, nil, "", 0))
	return classification.NewStore(db), mock
}

// The reader's own emptiness is not the account's. Reporting it as "collection
// is off" sends them to a setting in a console Astro does not control.
func TestComputeInsightsSource_UnresolvedViewer(t *testing.T) {
	adapter, ok := devtoolAdapterByKey("claude-code")
	if !ok {
		t.Fatal("claude-code adapter missing")
	}
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	cases := map[string]struct {
		viewer         sourceViewer
		accountHasRows bool
		readerRows     bool
		wantUnresolved bool
		wantContent    bool
	}{
		"restricted reader with nothing of their own on a busy account": {
			viewer: sourceViewer{Restricted: true, ActorKey: "user_1"}, accountHasRows: true,
			wantUnresolved: true, wantContent: true,
		},
		"restricted reader on an account with nothing collected": {
			viewer: sourceViewer{Restricted: true, ActorKey: "user_1"}, accountHasRows: false,
			wantUnresolved: false, wantContent: false,
		},
		"restricted reader who does have rows": {
			viewer: sourceViewer{Restricted: true, ActorKey: "user_1"}, accountHasRows: true,
			readerRows: true, wantUnresolved: false, wantContent: true,
		},
		"unrestricted reader is never unresolved": {
			viewer: sourceViewer{}, accountHasRows: true,
			wantUnresolved: false, wantContent: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store, mock := computeMocks(t)
			rows := sqlmock.NewRows([]string{"day", "axis", "label", "actor_kind", "actor_key", "traces", "cost_usd"})
			if tc.readerRows {
				rows.AddRow(now.AddDate(0, 0, -2), "purpose", "work", "member", "user_1", int64(4), 2.0)
			}
			mock.ExpectQuery(aggregateCols).WillReturnRows(rows)
			mock.ExpectQuery(`SELECT EXISTS`).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tc.accountHasRows))

			resp, err := computeInsightsSource(
				context.Background(), store, "acct-1", adapter, tc.viewer, "", now)
			if err != nil {
				t.Fatalf("computeInsightsSource: %v", err)
			}
			got := resp.Ranges[widestInsightsRange().key].People.ViewerUnresolved
			if got != tc.wantUnresolved {
				t.Errorf("viewer_unresolved = %v, want %v", got, tc.wantUnresolved)
			}
			if resp.Coverage.ContentAvailable != tc.wantContent {
				t.Errorf("content_available = %v, want %v", resp.Coverage.ContentAvailable, tc.wantContent)
			}
		})
	}
}

// The client turns 404 into a route-level not-found the reader cannot retry out
// of, so a failed gate read must not look like an absent page.
func TestGetAccountInsightsSource_GateFailureIsNot404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := map[string]struct {
		gate func(sqlmock.Sqlmock)
		want int
	}{
		"switch off is not found": {
			gate: func(m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM account_experiments`).
					WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(false))
			},
			want: http.StatusNotFound,
		},
		"read failure is a server error": {
			gate: func(m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM account_experiments`).WillReturnError(context.DeadlineExceeded)
			},
			want: http.StatusInternalServerError,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer db.Close()
			mock.MatchExpectationsInOrder(false)

			mock.ExpectQuery(`FROM accounts a`).WillReturnRows(
				sqlmock.NewRows([]string{
					"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at",
					"display_name", "avatar_colors", "avatar_updated_at",
					"account_number", "bio", "location", "local_timezone", "pronouns", "website",
					"social_links", "blueprint_order",
				}).AddRow("acct-1", "acme", "organization", "org_1", nil, time.Now(), time.Now(),
					"Acme", nil, nil, nil, nil, nil, nil, nil, nil, "{}", "{}"))
			mock.ExpectQuery(`FROM account_members`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			tc.gate(mock)

			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.UserContextKey), &auth.User{ID: "user_1"})
				c.Next()
			})
			router.GET("/accounts/:account/insights/sources/:source", GetAccountInsightsSource(
				logger.New("error", "json"),
				account.NewAccountStore(db),
				classification.NewStore(db),
				nil,
				experiment.NewGate(experiment.NewStore(db), experiment.PromptClassificationStats),
			))

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(
				http.MethodGet, "/accounts/acme/insights/sources/claude-code", nil))
			// GetByName failures are also 404, so a malformed row would make the
			// switch-off case pass without the gate ever being consulted.
			if body := response.Body.String(); strings.Contains(body, "account not found") {
				t.Fatalf("the account did not resolve: %s", body)
			}
			if response.Code != tc.want {
				t.Errorf("status = %d, want %d (body %s)", response.Code, tc.want, response.Body.String())
			}
		})
	}
}
