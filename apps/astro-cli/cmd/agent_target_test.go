package cmd

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJoinAgentTargetArgs(t *testing.T) {
	assert.Equal(t, "Pirate Parrot EU!", joinAgentTargetArgs([]string{"Pirate", "Parrot", "EU!"}))
	assert.Equal(t, "my-agent", joinAgentTargetArgs([]string{"my-agent"}))
}

func TestLooksLikeDeploymentID(t *testing.T) {
	assert.True(t, looksLikeDeploymentID("ze5-r2l-m16"))
	assert.True(t, looksLikeDeploymentID("  abc-def-ghi  "))
	assert.False(t, looksLikeDeploymentID("Pirate Parrot EU!"))
	assert.False(t, looksLikeDeploymentID("my-agent"))
	assert.False(t, looksLikeDeploymentID("ab-cd-e"))
}

func TestAgentTargetArgs(t *testing.T) {
	cmd := &cobra.Command{}
	registerAgentTargetFlags(cmd)

	require.NoError(t, agentTargetArgs(cmd, []string{"my-agent"}))
	require.NoError(t, agentTargetArgs(cmd, []string{"Pirate", "Parrot", "EU!"}))
	require.Error(t, agentTargetArgs(cmd, nil))

	require.NoError(t, cmd.Flags().Set("id", "ze5-r2l-m16"))
	require.NoError(t, agentTargetArgs(cmd, nil))
}

func TestAgentGetByIDFlagNoPositionalArg(t *testing.T) {
	fullPayload := map[string]any{
		"deployment": map[string]any{
			"id": "ze5-r2l-m16", "name": "pirate-parrot", "display_name": "Pirate Parrot EU!",
			"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-01-01T10:00:00Z",
			"workloads": []any{
				map[string]any{"name": "pirate-parrot-agent", "component": "agent", "pod_name": "pod-1"},
			},
		},
	}
	setupAgentTest(t, jsonHandler(http.StatusOK, fullPayload))

	require.NoError(t, agentGetCmd.Flags().Set("id", "ze5-r2l-m16"))
	t.Cleanup(func() { agentGetCmd.Flags().Set("id", "") }) //nolint:errcheck

	require.NoError(t, agentTargetArgs(agentGetCmd, nil))

	buf := &bytes.Buffer{}
	agentGetCmd.SetOut(buf)
	agentGetCmd.SetContext(context.Background())

	require.NoError(t, runAgentGet(agentGetCmd, nil))
	assert.Contains(t, buf.String(), "Pirate Parrot EU!")
}

func TestResolveAgentTargetByDisplayNameMultiWord(t *testing.T) {
	listPayload := map[string]any{
		"deployments": []any{
			map[string]any{
				"id": "ze5-r2l-m16", "name": "pirate-parrot", "display_name": "Pirate Parrot EU!",
				"build_id": "abc12345", "status": "active", "created_at": "2026-01-01T10:00:00Z",
			},
		},
		"count": 1,
	}
	setupAgentTest(t, jsonHandler(http.StatusOK, listPayload))

	cmd := &cobra.Command{}
	registerAgentTargetFlags(cmd)
	at := AccountToken{Account: "testaccount", Token: "token"}

	dep, err := resolveAgentTarget(cmd, []string{"Pirate", "Parrot", "EU!"}, at, false)
	require.NoError(t, err)
	assert.Equal(t, "ze5-r2l-m16", dep.ID)
	assert.Equal(t, "Pirate Parrot EU!", dep.DisplayName)
}

func TestResolveAgentTargetByPositionalID(t *testing.T) {
	fullPayload := map[string]any{
		"deployment": map[string]any{
			"id": "ze5-r2l-m16", "name": "pirate-parrot", "display_name": "Pirate Parrot EU!",
			"build_id": "abc12345", "status": "active", "created_at": "2026-01-01T10:00:00Z",
		},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/deployments/ze5-r2l-m16") {
			jsonHandler(http.StatusOK, fullPayload)(w, r)
		} else {
			jsonHandler(http.StatusOK, map[string]any{"deployments": []any{}, "count": 0})(w, r)
		}
	})
	setupAgentTest(t, handler)

	cmd := &cobra.Command{}
	registerAgentTargetFlags(cmd)
	at := AccountToken{Account: "testaccount", Token: "token"}

	dep, err := resolveAgentTarget(cmd, []string{"ze5-r2l-m16"}, at, false)
	require.NoError(t, err)
	assert.Equal(t, "Pirate Parrot EU!", dep.DisplayName)
}

func TestResolveAgentTargetByIDFlag(t *testing.T) {
	fullPayload := map[string]any{
		"deployment": map[string]any{
			"id": "ze5-r2l-m16", "name": "pirate-parrot", "display_name": "Pirate Parrot EU!",
			"build_id": "abc12345", "status": "active", "created_at": "2026-01-01T10:00:00Z",
		},
	}
	listCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/deployments") && r.URL.Query().Get("account") != "" {
			listCalled = true
		}
		jsonHandler(http.StatusOK, fullPayload)(w, r)
	})
	setupAgentTest(t, handler)

	cmd := &cobra.Command{}
	registerAgentTargetFlags(cmd)
	require.NoError(t, cmd.Flags().Set("id", "ze5-r2l-m16"))
	at := AccountToken{Account: "testaccount", Token: "token"}

	dep, err := resolveAgentTarget(cmd, nil, at, false)
	require.NoError(t, err)
	assert.Equal(t, "ze5-r2l-m16", dep.ID)
	assert.False(t, listCalled, "list lookup should be skipped when --id is set")
}

func TestAgentGetMultiWordDisplayName(t *testing.T) {
	listPayload := map[string]any{
		"deployments": []any{
			map[string]any{
				"id": "ze5-r2l-m16", "name": "pirate-parrot", "display_name": "Pirate Parrot EU!",
				"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-01-01T10:00:00Z",
			},
		},
		"count": 1,
	}
	detailPayload := map[string]any{
		"deployment": map[string]any{
			"id": "ze5-r2l-m16", "name": "pirate-parrot", "display_name": "Pirate Parrot EU!",
			"workloads": []any{
				map[string]any{"name": "pirate-parrot-agent", "component": "agent", "pod_name": "pod-1"},
			},
		},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/deployments/ze5-r2l-m16") {
			jsonHandler(http.StatusOK, detailPayload)(w, r)
		} else {
			jsonHandler(http.StatusOK, listPayload)(w, r)
		}
	})
	setupAgentTest(t, handler)

	buf := &bytes.Buffer{}
	agentGetCmd.SetOut(buf)
	agentGetCmd.SetContext(context.Background())

	require.NoError(t, runAgentGet(agentGetCmd, []string{"Pirate", "Parrot", "EU!"}))
	assert.Contains(t, buf.String(), "Pirate Parrot EU!")
}
