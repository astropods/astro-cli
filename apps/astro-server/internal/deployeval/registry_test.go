package deployeval

import "testing"

// BuildAll must wire every registered evaluator (the tenant-router-ingress
// evaluator registers itself via init() in tenant_router_ingress.go) — this
// is the whole point of the registry: a new evaluator's own file is enough,
// no caller needs to also list it by hand.
func TestBuildAll_IncludesRegisteredEvaluators(t *testing.T) {
	evaluators := BuildAll(Deps{})

	found := false
	for _, ev := range evaluators {
		if ev.ID() == TenantRouterIngressEvaluatorID {
			found = true
		}
		if ev.ID() == "" {
			t.Errorf("evaluator %T has an empty ID", ev)
		}
		if ev.Name() == "" {
			t.Errorf("evaluator %q has an empty Name", ev.ID())
		}
	}
	if !found {
		t.Errorf("BuildAll() did not include the tenant-router-ingress evaluator; got %d evaluators", len(evaluators))
	}
}
