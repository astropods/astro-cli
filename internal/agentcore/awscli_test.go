package agentcore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWSCLIRunDoesNotHangOnAnOrphanedGrandchild(t *testing.T) {
	dir := t.TempDir()
	stubAWS := filepath.Join(dir, "aws")
	require.NoError(t, os.WriteFile(stubAWS, []byte("#!/bin/sh\nsleep 30 & true\n"), 0o700)) //nolint:gosec
	t.Setenv("PATH", dir+":/usr/bin:/bin")

	rt := &AWSCLIRuntime{}
	start := time.Now()
	_, err := rt.run([]string{"sts", "get-caller-identity"})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, awsWaitDelay+2*time.Second,
		"took %s; must give up at roughly awsWaitDelay, not wait for the orphan's full lifetime", elapsed)
}
