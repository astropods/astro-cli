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
	// Guidance is the fix, as one imperative sentence appended to the alert's
	// details. It is separate from Description because the alert catalog serves
	// Description for conditions that are not firing, where advice reads as a
	// false alarm. Empty when the fix depends on the observed value, in which
	// case DetailsFor carries it.
	Guidance string
	Severity Severity
	Engine   Engine
	Query    string
	For      time.Duration
	// DetailsFor optionally renders the breaching series' scalar value into a
	// sentence appended to Description, so the alert can quote concrete numbers
	// (e.g. observed utilization). Nil leaves Description unadorned; an empty
	// return is treated the same. value is the query result for the worst pod of
	// the workload.
	DetailsFor func(value float64) string
}

// overProvisionedDetail builds the detail formatter for an over-provisioned
// resource ("CPU" or "memory"), which quotes the observed peak as a share of the
// reservation. It deliberately stops short of naming a smaller reservation: the
// alert knows the ratio, not the configured value, so any target it named would
// leave the reader to multiply. ratio is the fraction of the reservation that was
// used (0 to 1); outside that range it returns "" and the base description
// stands.
func overProvisionedDetail(resource string) func(float64) string {
	return func(ratio float64) string {
		if ratio <= 0 || ratio >= 1 {
			return ""
		}
		return fmt.Sprintf("At its busiest it used %d%% of the reserved %s.", int(math.Round(ratio*100)), resource)
	}
}

// restartStormDetail quotes the observed restart count (increase over the 5m
// window). count <= 0 falls back to the base description.
func restartStormDetail(count float64) string {
	n := int(math.Round(count))
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("It restarted %d times in the last 5 minutes.", n)
}

// memoryOverBudgetDetail quotes the observed working-set/limit ratio.
// ratio <= 0 falls back to the base description.
func memoryOverBudgetDetail(ratio float64) string {
	if ratio <= 0 {
		return ""
	}
	return fmt.Sprintf("Memory use peaked at %d%% of the limit.", int(math.Round(ratio*100)))
}

// computeThrottleDetail quotes the observed throttled-periods fraction.
// ratio <= 0 falls back to the base description.
func computeThrottleDetail(ratio float64) string {
	if ratio <= 0 {
		return ""
	}
	return fmt.Sprintf("It spent %d%% of the time waiting for CPU.", int(math.Round(ratio*100)))
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
		Description: "The agent crashes every time it starts, so it can't serve requests.",
		Guidance:    "Check the agent's logs for the error that prevents it from starting.",
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
		Description: "The agent used more memory than its limit allows, so it stopped.",
		Guidance:    "Raise the memory limit to keep it running.",
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
		Title:       "Frequent restarts",
		Description: "The agent keeps restarting, which interrupts any request it is handling.",
		Guidance:    "Check the agent's logs for the cause.",
		Severity:    SeverityWarning,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (increase(kube_pod_container_status_restarts_total[5m])) > 5`,
		For:         0,
		DetailsFor:  restartStormDetail,
	},
	{
		// Unschedulable: pods stuck Pending because the scheduler can't place them,
		// sustained past a grace window so a brief scheduling gap during a rollout
		// doesn't alert. The gauge does not say why, and the causes run from node
		// capacity and quota to taints, affinity, and topology constraints, so the
		// copy names the symptom rather than picking one.
		Name:        "unschedulable",
		Title:       "Can't schedule",
		Description: "The agent can't start because Astro has nowhere to run it right now.",
		Guidance:    "Check the deployment's events to see what is blocking it.",
		Severity:    SeverityCritical,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (kube_pod_status_unschedulable) > 0`,
		For:         10 * time.Minute,
	},
	{
		// Memory over budget: working set sustained above 90% of the limit.
		Name:        "memory_over_budget",
		Title:       "Near memory limit",
		Description: "The agent is close to its memory limit, which will stop it if it goes over.",
		Guidance:    "Raise the memory limit to give it room.",
		Severity:    SeverityWarning,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (container_memory_working_set_bytes / on (namespace, pod, container) group_left kube_pod_container_resource_limits{resource="memory"}) > 0.9`,
		For:         10 * time.Minute,
		DetailsFor:  memoryOverBudgetDetail,
	},
	{
		// Compute over budget: CPU CFS throttled a majority of periods.
		Name:        "compute_over_budget",
		Title:       "Slowed by CPU limit",
		Description: "The agent keeps hitting its CPU limit, which slows it down.",
		Guidance:    "Raise the CPU limit to speed it up.",
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
		Title:       "Unused CPU",
		Description: "The agent uses far less CPU than you reserved for it.",
		Guidance:    "You can lower the reserved CPU to cut cost.",
		Severity:    SeverityInfo,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (quantile_over_time(0.95, rate(container_cpu_usage_seconds_total[5m])[1h:5m]) / on (namespace, pod, container) group_left kube_pod_container_resource_requests{resource="cpu"}) < 0.4`,
		For:         6 * time.Hour,
		DetailsFor:  overProvisionedDetail("CPU"),
	},
	{
		// Memory over-provisioned: working set stays far below the request over a
		// long window — the reservation is wasted, right-size it down.
		Name:        "memory_over_provisioned",
		Title:       "Unused memory",
		Description: "The agent uses far less memory than you reserved for it.",
		Guidance:    "You can lower the reserved memory to cut cost.",
		Severity:    SeverityInfo,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (container_memory_working_set_bytes / on (namespace, pod, container) group_left kube_pod_container_resource_requests{resource="memory"}) < 0.5`,
		For:         6 * time.Hour,
		DetailsFor:  overProvisionedDetail("memory"),
	},
}
