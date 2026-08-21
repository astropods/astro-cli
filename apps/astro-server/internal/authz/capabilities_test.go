package authz_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type effectivePermissionListerFunc func(context.Context, authz.Subject, authz.ResourceRef) ([]authz.Action, error)

func (f effectivePermissionListerFunc) EffectivePermissions(ctx context.Context, subject authz.Subject, resource authz.ResourceRef) ([]authz.Action, error) {
	return f(ctx, subject, resource)
}

type concurrentDecisionLog struct {
	mu       sync.Mutex
	debug    int
	info     int
	warns    int
	debugMsg string
	warnMsg  string
	warnArgs []any
}

func (l *concurrentDecisionLog) Debug(message string, _ ...any) {
	l.mu.Lock()
	l.debug++
	l.debugMsg = message
	l.mu.Unlock()
}
func (l *concurrentDecisionLog) Info(string, ...any) {
	l.mu.Lock()
	l.info++
	l.mu.Unlock()
}
func (l *concurrentDecisionLog) Warn(message string, args ...any) {
	l.mu.Lock()
	l.warns++
	l.warnMsg = message
	l.warnArgs = append([]any(nil), args...)
	l.mu.Unlock()
}

func (l *concurrentDecisionLog) warning() (string, []any, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.warnMsg, append([]any(nil), l.warnArgs...), l.warns
}

func TestCapabilityServiceEvaluatesCompleteCatalogFromOneEffectivePermissionList(t *testing.T) {
	t.Parallel()

	actions := authz.DeploymentActions()
	calls := 0
	permissions := effectivePermissionListerFunc(func(_ context.Context, subject authz.Subject, resource authz.ResourceRef) ([]authz.Action, error) {
		calls++
		if subject.MembershipID != "om_123" || resource != authz.DeploymentResource("dep_123") {
			t.Fatalf("unexpected effective-permissions request: subject=%+v resource=%+v", subject, resource)
		}
		return []authz.Action{authz.ActionDeploymentRead}, nil
	})
	log := &concurrentDecisionLog{}
	service := authz.NewCapabilityService(
		log,
		enabledResourceGate,
		checkerFunc(func(_ context.Context, _ authz.Subject, action authz.Action, _ authz.ResourceRef) (bool, error) {
			if action != authz.ActionDeploymentRead {
				t.Fatalf("visibility action = %q, want %q", action, authz.ActionDeploymentRead)
			}
			return true, nil
		}),
		permissions,
		authz.ActionDeploymentRead,
	)

	set, err := service.Evaluate(
		authz.WithAuthorizationRoute(context.Background(), "/api/v1/deployments/:id/capabilities"),
		authz.Subject{UserID: "user_123", MembershipID: "om_123"},
		authz.DeploymentResource("dep_123"),
		actions,
	)
	if err != nil {
		t.Fatal(err)
	}
	if set.Mode != authz.CapabilityModeFGA || len(set.Actions) != len(actions) {
		t.Fatalf("capability set = mode %q actions %d", set.Mode, len(set.Actions))
	}
	if !set.Actions[authz.ActionDeploymentRead] || set.Actions[authz.ActionDeploymentDelete] {
		t.Fatalf("unexpected decisions: %+v", set.Actions)
	}
	if calls != 1 {
		t.Fatalf("effective-permissions calls = %d, want 1", calls)
	}
	log.mu.Lock()
	debug, info, warns := log.debug, log.info, log.warns
	message := log.debugMsg
	log.mu.Unlock()
	if debug != len(actions) || info != 0 || warns != 0 || message != "capabilities: deployment capability comparison" {
		t.Fatalf("logged comparisons = debug:%d info:%d warn:%d message:%q", debug, info, warns, message)
	}
}

func TestCapabilityServiceReturnsExistingLegacyCapabilitiesOutsideExperiment(t *testing.T) {
	t.Parallel()

	primaryCalls := 0
	service := authz.NewCapabilityService(
		&concurrentDecisionLog{},
		resourceGateFunc(func(context.Context, authz.ResourceRef) (bool, error) { return false, nil }),
		checkerFunc(func(context.Context, authz.Subject, authz.Action, authz.ResourceRef) (bool, error) {
			primaryCalls++
			return true, nil
		}),
		effectivePermissionListerFunc(func(context.Context, authz.Subject, authz.ResourceRef) ([]authz.Action, error) {
			t.Fatal("FGA checker called outside rollout")
			return nil, nil
		}),
		authz.ActionDeploymentRead,
	)

	actions := authz.DeploymentActions()
	set, err := service.Evaluate(context.Background(), authz.Subject{}, authz.DeploymentResource("dep_123"), actions)
	if err != nil || set.Mode != authz.CapabilityModeLegacy || primaryCalls != 1 {
		t.Fatalf("Evaluate() = (%+v, %v), primary calls=%d", set, err, primaryCalls)
	}
	for _, action := range actions {
		want := action != authz.ActionDeploymentManageAccess
		if set.Actions[action] != want {
			t.Fatalf("legacy action %q = %v, want %v", action, set.Actions[action], want)
		}
	}
}

func TestCapabilityServiceHidesNonMemberAndFailsOnLiveError(t *testing.T) {
	t.Parallel()

	nonMember := authz.NewCapabilityService(
		&concurrentDecisionLog{}, enabledResourceGate,
		checkerFunc(func(context.Context, authz.Subject, authz.Action, authz.ResourceRef) (bool, error) { return false, nil }),
		effectivePermissionListerFunc(func(context.Context, authz.Subject, authz.ResourceRef) ([]authz.Action, error) {
			return []authz.Action{authz.ActionDeploymentRead}, nil
		}),
		authz.ActionDeploymentRead,
	)
	if _, err := nonMember.Evaluate(context.Background(), authz.Subject{}, authz.DeploymentResource("dep_123"), authz.DeploymentActions()); !errors.Is(err, authz.ErrResourceNotVisible) {
		t.Fatalf("non-member error = %v", err)
	}

	liveErr := errors.New("workos unavailable")
	failing := authz.NewCapabilityService(
		&concurrentDecisionLog{}, enabledResourceGate,
		checkerFunc(func(context.Context, authz.Subject, authz.Action, authz.ResourceRef) (bool, error) { return true, nil }),
		effectivePermissionListerFunc(func(context.Context, authz.Subject, authz.ResourceRef) ([]authz.Action, error) {
			return nil, liveErr
		}),
		authz.ActionDeploymentRead,
	)
	if _, err := failing.Evaluate(context.Background(), authz.Subject{MembershipID: "om_123"}, authz.DeploymentResource("dep_123"), authz.DeploymentActions()); !errors.Is(err, liveErr) {
		t.Fatalf("live error = %v", err)
	}
}
