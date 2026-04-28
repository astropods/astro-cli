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

// C1: account_id + user_id together → reject.
func TestValidateAuth_AccountAndUser(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{AccountID: "acct-1", UserID: "alice"},
			},
		},
	})
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

// C2: account_id + anyone together → reject.
func TestValidateAuth_AccountAndAnyone(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{AccountID: "acct-1", Anyone: true},
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

// C5: user_id under slack.grants → reject.
func TestValidateAuth_UserOnSlack(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Slack: &spec.DeploymentSlackAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{UserID: "alice"},
			},
		},
	})
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

// C6: anyone under slack.grants → accepted (collapses to account_id:<owner>
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

// user_id under slack.grants → still rejected (slack identity is opaque).
func TestValidateAuth_UserIDOnSlack(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Slack: &spec.DeploymentSlackAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{UserID: "alice"},
			},
		},
	})
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

// C8: duplicate grant within an adapter → reject.
func TestValidateAuth_Duplicate(t *testing.T) {
	errs := validateAuthorizationSpec(&spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{AccountID: "acct-1"},
				{AccountID: "acct-1"},
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
				{AccountID: "acct-1"},
				{UserID: "alice"},
				{Anyone: true},
			},
		},
		Slack: &spec.DeploymentSlackAuth{
			Grants: []spec.DeploymentAuthorizationGrant{
				{AccountID: "acct-1"},
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
			AddRow("dep-1", "account", "acct-1", "slack").
			AddRow("dep-1", "anyone", "", "web").
			AddRow("dep-1", "user", "alice", "web"))

	auth := &spec.DeploymentInterfacesAuth{
		Web: &spec.DeploymentWebAuth{
			Grants: []spec.DeploymentAuthorizationGrant{{AccountID: "stale"}},
		},
	}
	mergeAuthorizationFromStore(log, store, "dep-1", auth)

	// Stored order is (subject_type, subject_id, adapter):
	// account/acct-1/slack → goes under Slack;
	// anyone//web and user/alice/web → go under Web.
	if auth.Slack == nil || len(auth.Slack.Grants) != 1 || auth.Slack.Grants[0].AccountID != "acct-1" {
		t.Fatalf("expected one slack account grant, got %+v", auth.Slack)
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
			name:    "account",
			in:      spec.DeploymentAuthorizationGrant{AccountID: "acct-1"},
			adapter: "web",
			want:    struct{ subjectType, subjectID string }{"account", "acct-1"},
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
