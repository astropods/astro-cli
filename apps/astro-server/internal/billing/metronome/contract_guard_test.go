package metronome

import (
	"testing"

	"github.com/Metronome-Industries/metronome-go/v3/shared"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// Coverage is by existence. There is one package, so a contract created by hand
// bills at the same rates as one created here, and creating a second would bill
// the customer twice.
func TestContractCoverageState(t *testing.T) {
	cases := []struct {
		name          string
		contracts     []shared.ContractV2
		wantState     string
		wantContracts int
	}{
		{"nothing covers the customer", nil, billing.CoverageNone, 0},
		{"empty slice", []shared.ContractV2{}, billing.CoverageNone, 0},
		{
			"contract this system created",
			[]shared.ContractV2{{ID: "con_1", UniquenessKey: contractKey("acct_1")}},
			billing.CoverageCovered, 1,
		},
		{
			"contract created by hand",
			[]shared.ContractV2{{ID: "con_2"}},
			billing.CoverageCovered, 1,
		},
		{
			"several contracts",
			[]shared.ContractV2{{ID: "con_3"}, {ID: "con_4"}},
			billing.CoverageCovered, 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := coverageFrom(tc.contracts)
			if got.State != tc.wantState {
				t.Errorf("state = %q, want %q", got.State, tc.wantState)
			}
			if len(got.Contracts) != tc.wantContracts {
				t.Errorf("contracts = %d, want %d", len(got.Contracts), tc.wantContracts)
			}
		})
	}
}

// The uniqueness key is what makes a repeat contract create 409 instead of
// stacking, so both paths must derive it identically.
func TestContractKeyIsAccountScoped(t *testing.T) {
	if got := contractKey("acct_1"); got != "contract:acct_1" {
		t.Errorf("contractKey = %q, want contract:acct_1", got)
	}
	if contractKey("acct_1") == contractKey("acct_2") {
		t.Error("contractKey collides across accounts")
	}
}
