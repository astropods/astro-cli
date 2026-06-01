package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrAgentTargetMessages(t *testing.T) {
	require.EqualError(t, errAgentUnexpectedArgument("my-agent"), `unexpected argument "my-agent" — use --name or --id`)
	require.EqualError(t, errAgentDeploymentNotFoundForID("ze5-r2l-m16"), `no deployment found for ID "ze5-r2l-m16"`)
	require.EqualError(t, errAgentDeploymentNotFound("Pirate Parrot EU!"), `no deployment found for "Pirate Parrot EU!"`)
}
