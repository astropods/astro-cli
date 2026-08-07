package metronome

import (
	"testing"

	"github.com/Metronome-Industries/metronome-go/v3/shared"
)

// A contract made by hand carries no uniqueness key, so Contracts.New would not
// 409 against it. Contracts.List can return several contracts covering the same
// date, so the decision must not depend on their order: ours anywhere means
// leave it alone, anything else covering the customer means refuse rather than
// stack a second contract or silently bill on the wrong rates.
func TestClassifyCoverage(t *testing.T) {
	const (
		ours = "pkg_ours"
		key  = "contract:acct_1"
	)

	cases := []struct {
		name        string
		contracts   []shared.Contract
		wantCov     coverage
		wantForeign string
	}{
		{"nothing covers the customer", nil, coverageNone, ""},
		{"empty slice", []shared.Contract{}, coverageNone, ""},
		{"on our package", []shared.Contract{{PackageID: ours}}, coverageOurs, ""},
		{"on someone else's", []shared.Contract{{PackageID: "pkg_other"}}, coverageForeign, "pkg_other"},
		// A hand-made contract often carries no package at all. It still covers
		// the customer, so creating ours alongside it would double-bill.
		{"hand-made with no package", []shared.Contract{{ID: "con_1"}}, coverageForeign, ""},
		// Order must not decide the outcome.
		{
			"ours listed after a packageless one",
			[]shared.Contract{{ID: "con_1"}, {PackageID: ours}},
			coverageOurs, "",
		},
		{
			"ours listed after a foreign one",
			[]shared.Contract{{PackageID: "pkg_other"}, {PackageID: ours}},
			coverageOurs, "",
		},
		{
			"foreign listed after a packageless one",
			[]shared.Contract{{ID: "con_1"}, {PackageID: "pkg_other"}},
			coverageForeign, "pkg_other",
		},
		// The list response may not carry package_id. Our own contract is still
		// recognisable by the uniqueness key we created it with, which is what
		// keeps a retry from cancelling the account permanently.
		{
			"ours, package_id absent from the list response",
			[]shared.Contract{{ID: "con_1", UniquenessKey: key}},
			coverageOurs, "",
		},
		{
			"ours by key, listed after a foreign contract",
			[]shared.Contract{{PackageID: "pkg_other"}, {UniquenessKey: key}},
			coverageOurs, "",
		},
		// Another account's contract must not match on an empty key.
		{
			"packageless contract with a different key",
			[]shared.Contract{{UniquenessKey: "contract:acct_2"}},
			coverageForeign, "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cov, foreign := classifyCoverage(tc.contracts, ours, key)
			if cov != tc.wantCov || foreign != tc.wantForeign {
				t.Errorf("classifyCoverage() = (%v, %q), want (%v, %q)", cov, foreign, tc.wantCov, tc.wantForeign)
			}
		})
	}
}
