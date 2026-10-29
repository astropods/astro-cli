package cmd

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRedeployTimeoutIncludesInitialReadBackoff(t *testing.T) {
	setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	cmd := &cobra.Command{}
	registerDeployCommonFlags(cmd)
	registerAgentTargetFlags(cmd)
	cmd.Flags().Bool("latest", false, "")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cmd.SetContext(ctx)
	require.NoError(t, cmd.Flags().Set("id", "dep"))
	require.NoError(t, cmd.Flags().Set("timeout", "20ms"))
	require.NoError(t, cmd.Flags().Set("wait", "true"))
	start := time.Now()
	err := runAgentRedeploy(cmd, nil)
	elapsed := time.Since(start)
	t.Logf("20ms command timeout: returned after %s, error=%v", elapsed, err)
	require.Error(t, err)
	require.Less(t, elapsed, 200*time.Millisecond, "initial reads must use the command's timeout")
}
