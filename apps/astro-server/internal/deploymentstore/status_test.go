package deploymentstore

import (
	"slices"
	"testing"
)

// Every status is either running or not, and the split decides whether an
// account can remove its payment method. A status added later and left out of
// both lists would silently count as not running, which is the direction that
// lets a spending account drop its card.
func TestRunningStatusesClassifyEveryStatus(t *testing.T) {
	notRunning := []string{StatusStopped, StatusSuspended, StatusUndeployed, StatusFailed}
	all := []string{
		StatusPending, StatusProvisioning, StatusDeploying, StatusActive,
		StatusFailed, StatusUndeploying, StatusUndeployed, StatusStopped, StatusSuspended,
	}

	for _, status := range all {
		running := slices.Contains(RunningStatuses, status)
		idle := slices.Contains(notRunning, status)
		if running == idle {
			t.Errorf("status %q is in %d of the two lists, want exactly 1", status, map[bool]int{true: 2, false: 0}[running])
		}
	}
	if len(RunningStatuses)+len(notRunning) != len(all) {
		t.Errorf("running(%d) + idle(%d) != all(%d): a status is unclassified",
			len(RunningStatuses), len(notRunning), len(all))
	}
}

// A deployment on its way up bills as soon as it lands, so it has to hold the
// card the same way an active one does.
func TestComingUpCountsAsRunning(t *testing.T) {
	for _, status := range []string{StatusPending, StatusProvisioning, StatusDeploying, StatusUndeploying} {
		if !slices.Contains(RunningStatuses, status) {
			t.Errorf("status %q is not running, so a card could be removed while it holds workloads up", status)
		}
	}
}
