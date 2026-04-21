package compose

import (
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// TestProjectName_Table locks the single-source-of-truth contract that
// compose project names are always the bare agent name, regardless of scope.
// Up/Down/Logs/health/.running must all agree on this name; any divergence
// reintroduces the scoped-name cleanup warning bug.
func TestProjectName_Table(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"my-agent", "my-agent"},
		{"@org/my-agent", "my-agent"},
		{"@example/release-note-helper", "release-note-helper"},
	}
	for _, c := range cases {
		got := ProjectNameFromSpecName(c.in)
		if got != c.want {
			t.Errorf("ProjectNameFromSpecName(%q) = %q, want %q", c.in, got, c.want)
		}
		s := &spec.AstroSpec{Name: c.in}
		if got := ProjectName(s); got != c.want {
			t.Errorf("ProjectName(&AstroSpec{Name:%q}) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestProjectName_MatchesBuildProject is the regression guard for the bug:
// whatever project name BuildProject stamps onto the compose project MUST be
// exactly what ProjectName returns, because every Down/Logs/health call site
// now derives the name via ProjectName. A divergence here reintroduces the
// "No resource found to remove for project @org/my-agent" warning.
func TestProjectName_MatchesBuildProject(t *testing.T) {
	for _, raw := range []string{"my-agent", "@org/my-agent", "@example/release-note-helper"} {
		s := &spec.AstroSpec{
			Name:  raw,
			Agent: spec.Container{Image: "agent:latest"},
		}
		project, err := BuildProject(s, "/work", nil)
		if err != nil {
			t.Fatalf("BuildProject(%q) error = %v", raw, err)
		}
		if got, want := project.Name, ProjectName(s); got != want {
			t.Errorf("BuildProject(%q).Name = %q, ProjectName = %q (must match)", raw, got, want)
		}
	}
}
