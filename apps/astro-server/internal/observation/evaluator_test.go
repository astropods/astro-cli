package observation

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
)

type fakeEngine struct{ vec []Series }

func (f fakeEngine) Query(context.Context, string) ([]Series, error) { return f.vec, nil }

type fakeDeploys struct{}

func (fakeDeploys) GetLatestDeploymentByNamespace(ns string) (*deploymentstore.Deployment, error) {
	return &deploymentstore.Deployment{ID: "dep_" + ns, AccountID: "acct", AgentName: "agent-" + ns}, nil
}

// GetRuntimeSnapshot maps two pods to two workloads so pod→workload resolution
// can be exercised; series without a pod label resolve to workload "".
func (fakeDeploys) GetRuntimeSnapshot(string) (*deploymentstore.RuntimeSnapshot, time.Time, error) {
	return &deploymentstore.RuntimeSnapshot{
		Workloads: []deploymentstore.RuntimeWorkload{
			{Name: "agent", Component: "agent", Pods: []deploymentstore.RuntimePod{{Name: "agent-abc"}}},
			{Name: "model-x", Component: "model-x", Pods: []deploymentstore.RuntimePod{{Name: "model-x-def"}}},
		},
	}, time.Time{}, nil
}

// memState is an in-memory stateStore keyed by "deploymentID|workload|condition".
// notif holds the daily-cap ledger keyed by "deploymentID|condition".
type memState struct {
	m     map[string]Tracked
	notif map[string]time.Time
	mutes map[string]time.Time // "deploymentID|condition" -> muted_until
}

func newMemState() *memState {
	return &memState{m: map[string]Tracked{}, notif: map[string]time.Time{}, mutes: map[string]time.Time{}}
}

func mkey(dep, wl, cond string) string { return dep + "|" + wl + "|" + cond }

func (s *memState) ForCondition(_ context.Context, cond string) ([]Tracked, error) {
	var out []Tracked
	for k, t := range s.m {
		if strings.HasSuffix(k, "|"+cond) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *memState) StartTracking(_ context.Context, dep, wl, cond string, since time.Time, notified bool) error {
	k := mkey(dep, wl, cond)
	if _, ok := s.m[k]; !ok {
		s.m[k] = Tracked{DeploymentID: dep, Workload: wl, ActiveSince: since, Notified: notified}
	}
	return nil
}

func (s *memState) MarkNotified(_ context.Context, dep, wl, cond string) error {
	k := mkey(dep, wl, cond)
	t := s.m[k]
	t.Notified = true
	s.m[k] = t
	return nil
}

func (s *memState) ClaimDailyNotify(_ context.Context, dep, cond string, at, cutoff time.Time) (bool, error) {
	k := dep + "|" + cond
	if last, ok := s.notif[k]; ok && !last.Before(cutoff) {
		return false, nil // last send is at/after the cutoff → throttled
	}
	s.notif[k] = at
	return true, nil
}

func (s *memState) Clear(_ context.Context, dep, wl, cond string) error {
	delete(s.m, mkey(dep, wl, cond))
	return nil
}

func (s *memState) IsMuted(_ context.Context, dep, cond string, now time.Time) (bool, error) {
	until, ok := s.mutes[dep+"|"+cond]
	return ok && until.After(now), nil
}

func breaching(ns string) []Series {
	return []Series{{Labels: map[string]string{"namespace": ns}, Value: 1}}
}

func breachingPods(ns string, pods ...string) []Series {
	out := make([]Series, 0, len(pods))
	for _, p := range pods {
		out = append(out, Series{Labels: map[string]string{"namespace": ns, "pod": p}, Value: 1})
	}
	return out
}

// For==0: fires on first detection, never re-fires while firing, a same-day
// re-breach is suppressed by the daily cap, and a re-breach past the window
// fires again with a fresh (per-episode) dedupe key.
func TestEvaluateFiresOnceAndDedups(t *testing.T) {
	var emitted []notify.Event
	emit := func(_ context.Context, ev notify.Event) error { emitted = append(emitted, ev); return nil }
	st := newMemState()
	cond := Condition{Name: "crash_loop", Title: "Crash loop", Severity: SeverityCritical, Engine: EnginePromQL, Query: "q", For: 0}
	e := NewEvaluator(nil, fakeDeploys{}, st, nil, emit, nil)
	fireQ := fakeEngine{vec: breaching("ns1")}
	clearQ := fakeEngine{vec: nil}
	t0 := time.Unix(1000, 0)

	_ = e.evaluate(context.Background(), cond, fireQ, t0)                    // fires
	_ = e.evaluate(context.Background(), cond, fireQ, t0.Add(1*time.Minute)) // still firing → no re-emit
	if len(emitted) != 1 {
		t.Fatalf("want 1 emit after firing+resweep, got %d", len(emitted))
	}
	if emitted[0].DedupeKey == "" || emitted[0].Type != notify.TypeObservationCritical {
		t.Fatalf("bad event: %+v", emitted[0])
	}
	if emitted[0].Payload[notify.PayloadReason] != "Crash loop" {
		t.Fatalf("want reason=Crash loop in payload (no workload), got %+v", emitted[0].Payload)
	}

	_ = e.evaluate(context.Background(), cond, clearQ, t0.Add(2*time.Minute)) // resolves → state cleared
	_ = e.evaluate(context.Background(), cond, fireQ, t0.Add(3*time.Minute))  // same-day re-breach → daily cap suppresses
	if len(emitted) != 1 {
		t.Fatalf("same-day re-breach must be suppressed by the daily cap, got %d emits", len(emitted))
	}

	_ = e.evaluate(context.Background(), cond, clearQ, t0.Add(4*time.Minute)) // resolves again
	_ = e.evaluate(context.Background(), cond, fireQ, t0.Add(25*time.Hour))   // re-breach past the window → fires again
	if len(emitted) != 2 {
		t.Fatalf("want 2 emits after re-breach past the daily window, got %d", len(emitted))
	}
	if emitted[0].DedupeKey == emitted[1].DedupeKey {
		t.Fatalf("re-fire must use a fresh dedupe key, both = %q", emitted[0].DedupeKey)
	}
}

// For>0: holds during the sustained window, then fires once.
func TestEvaluateHonorsForWindow(t *testing.T) {
	var emitted []notify.Event
	emit := func(_ context.Context, ev notify.Event) error { emitted = append(emitted, ev); return nil }
	st := newMemState()
	cond := Condition{Name: "memory_over_budget", Title: "Near memory limit", Severity: SeverityWarning, Engine: EnginePromQL, Query: "q", For: 5 * time.Minute}
	e := NewEvaluator(nil, fakeDeploys{}, st, nil, emit, nil)
	q := fakeEngine{vec: breaching("ns1")}
	t0 := time.Unix(1000, 0)

	_ = e.evaluate(context.Background(), cond, q, t0)                    // pending (window opens)
	_ = e.evaluate(context.Background(), cond, q, t0.Add(2*time.Minute)) // still pending (< 5m)
	if len(emitted) != 0 {
		t.Fatalf("must not fire before the for-window elapses, got %d", len(emitted))
	}
	_ = e.evaluate(context.Background(), cond, q, t0.Add(6*time.Minute)) // window elapsed → fire
	_ = e.evaluate(context.Background(), cond, q, t0.Add(7*time.Minute)) // already notified → no re-emit
	if len(emitted) != 1 {
		t.Fatalf("want exactly 1 emit after the window, got %d", len(emitted))
	}
}

// Two workloads of one deployment trip the same condition: both are tracked
// per-workload (so the UI can attribute each), but only one notification is
// emitted for the deployment, and its reason names a workload.
func TestEvaluatePerWorkloadStateSingleNotification(t *testing.T) {
	var emitted []notify.Event
	emit := func(_ context.Context, ev notify.Event) error { emitted = append(emitted, ev); return nil }
	st := newMemState()
	cond := Condition{Name: "crash_loop", Title: "Crash loop", Severity: SeverityCritical, Engine: EnginePromQL, Query: "q", For: 0}
	q := fakeEngine{vec: breachingPods("ns1", "agent-abc", "model-x-def")}
	e := NewEvaluator(nil, fakeDeploys{}, st, nil, emit, nil)

	_ = e.evaluate(context.Background(), cond, q, time.Unix(1000, 0))

	if len(emitted) != 1 {
		t.Fatalf("want 1 deployment-level emit for 2 breaching workloads, got %d", len(emitted))
	}
	if len(st.m) != 2 {
		t.Fatalf("want 2 per-workload state rows, got %d", len(st.m))
	}
	reason, _ := emitted[0].Payload[notify.PayloadReason].(string)
	if !strings.Contains(reason, "Crash loop") || !strings.Contains(reason, "(") {
		t.Fatalf("reason should name the affected workload, got %q", reason)
	}
}

// A condition with a DetailsFor formatter enriches the payload details with the
// breaching series' value; when several pods map to one workload, the highest
// value (least-wasteful pod) is reported.
func TestEvaluateEnrichesDetailsFromValue(t *testing.T) {
	var emitted []notify.Event
	emit := func(_ context.Context, ev notify.Event) error { emitted = append(emitted, ev); return nil }
	st := newMemState()
	cond := Condition{
		Name: "cpu_over_provisioned", Title: "Unused CPU",
		Description: "base.", Guidance: "fix it.", Severity: SeverityInfo, Engine: EnginePromQL, Query: "q", For: 0,
		DetailsFor: overProvisionedDetail("CPU"),
	}
	// Two pods of the same workload: ratios 0.1 and 0.3 → the 0.3 pod wins.
	q := fakeEngine{vec: []Series{
		{Labels: map[string]string{"namespace": "ns1", "pod": "agent-abc"}, Value: 0.1},
		{Labels: map[string]string{"namespace": "ns1", "pod": "agent-xyz"}, Value: 0.3},
	}}
	e := NewEvaluator(nil, fakeDeploys{}, st, nil, emit, nil)

	_ = e.evaluate(context.Background(), cond, q, time.Unix(1000, 0))

	if len(emitted) != 1 {
		t.Fatalf("want 1 emit, got %d", len(emitted))
	}
	details, _ := emitted[0].Payload[notify.PayloadDetails].(string)
	if !strings.HasPrefix(details, "base. ") {
		t.Fatalf("details should extend the base description, got %q", details)
	}
	// Two pods of one workload: the 0.3 ratio wins over the 0.1.
	if !strings.Contains(details, "30%") {
		t.Fatalf("details should quote the worst pod's ratio (30%%), got %q", details)
	}
	if !strings.HasSuffix(details, "fix it.") {
		t.Fatalf("guidance should close the details, got %q", details)
	}
}

// The per-condition detail formatters render the query value into a clause, and
// fall back to "" (base description only) for a non-positive value.
func TestDetailFormatters(t *testing.T) {
	cases := []struct {
		name string
		fn   func(float64) string
		val  float64
		want []string // all substrings must be present; nil means want == ""
	}{
		{"over-provisioned quotes the peak share", overProvisionedDetail("CPU"), 0.3, []string{"30%", "CPU"}},
		{"over-provisioned ignores out-of-range", overProvisionedDetail("CPU"), 1.5, nil},
		{"restart storm quotes the count", restartStormDetail, 7.4, []string{"7 times"}},
		{"restart storm ignores zero", restartStormDetail, 0, nil},
		{"memory over budget quotes % of limit", memoryOverBudgetDetail, 0.94, []string{"94%", "limit"}},
		{"compute throttle quotes % of periods", computeThrottleDetail, 0.72, []string{"72%", "waiting for CPU"}},
		{"compute throttle ignores zero", computeThrottleDetail, 0, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn(tc.val)
			if tc.want == nil {
				if got != "" {
					t.Fatalf("want empty clause, got %q", got)
				}
				return
			}
			for _, sub := range tc.want {
				if !strings.Contains(got, sub) {
					t.Fatalf("clause %q missing %q", got, sub)
				}
			}
		})
	}
}

// A muted (deployment, condition) is still detected and tracked (stays pending),
// but no notification is emitted while the mute is active. Once the mute expires,
// a later sweep fires it — the mute defers, it does not swallow.
func TestEvaluateMuteSuppressesThenFiresAfterExpiry(t *testing.T) {
	var emitted []notify.Event
	emit := func(_ context.Context, ev notify.Event) error { emitted = append(emitted, ev); return nil }
	st := newMemState()
	cond := Condition{Name: "crash_loop", Title: "Crash loop", Severity: SeverityCritical, Engine: EnginePromQL, Query: "q", For: 0}
	q := fakeEngine{vec: breaching("ns1")}
	e := NewEvaluator(nil, fakeDeploys{}, st, nil, emit, nil)
	t0 := time.Unix(1000, 0)

	// Mute the deployment+condition for 10 minutes.
	st.mutes["dep_ns1|crash_loop"] = t0.Add(10 * time.Minute)

	_ = e.evaluate(context.Background(), cond, q, t0)
	_ = e.evaluate(context.Background(), cond, q, t0.Add(5*time.Minute)) // still muted
	if len(emitted) != 0 {
		t.Fatalf("muted condition must not emit, got %d", len(emitted))
	}
	if tr, ok := st.m[mkey("dep_ns1", "", "crash_loop")]; !ok || tr.Notified {
		t.Fatalf("muted breach should stay tracked & pending (not notified), got %+v (present=%v)", tr, ok)
	}

	// Mute expires → next sweep fires (the breach is still active).
	_ = e.evaluate(context.Background(), cond, q, t0.Add(11*time.Minute))
	if len(emitted) != 1 {
		t.Fatalf("want 1 emit after the mute expires, got %d", len(emitted))
	}
}

// A disabled condition stays in the catalog (so stored rows still resolve a
// title) but is never evaluated or listed.
func TestActiveConditionsExcludesDisabled(t *testing.T) {
	active := ActiveConditions()
	if len(active) >= len(Conditions) {
		t.Fatalf("want fewer active than catalog conditions, got %d of %d", len(active), len(Conditions))
	}
	for _, c := range active {
		if c.Disabled {
			t.Fatalf("condition %q is disabled but active", c.Name)
		}
	}
	for _, name := range []string{"cpu_over_provisioned", "memory_over_provisioned"} {
		if !catalogHas(Conditions, name) {
			t.Fatalf("condition %q should remain in the catalog", name)
		}
		if catalogHas(active, name) {
			t.Fatalf("condition %q should not be active", name)
		}
	}
}

func catalogHas(conds []Condition, name string) bool {
	for _, c := range conds {
		if c.Name == name {
			return true
		}
	}
	return false
}

// Sweep routes each condition to its engine's querier and skips conditions whose
// engine is not wired. All active Conditions are PromQL, so wiring only a
// Langfuse engine must emit nothing.
func TestSweepSkipsConditionsWithUnwiredEngine(t *testing.T) {
	var emitted []notify.Event
	emit := func(_ context.Context, ev notify.Event) error { emitted = append(emitted, ev); return nil }
	engines := map[Engine]Querier{EngineLangfuse: fakeEngine{vec: breaching("ns1")}}
	e := NewEvaluator(engines, fakeDeploys{}, newMemState(), nil, emit, nil)

	if err := e.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(emitted) != 0 {
		t.Fatalf("PromQL conditions must be skipped when only Langfuse is wired, got %d", len(emitted))
	}
}
