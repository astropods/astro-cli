package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/fatih/color"
)

type serviceEndpointInfo struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Type    string `json:"type,omitempty"`
	Ready   bool   `json:"ready,omitempty"`
	Message string `json:"message,omitempty"`
}

type agentDeploymentFull struct {
	ID                 string                `json:"id"`
	Name               string                `json:"name"`
	DisplayName        string                `json:"display_name,omitempty"`
	EvalDatasetID      string                `json:"eval_dataset_id,omitempty"`
	BuildID            string                `json:"build_id"`
	Namespace          string                `json:"namespace"`
	Status             string                `json:"status"`
	CreatedAt          string                `json:"created_at"`
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

func pollDeploymentPublicURL(ctx context.Context, deploymentID string, at AccountToken, verbose bool, w io.Writer) error {
	const (
		pollInterval = 3 * time.Second
		timeout      = 15 * time.Minute
	)
	deadline := time.Now().Add(timeout)
	lastMessage := ""

	for time.Now().Before(deadline) {
		dep, err := getAgentDeploymentFull(ctx, deploymentID, at, verbose)
		if err == nil {
			messaging := messagingEndpoint(dep.ExternalURLs)
			if messaging == nil {
				return nil
			}
			if messaging.Ready {
				green := color.New(color.FgGreen)
				green.Fprintf(w, "\n  ✓ %s\n", messaging.URL) //nolint:errcheck,gosec
				return nil
			}
			if messaging.Message != "" && messaging.Message != lastMessage {
				lastMessage = messaging.Message
				fmt.Fprintf(w, "\n  %s%s%s", colorDim, messaging.Message, colorReset) //nolint:errcheck,gosec
			} else if lastMessage == "" {
				fmt.Fprint(w, ".") //nolint:errcheck,gosec
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	dep, err := getAgentDeploymentFull(ctx, deploymentID, at, verbose)
	if err == nil {
		if messaging := messagingEndpoint(dep.ExternalURLs); messaging != nil && messaging.URL != "" {
			yellow := color.New(color.FgYellow)
			yellow.Fprintf(w, "\n  %s\n", msgDeployURLNotReady(messaging.URL, messaging.Message)) //nolint:errcheck,gosec
		}
	}
	return nil
}
