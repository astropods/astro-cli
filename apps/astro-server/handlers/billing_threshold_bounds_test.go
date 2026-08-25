package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// thresholdRouter mounts both threshold writers with no provider. Every case
// here is decided before the provider is reached, so a rejected value answers
// 400 and an accepted one falls through to the no-provider 200.
func thresholdRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "acme"})
		c.Next()
	})
	log := logger.New("error", "json")
	r.PUT("/billing/spend/thresholds", SetBillingSpendThresholds(log, nil, nil, "metronome", nil, nil))
	r.PUT("/billing/usage/thresholds", SetBillingUsageThresholds(log, nil, nil, "metronome", nil, nil))
	return r
}

func putThreshold(t *testing.T, r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// A limit of 1e308 is finite, so the negative check passes it. The provider
// stores it and the settings page renders it, but no spend ever reaches it, so
// the account is uncapped while showing a cap.
func TestSetThresholds_AbsurdLimitIsRefused(t *testing.T) {
	r := thresholdRouter(t)
	for _, path := range []string{"/billing/spend/thresholds", "/billing/usage/thresholds"} {
		body := `{"limit":1e308}`
		if strings.Contains(path, "usage") {
			body = `{"metric":"gateway","limit":1e308}`
		}
		rec := putThreshold(t, r, path, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400: %s", path, rec.Code, rec.Body.String())
		}
	}
}

// The warning is written from the same request, so bounding only the limit
// leaves an alert that never fires. The limit is null here because a limit above
// the warning would be refused by its own bound first.
func TestSetThresholds_AbsurdWarningIsRefused(t *testing.T) {
	r := thresholdRouter(t)
	rec := putThreshold(t, r, "/billing/spend/thresholds", `{"warning":1e308,"limit":null}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	// A malformed body is also a 400, so the status alone would prove nothing.
	var got struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(got.Error, "cannot exceed") {
		t.Errorf("error = %q, want the bound to be what refused it", got.Error)
	}
}

// The bound is a value a customer is meant to be able to choose, so the largest
// one has to survive it.
func TestSetThresholds_TheBoundItselfIsAllowed(t *testing.T) {
	r := thresholdRouter(t)
	body := fmt.Sprintf(`{"limit":%.0f}`, float64(maxSpendThresholdCents))
	rec := putThreshold(t, r, "/billing/spend/thresholds", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// A spend limit is the one control that raises what we have to collect, so it
// stops where self-serve does. The reply has to name the ceiling and where to go
// for more, and that is a conversation with us: the self-serve quota increase
// route validates against the resource quota keys and cannot touch a spend
// limit, so pointing there would be a dead end.
func TestSetThresholds_SpendLimitStopsAtTheSelfServeCeiling(t *testing.T) {
	r := thresholdRouter(t)
	body := fmt.Sprintf(`{"limit":%.0f}`, float64(maxSpendThresholdCents)+1)
	rec := putThreshold(t, r, "/billing/spend/thresholds", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if want := fmt.Sprintf("$%.0f", billing.MaxSelfServeSpendUSD); !strings.Contains(got.Error, want) {
		t.Errorf("error = %q, want the ceiling %s named", got.Error, want)
	}
	if !strings.Contains(got.Error, "enterprise") {
		t.Errorf("error = %q, want the enterprise route named as the way past the ceiling", got.Error)
	}
	if strings.Contains(got.Error, "quota increase") {
		t.Errorf("error = %q, points at a route that cannot raise a spend limit", got.Error)
	}
}

// Usage thresholds carry a different unit per metric, so the self-serve dollar
// ceiling must not narrow them: a CU-hour budget well above $1,000 is ordinary.
func TestSetUsageThresholds_AreNotBoundedByTheSpendCeiling(t *testing.T) {
	r := thresholdRouter(t)
	body := fmt.Sprintf(`{"metric":"compute","limit":%.0f}`, float64(maxSpendThresholdCents)*10)
	rec := putThreshold(t, r, "/billing/usage/thresholds", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// Clearing a control sends null, which must not be read as a value to bound.
func TestSetThresholds_NullStillClears(t *testing.T) {
	r := thresholdRouter(t)
	rec := putThreshold(t, r, "/billing/spend/thresholds", `{"warning":null,"limit":null}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// recordingReconcileQueue records the jobs a threshold write enqueues.
type recordingReconcileQueue struct {
	gatewayBudget []string
	resumed       []string
}

func (q *recordingReconcileQueue) InsertBillingSuspend(context.Context, string) error { return nil }
func (q *recordingReconcileQueue) InsertBillingResume(_ context.Context, id string) error {
	q.resumed = append(q.resumed, id)
	return nil
}
func (q *recordingReconcileQueue) InsertBillingCollect(context.Context, string, string) error {
	return nil
}
func (q *recordingReconcileQueue) InsertBillingGatewayBudget(_ context.Context, id string) error {
	q.gatewayBudget = append(q.gatewayBudget, id)
	return nil
}

// The gateway enforces its ceiling in real time and derives it from this limit,
// so a raise that does not re-derive leaves the account capped at the old number
// until an unrelated event fires the job. Card writes enqueue it; this route did
// not.
func TestSetSpendThresholds_RaisingTheLimitRederivesTheGatewayBudget(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "acme"})
		c.Next()
	})
	q := &recordingReconcileQueue{}
	// A writer provider so the handler gets past the capability check and
	// actually reaches the write, rather than the no-provider early return.
	provider := thresholdWriterProvider{}
	r.PUT("/billing/spend/thresholds", SetBillingSpendThresholds(
		logger.New("error", "json"), account.NewAccountStore(db), provider, "metronome", nil, q))

	mock.ExpectQuery("SELECT metronome_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))

	rec := putThreshold(t, r, "/billing/spend/thresholds", `{"warning":null,"limit":50000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(q.gatewayBudget) != 1 || q.gatewayBudget[0] != "acct-1" {
		t.Errorf("gateway budget re-derive = %v, want [acct-1]: the ceiling stays stale", q.gatewayBudget)
	}
}

// thresholdWriterProvider accepts threshold writes and reports no existing
// thresholds. The embedded interface is nil, so any other call panics.
type thresholdWriterProvider struct {
	billing.BillingProvider
}

func (thresholdWriterProvider) SetCustomerSpendThreshold(context.Context, string, billing.SpendThresholdKind, float64) error {
	return nil
}
func (thresholdWriterProvider) ClearCustomerSpendThreshold(context.Context, string, billing.SpendThresholdKind) error {
	return nil
}

// partialWriterProvider applies the limit and then fails the warning, which is
// the reachable partial failure: writeSpendThresholds writes the limit first.
type partialWriterProvider struct {
	billing.BillingProvider
}

func (partialWriterProvider) SetCustomerSpendThreshold(_ context.Context, _ string, kind billing.SpendThresholdKind, _ float64) error {
	if kind == billing.SpendThresholdWarning {
		return errors.New("provider unavailable")
	}
	return nil
}
func (partialWriterProvider) ClearCustomerSpendThreshold(_ context.Context, _ string, kind billing.SpendThresholdKind) error {
	if kind == billing.SpendThresholdWarning {
		return errors.New("provider unavailable")
	}
	return nil
}

// The limit landed and the warning did not, so the response is a 502 and the
// ceiling is nonetheless out of date. Enqueueing only on full success leaves the
// gateway enforcing the old number with no event coming to correct it.
func TestSetSpendThresholds_PartialFailureStillRederivesTheGatewayBudget(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "acme"})
		c.Next()
	})
	q := &recordingReconcileQueue{}
	r.PUT("/billing/spend/thresholds", SetBillingSpendThresholds(
		logger.New("error", "json"), account.NewAccountStore(db), partialWriterProvider{}, "metronome", nil, q))

	mock.ExpectQuery("SELECT metronome_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))

	rec := putThreshold(t, r, "/billing/spend/thresholds", `{"warning":2500,"limit":50000}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", rec.Code, rec.Body.String())
	}
	if len(q.gatewayBudget) != 1 || q.gatewayBudget[0] != "acct-1" {
		t.Errorf("gateway budget re-derive = %v, want [acct-1]: the limit changed but the ceiling did not", q.gatewayBudget)
	}
}

// A failed limit write is the case that most needs re-deriving, not the one to
// skip. Replacing a threshold archives the old alert before creating the
// replacement, so a create that fails leaves the account with no limit at all
// while the gateway still enforces the old number.
func TestSetSpendThresholds_LimitFailureStillRederives(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "acme"})
		c.Next()
	})
	q := &recordingReconcileQueue{}
	r.PUT("/billing/spend/thresholds", SetBillingSpendThresholds(
		logger.New("error", "json"), account.NewAccountStore(db), limitFailingProvider{}, "metronome", nil, q))

	mock.ExpectQuery("SELECT metronome_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))

	if rec := putThreshold(t, r, "/billing/spend/thresholds", `{"limit":50000}`); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if len(q.gatewayBudget) != 1 {
		t.Errorf("gateway budget re-derive = %v, want [acct-1]: the limit may now be unset", q.gatewayBudget)
	}
}

type limitFailingProvider struct {
	billing.BillingProvider
}

func (limitFailingProvider) SetCustomerSpendThreshold(context.Context, string, billing.SpendThresholdKind, float64) error {
	return errors.New("provider unavailable")
}
func (limitFailingProvider) ClearCustomerSpendThreshold(context.Context, string, billing.SpendThresholdKind) error {
	return errors.New("provider unavailable")
}

// ctxRecordingQueue records whether the context it was handed was already dead.
type ctxRecordingQueue struct {
	recordingReconcileQueue
	sawCanceled bool
}

func (q *ctxRecordingQueue) InsertBillingGatewayBudget(ctx context.Context, id string) error {
	if ctx.Err() != nil {
		q.sawCanceled = true
		return ctx.Err()
	}
	return q.recordingReconcileQueue.InsertBillingGatewayBudget(ctx, id)
}

// The provider write lands before the enqueue, so a client that hangs up mid-save
// must not take the re-derive with it: the limit would be changed and the gateway
// left enforcing the old number with nothing queued to correct it.
func TestSetSpendThresholds_CanceledRequestStillEnqueuesTheRederive(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Cancel before the handler runs, which is what a hung-up client leaves
	// behind by the time the post-write reconcile is reached.
	ctx, cancel := context.WithCancel(context.Background())
	r.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "acme"})
		c.Request = c.Request.WithContext(ctx)
		cancel()
		c.Next()
	})
	q := &ctxRecordingQueue{}
	r.PUT("/billing/spend/thresholds", SetBillingSpendThresholds(
		logger.New("error", "json"), account.NewAccountStore(db), thresholdWriterProvider{}, "metronome", nil, q))

	mock.ExpectQuery("SELECT metronome_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))

	putThreshold(t, r, "/billing/spend/thresholds", `{"limit":50000}`)

	if q.sawCanceled {
		t.Error("the re-derive was handed a canceled context, so the insert would not persist")
	}
	if len(q.gatewayBudget) != 1 {
		t.Errorf("gateway budget re-derive = %v, want [acct-1]", q.gatewayBudget)
	}
}
