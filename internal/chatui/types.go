package chatui

import (
	"encoding/json"
	"log"
	"net/http"
)

// The JSON shapes below mirror the subset of astro-server responses the chat
// shell reads. They intentionally match the chat client's TypeScript response
// interfaces (DeploymentsSummaryResponse, DeploymentsListResponse,
// AgentDeploymentSummary, DeploymentStatus, DeploymentRuntime) so the embedded
// client deserializes them unchanged.

type deploymentsSummaryResponse struct {
	Accounts []accountDeploymentsSummary `json:"accounts"`
}

type accountDeploymentsSummary struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Type        string                  `json:"type"`
	DisplayName string                  `json:"display_name"`
	Deployments []deploymentSummaryItem `json:"deployments"`
}

type deploymentSummaryItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Status      string `json:"status"`
}

type deploymentsListResponse struct {
	Deployments []agentDeploymentSummary `json:"deployments"`
	Count       int                      `json:"count"`
}

type agentDeploymentSummary struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	DisplayName            string `json:"display_name,omitempty"`
	BuildID                string `json:"build_id"`
	MessagingWebConfigured bool   `json:"messaging_web_configured"`
	CreatedAt              string `json:"created_at"`
}

type deploymentStatus struct {
	Value   string `json:"value"`
	Reason  string `json:"reason"`
	Details string `json:"details"`
}

type deploymentRuntimeResponse struct {
	Runtime deploymentRuntime `json:"runtime"`
}

type deploymentRuntime struct {
	Ready              int  `json:"ready"`
	Replicas           int  `json:"replicas"`
	MessagingReachable bool `json:"messaging_reachable"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("chatui: encode response: %v", err)
	}
}
