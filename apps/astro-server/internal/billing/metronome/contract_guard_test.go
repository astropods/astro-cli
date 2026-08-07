package metronome

import (
	"testing"

	"github.com/Metronome-Industries/metronome-go/v3/shared"
)

// A contract made by hand carries no uniqueness key, so Contracts.New would not
// 409 against it, and the list response carries no package to tell it apart
// from ours. Contracts.List can return several contracts covering the same
// date, so the decision must not depend on their order: ours anywhere means
// leave it alone, anything else covering the customer means refuse rather than
// stack a second contract.
func TestClassifyCoverage(t *testing.T) {
	const key = "contract:acct_1"

	cases := []struct {
		name        string
		contracts   []shared.ContractV2
		wantCov     coverage
		wantForeign string
	}{
		{"nothing covers the customer", nil, coverageNone, ""},
		{"empty slice", []shared.ContractV2{}, coverageNone, ""},
		{"ours by uniqueness key", []shared.ContractV2{{UniquenessKey: key}}, coverageOurs, ""},
		{"hand-made, no key", []shared.ContractV2{{ID: "con_1"}}, coverageForeign, "con_1"},
		// Another account's contract must not match on an empty key.
		{
			"another account's contract",
			[]shared.ContractV2{{ID: "con_2", UniquenessKey: "contract:acct_2"}},
			coverageForeign, "con_2",
		},
		// Order must not decide the outcome: a retry that already created our
		// contract has to find it however the list is ordered, or the account is
		// cancelled for good.
		{
			"ours listed after a stray",
			[]shared.ContractV2{{ID: "con_1"}, {UniquenessKey: key}},
			coverageOurs, "",
		},
		{
			"ours listed first",
			[]shared.ContractV2{{UniquenessKey: key}, {ID: "con_1"}},
			coverageOurs, "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cov, foreign := classifyCoverage(tc.contracts, key)
			if cov != tc.wantCov || foreign != tc.wantForeign {
				t.Errorf("classifyCoverage() = (%v, %q), want (%v, %q)", cov, foreign, tc.wantCov, tc.wantForeign)
			}
		})
	}
}
