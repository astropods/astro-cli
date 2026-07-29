// Package observation is the in-process alerting evaluator for running-agent
// health and resource-budget conditions. A periodic sweep runs each Condition's
// PromQL against VictoriaMetrics, resolves each breaching pod to its deployment
// and workload, tracks per-(deployment, workload, condition) firing state in
// Postgres (the `for` sustained window + edge-only firing), and emits one
// notification per (deployment, condition) firing episode. Novu owns
// channels/preferences; this package owns detection + dedup.
package observation

import (
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

// Severity picks which of the two observation workflows a condition triggers:
// Warning for a degraded-but-running agent, Critical for a failing one. Every
// condition maps to one; the specific condition rides in the payload `reason`.
type Severity int

const (
	SeverityWarning Severity = iota
	SeverityCritical
)

// String is the wire/display name of a severity.
func (s Severity) String() string {
	if s == SeverityCritical {
		return "critical"
	}
	return "warning"
}

// notifyType maps a severity to its Novu workflow / notification type.
func (s Severity) notifyType() notify.Type {
	if s == SeverityCritical {
		return notify.TypeObservationCritical
	}
	return notify.TypeObservationWarning
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
		Description: "A container is stuck restarting in CrashLoopBackOff.",
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
		Description: "A container was killed for exceeding its memory limit.",
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
		Description: "A container restarted many times in a short window.",
		Severity:    SeverityWarning,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (increase(kube_pod_container_status_restarts_total[5m])) > 5`,
		For:         0,
	},
	{
		// Unschedulable: pods stuck Pending because the scheduler can't place them
		// (insufficient node capacity or quota), sustained past a grace window so a
		// brief scheduling gap during a rollout doesn't alert.
		Name:        "unschedulable",
		Title:       "Cannot schedule",
		Description: "Pods can't be scheduled — insufficient capacity or quota.",
		Severity:    SeverityCritical,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (kube_pod_status_unschedulable) > 0`,
		For:         10 * time.Minute,
	},
	{
		// Memory over budget: working set sustained above 90% of the limit.
		Name:        "memory_over_budget",
		Title:       "Memory over budget",
		Description: "Memory use stayed above 90% of its limit.",
		Severity:    SeverityWarning,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (container_memory_working_set_bytes / on (namespace, pod, container) group_left kube_pod_container_resource_limits{resource="memory"}) > 0.9`,
		For:         10 * time.Minute,
	},
	{
		// Compute over budget: CPU CFS throttled a majority of periods.
		Name:        "compute_over_budget",
		Title:       "Compute over budget",
		Description: "CPU was throttled at its limit most of the time.",
		Severity:    SeverityWarning,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (rate(container_cpu_cfs_throttled_periods_total[10m]) / rate(container_cpu_cfs_periods_total[10m])) > 0.5`,
		For:         10 * time.Minute,
	},
	{
		// CPU over-provisioned: even peak CPU use stays far below the request over
		// a long window — the reservation is wasted, right-size it down. Only pods
		// with a CPU request are evaluated (the join drops the rest).
		Name:        "cpu_over_provisioned",
		Title:       "CPU over-provisioned",
		Description: "CPU usage stayed far below its request — consider lowering it.",
		Severity:    SeverityWarning,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (rate(container_cpu_usage_seconds_total[1h]) / on (namespace, pod, container) group_left kube_pod_container_resource_requests{resource="cpu"}) < 0.1`,
		For:         6 * time.Hour,
	},
	{
		// Memory over-provisioned: working set stays far below the request over a
		// long window — the reservation is wasted, right-size it down.
		Name:        "memory_over_provisioned",
		Title:       "Memory over-provisioned",
		Description: "Memory usage stayed far below its request — consider lowering it.",
		Severity:    SeverityWarning,
		Engine:      EnginePromQL,
		Query:       `max by (namespace, pod) (container_memory_working_set_bytes / on (namespace, pod, container) group_left kube_pod_container_resource_requests{resource="memory"}) < 0.4`,
		For:         6 * time.Hour,
	},
}
