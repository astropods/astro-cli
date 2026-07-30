package observation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
)

// Series is one result vector element from any query engine: a set of labels
// (must include `namespace` and `pod`) and a scalar value. It is engine-neutral
// so PromQL and other backends (e.g. Langfuse) share the evaluator's diff logic.
type Series struct {
	Labels map[string]string
	Value  float64
}

// Querier runs a condition's query against one backend and returns the currently
// breaching series. Each engine (PromQL over VictoriaMetrics, Langfuse metrics,
// …) implements it; a Condition names which engine evaluates it via Engine.
type Querier interface {
	Query(ctx context.Context, query string) ([]Series, error)
}

// deployments resolves a namespace to its deployment and loads a deployment's
// runtime snapshot (to map a breaching pod to its workload).
// *deploymentstore.Store satisfies it.
type deployments interface {
	GetLatestDeploymentByNamespace(namespace string) (*deploymentstore.Deployment, error)
	GetRuntimeSnapshot(deploymentID string) (*deploymentstore.RuntimeSnapshot, time.Time, error)
}

// accountNamer resolves an account id to its URL handle (Name), used to
// deep-link an alert to the affected deployment. *account.AccountStore satisfies
// it. Optional: a nil namer just yields an alert without an account-scoped CTA.
type accountNamer interface {
	GetByID(id string) (*account.Account, error)
}

// stateStore is the firing-state persistence the evaluator needs, keyed per
// (deployment, workload, condition). *Store satisfies it.
type stateStore interface {
	ForCondition(ctx context.Context, condition string) ([]Tracked, error)
	StartTracking(ctx context.Context, deploymentID, workload, condition string, since time.Time, notified bool) error
	MarkNotified(ctx context.Context, deploymentID, workload, condition string) error
	Clear(ctx context.Context, deploymentID, workload, condition string) error
}

// Evaluator runs the condition set against per-engine queriers and emits alerts
// on firing edges, tracking state in the Store so it never double-alerts.
type Evaluator struct {
	engines  map[Engine]Querier
	deploys  deployments
	state    stateStore
	accounts accountNamer
	emit     func(ctx context.Context, ev notify.Event) error
	log      *logger.Logger
}

func NewEvaluator(engines map[Engine]Querier, deploys deployments, state stateStore, accounts accountNamer, emit func(context.Context, notify.Event) error, log *logger.Logger) *Evaluator {
	return &Evaluator{engines: engines, deploys: deploys, state: state, accounts: accounts, emit: emit, log: log}
}

// entKey is the per-(deployment, workload) tracking key.
func entKey(deploymentID, workload string) string { return deploymentID + "\x00" + workload }

// Sweep evaluates every condition once, routing each to the querier for its
// engine. A condition whose engine is not wired (e.g. Langfuse before it is
// configured) is skipped. A per-condition failure is logged and the sweep
// continues; it returns nil so one flaky query doesn't fail the job.
func (e *Evaluator) Sweep(ctx context.Context) error {
	if len(e.engines) == 0 {
		return nil
	}
	now := time.Now()
	for _, c := range Conditions {
		q, ok := e.engines[c.Engine]
		if !ok {
			continue
		}
		if err := e.evaluate(ctx, c, q, now); err != nil && e.log != nil {
			e.log.Error("observation: condition eval failed", "condition", c.Name, "error", err)
		}
	}
	return nil
}

// breach is a currently-breaching workload of a deployment.
type breach struct {
	dep      *deploymentstore.Deployment
	workload string
}

func (e *Evaluator) evaluate(ctx context.Context, c Condition, q Querier, now time.Time) error {
	samples, err := q.Query(ctx, c.Query)
	if err != nil {
		return err
	}

	// Resolve each breaching (namespace, pod) series to a (deployment, workload).
	// A pod that maps to no deployment/workload is skipped. Firing state is keyed
	// per workload so the UI can attribute the alert; notifications are still
	// deduped to one per (deployment, condition) episode below.
	breaching := make(map[string]breach, len(samples))
	depCache := map[string]*deploymentstore.Deployment{}
	podMaps := map[string]podWorkloadMap{}
	for _, s := range samples {
		ns := s.Labels["namespace"]
		if ns == "" {
			continue
		}
		dep, seen := depCache[ns]
		if !seen {
			d, derr := e.deploys.GetLatestDeploymentByNamespace(ns)
			if derr != nil || d == nil {
				if e.log != nil {
					e.log.Warn("observation: no deployment for namespace, skipping",
						"condition", c.Name, "namespace", ns, "error", derr)
				}
				depCache[ns] = nil
				continue
			}
			dep = d
			depCache[ns] = d
		}
		if dep == nil {
			continue
		}
		pm, ok := podMaps[dep.ID]
		if !ok {
			pm = e.podWorkloads(dep.ID)
			podMaps[dep.ID] = pm
		}
		workload := pm.workloadFor(s.Labels["pod"])
		breaching[entKey(dep.ID, workload)] = breach{dep: dep, workload: workload}
	}

	tracked, err := e.state.ForCondition(ctx, c.Name)
	if err != nil {
		return err
	}
	trackedMap := make(map[string]Tracked, len(tracked))
	// A deployment is considered already-notified for this condition/episode if
	// any of its workloads has a notified row — this is what keeps mail to one
	// per (deployment, condition) even when several workloads trip it.
	notifiedDep := map[string]bool{}
	for _, t := range tracked {
		trackedMap[entKey(t.DeploymentID, t.Workload)] = t
		if t.Notified {
			notifiedDep[t.DeploymentID] = true
		}
	}

	// Firing edges.
	for key, b := range breaching {
		tr, existed := trackedMap[key]
		activeSince := now
		if existed {
			activeSince = tr.ActiveSince
		} else if err := e.state.StartTracking(ctx, b.dep.ID, b.workload, c.Name, now, false); err != nil {
			return err
		}
		if existed && tr.Notified {
			continue // this workload already fired this episode
		}
		if now.Sub(activeSince) >= c.For {
			if err := e.state.MarkNotified(ctx, b.dep.ID, b.workload, c.Name); err != nil {
				return err
			}
			if !notifiedDep[b.dep.ID] {
				e.fire(ctx, c, b.dep, b.workload, activeSince)
				notifiedDep[b.dep.ID] = true
			}
		}
	}

	// Resolve edges: tracked but no longer breaching → clear (silent resolve).
	for key, tr := range trackedMap {
		if _, ok := breaching[key]; !ok {
			if err := e.state.Clear(ctx, tr.DeploymentID, tr.Workload, c.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// fire emits the alert for a breaching workload. Best-effort: an emit error is
// logged, never fatal.
func (e *Evaluator) fire(ctx context.Context, c Condition, dep *deploymentstore.Deployment, workload string, since time.Time) {
	reason := c.Title
	if workload != "" {
		reason = fmt.Sprintf("%s — %s", c.Title, workload)
	}
	accountName := ""
	if e.accounts != nil {
		if acct, err := e.accounts.GetByID(dep.AccountID); err == nil && acct != nil {
			accountName = acct.Name
		} else if err != nil && e.log != nil {
			e.log.Warn("observation: account name lookup failed, alert CTA will be dropped",
				"error", err, "account_id", dep.AccountID, "deployment", dep.ID)
		}
	}
	ev := notify.Observation(c.Severity.notifyType(), dep.AccountID, accountName, dep.AgentName, dep.ID, reason, c.Description)
	// Per-episode dedupe keyed by condition + deployment + workload + window start.
	ev.DedupeKey = fmt.Sprintf("%s:%s:%s:%d", c.Name, dep.ID, workload, since.Unix())
	if err := e.emit(ctx, ev); err != nil && e.log != nil {
		e.log.Warn("observation: emit alert failed", "condition", c.Name, "deployment", dep.ID, "workload", workload, "error", err)
	}
}

// podWorkloadMap maps a pod name to its workload id (the workload's component,
// falling back to its object name) using a deployment's runtime snapshot.
type podWorkloadMap struct {
	exact    map[string]string // pod name -> workload id
	prefixes []workloadPrefix  // fallback for pods not in the snapshot's pod list
}

type workloadPrefix struct {
	prefix   string // "<workload object name>-"
	workload string
}

func (m podWorkloadMap) workloadFor(pod string) string {
	if pod == "" {
		return ""
	}
	if w, ok := m.exact[pod]; ok {
		return w
	}
	for _, p := range m.prefixes {
		if strings.HasPrefix(pod, p.prefix) {
			return p.workload
		}
	}
	return ""
}

// podWorkloads builds a pod→workload resolver from a deployment's runtime
// snapshot. Returns an empty map (everything resolves to "") if the snapshot is
// unavailable — the alert still fires, just without a workload attribution.
func (e *Evaluator) podWorkloads(deploymentID string) podWorkloadMap {
	m := podWorkloadMap{exact: map[string]string{}}
	snap, _, err := e.deploys.GetRuntimeSnapshot(deploymentID)
	if err != nil || snap == nil {
		return m
	}
	for _, w := range snap.Workloads {
		id := w.Component
		if id == "" {
			id = w.Name
		}
		for _, p := range w.Pods {
			if p.Name != "" {
				m.exact[p.Name] = id
			}
		}
		if w.Name != "" {
			m.prefixes = append(m.prefixes, workloadPrefix{prefix: w.Name + "-", workload: id})
		}
	}
	return m
}
