package cmd

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRedeployTimeoutBoundsTheBuildWait(t *testing.T) {
	buildPolls := 0
	setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The target resolves, then the build wait stalls on Retry-After.
		if strings.HasSuffix(r.URL.Path, "/builds") {
			buildPolls++
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, err := io.WriteString(w, `{"deployment":{"id":"dep","name":"dummy","build_id":"one","current_revision":3,"status":"Running"}}`)
		require.NoError(t, err)
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
	require.NoError(t, cmd.Flags().Set("commit", strings.Repeat("a", 40)))
	start := time.Now()
	err := runAgentRedeploy(cmd, nil)
	elapsed := time.Since(start)
	t.Logf("20ms command timeout: returned after %s, error=%v", elapsed, err)
	require.Error(t, err)
	require.Less(t, elapsed, 200*time.Millisecond, "the build wait must use the command's timeout")
	require.NotZero(t, buildPolls, "the deadline must be exercised at the build wait")
}
