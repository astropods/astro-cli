package chatui

import (
	"encoding/json"
	"log"
	"net/http"
)

// The JSON shapes below mirror the subset of astro-server responses the chat
// shell reads. They intentionally match the chat client's TypeScript response
// interfaces (DeploymentsSummaryResponse, WebChatDeployment,
// DeploymentsListResponse, AgentDeploymentSummary, DeploymentStatus,
// DeploymentRuntime, DeploymentConnectionsResponse) so the embedded
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
	// WebChatDeployments are the agents the viewer may chat with on the web.
	// Always sent: the chat client reads a missing list as a server that
	// doesn't say, and falls back to every agent with web chat on.
	WebChatDeployments []webChatDeployment `json:"web_chat_deployments"`
}

type deploymentSummaryItem struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	DisplayName            string `json:"display_name,omitempty"`
	Status                 string `json:"status"`
	MessagingWebConfigured bool   `json:"messaging_web_configured"`
}

type webChatDeployment struct {
	deploymentSummaryItem
	CanReadDeployment bool `json:"can_read_deployment"`
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

type deploymentConnectionsResponse struct {
	PersonalAccount string `json:"personal_account"`
	Connections     []any  `json:"connections"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("chatui: encode response: %v", err)
	}
}
