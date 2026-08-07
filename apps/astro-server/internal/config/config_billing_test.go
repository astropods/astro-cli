package config

import "testing"

func TestValidateBilling_RejectsPartialProvisioningConfig(t *testing.T) {
	cases := []struct {
		name      string
		pkg       string
		creditTyp string
		credit    int
		expiry    int
		wantErr   bool
	}{
		{"package with credit but no credit type", "pkg_1", "", 50, 365, true},
		// Metronome requires an expiry, so 0 days grants already-dead credit.
		{"credit with no expiry", "pkg_1", "ct_1", 50, 0, true},
		{"complete", "pkg_1", "ct_1", 50, 365, false},
		// Defaults: provisioning creates the contract, grants nothing.
		{"defaults", "pkg_1", "", 0, 0, false},
		// Provisioning is off entirely without a package.
		{"no package", "", "", 50, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{}
			c.MetronomePackageID = tc.pkg
			c.MetronomeCreditTypeID = tc.creditTyp
			c.MetronomeSignupCredit = tc.credit
			c.MetronomeCreditExpiryDays = tc.expiry
			if err := c.validateBilling(); (err != nil) != tc.wantErr {
				t.Errorf("validateBilling() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
