package metronome

import (
	"encoding/json"
	"testing"

	"github.com/Metronome-Industries/metronome-go/v3/shared"
)

func TestContractPackageID_SurvivesTheSDKStruct(t *testing.T) {
	const body = `{
		"id": "02c77612-66bc-4e3f-aa9b-5066e2236fa9",
		"customer_id": "3794e396-a975-4199-8112-b5d1ab59b0a6",
		"package_id": "2fde31d1-cf10-46f8-aaf7-750be61907d8",
		"rate_card_id": "baa427df-79e5-4ca4-96ce-4d0b8a013654",
		"starting_at": "2026-08-19T15:00:00+00:00",
		"name": "Standard Rate"
	}`
	var c shared.ContractV2
	if err := json.Unmarshal([]byte(body), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := contractPackageID(c); got != "2fde31d1-cf10-46f8-aaf7-750be61907d8" {
		t.Errorf("contractPackageID = %q, want the package the API sent", got)
	}
}

func TestCustomerPlan_UnconfiguredPackageIsNotLabelled(t *testing.T) {
	p := &Provider{cfg: Config{PackageID: "pkg_credit"}}
	for _, tc := range []struct{ name, pkg, want string }{
		{"configured", "pkg_credit", "credit"},
		{"unknown", "pkg_unlimited", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c shared.ContractV2
			if err := json.Unmarshal([]byte(`{"package_id":"`+tc.pkg+`"}`), &c); err != nil {
				t.Fatal(err)
			}
			got := ""
			if plan, ok := p.planForPackage(contractPackageID(c)); ok {
				got = string(plan)
			}
			if got != tc.want {
				t.Errorf("plan = %q, want %q", got, tc.want)
			}
		})
	}
}
