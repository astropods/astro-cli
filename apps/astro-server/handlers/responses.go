package handlers

import (
	"encoding/json"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/astropods/astro/apps/astro-server/internal/waitlist"
)

// Response types for endpoints that currently return untyped gin.H maps.
// These enable automatic OpenAPI schema generation via reflection.

// --- Health ---

// HealthResponse is returned by health/readiness endpoints.
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// --- Error ---

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// --- Agents ---

// ListAgentsResponse wraps the agent catalog listing.
type ListAgentsResponse struct {
	Agents []AgentResponse `json:"agents"`
	Count  int             `json:"count"`
}

// RegisterAgentResponse is returned on successful agent registration.
type RegisterAgentResponse struct {
	Message            string                       `json:"message"`
	Account            string                       `json:"account"`
	Name               string                       `json:"name"`
	BuildID            string                       `json:"build_id"`
	ValidationWarnings []deployment.ValidationError `json:"validation_warnings,omitempty"`
}

// SetVisibilityResponse is returned after updating agent visibility.
type SetVisibilityResponse struct {
	Message    string `json:"message"`
	Account    string `json:"account"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
}

// --- Accounts ---

// CheckAccountNameResponse indicates whether an account name is available.
type CheckAccountNameResponse struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// SearchAccountsResponse wraps account search results.
type SearchAccountsResponse struct {
	Results []SearchAccountResult `json:"results"`
}

// RenameAccountResponse is returned after renaming an account.
type RenameAccountResponse struct {
	Message string `json:"message"`
	Name    string `json:"name"`
}

// TransferAgentResponse is returned after transferring an agent to another account.
type TransferAgentResponse struct {
	Message       string `json:"message"`
	Agent         string `json:"agent"`
	SourceAccount string `json:"source_account"`
	TargetAccount string `json:"target_account"`
}

// UpdateProfileResponse is returned after updating user profile.
type UpdateProfileResponse struct {
	User *ProfileUser `json:"user"`
}

// --- Members ---

// ListMembersResponse wraps the member list.
type ListMembersResponse struct {
	Members []account.AccountMember `json:"members"`
}

// AddMemberResponse wraps a newly-added member.
type AddMemberResponse struct {
	Member account.AccountMember `json:"member"`
}

// MessageResponse is a simple success message.
type MessageResponse struct {
	Message string `json:"message"`
}

// --- Invitations ---

// ListInvitationsResponse wraps the invitation list.
type ListInvitationsResponse struct {
	Invitations []org.Invitation `json:"invitations"`
}

// BulkInvitationResponse wraps bulk invitation results.
type BulkInvitationResponse struct {
	Results []org.InviteResult `json:"results"`
}

// --- Waitlist ---

// WaitlistEntryResponse re-exports waitlist.Entry for OpenAPI schema generation.
type WaitlistEntryResponse = waitlist.Entry

// --- Deployments ---

// DeployResponse re-exports deployment.DeployResponse for OpenAPI schema generation.
type DeployResponseAlias = deployment.DeployResponse

// UndeployResponse re-exports deployment.UndeployResponse for OpenAPI schema generation.
type UndeployResponseAlias = deployment.UndeployResponse

// ValidateDeploymentResponse is returned when a deployment spec passes validation.
type ValidateDeploymentResponse struct {
	Valid   bool   `json:"valid"`
	Name    string `json:"name"`
	BuildID string `json:"build_id"`
}

// RestartPodResponse is returned after triggering a pod restart.
type RestartPodResponse struct {
	Status string `json:"status"`
	Pod    string `json:"pod"`
}

// RestartDeploymentResponse is returned after restarting all pods in a deployment.
type RestartDeploymentResponse struct {
	Status string   `json:"status"`
	Pods   []string `json:"pods"`
}

// TriggerIngestionResponse is returned after triggering an ingestion job.
type TriggerIngestionResponse struct {
	Status    string `json:"status"`
	JobName   string `json:"job_name"`
	Namespace string `json:"namespace"`
}

// ListDeploymentsResponse wraps the deployments list.
type ListDeploymentsResponse struct {
	Deployments []AgentDeployment `json:"deployments"`
	Count       int               `json:"count"`
}

// GetDeploymentDetailResponse wraps a single deployment for the detail endpoint.
type GetDeploymentDetailResponse struct {
	Deployment AgentDeployment `json:"deployment"`
}

// ActiveDeploymentResponse is returned by the active deployment endpoint.
type ActiveDeploymentResponse struct {
	ID         string          `json:"id"`
	AgentName  string          `json:"agent_name"`
	BuildID    string          `json:"build_id"`
	Namespace  string          `json:"namespace"`
	Status     string          `json:"status"`
	DeployedAt time.Time       `json:"deployed_at"`
	Spec       json.RawMessage `json:"spec"`
}

// DeploymentHistoryRecord represents a single entry in deployment history.
type DeploymentHistoryRecord struct {
	ID           string          `json:"id"`
	AgentName    string          `json:"agent_name"`
	BuildID      string          `json:"build_id"`
	Namespace    string          `json:"namespace"`
	Status       string          `json:"status"`
	DeployedAt   time.Time       `json:"deployed_at"`
	UndeployedAt *time.Time      `json:"undeployed_at,omitempty"`
	Spec         json.RawMessage `json:"spec"`
}

// DeploymentHistoryResponse wraps deployment history records.
type DeploymentHistoryResponse struct {
	Deployments []DeploymentHistoryRecord `json:"deployments"`
	Count       int                       `json:"count"`
}

// ConfigMapDataResponse is returned by the ConfigMap data endpoint.
type ConfigMapDataResponse struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Data      map[string]string `json:"data"`
}

// SecretKeysResponse is returned by the Secret keys endpoint.
type SecretKeysResponse struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Keys      []string `json:"keys"`
}

// --- Observability ---

// MetricsBucket represents a single time bucket of metrics.
type MetricsBucket struct {
	Timestamp    string  `json:"timestamp"`
	TraceCount   int     `json:"trace_count"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	ErrorCount   int     `json:"error_count"`
}

// MetricsTimeRange represents a time range.
type MetricsTimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ObservabilityMetricsResponse is returned by the metrics endpoint.
type ObservabilityMetricsResponse struct {
	Buckets         []MetricsBucket  `json:"buckets"`
	TimeRange       MetricsTimeRange `json:"time_range"`
	IntervalMinutes int              `json:"interval_minutes"`
}

// ObservabilitySummaryMetrics holds the computed summary metrics.
type ObservabilitySummaryMetrics struct {
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	P95LatencyMs  float64 `json:"p95_latency_ms"`
	TotalTokens   int     `json:"total_tokens"`
	ErrorRate     float64 `json:"error_rate"`
	TracesPerHour float64 `json:"traces_per_hour"`
}

// ObservabilitySummaryResponse is returned by the summary endpoint.
type ObservabilitySummaryResponse struct {
	TotalTraces int                         `json:"total_traces"`
	TimeRange   MetricsTimeRange            `json:"time_range"`
	Metrics     ObservabilitySummaryMetrics `json:"metrics"`
}

// AccountObservabilitySummaryResponse is returned by the account-level summary endpoint.
type AccountObservabilitySummaryResponse struct {
	TotalTraces  int              `json:"total_traces"`
	InputTokens  int              `json:"input_tokens"`
	OutputTokens int              `json:"output_tokens"`
	TimeRange    MetricsTimeRange `json:"time_range"`
}

// TraceEntry represents a single trace in the traces list.
type TraceEntry struct {
	TraceID   string  `json:"trace_id"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	LatencyMs float64 `json:"latency_ms"`
	Input     string  `json:"input"`
	Output    string  `json:"output"`
	Timestamp string  `json:"timestamp"`
}

// ObservabilityTracesResponse is returned by the traces endpoint.
type ObservabilityTracesResponse struct {
	Traces []TraceEntry `json:"traces"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}
