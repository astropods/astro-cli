package cmd

import (
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setAccessFlag(t *testing.T, name, value string) {
	t.Helper()
	flags := agentAccessSetCmd.Flags()
	require.NoError(t, flags.Set(name, value))
	t.Cleanup(func() {
		f := flags.Lookup(name)
		if sv, ok := f.Value.(interface{ Replace([]string) error }); ok {
			require.NoError(t, sv.Replace(nil))
		} else {
			require.NoError(t, f.Value.Set(f.DefValue))
		}
		f.Changed = false
	})
}

func TestParseAccessGrants(t *testing.T) {
	cases := []struct {
		name    string
		values  []string
		want    []accessGrant
		wantErr error
	}{
		{name: "anyone", values: []string{"anyone"}, want: []accessGrant{{Anyone: true}}},
		{name: "user id", values: []string{"user=user_01ABC"}, want: []accessGrant{{UserID: "user_01ABC"}}},
		{name: "audience", values: []string{"audience=arn:audience:acct_acme:aud1"}, want: []accessGrant{{Audience: "arn:audience:acct_acme:aud1"}}},
		{name: "org by account name", values: []string{"org=acme-corp"}, want: []accessGrant{{Org: "acme-corp"}}},
		{name: "several subjects", values: []string{"user=u1", "org=acme-corp"}, want: []accessGrant{{UserID: "u1"}, {Org: "acme-corp"}}},
		{name: "unknown subject", values: []string{"everyone"}, wantErr: errAccessInvalidSubject("web", "everyone")},
		{name: "empty user id", values: []string{"user="}, wantErr: errAccessInvalidSubject("web", "user=")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAccessGrants("web", tc.values)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func accessTestHandler(t *testing.T, stored interfaceAccess, putBody *[]byte) http.Handler {
	t.Helper()
	deployments := []agentDeployment{{ID: "dep-1", Name: "support-bp", DisplayName: "support-bot", Status: "active"}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/deployments":
			jsonHandler(http.StatusOK, listDeploymentsResponse{Deployments: deployments, Count: 1})(w, r)
		case r.URL.Path == "/api/v1/deployments/dep-1/interface-access" && r.Method == http.MethodGet:
			jsonHandler(http.StatusOK, stored)(w, r)
		case r.URL.Path == "/api/v1/deployments/dep-1/interface-access" && r.Method == http.MethodPut:
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			*putBody = body
			jsonHandler(http.StatusOK, interfaceAccess{
				Web:   &interfaceGrants{Grants: []accessGrant{{Org: "acct_acme"}}},
				Slack: &interfaceGrants{Grants: []accessGrant{{Anyone: true}}},
			})(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func TestRunAgentAccessSetSendsOnlyNamedInterfaces(t *testing.T) {
	var putBody []byte
	setupAgentRedeployTest(t, accessTestHandler(t, interfaceAccess{}, &putBody))
	setAgentTargetName(t, agentAccessSetCmd, "support-bot")
	setAccessFlag(t, "web", "org=acme-corp")
	setAccessFlag(t, "slack", "anyone")

	out, err := runWithOutput(t, agentAccessSetCmd, runAgentAccessSet)
	require.NoError(t, err)

	assert.JSONEq(t, `{"web":{"grants":[{"org":"acme-corp"}]},"slack":{"grants":[{"anyone":true}]}}`, string(putBody),
		"the PUT names only the interfaces given on the command line, so custom keeps its grants")
	assert.Contains(t, out, "org=acme-corp", "the server returns the account ID, and the output prints its name")
}

func TestRunAgentAccessPrintsEachInterface(t *testing.T) {
	stored := interfaceAccess{
		Web:    &interfaceGrants{Grants: []accessGrant{{Org: "acct_acme"}, {UserID: "user_01ABC"}}},
		Slack:  &interfaceGrants{Grants: []accessGrant{{Anyone: true}}},
		Custom: &interfaceGrants{Grants: []accessGrant{}},
	}
	setupAgentRedeployTest(t, accessTestHandler(t, stored, new([]byte)))
	setAgentTargetName(t, agentAccessGetCmd, "support-bot")

	out, err := runWithOutput(t, agentAccessGetCmd, runAgentAccess)
	require.NoError(t, err)

	assert.Regexp(t, `web\s+org=acme-corp, user=user_01ABC`, out)
	assert.Regexp(t, `slack\s+anyone`, out)
	assert.Regexp(t, `custom\s+`+regexp.QuoteMeta(msgAccessNoGrants()), out,
		"an interface with no grants falls back to the agent's account, and the output says so")
}
