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
	provisioned, err := p.ProvisionCustomer(context.Background(), "cust_1", "acct_1", true)
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

// The two packages differ only in the signup credit, so picking the wrong one is
// either a free grant or a missing rate card. A missing no-credit package is a
// configuration error rather than a reason to fall back: falling back to the
// credit package restores the grant silently, which is what this removes.
func TestProvisionPackageSelection(t *testing.T) {
	cases := []struct {
		name       string
		cfg        Config
		withCredit bool
		want       string
		wantErr    bool
	}{
		{"first account takes the credit package", Config{PackageID: "pkg_credit", PackageIDNoCredit: "pkg_bare"}, true, "pkg_credit", false},
		{"later account takes the bare package", Config{PackageID: "pkg_credit", PackageIDNoCredit: "pkg_bare"}, false, "pkg_bare", false},
		{"unconfigured provisioning is a no-op", Config{}, true, "", false},
		{"missing bare package refuses rather than granting", Config{PackageID: "pkg_credit"}, false, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&Provider{cfg: tc.cfg}).provisionPackage(tc.withCredit)
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
