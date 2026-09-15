package cmd

import (
	"runtime"
	"strings"
	"testing"
)

// Whatever checkDockerRunning returns has to come from the daemon probe, so
// the message names Docker rather than the operating system.
func TestCheckDockerRunningReportsDockerNotThePlatform(t *testing.T) {
	err := checkDockerRunning()
	if err == nil {
		t.Skipf("Docker is reachable on this %s machine, so there is no error to inspect", runtime.GOOS)
	}
	if !strings.Contains(err.Error(), "Docker") {
		t.Errorf("error does not mention Docker, so something other than the daemon probe answered: %v", err)
	}
	if strings.Contains(err.Error(), "not supported") {
		t.Errorf("error refuses the platform instead of reporting the daemon: %v", err)
	}
}

// Every platform's wording is asserted from whichever platform runs the tests,
// which is the point of passing goos in.
func TestDockerUnreachableErrorNamesThePlatformsInstaller(t *testing.T) {
	tests := []struct {
		goos            string
		endpointMissing bool
		want            string
	}{
		{"windows", true, "Docker Desktop for Windows"},
		{"darwin", true, "Docker Desktop for Mac"},
		{"linux", true, "Docker Engine"},
		{"windows", false, "Start menu"},
		{"darwin", false, "Applications folder"},
		{"linux", false, "systemctl start docker"},
	}
	for _, tc := range tests {
		t.Run(tc.goos+"/"+map[bool]string{true: "missing", false: "stopped"}[tc.endpointMissing], func(t *testing.T) {
			err := dockerUnreachableError(tc.goos, tc.endpointMissing)
			if err == nil {
				t.Fatal("no error, but the daemon was unreachable")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("message does not mention %q: %v", tc.want, err)
			}
			// A stopped daemon is a different fix from an absent one, so the
			// two must never share wording.
			installed := strings.Contains(err.Error(), "not installed")
			if installed != tc.endpointMissing {
				t.Errorf("reported installed=%v, want %v: %v", !installed, !tc.endpointMissing, err)
			}
		})
	}
}

// An unrecognised GOOS still has to say something actionable rather than
// falling through to an empty hint.
func TestDockerUnreachableErrorFallsBackForAnUnknownPlatform(t *testing.T) {
	for _, missing := range []bool{true, false} {
		err := dockerUnreachableError("plan9", missing)
		if err == nil || strings.TrimSpace(err.Error()) == "" {
			t.Fatalf("empty error for an unknown platform (endpointMissing=%v)", missing)
		}
		if !strings.Contains(err.Error(), "Docker") {
			t.Errorf("fallback does not mention Docker: %v", err)
		}
	}
}
