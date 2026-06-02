package riverqueue

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// fakeInsightsComputer is a controllable stand-in for InsightsSummaryComputer.
// Per-endpoint canned responses let us mix success/failure within one run.
type fakeInsightsComputer struct {
	mu      sync.Mutex
	results map[insightscache.Endpoint]struct {
		data []byte
		err  error
	}
	calls map[insightscache.Endpoint]int
}

func newFakeInsightsComputer() *fakeInsightsComputer {
	return &fakeInsightsComputer{
		results: make(map[insightscache.Endpoint]struct {
			data []byte
			err  error
		}),
		calls: make(map[insightscache.Endpoint]int),
	}
}

func (f *fakeInsightsComputer) set(endpoint insightscache.Endpoint, data []byte, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[endpoint] = struct {
		data []byte
		err  error
	}{data, err}
}

func (f *fakeInsightsComputer) callsFor(endpoint insightscache.Endpoint) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[endpoint]
}

func (f *fakeInsightsComputer) respond(endpoint insightscache.Endpoint) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[endpoint]++
	r, ok := f.results[endpoint]
	if !ok {
		return []byte(`{}`), nil
	}
	return r.data, r.err
}

func (f *fakeInsightsComputer) ComputeSummary(_ context.Context, _, _ string, _ bool) ([]byte, error) {
	return f.respond(insightscache.EndpointSummary)
}

func (f *fakeInsightsComputer) ComputeDeploymentsSummary(_ context.Context, _ string, _ bool) ([]byte, error) {
	return f.respond(insightscache.EndpointDeploymentsSummary)
}

func (f *fakeInsightsComputer) ComputeUsersSummary(_ context.Context, _ string) ([]byte, error) {
	return f.respond(insightscache.EndpointUsersSummary)
}

var _ InsightsSummaryComputer = (*fakeInsightsComputer)(nil)

// ── Args.Kind sanity ───────────────────────────────────────────────────────

func TestInsightsRefreshArgs_Kind(t *testing.T) {
	if got := (InsightsRefreshArgs{}).Kind(); got != "insights.refresh" {
		t.Errorf("Kind() = %q, want insights.refresh", got)
	}
}

func TestInsightsRefreshAccountArgs_Kind(t *testing.T) {
	if got := (InsightsRefreshAccountArgs{}).Kind(); got != "insights.refresh_account" {
		t.Errorf("Kind() = %q, want insights.refresh_account", got)
	}
}

// ── InsightsRefreshAccountWorker: the per-account refresh ─────────────────

func newAccountJob(accountID string) *river.Job[InsightsRefreshAccountArgs] {
	return &river.Job[InsightsRefreshAccountArgs]{Args: InsightsRefreshAccountArgs{AccountID: accountID}}
}

func TestInsightsRefreshAccountWorker_NoCache_Skips(t *testing.T) {
	// nil cache → short-circuit. computer is wired but should never be
	// called — reaching the end without a call is the assertion (a real
	// call would inflate the call counter, and we'd notice).
	computer := newFakeInsightsComputer()
	w := &InsightsRefreshAccountWorker{
		cache:    nil,
		computer: computer,
		log:      logger.New("error", "text"),
	}
	if err := w.Work(context.Background(), newAccountJob("acct-1")); err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
	if got := computer.callsFor(insightscache.EndpointSummary); got != 0 {
		t.Errorf("computer should not have been called when cache is nil; got %d calls", got)
	}
}

func TestInsightsRefreshAccountWorker_NoComputer_Skips(t *testing.T) {
	w := &InsightsRefreshAccountWorker{
		cache:    newMapCache(),
		computer: nil,
		log:      logger.New("error", "text"),
	}
	if err := w.Work(context.Background(), newAccountJob("acct-1")); err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
}

func TestInsightsRefreshAccountWorker_EmptyAccountID_NoOp(t *testing.T) {
	// Malformed args (no AccountID) — return nil, don't retry, don't
	// touch the computer.
	computer := newFakeInsightsComputer()
	w := &InsightsRefreshAccountWorker{
		cache:    newMapCache(),
		computer: computer,
		log:      logger.New("error", "text"),
	}
	if err := w.Work(context.Background(), newAccountJob("")); err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
	if got := computer.callsFor(insightscache.EndpointSummary); got != 0 {
		t.Errorf("computer called with empty accountID; got %d calls", got)
	}
}

func TestInsightsRefreshAccountWorker_AllEndpointsSucceed(t *testing.T) {
	cache := newMapCache()
	computer := newFakeInsightsComputer()
	computer.set(insightscache.EndpointSummary, []byte(`{"summary":1}`), nil)
	computer.set(insightscache.EndpointDeploymentsSummary, []byte(`{"deployments":1}`), nil)
	computer.set(insightscache.EndpointUsersSummary, []byte(`{"users":1}`), nil)

	w := &InsightsRefreshAccountWorker{cache: cache, computer: computer, log: logger.New("error", "text")}
	if err := w.Work(context.Background(), newAccountJob("acct-1")); err != nil {
		t.Fatalf("Work returned error on full success: %v", err)
	}

	cases := []struct {
		endpoint insightscache.Endpoint
		params   insightscache.Params
		want     string
	}{
		{insightscache.EndpointSummary, insightscache.Params{GroupBy: "user"}, `{"summary":1}`},
		{insightscache.EndpointDeploymentsSummary, insightscache.Params{}, `{"deployments":1}`},
		{insightscache.EndpointUsersSummary, insightscache.Params{}, `{"users":1}`},
	}
	for _, c := range cases {
		got, ok := insightscache.Get(context.Background(), cache, "acct-1", c.endpoint, c.params)
		if !ok {
			t.Errorf("%s: no cache entry written", c.endpoint)
			continue
		}
		if string(got) != c.want {
			t.Errorf("%s: cache bytes = %q, want %q", c.endpoint, got, c.want)
		}
	}
}

func TestInsightsRefreshAccountWorker_AllLangfuseFailed_NoRetry_PreservesCache(t *testing.T) {
	// All three endpoints return ErrAllLangfuseCallsFailed → worker must
	// return nil (no River retry) AND must NOT overwrite any pre-existing
	// cache entry. This is the "preserve last-good value during outage"
	// invariant the long TTL is designed for.
	cache := newMapCache()
	preExisting := []byte(`{"prior":"value"}`)
	_ = insightscache.Put(context.Background(), cache, "acct-1",
		insightscache.EndpointSummary,
		insightscache.Params{GroupBy: "user"}, preExisting)

	computer := newFakeInsightsComputer()
	computer.set(insightscache.EndpointSummary, nil, insightscache.ErrAllLangfuseCallsFailed)
	computer.set(insightscache.EndpointDeploymentsSummary, nil, insightscache.ErrAllLangfuseCallsFailed)
	computer.set(insightscache.EndpointUsersSummary, nil, insightscache.ErrAllLangfuseCallsFailed)

	w := &InsightsRefreshAccountWorker{cache: cache, computer: computer, log: logger.New("error", "text")}
	if err := w.Work(context.Background(), newAccountJob("acct-1")); err != nil {
		t.Fatalf("expected nil error (no River retry) on Langfuse outage, got: %v", err)
	}

	got, ok := insightscache.Get(context.Background(), cache, "acct-1",
		insightscache.EndpointSummary, insightscache.Params{GroupBy: "user"})
	if !ok {
		t.Fatal("pre-existing cache entry was deleted on Langfuse failure")
	}
	if string(got) != string(preExisting) {
		t.Errorf("cache bytes overwritten on Langfuse failure: got %q, want %q", got, preExisting)
	}
}

func TestInsightsRefreshAccountWorker_TransientError_Retries(t *testing.T) {
	// A non-ErrAllLangfuseCallsFailed error (DB blip, generic failure)
	// should bubble up so River retries. Distinguish from the Langfuse-out
	// case: that returns nil because retries are wasted; this returns the
	// error.
	cache := newMapCache()
	computer := newFakeInsightsComputer()
	computer.set(insightscache.EndpointSummary, nil, errors.New("transient db hiccup"))
	computer.set(insightscache.EndpointDeploymentsSummary, []byte(`{"d":1}`), nil)
	computer.set(insightscache.EndpointUsersSummary, []byte(`{"u":1}`), nil)

	w := &InsightsRefreshAccountWorker{cache: cache, computer: computer, log: logger.New("error", "text")}
	err := w.Work(context.Background(), newAccountJob("acct-2"))
	if err == nil {
		t.Fatal("expected non-nil error so River retries the transient failure")
	}

	// The endpoints that DID succeed should still have their cache written —
	// we don't roll back successful writes just because one endpoint blew up.
	if _, ok := insightscache.Get(context.Background(), cache, "acct-2",
		insightscache.EndpointDeploymentsSummary, insightscache.Params{}); !ok {
		t.Error("deployments-summary cache should have been written despite summary failure")
	}
}

func TestInsightsRefreshAccountWorker_PartialFailure_WritesSucceeded(t *testing.T) {
	// Langfuse-out on one endpoint, success on the other two → partial
	// failure with no River retry (Langfuse-out is the dominant signal).
	cache := newMapCache()
	computer := newFakeInsightsComputer()
	computer.set(insightscache.EndpointSummary, []byte(`{"summary":"ok"}`), nil)
	computer.set(insightscache.EndpointDeploymentsSummary, nil, insightscache.ErrAllLangfuseCallsFailed)
	computer.set(insightscache.EndpointUsersSummary, []byte(`{"users":"ok"}`), nil)

	w := &InsightsRefreshAccountWorker{cache: cache, computer: computer, log: logger.New("error", "text")}
	if err := w.Work(context.Background(), newAccountJob("acct-3")); err != nil {
		t.Fatalf("partial Langfuse-out should not trigger River retry; got error: %v", err)
	}

	if _, ok := insightscache.Get(context.Background(), cache, "acct-3",
		insightscache.EndpointSummary, insightscache.Params{GroupBy: "user"}); !ok {
		t.Error("summary should have been written")
	}
	if _, ok := insightscache.Get(context.Background(), cache, "acct-3",
		insightscache.EndpointDeploymentsSummary, insightscache.Params{}); ok {
		t.Error("deployments-summary should NOT have been written when its compute returned ErrAllLangfuseCallsFailed")
	}
	if _, ok := insightscache.Get(context.Background(), cache, "acct-3",
		insightscache.EndpointUsersSummary, insightscache.Params{}); !ok {
		t.Error("users-summary should have been written")
	}
}

func TestInsightsRefreshAccountWorker_AllEndpointsAttempted(t *testing.T) {
	// One endpoint failing must not short-circuit the rest — each runs
	// independently. Verified by call counters on the fake computer.
	cache := newMapCache()
	computer := newFakeInsightsComputer()
	computer.set(insightscache.EndpointSummary, nil, errors.New("first endpoint blows up"))
	computer.set(insightscache.EndpointDeploymentsSummary, []byte(`{"d":1}`), nil)
	computer.set(insightscache.EndpointUsersSummary, []byte(`{"u":1}`), nil)

	w := &InsightsRefreshAccountWorker{cache: cache, computer: computer, log: logger.New("error", "text")}
	_ = w.Work(context.Background(), newAccountJob("acct-4"))

	for _, ep := range []insightscache.Endpoint{
		insightscache.EndpointSummary,
		insightscache.EndpointDeploymentsSummary,
		insightscache.EndpointUsersSummary,
	} {
		if got := computer.callsFor(ep); got != 1 {
			t.Errorf("%s calls = %d, want 1", ep, got)
		}
	}
}
