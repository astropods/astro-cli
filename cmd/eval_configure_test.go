package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setEvalConfigureFlag(t *testing.T, name, value string) {
	t.Helper()
	require.NoError(t, evalConfigureCmd.Flags().Set(name, value))
	t.Cleanup(func() {
		f := evalConfigureCmd.Flags().Lookup(name)
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

func TestEvalConfigure(t *testing.T) {
	dep := map[string]any{
		"id": "dep-abc-123", "name": "coach", "display_name": "coach-dev",
		"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}
	off := datasetAutoAddSettings{MinConfidence: 0.9}
	on := datasetAutoAddSettings{Enabled: true, MinConfidence: 0.8, UpdateOnReevaluation: true}
	const settingsPath = "/api/v1/agents/testaccount/coach/evaluations/dataset-auto-add"
	const getRequest = "GET " + settingsPath + "?deployment_id=dep-abc-123"

	cases := []struct {
		name         string
		flags        map[string]string
		current      datasetAutoAddSettings
		writeStatus  int
		writeBody    any
		wantRequests []string
		wantPutBody  map[string]any
		wantErr      string
		wantOut      []string
	}{
		{name: "no flags prints the settings when auto-add is on", current: on,
			wantRequests: []string{getRequest},
			wantOut:      []string{"Auto-add to dataset:      on", "Minimum confidence:       80%", "Update on re-evaluation:  on"}},
		{name: "no flags prints only the switch when auto-add is off", current: off,
			wantRequests: []string{getRequest},
			wantOut:      []string{"Auto-add to dataset:      off"}},
		{name: "turning auto-add on starts from the defaults", current: off,
			flags:        map[string]string{"auto-add": "on"},
			writeBody:    datasetAutoAddSettings{Enabled: true, MinConfidence: 0.9},
			wantRequests: []string{getRequest, "PUT " + settingsPath},
			wantPutBody:  map[string]any{"deployment_id": "dep-abc-123", "min_confidence": 0.9, "update_on_reevaluation": false},
			wantOut:      []string{"Auto-add to dataset:      on", "Minimum confidence:       90%"}},
		{name: "turning auto-add on applies the settings passed with it", current: off,
			flags:        map[string]string{"auto-add": "on", "auto-add-min-confidence": "0.75", "auto-add-update-on-reevaluation": "true"},
			writeBody:    datasetAutoAddSettings{Enabled: true, MinConfidence: 0.75, UpdateOnReevaluation: true},
			wantRequests: []string{getRequest, "PUT " + settingsPath},
			wantPutBody:  map[string]any{"deployment_id": "dep-abc-123", "min_confidence": 0.75, "update_on_reevaluation": true}},
		{name: "changing the minimum keeps the saved update setting", current: on,
			flags:        map[string]string{"auto-add-min-confidence": "0.95"},
			writeBody:    datasetAutoAddSettings{Enabled: true, MinConfidence: 0.95, UpdateOnReevaluation: true},
			wantRequests: []string{getRequest, "PUT " + settingsPath},
			wantPutBody:  map[string]any{"deployment_id": "dep-abc-123", "min_confidence": 0.95, "update_on_reevaluation": true}},
		{name: "update flag set to false turns updates off and keeps the minimum", current: on,
			flags:        map[string]string{"auto-add-update-on-reevaluation": "false"},
			writeBody:    datasetAutoAddSettings{Enabled: true, MinConfidence: 0.8},
			wantRequests: []string{getRequest, "PUT " + settingsPath},
			wantPutBody:  map[string]any{"deployment_id": "dep-abc-123", "min_confidence": 0.8, "update_on_reevaluation": false}},
		{name: "turning auto-add off deletes the setting", current: on,
			flags:        map[string]string{"auto-add": "off"},
			writeBody:    off,
			wantRequests: []string{getRequest, "DELETE " + settingsPath + "?deployment_id=dep-abc-123"},
			wantOut:      []string{"Auto-add to dataset:      off"}},
		{name: "json output prints the saved settings", current: on,
			flags:        map[string]string{"json": "true"},
			wantRequests: []string{getRequest},
			wantOut:      []string{`"min_confidence": 0.8`}},
		{name: "a setting flag while auto-add is off is rejected before writing", current: off,
			flags:        map[string]string{"auto-add-min-confidence": "0.8"},
			wantRequests: []string{getRequest},
			wantErr:      errEvalConfigureAutoAddIsOff("coach-dev").Error()},
		{name: "a server validation error is reported", current: on,
			flags:        map[string]string{"auto-add-min-confidence": "0.5"},
			writeStatus:  http.StatusBadRequest,
			writeBody:    map[string]any{"error": "min_confidence must be a number from 0 to 1"},
			wantRequests: []string{getRequest, "PUT " + settingsPath},
			wantErr:      errEvalConfigureInvalid("min_confidence must be a number from 0 to 1").Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var requests []string
			var putBody map[string]any
			setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/dataset-auto-add") {
					jsonHandler(http.StatusOK, listPayload)(w, r)
					return
				}
				request := r.Method + " " + r.URL.Path
				if r.URL.RawQuery != "" {
					request += "?" + r.URL.RawQuery
				}
				requests = append(requests, request)
				switch r.Method {
				case http.MethodGet:
					jsonHandler(http.StatusOK, tc.current)(w, r)
				default:
					if r.Method == http.MethodPut {
						_ = json.NewDecoder(r.Body).Decode(&putBody)
					}
					status := tc.writeStatus
					if status == 0 {
						status = http.StatusOK
					}
					jsonHandler(status, tc.writeBody)(w, r)
				}
			}))
			evalServerURLOverride = agentServerURLOverride
			t.Cleanup(func() { evalServerURLOverride = "" })

			setAgentTargetName(t, evalConfigureCmd, "coach")
			for name, value := range tc.flags {
				setEvalConfigureFlag(t, name, value)
			}
			buf := &bytes.Buffer{}
			evalConfigureCmd.SetOut(buf)
			evalConfigureCmd.SetContext(context.Background())

			err := runEvalConfigure(evalConfigureCmd, nil)

			assert.Equal(t, tc.wantRequests, requests, "settings requests in order")
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.wantPutBody != nil {
				assert.Equal(t, tc.wantPutBody, putBody)
			}
			for _, line := range tc.wantOut {
				assert.Contains(t, buf.String(), line)
			}
		})
	}
}

func TestEvalConfigureRejectsInvalidFlagsBeforeCallingTheServer(t *testing.T) {
	cases := []struct {
		name    string
		flags   map[string]string
		wantErr string
	}{
		{name: "auto-add value other than on or off",
			flags: map[string]string{"auto-add": "yes"}, wantErr: errEvalConfigureAutoAddValue("yes").Error()},
		{name: "minimum above 1",
			flags: map[string]string{"auto-add-min-confidence": "1.5"}, wantErr: errEvalConfigureMinConfidence("1.5").Error()},
		{name: "minimum below 0",
			flags: map[string]string{"auto-add-min-confidence": "-0.1"}, wantErr: errEvalConfigureMinConfidence("-0.1").Error()},
		{name: "minimum that is not a number",
			flags: map[string]string{"auto-add-min-confidence": "high"}, wantErr: errEvalConfigureMinConfidence("high").Error()},
		{name: "minimum that is NaN",
			flags: map[string]string{"auto-add-min-confidence": "NaN"}, wantErr: errEvalConfigureMinConfidence("NaN").Error()},
		{name: "off with a minimum",
			flags: map[string]string{"auto-add": "off", "auto-add-min-confidence": "0.8"}, wantErr: errEvalConfigureOffWithSettings().Error()},
		{name: "off with the update flag",
			flags: map[string]string{"auto-add": "off", "auto-add-update-on-reevaluation": "true"}, wantErr: errEvalConfigureOffWithSettings().Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				jsonHandler(http.StatusOK, nil)(w, r)
			}))
			evalServerURLOverride = agentServerURLOverride
			t.Cleanup(func() { evalServerURLOverride = "" })

			setAgentTargetName(t, evalConfigureCmd, "coach")
			for name, value := range tc.flags {
				setEvalConfigureFlag(t, name, value)
			}
			evalConfigureCmd.SetContext(context.Background())

			require.EqualError(t, runEvalConfigure(evalConfigureCmd, nil), tc.wantErr)
			assert.False(t, called, "an invalid flag is rejected before any request")
		})
	}
}

func TestParseMinConfidenceAcceptsAnyNumberFromZeroToOne(t *testing.T) {
	for value, want := range map[string]float64{"0": 0, "1": 1, "0.9": 0.9, "0.75": 0.75, ".5": 0.5, "0.855": 0.855, "9e-1": 0.9} {
		got, err := parseMinConfidence(value)
		require.NoError(t, err, value)
		assert.InDelta(t, want, got, 1e-9, value)
	}
}

func TestEvalConfigureRejectsPositionalArgs(t *testing.T) {
	require.EqualError(t, agentTargetArgs(evalConfigureCmd, []string{"coach"}), errAgentUnexpectedArgument("coach").Error())
}
