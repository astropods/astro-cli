// Package observation is the in-process alerting evaluator for running-agent
// health and resource-budget conditions. A periodic sweep runs each Condition's
// PromQL against VictoriaMetrics, resolves each breaching pod to its deployment
// and workload, tracks per-(deployment, workload, condition) firing state in
// Postgres (the `for` sustained window + edge-only firing), and emits one
// notification per (deployment, condition) firing episode. Novu owns
// channels/preferences; this package owns detection + dedup.
package observation

import (
	"fmt"
	"math"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/notify"
)

// Engine names the query backend a Condition targets. Signals live behind
// different endpoints — resource/health metrics in VictoriaMetrics (PromQL),
// error rate / latency in Langfuse — so each condition declares which engine
// evaluates its Query. The evaluator routes to the matching Querier and skips
// conditions whose engine is not wired.
type Engine string

const (
	EnginePromQL   Engine = "promql"   // VictoriaMetrics / Prometheus-compatible instant query
	EngineLangfuse Engine = "langfuse" // Langfuse-sourced metrics; not wired yet
)

// Severity picks which of the three observation workflows a condition triggers:
// Info for a healthy agent wasting resources (over-provisioned), Warning for a
// degraded-but-running agent, Critical for a failing one. Every condition maps
// to one; the specific condition rides in the payload `reason`.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityCritical
)

// String is the wire/display name of a severity.
func (s Severity) String() string {
	switch s {
	case SeverityCritical:
		return "critical"
	case SeverityWarning:
		return "warning"
	default:
		return "info"
	}
}

// notifyType maps a severity to its Novu workflow / notification type.
func (s Severity) notifyType() notify.Type {
	switch s {
	case SeverityCritical:
		return notify.TypeObservationCritical
	case SeverityWarning:
		return notify.TypeObservationWarning
	default:
		return notify.TypeObservationInfo
	}
}

// Condition is one alertable resource/health rule. Name is the stable
// identifier used for firing-state and the per-episode dedupe key. Title is the
// human label the template renders (payload `reason`). Severity selects the
// warning/critical workflow. Engine selects the query backend; Query is that
// engine's expression whose returned series are the currently-breaching pods
// (each must carry `namespace` and `pod` labels); a pod resolves to a
// deployment + workload via the deployment store and runtime snapshot. For is
// the sustained window a pod must stay breaching before the alert fires
// (0 = fire on first detection — use when the query itself already spans a
// window, e.g. an increase() over 15m).
type Condition struct {
	Name        string
	Title       string
	Description string // one-line human summary of what the rule detects
	Severity    Severity
	Engine      Engine
	Query       string
	For         time.Duration
	// DetailsFor optionally renders the breaching series' scalar value into a
	// clause appended to Description, so the alert can quote concrete numbers
	// (e.g. observed utilization). Nil leaves Description unadorned; an empty
	// return is treated the same. value is the query result for the worst pod of
	// the workload.
	DetailsFor func(value float64) string
}

// targetUtilization is the request utilization the over-provisioned rules steer
// toward when suggesting a smaller request: run at ~50% of the request, leaving
// ~2x headroom. Used only to phrase the recommendation, not to fire the alert.
const targetUtilization = 0.5

// overProvisionedDetail turns an observed usage/request ratio into a concrete
// right-sizing suggestion. ratio is the fraction of the request that was used
// (0–1); outside that range it returns "" and the base description stands.
func overProvisionedDetail(ratio float64) string {
	if ratio <= 0 || ratio >= 1 {
		return ""
	}
	pct := int(math.Round(ratio * 100))
	// New request that would put observed peak at targetUtilization, as a
	// percentage of the current request. Clamped to [1, 99] so the suggestion
	// stays a strict reduction.
	suggested := int(math.Round(ratio / targetUtilization * 100))
	suggested = min(max(suggested, 1), 99)
	return fmt.Sprintf("At its busiest, the agent used only about %d%% of what you reserved over the last 6 hours — you could lower the reserved amount to around %d%% of what it is now.", pct, suggested)
}

// restartStormDetail turns the observed restart count (increase over the 5m
// window) into a concrete clause. count <= 0 falls back to the base description.
func restartStormDetail(count float64) string {
	n := int(math.Round(count))
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("Restarted about %d times in the last 5 minutes.", n)
}

// memoryOverBudgetDetail turns the observed working-set/limit ratio into a "% of
// limit" clause. ratio <= 0 falls back to the base description.
func memoryOverBudgetDetail(ratio float64) string {
	if ratio <= 0 {
		return ""
	}
	return fmt.Sprintf("Memory use reached about %d%% of the limit — consider raising the memory limit so the agent doesn't run out and restart.", int(math.Round(ratio*100)))
}

// computeThrottleDetail turns the observed throttled-periods fraction into a "%
// of periods" clause. ratio <= 0 falls back to the base description.
func computeThrottleDetail(ratio float64) string {
	if ratio <= 0 {
		return ""
	}
	return fmt.Sprintf("The agent hit its CPU limit about %d%% of the time, which slows it down — consider raising the CPU limit.", int(math.Round(ratio*100)))
}

// Conditions is the evaluated rule set. The PromQL below targets kube-state-
// metrics + cAdvisor series, aggregated `by (namespace, pod)`; the evaluator
// maps each breaching pod to its deployment + workload in code, so the queries
// carry no tenant-namespace assumptions. The exact metric/label names may need
// tuning against the deployed exporters.
//
// error_spike/latency (Langfuse-sourced) are intentionally absent — those
// metrics are not in VictoriaMetrics yet.
var Conditions = []Condition{
	{
		// Crash loop: a container sitting in CrashLoopBackOff (the kubelet has
		// given up fast-restarting it), sustained past a short grace window so a
		// single transient backoff doesn't alert.
		Name:        "crash_loop",
		Title:       "Crash loop",
		Description: "The agent keeps crashing and restarting, and can't stay running.",
		Severity:    SeverityCritical,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (kube_pod_container_status_waiting_reason{reason="CrashLoopBackOff"}) > 0`,
		For:         5 * time.Minute,
	},
	{
		// Out of memory: a container's last termination was an OOM kill — the
		// actionable "raise the memory budget" signal, distinct from a generic
		// crash loop. The gauge reflects the most recent termination, so edge-only
		// firing means one alert per episode until the container recovers.
		Name:        "oom_killed",
		Title:       "Out of memory",
		Description: "The agent ran out of memory and was stopped.",
		Severity:    SeverityCritical,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (kube_pod_container_status_last_terminated_reason{reason="OOMKilled"}) > 0`,
		For:         0,
	},
	{
		// Restart storm: an acute burst of restarts in a short window — sharper and
		// faster than a sustained CrashLoopBackOff, catches flapping that hasn't yet
		// tripped the kubelet's backoff.
		Name:        "restart_storm",
		Title:       "Restart storm",
		Description: "The agent restarted many times in a short period.",
		Severity:    SeverityWarning,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (increase(kube_pod_container_status_restarts_total[5m])) > 5`,
		For:         0,
		DetailsFor:  restartStormDetail,
	},
	{
		// Unschedulable: pods stuck Pending because the scheduler can't place them
		// (insufficient node capacity or quota), sustained past a grace window so a
		// brief scheduling gap during a rollout doesn't alert.
		Name:        "unschedulable",
		Title:       "Cannot schedule",
		Description: "The agent can't start — there isn't enough capacity available right now.",
		Severity:    SeverityCritical,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (kube_pod_status_unschedulable) > 0`,
		For:         10 * time.Minute,
	},
	{
		// Memory over budget: working set sustained above 90% of the limit.
		Name:        "memory_over_budget",
		Title:       "Memory over budget",
		Description: "The agent used almost all of its memory.",
		Severity:    SeverityWarning,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (container_memory_working_set_bytes / on (namespace, pod, container) group_left kube_pod_container_resource_limits{resource="memory"}) > 0.9`,
		For:         10 * time.Minute,
		DetailsFor:  memoryOverBudgetDetail,
	},
	{
		// Compute over budget: CPU CFS throttled a majority of periods.
		Name:        "compute_over_budget",
		Title:       "Compute over budget",
		Description: "The agent kept hitting its CPU limit most of the time.",
		Severity:    SeverityWarning,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (rate(container_cpu_cfs_throttled_periods_total[10m]) / rate(container_cpu_cfs_periods_total[10m])) > 0.5`,
		For:         10 * time.Minute,
		DetailsFor:  computeThrottleDetail,
	},
	{
		// CPU over-provisioned: even the P95 peak CPU use stays far below the
		// request over a long window — the reservation is wasted, right-size it
		// down. Using a P95 over 5m rate samples (not a 1h average) so bursty
		// agents aren't flagged for headroom they actually need at peak. Only
		// pods with a CPU request are evaluated (the join drops the rest).
		Name:        "cpu_over_provisioned",
		Title:       "CPU over-provisioned",
		Description: "The agent used far less CPU than you reserved for it.",
		Severity:    SeverityInfo,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (quantile_over_time(0.95, rate(container_cpu_usage_seconds_total[5m])[1h:5m]) / on (namespace, pod, container) group_left kube_pod_container_resource_requests{resource="cpu"}) < 0.4`,
		For:         6 * time.Hour,
		DetailsFor:  overProvisionedDetail,
	},
	{
		// Memory over-provisioned: working set stays far below the request over a
		// long window — the reservation is wasted, right-size it down.
		Name:        "memory_over_provisioned",
		Title:       "Memory over-provisioned",
		Description: "The agent used far less memory than you reserved for it.",
		Severity:    SeverityInfo,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (container_memory_working_set_bytes / on (namespace, pod, container) group_left kube_pod_container_resource_requests{resource="memory"}) < 0.5`,
		For:         6 * time.Hour,
		DetailsFor:  overProvisionedDetail,
	},
}
