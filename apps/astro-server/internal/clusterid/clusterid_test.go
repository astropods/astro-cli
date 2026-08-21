package clusterid

import "testing"

func TestCanonical(t *testing.T) {
	tests := []struct {
		name           string
		clusterID      string
		defaultCluster string
		want           string
	}{
		{name: "an unrecorded target resolves to the primary", defaultCluster: "primary", want: "primary"},
		{name: "an id resolves to itself", clusterID: "eu", defaultCluster: "primary", want: "eu"},
		{name: "the primary's own id resolves to itself", clusterID: "primary", defaultCluster: "primary", want: "primary"},
		{name: "no configured primary leaves an unrecorded target unresolved", want: ""},
		{name: "the word default is an ordinary id", clusterID: "default", defaultCluster: "primary", want: "default"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := New(tc.defaultCluster).Canonical(tc.clusterID); got != tc.want {
				t.Fatalf("New(%q).Canonical(%q) = %q, want %q", tc.clusterID, tc.defaultCluster, got, tc.want)
			}
		})
	}
}

func TestSame(t *testing.T) {
	tests := []struct {
		name           string
		clusterIDA     string
		clusterIDB     string
		defaultCluster string
		want           bool
	}{
		{name: "an unrecorded target matches the primary's id", clusterIDB: "primary", defaultCluster: "primary", want: true},
		{name: "the same id matches", clusterIDA: "eu", clusterIDB: "eu", defaultCluster: "primary", want: true},
		{name: "two unrecorded targets match", defaultCluster: "primary", want: true},
		{name: "an additional cluster differs from the primary", clusterIDA: "eu", clusterIDB: "primary", defaultCluster: "primary", want: false},
		{name: "an additional cluster differs from an unrecorded target", clusterIDA: "eu", defaultCluster: "primary", want: false},
		{name: "no configured primary compares ids plainly", clusterIDA: "eu", want: false},
		{name: "the word default is not the primary", clusterIDA: "default", clusterIDB: "primary", defaultCluster: "primary", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := New(tc.defaultCluster).Same(tc.clusterIDA, tc.clusterIDB); got != tc.want {
				t.Fatalf("New(%q).Same(%q, %q) = %v, want %v", tc.clusterIDA, tc.clusterIDB, tc.defaultCluster, got, tc.want)
			}
		})
	}
}

func TestIsPrimary(t *testing.T) {
	if !New("primary").IsPrimary("") {
		t.Error("an unrecorded target is the primary")
	}
	if !New("primary").IsPrimary("primary") {
		t.Error("the primary's own id is the primary")
	}
	if New("primary").IsPrimary("eu") {
		t.Error("an additional cluster is not the primary")
	}
	if New("primary").IsPrimary("default") {
		t.Error(`the word "default" does not name the primary`)
	}
	if !New("").IsPrimary("") {
		t.Error("with no primary configured, an unrecorded target is still the primary")
	}
	if New("").IsPrimary("eu") {
		t.Error("with no primary configured, a named cluster is not the primary")
	}
}

func TestLabel(t *testing.T) {
	if got := New("primary").Label(""); got != "primary" {
		t.Errorf("Label = %q, want the primary's id", got)
	}
	if got := New("primary").Label("eu"); got != "eu" {
		t.Errorf("Label = %q, want eu", got)
	}
	if got := New("").Label(""); got != "unrecorded" {
		t.Errorf("Label = %q, want unrecorded", got)
	}
}
