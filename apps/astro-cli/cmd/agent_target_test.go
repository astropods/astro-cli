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

func setAgentTargetName(t *testing.T, cmd *cobra.Command, name string) {
	t.Helper()
	require.NoError(t, cmd.Flags().Set("name", name))
	t.Cleanup(func() { _ = cmd.Flags().Set("name", "") })
}

func setAgentTargetID(t *testing.T, cmd *cobra.Command, id string) {
	t.Helper()
	require.NoError(t, cmd.Flags().Set("id", id))
	t.Cleanup(func() { _ = cmd.Flags().Set("id", "") })
}

func TestAgentTargetArgs(t *testing.T) {
	cmd := &cobra.Command{}
	registerAgentTargetFlags(cmd)

	require.EqualError(t, agentTargetArgs(cmd, []string{"my-agent"}), errAgentUnexpectedArgument("my-agent").Error())
	require.NoError(t, agentTargetArgs(cmd, nil))

	setAgentTargetName(t, cmd, "my-agent")
	require.NoError(t, agentTargetArgs(cmd, nil))

	cmd2 := &cobra.Command{}
	registerAgentTargetFlags(cmd2)
	setAgentTargetID(t, cmd2, "ze5-r2l-m16")
	require.NoError(t, agentTargetArgs(cmd2, nil))
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

	setAgentTargetID(t, agentGetCmd, "ze5-r2l-m16")
	require.NoError(t, agentTargetArgs(agentGetCmd, nil))

	buf := &bytes.Buffer{}
	agentGetCmd.SetOut(buf)
	agentGetCmd.SetContext(context.Background())

	require.NoError(t, runAgentGet(agentGetCmd, nil))
	assert.Contains(t, buf.String(), "Pirate Parrot EU!")
}

func TestResolveAgentTargetByDisplayName(t *testing.T) {
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
	setAgentTargetName(t, cmd, "Pirate Parrot EU!")
	at := AccountToken{Account: "testaccount", Token: "token"}

	dep, err := resolveAgentTarget(cmd, at, false)
	require.NoError(t, err)
	assert.Equal(t, "ze5-r2l-m16", dep.ID)
	assert.Equal(t, "Pirate Parrot EU!", dep.DisplayName)
}

func TestResolveAgentTargetByNameDoesNotMatchDeploymentID(t *testing.T) {
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
	setAgentTargetName(t, cmd, "ze5-r2l-m16")
	at := AccountToken{Account: "testaccount", Token: "token"}

	_, err := resolveAgentTarget(cmd, at, false)
	require.EqualError(t, err, errAgentDeploymentNotFound("ze5-r2l-m16").Error())
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
	setAgentTargetID(t, cmd, "ze5-r2l-m16")
	at := AccountToken{Account: "testaccount", Token: "token"}

	dep, err := resolveAgentTarget(cmd, at, false)
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
	setAgentTargetName(t, agentGetCmd, "Pirate Parrot EU!")

	require.NoError(t, runAgentGet(agentGetCmd, nil))
	assert.Contains(t, buf.String(), "Pirate Parrot EU!")
}
