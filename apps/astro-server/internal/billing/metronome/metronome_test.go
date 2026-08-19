package metronome

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Metronome-Industries/metronome-go/v3"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

func TestNew_NilWithoutAPIKey(t *testing.T) {
	if p := New(Config{}); p != nil {
		t.Error("expected nil provider when API key is empty")
	}
}

func TestNew_ImplementsInterface(t *testing.T) {
	p := New(Config{APIKey: "test-key"})
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	var _ billing.BillingProvider = p
	var _ billing.Provisioner = p
}

// No package means provisioning is off; it must not call Metronome, and must
// report false so the caller leaves the account in the backfill sweep.
func TestProvisionCustomer_NoPackageIsNoop(t *testing.T) {
	p := New(Config{APIKey: "test-key"})
	provisioned, err := p.ProvisionCustomer(context.Background(), "cust_1", "acct_1", billing.PlanCredit)
	if err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
	if provisioned {
		t.Fatal("unconfigured provider reported the account as provisioned")
	}
}

func TestIsConflict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"409 from uniqueness key", &metronome.Error{StatusCode: http.StatusConflict}, true},
		{"wrapped 409", fmt.Errorf("create: %w", &metronome.Error{StatusCode: http.StatusConflict}), true},
		{"other api error", &metronome.Error{StatusCode: http.StatusBadRequest}, false},
		{"non-api error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConflict(tt.err); got != tt.want {
				t.Errorf("isConflict = %v, want %v", got, tt.want)
			}
		})
	}
}

// Picking the wrong package is either a free grant or a wrongly billed account.
// A missing one is a configuration error rather than a reason to fall back,
// because falling back does either of those silently.
func TestProvisionPackageSelection(t *testing.T) {
	full := Config{PackageID: "pkg_credit", PackageIDNoCredit: "pkg_bare", PackageIDUnlimited: "pkg_free"}
	cases := []struct {
		name    string
		cfg     Config
		plan    billing.Plan
		want    string
		wantErr bool
	}{
		{"first account takes the credit package", full, billing.PlanCredit, "pkg_credit", false},
		{"later account takes the bare package", full, billing.PlanNoCredit, "pkg_bare", false},
		{"internal account takes the unlimited package", full, billing.PlanUnlimited, "pkg_free", false},
		{"unconfigured provisioning is a no-op", Config{}, billing.PlanCredit, "", false},
		{"missing bare package refuses rather than granting", Config{PackageID: "pkg_credit"}, billing.PlanNoCredit, "", true},
		{"missing unlimited package refuses rather than billing", Config{PackageID: "pkg_credit", PackageIDNoCredit: "pkg_bare"}, billing.PlanUnlimited, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&Provider{cfg: tc.cfg}).provisionPackage(tc.plan)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("provisionPackage = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("provisionPackage: %v", err)
			}
			if got != tc.want {
				t.Errorf("provisionPackage = %q, want %q", got, tc.want)
			}
		})
	}
}
