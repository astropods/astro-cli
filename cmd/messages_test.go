package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrAgentTargetMessages(t *testing.T) {
	require.EqualError(t, errAgentUnexpectedArgument("my-agent"), `unexpected argument "my-agent" — use --name or --id`)
	require.EqualError(t, errAgentDeploymentNotFoundForID("ze5-r2l-m16"), `no deployment found for ID "ze5-r2l-m16"`)
	require.EqualError(t, errAgentDeploymentNotFound("Pirate Parrot EU!"), `no deployment found for "Pirate Parrot EU!"`)
	require.EqualError(t, errNonNegativeIntFlag("limit"), "--limit must be zero or greater")
	require.EqualError(t, errPositiveIntFlag("limit"), "--limit must be greater than zero")
	require.EqualError(t, errRFC3339TimeFlag("start", "not-a-date"), `--start "not-a-date" is not a valid RFC3339 timestamp`)
	require.EqualError(t, errTraceStartAfterEnd(), "--start must be before --end")
	require.EqualError(t, errAgentTraceNotFound("abc123", "coach"), `no trace "abc123" found for "coach"`)
	require.Equal(t, msgNoTracesForAgent("coach"), "No traces found for coach")
	require.EqualError(t, errAgentTraceSummaryWithTraceID(), "--summary cannot be used with --trace-id")
	require.Equal(t, msgNoObsSummaryForAgent("coach"), "No activity summary for coach yet")
	require.Equal(t, msgLoginPriorAccountUnavailable("acme-corp"), "  Note: previous account \"acme-corp\" is no longer available; using personal account.\n")
	require.EqualError(t, errLoginAccountsLoadEmpty(), "could not load your accounts from the server (empty response). Try again in a moment")
}
