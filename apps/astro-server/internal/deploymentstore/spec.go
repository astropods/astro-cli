package deploymentstore

import "encoding/json"

// SourceAccountFromSpec extracts the source.account field from a deployment spec JSON.
//
// Cross-account deployments store the publisher account in
// deployment_spec_json.source.account even when the deployment row itself
// lives under a different (target) account. Any code that needs to look up
// the agent build for a deployment should resolve the source account from
// this field, not from the target account's URL parameter.
//
// Returns empty string when the spec is empty, omits the field, or fails to
// parse; callers should treat that as "legacy / same-account" and fall back
// to their prior behavior.
func SourceAccountFromSpec(specJSON string) string {
	if specJSON == "" || specJSON == "{}" {
		return ""
	}
	var parsed struct {
		Source struct {
			Account string `json:"account"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(specJSON), &parsed); err != nil {
		return ""
	}
	return parsed.Source.Account
}
