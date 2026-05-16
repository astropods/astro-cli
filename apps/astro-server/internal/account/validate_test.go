package account

import (
	"testing"
)

func TestValidateAccountName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid names
		{"valid simple", "myorg", false, ""},
		{"valid with digits", "myorg123", false, ""},
		{"valid with hyphens", "my-org-name", false, ""},
		{"valid four chars", "abcd", false, ""},
		{"valid 39 chars", "abcdefghijklmnopqrstuvwxyz1234567890abc", false, ""},

		// Too short
		{"too short 1 char", "a", true, "at least 4 characters"},
		{"too short 3 chars", "abc", true, "at least 4 characters"},

		// Too long
		{"too long 40 chars", "abcdefghijklmnopqrstuvwxyz12345678901234", true, "at most 39 characters"},

		// Must be lowercase
		{"uppercase", "MyOrg", true, "must be lowercase"},
		{"mixed case", "myOrg", true, "must be lowercase"},

		// Must start with letter
		{"starts with digit", "1org", true, "must start with a letter"},
		{"starts with hyphen", "-org", true, "must start with a letter"},

		// Must not end with hyphen
		{"ends with hyphen", "myorg-", true, "must not end with a hyphen"},

		// No consecutive hyphens
		{"consecutive hyphens", "my--org", true, "consecutive hyphens"},

		// Invalid characters
		{"underscore", "my_org", true, "lowercase letters, digits, and hyphens"},
		{"space", "my org", true, "lowercase letters, digits, and hyphens"},
		{"special char", "my@org", true, "lowercase letters, digits, and hyphens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateAccountName(%q) = nil, want error containing %q", tt.input, tt.errMsg)
				} else if tt.errMsg != "" {
					if !containsSubstring(err.Error(), tt.errMsg) {
						t.Errorf("ValidateAccountName(%q) error = %q, want to contain %q", tt.input, err.Error(), tt.errMsg)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateAccountName(%q) = %v, want nil", tt.input, err)
				}
			}
		})
	}
}

func TestCheckAccountNameRestricted(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"allowed name", "myorg", false, ""},
		{"reserved admin", "admin", true, "reserved"},
		{"reserved login", "login", true, "reserved"},
		{"reserved agents", "agents", true, "reserved"},
		{"reserved accounts", "accounts", true, "reserved"},
		{"reserved settings", "settings", true, "reserved"},

		// reservedVariants: singulars of plurals in reservedNames
		{"variant singular agent", "agent", true, "reserved"},
		{"variant singular blueprint", "blueprint", true, "reserved"},
		{"variant singular setting", "setting", true, "reserved"},
		{"variant singular task", "task", true, "reserved"},
		{"variant singular workflow", "workflow", true, "reserved"},

		// reservedVariants: plurals of singulars in reservedNames
		{"variant plural admins", "admins", true, "reserved"},
		{"variant plural dashboards", "dashboards", true, "reserved"},
		{"variant plural sandboxes", "sandboxes", true, "reserved"},
		{"variant plural systems", "systems", true, "reserved"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckAccountNameRestricted(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("CheckAccountNameRestricted(%q) = nil, want error containing %q", tt.input, tt.errMsg)
				} else if tt.errMsg != "" && !containsSubstring(err.Error(), tt.errMsg) {
					t.Errorf("CheckAccountNameRestricted(%q) error = %q, want to contain %q", tt.input, err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("CheckAccountNameRestricted(%q) = %v, want nil", tt.input, err)
				}
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
