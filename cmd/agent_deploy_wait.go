package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type serviceEndpointInfo struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Type    string `json:"type,omitempty"`
	Ready   bool   `json:"ready,omitempty"`
	Message string `json:"message,omitempty"`
}

type agentDeploymentFull struct {
	CurrentRevision    int                   `json:"current_revision"`
	ErrorMessage       string                `json:"error_message,omitempty"`
	ID                 string                `json:"id"`
	Name               string                `json:"name"`
	DisplayName        string                `json:"display_name,omitempty"`
	BuildID            string                `json:"build_id"`
	Namespace          string                `json:"namespace"`
	Status             string                `json:"status"`
	CreatedAt          string                `json:"created_at"`
	EnvironmentID      string                `json:"environment_id,omitempty"`
	EnvironmentName    string                `json:"environment_name,omitempty"`
	SourceAccount      string                `json:"source_account,omitempty"`
	ExternalURLs       []serviceEndpointInfo `json:"external_urls,omitempty"`
	MessagingAvailable bool                  `json:"messaging_available,omitempty"`
	Workloads          []workloadDetail      `json:"workloads,omitempty"`
}

type agentDeploymentFullResponse struct {
	Deployment agentDeploymentFull `json:"deployment"`
}

func getAgentDeploymentFull(ctx context.Context, id string, at AccountToken, verbose bool) (*agentDeploymentFull, error) {
	u := fmt.Sprintf("%s/api/v1/deployments/%s", agentBaseURL(), url.PathEscape(id))
	var result agentDeploymentFullResponse
	if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return nil, err
	}
	return &result.Deployment, nil
}

func messagingEndpoint(urls []serviceEndpointInfo) *serviceEndpointInfo {
	for i := range urls {
		if urls[i].Type == "messaging" {
			return &urls[i]
		}
	}
	return nil
}
