package agentcore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitReady(t *testing.T) {
	tests := []struct {
		name        string
		statuses    []RuntimeStatus
		wantVersion string
		wantErr     string
		wantTimeout bool
	}{
		{
			name:     "reaches READY",
			statuses: []RuntimeStatus{{Status: "CREATING"}, {Status: "READY", Version: "1"}},
		},
		{
			// The AZ failure that reported success: the API accepts the call and
			// the runtime fails afterwards.
			name: "a failed status carries its reason",
			statuses: []RuntimeStatus{
				{Status: "UPDATING"},
				{Status: "UPDATE_FAILED", FailureReason: "subnet-0220 in us-east-1f (ID: use1-az5)"},
			},
			wantErr: "runtime reported UPDATE_FAILED: subnet-0220 in us-east-1f (ID: use1-az5)",
		},
		{
			name:     "a failed status with no reason still errors",
			statuses: []RuntimeStatus{{Status: "CREATE_FAILED"}},
			wantErr:  "runtime reported CREATE_FAILED",
		},
		{
			// A dry-run backend reports nothing, which must not spin.
			name:     "an unreportable status ends the wait",
			statuses: nil,
		},
		{
			name:        "READY at the old version keeps waiting",
			statuses:    []RuntimeStatus{{Status: "READY", Version: "1"}, {Status: "READY", Version: "2"}},
			wantVersion: "2",
		},
		{
			name:        "a runtime that never settles times out",
			statuses:    []RuntimeStatus{{Status: "CREATING"}},
			wantTimeout: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &fakeRuntime{statuses: tt.statuses}

			err := WaitReady(context.Background(), rt, "id-1", tt.wantVersion, 20*time.Millisecond, time.Millisecond, nil)

			switch {
			case tt.wantTimeout:
				assert.ErrorIs(t, err, ErrWaitTimeout)
			case tt.wantErr != "":
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
			default:
				assert.NoError(t, err)
			}
		})
	}
}

func TestWaitReady_ReportsEachNewStatus(t *testing.T) {
	rt := &fakeRuntime{statuses: []RuntimeStatus{
		{Status: "CREATING"}, {Status: "CREATING"}, {Status: "READY"},
	}}
	var seen []string

	require.NoError(t, WaitReady(context.Background(), rt, "id-1", "", time.Second, time.Millisecond,
		func(s string) { seen = append(seen, s) }))

	assert.Equal(t, []string{"CREATING", "READY"}, seen, "a repeated status is reported once")
}

func TestWaitReady_HonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rt := &fakeRuntime{statuses: []RuntimeStatus{{Status: "CREATING"}}}

	assert.ErrorIs(t, WaitReady(ctx, rt, "id-1", "", time.Second, time.Millisecond, nil), context.Canceled)
}

func TestStaleAWSCLIHint(t *testing.T) {
	tests := []struct {
		name     string
		stderr   string
		wantHint bool
	}{
		{
			name:     "the VPC parameter rejection is named",
			stderr:   `Unknown parameter in networkConfiguration: "networkModeConfig", must be one of: networkMode`,
			wantHint: true,
		},
		{name: "an unrelated failure is left alone", stderr: "AccessDeniedException"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := staleAWSCLIHint(tt.stderr)
			if !tt.wantHint {
				assert.Empty(t, hint)
				return
			}
			assert.Contains(t, hint, "does not know VPC network mode")
			assert.Contains(t, hint, "brew upgrade awscli")
		})
	}
}
