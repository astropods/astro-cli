package authz_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type checkerFunc func(context.Context, authz.Subject, authz.Action, authz.ResourceRef) (bool, error)

func (f checkerFunc) Authorize(ctx context.Context, subject authz.Subject, action authz.Action, resource authz.ResourceRef) (bool, error) {
	return f(ctx, subject, action, resource)
}

type decisionLog struct {
	debug []string
	info  []string
	warn  []string
}

func (l *decisionLog) Debug(message string, _ ...any) { l.debug = append(l.debug, message) }
func (l *decisionLog) Info(message string, _ ...any)  { l.info = append(l.info, message) }
func (l *decisionLog) Warn(message string, _ ...any)  { l.warn = append(l.warn, message) }

func TestShadowCheckerReturnsPrimaryAndLogsShadowOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		shadowAllowed bool
		shadowErr     error
		wantDebug     []string
		wantInfo      []string
		wantWarn      []string
	}{
		{
			name:          "matching decision",
			shadowAllowed: true,
			wantInfo:      []string{"shadow checker: FGA shadow decision matched"},
		},
		{
			name:     "mismatched decision",
			wantWarn: []string{"shadow checker: FGA shadow decision mismatch"},
		},
		{
			name:      "missing membership",
			shadowErr: authz.ErrWorkOSMembershipUnavailable,
			wantDebug: []string{"shadow checker: FGA shadow identity unavailable"},
		},
		{
			name:      "resource outside rollout",
			shadowErr: authz.ErrFGAResourceNotEnabled,
			wantDebug: []string{"shadow checker: FGA shadow check skipped"},
		},
		{
			name:      "live check failure",
			shadowErr: errors.New("workos unavailable"),
			wantWarn:  []string{"shadow checker: FGA shadow check failed"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			log := &decisionLog{}
			checker := authz.NewShadowChecker(
				log,
				checkerFunc(func(context.Context, authz.Subject, authz.Action, authz.ResourceRef) (bool, error) {
					return true, nil
				}),
				checkerFunc(func(context.Context, authz.Subject, authz.Action, authz.ResourceRef) (bool, error) {
					return test.shadowAllowed, test.shadowErr
				}),
			)

			allowed, err := checker.Authorize(context.Background(), authz.Subject{}, authz.ActionDeploymentRead, authz.DeploymentResource("dep_123"))
			if err != nil || !allowed {
				t.Fatalf("Authorize() = (%v, %v), want primary (true, nil)", allowed, err)
			}
			if !slices.Equal(log.debug, test.wantDebug) || !slices.Equal(log.info, test.wantInfo) || !slices.Equal(log.warn, test.wantWarn) {
				t.Fatalf("logs = debug:%v info:%v warn:%v", log.debug, log.info, log.warn)
			}
		})
	}
}

func TestShadowCheckerReturnsPrimaryErrorWithoutCallingShadow(t *testing.T) {
	t.Parallel()

	primaryErr := errors.New("membership lookup failed")
	shadowCalled := false
	checker := authz.NewShadowChecker(
		&decisionLog{},
		checkerFunc(func(context.Context, authz.Subject, authz.Action, authz.ResourceRef) (bool, error) {
			return false, primaryErr
		}),
		checkerFunc(func(context.Context, authz.Subject, authz.Action, authz.ResourceRef) (bool, error) {
			shadowCalled = true
			return true, nil
		}),
	)

	_, err := checker.Authorize(context.Background(), authz.Subject{}, authz.ActionDeploymentEdit, authz.DeploymentResource("dep_123"))
	if !errors.Is(err, primaryErr) || shadowCalled {
		t.Fatalf("Authorize() error = %v, shadowCalled=%v", err, shadowCalled)
	}
}

func TestGatedShadowCheckerSkipsComparisonOutsideRollout(t *testing.T) {
	t.Parallel()

	log := &decisionLog{}
	comparisonCalled := false
	comparison := checkerFunc(func(context.Context, authz.Subject, authz.Action, authz.ResourceRef) (bool, error) {
		comparisonCalled = true
		return true, nil
	})
	checker := authz.NewGatedShadowChecker(
		log,
		resourceGateFunc(func(context.Context, authz.ResourceRef) (bool, error) { return false, nil }),
		comparison,
		comparison,
	)

	allowed, err := checker.Authorize(context.Background(), authz.Subject{}, authz.ActionDeploymentRead, authz.DeploymentResource("dep_123"))
	if err != nil || allowed || comparisonCalled {
		t.Fatalf("Authorize() = (%v, %v), comparisonCalled=%v", allowed, err, comparisonCalled)
	}
	if !slices.Equal(log.debug, []string{"shadow checker: FGA shadow check skipped"}) {
		t.Fatalf("debug logs = %v", log.debug)
	}
}
