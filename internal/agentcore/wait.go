package agentcore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrWaitTimeout means no terminal status arrived in time. The deploy was still
// submitted, so callers warn rather than fail.
var ErrWaitTimeout = errors.New("runtime did not report a terminal status in time")

// RuntimeStatus is what the control plane reports about one runtime. An empty
// Status means the backend cannot report one, which ends a wait.
type RuntimeStatus struct {
	Status        string
	Version       string
	FailureReason string
}

// WaitReady polls until the runtime is READY at wantVersion, or reports a
// failure. A blank wantVersion, or a backend that omits the version, skips that check.
func WaitReady(ctx context.Context, rt Runtime, id, wantVersion string, timeout, interval time.Duration, onStatus func(string)) error {
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		st, err := rt.Status(id)
		if err != nil {
			return fmt.Errorf("read runtime status: %w", err)
		}
		if st.Status == "" {
			return nil
		}
		if st.Status != last {
			last = st.Status
			if onStatus != nil {
				onStatus(st.Status)
			}
		}
		if strings.HasSuffix(st.Status, "_FAILED") {
			if reason := strings.TrimSpace(st.FailureReason); reason != "" {
				return fmt.Errorf("runtime reported %s: %s", st.Status, reason)
			}
			return fmt.Errorf("runtime reported %s", st.Status)
		}
		if st.Status == "READY" && (wantVersion == "" || st.Version == "" || st.Version == wantVersion) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("%w (last status %q)", ErrWaitTimeout, last)
}
