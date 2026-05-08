package handlers

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/authorizationstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// validateAuthorizationSpec — test cases C1..C10.

// C4: a grant with no subject set must be rejected.
func TestValidateAuth_NoSubject(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{{}},
		},
	})
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

// C1: org + user_id together → reject.
func TestValidateAuth_AccountAndUser(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{Org: "acct-1", UserID: "alice"},
			},
		},
	})
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

// C2: org + anyone together → reject.
func TestValidateAuth_AccountAndAnyone(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{Org: "acct-1", Anyone: true},
			},
		},
	})
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

// C3: user_id + anyone → reject.
func TestValidateAuth_UserAndAnyone(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{UserID: "alice", Anyone: true},
			},
		},
	})
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

// C5 (lifted): user_id under slack.grants is now accepted. The messaging
// container forwards (team_id, slack_user_id), the resolver looks up the
// linked WorkOS user, and the user grant matches via the same path as web.
func TestValidateAuth_UserOnSlack(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Slack: &spec.DeploymentSlackAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{UserID: "alice"},
			},
		},
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

// C6: anyone under slack.grants → accepted (collapses to org:<owner>
// for slack since the bot is per-account; this is the seeded fresh-deploy
// default for slack-enabled deployments).
func TestValidateAuth_AnyoneOnSlack(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Slack: &spec.DeploymentSlackAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{Anyone: true},
			},
		},
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

// C8: duplicate grant within an adapter → reject.
func TestValidateAuth_Duplicate(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{Org: "acct-1"},
				{Org: "acct-1"},
			},
		},
	})
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

// All valid forms succeed.
func TestValidateAuth_ValidMix(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{Org: "acct-1"},
				{UserID: "alice"},
				{Anyone: true},
			},
		},
		Slack: &spec.DeploymentSlackAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{Org: "acct-1"},
			},
		},
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

// C11: nil block (auth omitted) → no errors.
func TestValidateAuth_NilBlock(t *testing.T) {
	errs := validateAuthorizationSpec(nil)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

// C12: empty grants → no validation errors (semantics handled by apply path).
func TestValidateAuth_EmptyGrants(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{Grants: []spec.DeploymentAuthorizationGrant{}},
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

// E1: fresh deploy with slack disabled seeds only the deployer's user grant.
func TestSeedFreshAuthGrants_WebOnly(t *testing.T) {
	tmpl := &spec.AstroDeploymentSpec{
		Interfaces: &spec.DeploymentInterfaces{Adapters: []string{"web"}},
	}
	seedFreshAuthGrants(tmpl, "alice")
	if tmpl.Interfaces.Auth == nil || tmpl.Interfaces.Auth.Web == nil {
		t.Fatal("expected auth.web block populated")
	}
	if len(tmpl.Interfaces.Auth.Web.Grants) != 1 {
		t.Fatalf("expected 1 web grant, got %d", len(tmpl.Interfaces.Auth.Web.Grants))
	}
	if g := tmpl.Interfaces.Auth.Web.Grants[0]; g.UserID != "alice" {
		t.Errorf("unexpected grant: %+v", g)
	}
	if tmpl.Interfaces.Auth.Slack != nil {
		t.Errorf("expected no slack block, got %+v", tmpl.Interfaces.Auth.Slack)
	}
}

// E1: with slack enabled, an `anyone` grant is seeded under slack so the
// channel is reachable out of the box.
func TestSeedFreshAuthGrants_SlackEnabled(t *testing.T) {
	tmpl := &spec.AstroDeploymentSpec{
		Interfaces: &spec.DeploymentInterfaces{Adapters: []string{"web", "slack"}},
	}
	seedFreshAuthGrants(tmpl, "alice")
	if tmpl.Interfaces.Auth.Web == nil || len(tmpl.Interfaces.Auth.Web.Grants) != 1 {
		t.Fatalf("expected one web grant, got %+v", tmpl.Interfaces.Auth.Web)
	}
	if g := tmpl.Interfaces.Auth.Web.Grants[0]; g.UserID != "alice" {
		t.Errorf("unexpected web grant: %+v", g)
	}
	if tmpl.Interfaces.Auth.Slack == nil || len(tmpl.Interfaces.Auth.Slack.Grants) != 1 {
		t.Fatalf("expected one slack grant, got %+v", tmpl.Interfaces.Auth.Slack)
	}
	if g := tmpl.Interfaces.Auth.Slack.Grants[0]; !g.Anyone {
		t.Errorf("unexpected slack grant: %+v", g)
	}
}

// E2: existing grants on the template (from astropods.yml) are not overwritten.
func TestSeedFreshAuthGrants_PreservesExisting(t *testing.T) {
	tmpl := &spec.AstroDeploymentSpec{
		Interfaces: &spec.DeploymentInterfaces{
			Adapters: []string{"web"},
			Auth: &spec.DeploymentInterfacesAuth{
				Web: &spec.DeploymentWebAuth{
					Grants: []spec.DeploymentAuthorizationGrant{{Anyone: true}},
				},
			},
		},
	}
	seedFreshAuthGrants(tmpl, "alice")
	if len(tmpl.Interfaces.Auth.Web.Grants) != 1 || !tmpl.Interfaces.Auth.Web.Grants[0].Anyone {
		t.Errorf("expected existing anyone grant preserved, got %+v", tmpl.Interfaces.Auth.Web.Grants)
	}
}

// Robust to nil Interfaces — no panic.
func TestSeedFreshAuthGrants_NilInterfaces(t *testing.T) {
	tmpl := &spec.AstroDeploymentSpec{}
	seedFreshAuthGrants(tmpl, "alice")
	if tmpl.Interfaces != nil {
		t.Errorf("did not expect Interfaces to be created")
	}
}

// Deploy-time invariant: when the submitted spec has slack in adapters but
// no slack grants are mentioned, auto-seed an `anyone` grant so the bot is
// reachable. Complements seedFreshAuthGrants (template path): the deploy
// path can also bypass the template (CLI direct deploy, or a template where
// web grants were set but slack grants weren't), and a slack-enabled
// deployment with no slack grants would otherwise sign a token with empty
// anyone_adapters and leave the channel unreachable.
func TestEnsureSlackAnyoneGrant_NoAuthBlock(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Interfaces: &spec.DeploymentInterfaces{Adapters: []string{"web", "slack"}},
	}
	ensureSlackAnyoneGrant(ds)
	if ds.Interfaces.Auth == nil || ds.Interfaces.Auth.Slack == nil {
		t.Fatal("expected slack auth block populated")
	}
	if len(ds.Interfaces.Auth.Slack.Grants) != 1 || !ds.Interfaces.Auth.Slack.Grants[0].Anyone {
		t.Fatalf("expected one slack anyone grant, got %+v", ds.Interfaces.Auth.Slack.Grants)
	}
}

// Web grants present but slack grants missing — the existing seed bails out
// in this case, so the deploy-time invariant must seed slack independently.
func TestEnsureSlackAnyoneGrant_WebGrantsOnly(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Interfaces: &spec.DeploymentInterfaces{
			Adapters: []string{"web", "slack"},
			Auth: &spec.DeploymentInterfacesAuth{
				Web: &spec.DeploymentWebAuth{
					Grants: []spec.DeploymentAuthorizationGrant{{UserID: "alice"}},
				},
			},
		},
	}
	ensureSlackAnyoneGrant(ds)
	if ds.Interfaces.Auth.Slack == nil || len(ds.Interfaces.Auth.Slack.Grants) != 1 {
		t.Fatalf("expected one slack grant, got %+v", ds.Interfaces.Auth.Slack)
	}
	if !ds.Interfaces.Auth.Slack.Grants[0].Anyone {
		t.Errorf("expected anyone grant, got %+v", ds.Interfaces.Auth.Slack.Grants[0])
	}
	// Web grants must not be touched.
	if len(ds.Interfaces.Auth.Web.Grants) != 1 || ds.Interfaces.Auth.Web.Grants[0].UserID != "alice" {
		t.Errorf("web grants modified: %+v", ds.Interfaces.Auth.Web.Grants)
	}
}

// Existing slack grants must be preserved — do not overwrite a user's
// org-scoped slack grant with anyone.
func TestEnsureSlackAnyoneGrant_PreservesExistingSlackGrants(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Interfaces: &spec.DeploymentInterfaces{
			Adapters: []string{"slack"},
			Auth: &spec.DeploymentInterfacesAuth{
				Slack: &spec.DeploymentSlackAuth{
					Grants: []spec.DeploymentAuthorizationGrant{{Org: "acct-1"}},
				},
			},
		},
	}
	ensureSlackAnyoneGrant(ds)
	if len(ds.Interfaces.Auth.Slack.Grants) != 1 {
		t.Fatalf("expected one slack grant, got %+v", ds.Interfaces.Auth.Slack.Grants)
	}
	if ds.Interfaces.Auth.Slack.Grants[0].Org != "acct-1" {
		t.Errorf("expected existing org grant preserved, got %+v", ds.Interfaces.Auth.Slack.Grants[0])
	}
}

// Slack adapter not selected → no slack block created.
func TestEnsureSlackAnyoneGrant_NoSlackAdapter(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Interfaces: &spec.DeploymentInterfaces{Adapters: []string{"web"}},
	}
	ensureSlackAnyoneGrant(ds)
	if ds.Interfaces.Auth != nil && ds.Interfaces.Auth.Slack != nil {
		t.Errorf("expected no slack block, got %+v", ds.Interfaces.Auth.Slack)
	}
}

// Robust to nil Interfaces — no panic, no allocation.
func TestEnsureSlackAnyoneGrant_NilInterfaces(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{}
	ensureSlackAnyoneGrant(ds)
	if ds.Interfaces != nil {
		t.Errorf("did not expect Interfaces to be created")
	}
}

// E5/E6: see internal/authorizationstore/store_test.go
// (TestReplaceGrantsTx_RunsOnCallerTransaction,
// TestReplaceGrantsTx_InsertFails_CallerRollback) for the
// replace-on-an-existing-tx semantics. The handler-level wrapper is now
// `buildAuthorizationGrants` + `ReplaceGrantsTx` called directly from
// `txFn` in DeployAgent — no separate handler entry point to test.

// E4: prefill reads grants from the live table and translates back to spec form.
func TestMergeAuthorizationFromStore(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	defer db.Close()
	store := authorizationstore.NewStore(db)
	log := logger.New("error", "text")

	mock.ExpectQuery("\n\t\tSELECT deployment_id, subject_type, subject_id, adapter\n\t\tFROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1\n\t\tORDER BY subject_type, subject_id, adapter\n\t").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "subject_type", "subject_id", "adapter"}).
			AddRow("dep-1", "org", "acct-1", "slack").
			AddRow("dep-1", "anyone", "", "web").
			AddRow("dep-1", "user", "alice", "web"))

	auth := &spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{{Org: "stale"}},
		},
	}
	mergeAuthorizationFromStore(log, store, "dep-1", auth)

	// Stored order is (subject_type, subject_id, adapter):
	// org/acct-1/slack → goes under Slack;
	// anyone//web and user/alice/web → go under Web.
	if auth.Slack == nil || len(auth.Slack.Grants) != 1 || auth.Slack.Grants[0].Org != "acct-1" {
		t.Fatalf("expected one slack org grant, got %+v", auth.Slack)
	}
	if auth.Web == nil || len(auth.Web.Grants) != 2 {
		t.Fatalf("expected two web grants, got %+v", auth.Web)
	}
	if !auth.Web.Grants[0].Anyone {
		t.Errorf("web grant[0]: %+v", auth.Web.Grants[0])
	}
	if auth.Web.Grants[1].UserID != "alice" {
		t.Errorf("web grant[1]: %+v", auth.Web.Grants[1])
	}
}

// specGrantToStore translates the three subject forms correctly.
func TestSpecGrantToStore(t *testing.T) {
	cases := []struct {
		name    string
		in      spec.DeploymentAuthorizationGrant
		adapter string
		want    struct {
			subjectType, subjectID string
		}
	}{
		{
			name:    "org",
			in:      spec.DeploymentAuthorizationGrant{Org: "acct-1"},
			adapter: "web",
			want:    struct{ subjectType, subjectID string }{"org", "acct-1"},
		},
		{
			name:    "user",
			in:      spec.DeploymentAuthorizationGrant{UserID: "alice"},
			adapter: "web",
			want:    struct{ subjectType, subjectID string }{"user", "alice"},
		},
		{
			name:    "anyone",
			in:      spec.DeploymentAuthorizationGrant{Anyone: true},
			adapter: "web",
			want:    struct{ subjectType, subjectID string }{"anyone", ""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := specGrantToStore("dep-1", tc.in, tc.adapter)
			if got.SubjectType != tc.want.subjectType || got.SubjectID != tc.want.subjectID {
				t.Errorf("got %+v, want subject_type=%s subject_id=%s", got, tc.want.subjectType, tc.want.subjectID)
			}
			if got.DeploymentID != "dep-1" {
				t.Errorf("DeploymentID lost: %q", got.DeploymentID)
			}
			if got.Adapter != tc.adapter {
				t.Errorf("Adapter lost: %q", got.Adapter)
			}
		})
	}
}
