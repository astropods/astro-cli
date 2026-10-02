package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setConnectFlags(t *testing.T, values map[string]string, clusters []string) {
	t.Helper()
	flags := knowledgeConnectCmd.Flags()
	reset := func() {
		flags.VisitAll(func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
	}
	reset()
	t.Cleanup(reset)
	for name, value := range values {
		require.NoError(t, flags.Set(name, value))
	}
	for _, c := range clusters {
		require.NoError(t, flags.Set("cluster", c))
	}
}

var privateLinkConnectFlags = map[string]string{
	"provider":     "postgres",
	"name":         "orders",
	"host":         "com.amazonaws.vpce.us-east-1.vpce-svc-0abc",
	"port":         "5432",
	"password":     "secret",
	"private-link": "true",
}

func TestKnowledgeConnectPrivateLinkRequiresACluster(t *testing.T) {
	called := false
	setupKnowledgeTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/knowledge/connect") {
			called = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	setConnectFlags(t, privateLinkConnectFlags, nil)
	knowledgeConnectCmd.SetContext(context.Background())

	err := runKnowledgeConnect(knowledgeConnectCmd, nil)

	require.Error(t, err)
	assert.Equal(t, errPrivateLinkNeedsCluster().Error(), err.Error())
	assert.False(t, called, "the CLI must reject the flags before calling the server")
}

func TestKnowledgeConnectPrivateLinkSendsTheChosenClusters(t *testing.T) {
	var body map[string]any
	setupKnowledgeTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/knowledge/connect") {
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "stop here"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	setConnectFlags(t, privateLinkConnectFlags, []string{"prod-managed-eks", "prod-managed-eu-west-1-a"})
	knowledgeConnectCmd.SetContext(context.Background())

	_ = runKnowledgeConnect(knowledgeConnectCmd, nil)

	require.NotNil(t, body, "the connect request must reach the server")
	assert.Equal(t, true, body["private_link"])
	assert.Equal(t, []any{"prod-managed-eks", "prod-managed-eu-west-1-a"}, body["private_link_clusters"])
}
