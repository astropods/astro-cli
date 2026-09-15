package cmd

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Whatever checkDockerRunning returns has to come from the daemon probe, so
// the message names Docker rather than the operating system.
func TestCheckDockerRunningReportsDockerNotThePlatform(t *testing.T) {
	err := checkDockerRunning()
	if err == nil {
		t.Skipf("Docker is reachable on this %s machine, so there is no error to inspect", runtime.GOOS)
	}
	assert.Contains(t, err.Error(), "Docker", "something other than the daemon probe answered")
	assert.NotContains(t, err.Error(), "not supported", "the error refuses the platform instead of reporting the daemon")
}

// Every platform's wording is asserted from whichever platform runs the tests,
// which is the point of passing goos in.
func TestDockerUnreachableErrorNamesThePlatformsInstaller(t *testing.T) {
	tests := []struct {
		name            string
		goos            string
		endpointMissing bool
		want            string
	}{
		{"windows, absent", "windows", true, "Docker Desktop for Windows"},
		{"darwin, absent", "darwin", true, "Docker Desktop for Mac"},
		{"linux, absent", "linux", true, "Docker Engine"},
		{"windows, stopped", "windows", false, "Start menu"},
		{"darwin, stopped", "darwin", false, "Applications folder"},
		{"linux, stopped", "linux", false, "systemctl start docker"},
		// An unrecognised platform still has to say something actionable
		// rather than fall through to an empty hint.
		{"unknown, absent", "plan9", true, "Docker"},
		{"unknown, stopped", "plan9", false, "Docker"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := dockerUnreachableError(tc.goos, tc.endpointMissing)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			// A stopped daemon is a different fix from an absent one, so the
			// two must never share wording.
			assert.Equal(t, tc.endpointMissing, strings.Contains(err.Error(), "not installed"),
				"the installed/absent wording disagrees with the probe result")
		})
	}
}
