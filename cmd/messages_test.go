package cmd

import (
	"bytes"
	"errors"
	"testing"

	"github.com/astropods/astro-cli/internal/buildinfo"
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
	require.Equal(t, msgLoginPriorAccountUnavailable("acme-corp"), "  Note: previous account \"acme-corp\" is no longer available; using personal account.\n")
	require.EqualError(t, errLoginAccountsLoadEmpty(), "could not load your accounts from the server (empty response). Try again in a moment")
	require.Equal(t, msgAdapterSkipped("slack", "SLACK_BOT_TOKEN"),
		"⚠ slack adapter listed but SLACK_BOT_TOKEN not set, skipping (run 'ast-dev project configure' to add it)")
}

func TestErrBlueprintNameConflictNamesTheArchiveOnlyWhenArchived(t *testing.T) {
	require.EqualError(t, errBlueprintNameConflict("my-agent", "acme", false),
		"\"my-agent\" already exists in \"acme\"\n\n"+
			"To push under a different name, change name in astropods.yml, or run:\n  "+buildinfo.BinaryName+" push <new blueprint name>\n\n"+
			"If \"my-agent\" in \"acme\" is yours, push to it anyway with --yes")
	require.EqualError(t, errBlueprintNameConflict("my-agent", "acme", true),
		"\"my-agent\" already exists (archived) in \"acme\"\n\n"+
			"To push under a different name, change name in astropods.yml, or run:\n  "+buildinfo.BinaryName+" push <new blueprint name>\n\n"+
			"If \"my-agent\" in \"acme\" is yours, push to it anyway with --yes — this will restore it")
}

func TestErrBlueprintExistenceCheckFailedSuggestsRetryOnlyWhenRetrying(t *testing.T) {
	require.EqualError(t, errBlueprintExistenceCheckFailed("my-agent", "acme", &apiError{StatusCode: 403, Body: "forbidden"}),
		`could not check whether "my-agent" already exists in "acme", so the push stopped: server returned status 403: forbidden`,
		"a 403 fails the same way every time, so retrying is not useful advice")
	require.EqualError(t, errBlueprintExistenceCheckFailed("my-agent", "acme", &apiError{StatusCode: 500, Body: "boom"}),
		"could not check whether \"my-agent\" already exists in \"acme\", so the push stopped: server returned status 500: boom\nRetry the push")
	require.EqualError(t, errBlueprintExistenceCheckFailed("my-agent", "acme", errors.New("dial tcp: connection refused")),
		"could not check whether \"my-agent\" already exists in \"acme\", so the push stopped: dial tcp: connection refused\nRetry the push",
		"an unstructured error is a network failure, which can succeed on retry")
}

func TestPrintCanceledWritesDimCanceledLine(t *testing.T) {
	var buf bytes.Buffer
	printCanceled(&buf)
	require.Equal(t, colorDim+"Canceled."+colorReset+"\n", buf.String())
}
