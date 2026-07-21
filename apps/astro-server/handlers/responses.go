package handlers

import (
	"encoding/json"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/org"
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
	Agents  []AgentResponse `json:"agents"`
	Count   int             `json:"count"`
	Limit   int             `json:"limit"`
	Offset  int             `json:"offset"`
	HasMore bool            `json:"has_more"`
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
	Deployments []AgentDeploymentSummary `json:"deployments"`
	Count       int                      `json:"count"`
}

// GetDeploymentDetailResponse wraps the DB-only deployment record returned by
// GET /deployments/:id. Live K8s state is served separately by the runtime
// endpoint and surfaced as GetDeploymentRuntimeResponse.
type GetDeploymentDetailResponse struct {
	Deployment DeploymentRecord `json:"deployment"`
}

// GetDeploymentRuntimeResponse wraps the K8s-sourced runtime view returned by
// GET /deployments/:id/runtime.
type GetDeploymentRuntimeResponse struct {
	Runtime DeploymentRuntime `json:"runtime"`
}

// GetDeploymentStatusResponse is the body returned by GET /deployments/:id/status.
// It's just DeploymentStatus itself — no envelope. The endpoint answers a
// single narrow question (what's the current status?), so the body is the
// status object verbatim.
type GetDeploymentStatusResponse = DeploymentStatus

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
	P95LatencyMs float64 `json:"p95_latency_ms"`
	MinLatencyMs float64 `json:"min_latency_ms"`
	MaxLatencyMs float64 `json:"max_latency_ms"`
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

// DeploymentTraceSummary is the lightweight per-deployment trace count
// projection returned by the bulk obs-summary endpoint.
type DeploymentTraceSummary struct {
	TotalTraces   int       `json:"total_traces"`
	LastTraceAt   string    `json:"last_trace_at"`
	CostUSD       float64   `json:"cost_usd"`
	RequestSeries []int     `json:"request_series"`
	TokenSeries   []int     `json:"token_series"`
	CostSeries    []float64 `json:"cost_series"`
}

// DeploymentSummariesResponse is returned by the bulk deployment summaries endpoint.
type DeploymentSummariesResponse struct {
	Summaries map[string]DeploymentTraceSummary `json:"summaries"`
}

// AccountSummaryPeriod describes the queried time window.
type AccountSummaryPeriod struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
	Days  int    `json:"days"`
}

// AccountSummaryTotals holds aggregate totals for the period.
//
// TotalTokens is the new source of truth: clients should prefer it over
// (InputTokens + OutputTokens). InputTokens/OutputTokens are retained for
// backwards compatibility but may be 0 in views that derive tokens from the
// traces view (which only exposes the combined sum).
type AccountSummaryTotals struct {
	CostUSD      float64 `json:"cost_usd"`
	Requests     int     `json:"requests"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	ActiveAgents int     `json:"active_agents"`
}

// AccountSummaryDailyAvg holds per-day averages for the period.
type AccountSummaryDailyAvg struct {
	CostUSD  float64 `json:"cost_usd"`
	Requests float64 `json:"requests"`
	Tokens   float64 `json:"tokens"`
}

// AccountSummaryChange holds period-over-period % deltas.
// Fields are nil when the prior period value was 0.
type AccountSummaryChange struct {
	CostPct     *float64 `json:"cost_pct"`
	RequestsPct *float64 `json:"requests_pct"`
	TokensPct   *float64 `json:"tokens_pct"`
}

// AccountCostOverTimeEntry is one day's model-level cost breakdown for the chart.
type AccountCostOverTimeEntry struct {
	Date   string             `json:"date"`
	Models []AccountModelCost `json:"models"`
}

// AccountModelCost is a per-model cost contribution within a single day.
type AccountModelCost struct {
	Model   string  `json:"model"`
	CostUSD float64 `json:"cost_usd"`
}

// AccountCostByModelEntry is one row in the aggregate per-model breakdown:
// spend, token volume, request count, and latency percentiles. Powers the
// model-optimization view (see #1374). Latency and request count come from a
// model-grouped observations query; cost and tokens are rolled up from the
// daily metrics.
type AccountCostByModelEntry struct {
	Model        string  `json:"model"`
	CostUSD      float64 `json:"cost_usd"`
	CostPct      float64 `json:"cost_pct"`
	TotalTokens  int     `json:"total_tokens"`
	TokenPct     float64 `json:"token_pct"`
	Requests     int     `json:"requests"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
}

// AccountSparklines holds per-day value arrays (date-ascending) for frontend sparklines.
type AccountSparklines struct {
	Cost     []float64 `json:"cost"`
	Requests []int     `json:"requests"`
	Tokens   []int     `json:"tokens"`
}

// AccountUserCost is a per-user activity contribution within a single day,
// used in the user-grouped cost-over-time view. Carries cost + requests +
// tokens so the client can slice the per-(day, user) data into any range
// window and recompute per-user totals without an extra round-trip.
type AccountUserCost struct {
	UserIdentity
	CostUSD  float64 `json:"cost_usd"`
	Requests int     `json:"requests"`
	Tokens   int     `json:"tokens"`
}

// AccountCostOverTimeByUserEntry is one day's user-level cost breakdown for the chart.
// Only populated when the summary endpoint is called with group_by=user.
type AccountCostOverTimeByUserEntry struct {
	Date  string            `json:"date"`
	Users []AccountUserCost `json:"users"`
}

// AccountObservabilitySummaryResponse is returned by the account-level summary endpoint.
//
// MetricsUnavailable is set when the upstream Langfuse query failed (e.g.
// ClickHouse unreachable). The response carries zero-valued metrics so the
// page can render gracefully instead of erroring; the frontend uses the flag
// to surface a "metrics temporarily unavailable" banner.
type AccountObservabilitySummaryResponse struct {
	Period             AccountSummaryPeriod             `json:"period"`
	Totals             AccountSummaryTotals             `json:"totals"`
	DailyAvg           AccountSummaryDailyAvg           `json:"daily_avg"`
	Change             *AccountSummaryChange            `json:"change,omitempty"`
	CostOverTime       []AccountCostOverTimeEntry       `json:"cost_over_time"`
	CostByModel        []AccountCostByModelEntry        `json:"cost_by_model"`
	Sparklines         AccountSparklines                `json:"sparklines"`
	CostOverTimeByUser []AccountCostOverTimeByUserEntry `json:"cost_over_time_by_user,omitempty"`
	MetricsUnavailable bool                             `json:"metrics_unavailable,omitempty"`
}

// UserAgentRef is one entry in UserSummaryEntry.AgentsUsed. One entry per
// deployment the user has touched — two deployments of the same blueprint
// produce two refs (with identical Name/Account but distinct DeploymentID)
// so the People-tab chip column mirrors the Agents-tab "one row per
// deployment" shape instead of collapsing to one chip per blueprint.
//
// Account is the avatar/route-segment account — the publishing account
// when the deployment was sourced from a public blueprint (SourceAccountID
// set), otherwise the deploying account. Carrying it per entry lets the
// client resolve avatars for both same-account and cross-account (public)
// deployments.
type UserAgentRef struct {
	DeploymentID string `json:"deployment_id"`
	Name         string `json:"name"`
	Account      string `json:"account"`
}

// UserDetailsKind is the discriminator for UserDetails — every row that
// represents a user carries one of these three values.
//   - astro: a WorkOS-shaped user_id (an Astro account). Display name and
//     username may be populated from the user's personal account profile.
//   - slack: a bare-Slack-shaped user_id. Profile + workspace fields may
//     be populated when the Slack directory has the user observed.
//   - unknown: anything else (opaque session tokens, system actors, etc.).
//     No metadata; the frontend renders the raw user_id.
type UserDetailsKind string

const (
	UserDetailsKindAstro   UserDetailsKind = "astro"
	UserDetailsKindSlack   UserDetailsKind = "slack"
	UserDetailsKindUnknown UserDetailsKind = "unknown"
)

// UserDetails is the discriminated union of identity facts for one user row.
// Kind is always set. Shared profile fields may be present for Astro or Slack;
// team and bot state are populated only for Slack directory matches.
type UserDetails struct {
	Kind UserDetailsKind `json:"kind"`
	// Shared profile fields may be populated for Astro or Slack identities.
	DisplayName string `json:"display_name,omitempty"`
	Username    string `json:"username,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	// Slack-only fields.
	TeamID  string `json:"team_id,omitempty"`
	IsBot   bool   `json:"is_bot,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

// UserIdentity is the user_id + user_details pair surfaced by every
// Insights endpoint and trace row. Embedded in UserSummaryEntry,
// AccountUserCost, and used as the element type for users_used_details.
type UserIdentity struct {
	UserID      string      `json:"user_id"`
	UserDetails UserDetails `json:"user_details"`
}

// UserSummaryEntry holds per-user aggregated observability for the users summary.
// Counts traces (one per user-facing request) to match the agent-view "requests"
// unit; tokens are combined (input + output) since the traces view only exposes
// the sum, not the split.
type UserSummaryEntry struct {
	UserIdentity
	Requests   int            `json:"requests"`
	CostUSD    float64        `json:"cost_usd"`
	Tokens     int            `json:"tokens"`
	LastSeen   string         `json:"last_seen,omitempty"` // RFC3339, omitted when no activity bucket
	AgentsUsed []UserAgentRef `json:"agents_used"`
}

// AccountUsersSummaryResponse is returned by the users-summary endpoint.
// See MetricsUnavailable on AccountObservabilitySummaryResponse for semantics.
type AccountUsersSummaryResponse struct {
	Users              []UserSummaryEntry   `json:"users"`
	Period             AccountSummaryPeriod `json:"period"`
	MetricsUnavailable bool                 `json:"metrics_unavailable,omitempty"`
}

// DeploymentDailyCost is one day's total cost for a single deployment.
type DeploymentDailyCost struct {
	Date    string  `json:"date"`
	CostUSD float64 `json:"cost_usd"`
}

// DeploymentDailyRequests is one day's request count for a deployment.
type DeploymentDailyRequests struct {
	Date     string `json:"date"`
	Requests int    `json:"requests"`
}

// DeploymentDailyTokens is one day's token usage for a deployment.
type DeploymentDailyTokens struct {
	Date         string `json:"date"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
}

// DeploymentSummaryEntry holds per-deployment observability totals for the
// deployments-summary endpoint. One entry per row of the Insights table —
// the deployment is the unit of measure, not the agent_name (multi-region
// deployments of the same blueprint show up as separate rows).
//
// TotalTokens is the new source of truth — clients should prefer it. Input/
// OutputTokens stay populated when the daily-metrics fan-out provides them
// (legacy per-deployment endpoint), but are 0 when the batched traces-view
// path supplies tokens (combined-only).
type DeploymentSummaryEntry struct {
	// Deployment identity — DeploymentID is the canonical row key and the
	// target of the Insights row's deep-link to the Monitor tab.
	DeploymentID string `json:"deployment_id"`
	AgentName    string `json:"agent_name"`
	DisplayName  string `json:"display_name,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	// Observability totals for the period.
	Requests         int                       `json:"requests"`
	CostUSD          float64                   `json:"cost_usd"`
	CostPerRequest   float64                   `json:"cost_per_request"`
	InputTokens      int                       `json:"input_tokens"`
	OutputTokens     int                       `json:"output_tokens"`
	TotalTokens      int                       `json:"total_tokens"`
	TokPerRequest    float64                   `json:"tok_per_request"`
	P95LatencyMs     int                       `json:"p95_latency_ms"`
	TopModel         string                    `json:"top_model"`
	CostOverTime     []DeploymentDailyCost     `json:"cost_over_time"`
	RequestsOverTime []DeploymentDailyRequests `json:"requests_over_time"`
	TokensOverTime   []DeploymentDailyTokens   `json:"tokens_over_time"`
	// UsersUsed lists WorkOS user IDs that drove ≥1 trace against this specific
	// deployment in the period. Mirrors agents_used on the users-summary
	// endpoint — the same (userId, tag) → deployment mapping, just inverted.
	UsersUsed []string `json:"users_used"`
	// UsersUsedDetails is the richer, display-ready identity list for the
	// Agents view's "Used by" column. Carries one UserIdentity per resolved
	// identity (post-translation, post-stamp): WorkOS users surface as
	// kind="astro", unlinked observed Slack users surface as kind="slack"
	// with workspace + profile metadata, and unknown ids surface as
	// kind="unknown".
	UsersUsedDetails []UserIdentity `json:"users_used_details,omitempty"`
	// UndeployedAt is set when the deployment has been soft-deleted (status
	// transitioned to 'undeployed' via the undeploy worker). Used by the
	// frontend to render the "Deleted MMM DD" suffix when known. Can be nil
	// even for archived rows — a deployment can be archived (not in the
	// visible list) without having a populated undeployed_at, e.g. status
	// 'undeploying' mid-tear-down. Use IsArchived for the boolean tombstone
	// signal; UndeployedAt is only for the date suffix.
	UndeployedAt *time.Time `json:"undeployed_at,omitempty"`
	// IsArchived is true when this entry corresponds to a deployment that
	// isn't in the currently-visible deployment list (i.e. it was discovered
	// via the Q_tags tombstone pass). The frontend uses this — not
	// UndeployedAt — to decide whether to render the row as a tombstone,
	// since some archived states leave UndeployedAt nil.
	IsArchived bool `json:"is_archived,omitempty"`
}

// AccountDeploymentsSummaryResponse is returned by the deployments-summary endpoint.
// See MetricsUnavailable on AccountObservabilitySummaryResponse for semantics.
type AccountDeploymentsSummaryResponse struct {
	Deployments        []DeploymentSummaryEntry `json:"deployments"`
	Period             AccountSummaryPeriod     `json:"period"`
	MetricsUnavailable bool                     `json:"metrics_unavailable,omitempty"`
}

// InsightsResponse is the server-owned view model for the Insights page.
// It intentionally returns display-ready ranges and table rows so the
// frontend renders instead of re-deriving account membership, Slack identity,
// deployment labels, totals, and chart windows from several lower-level APIs.
// Ranges may be empty when the request sets skip_ranges=true for a table-only
// refresh.
type InsightsResponse struct {
	MetricsUnavailable bool                     `json:"metrics_unavailable,omitempty"`
	Ranges             map[string]InsightsRange `json:"ranges"`
	Tables             InsightsTables           `json:"tables"`
	// DevtoolSources lists the dev-tool sources present in the account (Claude
	// Code, Codex, …), for the Sources filter. Their usage is already folded into
	// the ranges and tables above unless excluded via the hide_sources param.
	DevtoolSources []DevtoolSourceRef `json:"devtool_sources,omitempty"`
}

// DevtoolSourceRef identifies one dev-tool source for the Sources filter.
type DevtoolSourceRef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
}

type InsightsRange struct {
	Days             int                        `json:"days"`
	Period           AccountSummaryPeriod       `json:"period"`
	StatCards        InsightsStatCards          `json:"stat_cards"`
	AgentSpendChart  []AccountCostOverTimeEntry `json:"agent_spend_chart"`
	PeopleSpendChart []InsightsPeopleSpendPoint `json:"people_spend_chart"`
	SeriesLabels     map[string]string          `json:"series_labels"`
}

type InsightsStatCards struct {
	Totals AccountSummaryTotals  `json:"totals"`
	Change *AccountSummaryChange `json:"change,omitempty"`
}

type InsightsPeopleSpendPoint struct {
	Date  string  `json:"date"`
	Users int     `json:"users"`
	Cost  float64 `json:"cost"`
}

type InsightsTables struct {
	Agents InsightsAgentsTable `json:"agents"`
	People InsightsPeopleTable `json:"people"`
}

type InsightsTablePagination struct {
	Limit         int  `json:"limit"`
	Offset        int  `json:"offset"`
	TotalCount    int  `json:"total_count"`
	FilteredCount int  `json:"filtered_count"`
	HasMore       bool `json:"has_more"`
}

type InsightsAgentsTable struct {
	Rows       []InsightsAgentRow      `json:"rows"`
	TotalCost  float64                 `json:"total_cost"`
	Count      int                     `json:"count"`
	Pagination InsightsTablePagination `json:"pagination"`
}

type InsightsPeopleTable struct {
	Rows                     []InsightsPersonRow     `json:"rows"`
	TotalCost                float64                 `json:"total_cost"`
	Count                    int                     `json:"count"`
	MissingSlackDetailsCount int                     `json:"missing_slack_details_count,omitempty"`
	Pagination               InsightsTablePagination `json:"pagination"`
}

type InsightsIdentityRef struct {
	Kind          string       `json:"kind"`
	IdentityKey   string       `json:"identity_key,omitempty"`
	ID            string       `json:"id,omitempty"`
	Label         string       `json:"label"`
	Href          string       `json:"href,omitempty"`
	AvatarAccount string       `json:"avatar_account,omitempty"`
	AvatarName    string       `json:"avatar_name,omitempty"`
	AvatarHandle  string       `json:"avatar_handle,omitempty"`
	UserID        string       `json:"user_id,omitempty"`
	UserDetails   *UserDetails `json:"user_details,omitempty"`
	Tooltip       string       `json:"tooltip,omitempty"`
	Icon          string       `json:"icon,omitempty"` // integration-icon key (dev-tool sources) → themed logo
}

type InsightsAgentChip struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	Href          string `json:"href"`
	AvatarAccount string `json:"avatar_account"`
	AvatarName    string `json:"avatar_name"`
	IsDeleted     bool   `json:"is_deleted,omitempty"`
	Icon          string `json:"icon,omitempty"` // integration-icon key (dev-tool chips) → themed logo
}

type InsightsAgentMetrics struct {
	Requests       int     `json:"requests"`
	CostUSD        float64 `json:"cost_usd"`
	CostPct        float64 `json:"cost_pct"`
	CostPerRequest float64 `json:"cost_per_request"`
	TokPerRequest  float64 `json:"tok_per_request"`
	P95LatencyMs   int     `json:"p95_latency_ms"`
}

type InsightsPersonMetrics struct {
	Requests int     `json:"requests"`
	CostUSD  float64 `json:"cost_usd"`
	CostPct  float64 `json:"cost_pct"`
	Tokens   int     `json:"tokens"`
	LastSeen string  `json:"last_seen,omitempty"`
}

type InsightsAgentRow struct {
	Key             string                `json:"key"`
	SearchText      string                `json:"search_text"`
	Identity        InsightsIdentityRef   `json:"identity"`
	UsedBy          []InsightsIdentityRef `json:"used_by"`
	Metrics         InsightsAgentMetrics  `json:"metrics"`
	NotInstrumented bool                  `json:"not_instrumented,omitempty"`
}

type InsightsPersonRow struct {
	Key        string                `json:"key"`
	SearchText string                `json:"search_text"`
	Identity   InsightsIdentityRef   `json:"identity"`
	AgentsUsed []InsightsAgentChip   `json:"agents_used"`
	Metrics    InsightsPersonMetrics `json:"metrics"`
}

// TraceEntry represents a single trace in the traces list. UserID is the
// raw Langfuse user_id; UserDetails is the resolved discriminated identity
// (nil when the trace has no user attached or when classification can't
// produce useful data). Status remains a compatibility placeholder for legacy
// consumers such as Chat Inspector: Langfuse does not expose a reliable
// trace-level status, so new list consumers must not display or filter on it.
type TraceEntry struct {
	TraceID     string       `json:"trace_id"`
	Name        string       `json:"name"`
	Status      string       `json:"status"`
	LatencyMs   float64      `json:"latency_ms"`
	TotalCost   float64      `json:"total_cost"`
	Timestamp   string       `json:"timestamp"`
	UserID      string       `json:"user_id,omitempty"`
	UserDetails *UserDetails `json:"user_details,omitempty"`
}

// TraceDetail describes a trace's full content, including the conversation
// metadata (session/user/tags/release) plus environment fields used for filtering.
type TraceDetail struct {
	TraceID     string         `json:"trace_id"`
	Name        string         `json:"name"`
	Timestamp   string         `json:"timestamp"`
	LatencyMs   float64        `json:"latency_ms"`
	TotalCost   float64        `json:"total_cost"`
	Input       any            `json:"input"`
	Output      any            `json:"output"`
	SessionID   string         `json:"session_id,omitempty"`
	UserID      string         `json:"user_id,omitempty"`
	UserDetails *UserDetails   `json:"user_details,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Environment string         `json:"environment,omitempty"`
	Release     string         `json:"release,omitempty"`
	Version     string         `json:"version,omitempty"`
}

// ObservationUsage holds token usage for a generation observation.
type ObservationUsage struct {
	Input  int    `json:"input"`
	Output int    `json:"output"`
	Total  int    `json:"total"`
	Unit   string `json:"unit,omitempty"`
}

// Observation describes a single span / generation / event within a trace.
type Observation struct {
	ID              string            `json:"id"`
	ParentID        string            `json:"parent_id,omitempty"`
	Type            string            `json:"type"` // span | generation | event
	Name            string            `json:"name"`
	StartTime       string            `json:"start_time"`
	EndTime         string            `json:"end_time,omitempty"`
	LatencyMs       float64           `json:"latency_ms"`
	Level           string            `json:"level,omitempty"`
	StatusMessage   string            `json:"status_message,omitempty"`
	Input           any               `json:"input,omitempty"`
	Output          any               `json:"output,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
	Cost            float64           `json:"cost,omitempty"`
	Model           string            `json:"model,omitempty"`
	ModelParameters map[string]any    `json:"model_parameters,omitempty"`
	Usage           *ObservationUsage `json:"usage,omitempty"`
}

// Score is a Langfuse evaluation score attached to a trace.
type Score struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Value         float64 `json:"value"`
	StringValue   string  `json:"string_value,omitempty"`
	DataType      string  `json:"data_type,omitempty"`
	Comment       string  `json:"comment,omitempty"`
	ObservationID string  `json:"observation_id,omitempty"`
	Source        string  `json:"source,omitempty"`
	CreatedAt     string  `json:"created_at,omitempty"`
}

// TraceDetailResponse is returned by the trace detail endpoint.
type TraceDetailResponse struct {
	Trace        TraceDetail   `json:"trace"`
	Observations []Observation `json:"observations"`
	Scores       []Score       `json:"scores"`
}

// ObservabilityTracesResponse is returned by the traces endpoint.
type ObservabilityTracesResponse struct {
	Traces       []TraceEntry `json:"traces"`
	Total        int          `json:"total"`
	Limit        int          `json:"limit"`
	Offset       int          `json:"offset"`
	Truncated    bool         `json:"truncated,omitempty"`
	ScannedCount int          `json:"scanned_count,omitempty"`
}

// TraceUserFacet is one user and their trace count for a deployment window.
type TraceUserFacet struct {
	UserID      string       `json:"user_id,omitempty"`
	UserDetails *UserDetails `json:"user_details,omitempty"`
	Count       int          `json:"count"`
}

// TraceUserFacetsResponse is returned by the trace-user facets endpoint.
type TraceUserFacetsResponse struct {
	Users []TraceUserFacet `json:"users"`
}

// --- Network (Beyla eBPF) ---

// DirectionSummary is one direction's aggregate RED metrics over a window.
// Latency fields are nil when no samples landed in the window.
type DirectionSummary struct {
	RequestCount    int64    `json:"request_count"`
	ErrorCount      int64    `json:"error_count"`
	ErrorRate       float64  `json:"error_rate"`
	LatencyP50Ms    *float64 `json:"latency_p50_ms"`
	LatencyP95Ms    *float64 `json:"latency_p95_ms"`
	LatencyP99Ms    *float64 `json:"latency_p99_ms"`
	UniquePeerCount int      `json:"unique_peer_count"`
	BytesTotal      int64    `json:"bytes_total"`
}

// NetworkSummaryResponse is returned by the per-deployment network summary endpoint.
type NetworkSummaryResponse struct {
	Inbound    DirectionSummary `json:"inbound"`
	Outbound   DirectionSummary `json:"outbound"`
	Database   DirectionSummary `json:"database"`
	WindowFrom time.Time        `json:"window_from"`
	WindowTo   time.Time        `json:"window_to"`
}

// NetworkFlow is one peer's aggregate over the requested window.
// StatusCodes is populated only for HTTP directions; keys are "2xx"/"4xx"/"5xx".
type NetworkFlow struct {
	Peer         string           `json:"peer"`
	PeerKind     string           `json:"peer_kind"` // "route" | "address" | "db_system"
	RequestCount int64            `json:"request_count"`
	ErrorCount   int64            `json:"error_count"`
	ErrorRate    float64          `json:"error_rate"`
	LatencyP50Ms *float64         `json:"latency_p50_ms"`
	LatencyP95Ms *float64         `json:"latency_p95_ms"`
	BytesTotal   int64            `json:"bytes_total"`
	StatusCodes  map[string]int64 `json:"status_codes,omitempty"`
}

// NetworkFlowsResponse is returned by the per-deployment network flows endpoint.
type NetworkFlowsResponse struct {
	Direction string        `json:"direction"`
	Flows     []NetworkFlow `json:"flows"`
}

// NetworkPoint is one (timestamp, value) sample in a NetworkSeries.
type NetworkPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// NetworkSeries is one labeled time series. Label is "total" when no group_by
// was requested, or the group-by value (route name, peer address, status class).
type NetworkSeries struct {
	Label  string         `json:"label"`
	Points []NetworkPoint `json:"points"`
}

// NetworkTimeseriesResponse is returned by the per-deployment network timeseries endpoint.
type NetworkTimeseriesResponse struct {
	Direction string          `json:"direction"`
	Metric    string          `json:"metric"`
	Step      string          `json:"step"`
	Series    []NetworkSeries `json:"series"`
}

// --- Deployment Events ---

// K8sEventItem represents a single Kubernetes event for API responses.
//
// Title and Guidance are server-populated, action-oriented copy for event
// reasons we recognize as "the deployment is stuck and the user must act"
// (see humanizeDeploymentEvent). They are empty for events we don't have copy
// for, in which case the UI falls back to the raw Reason/Message.
type K8sEventItem struct {
	Type           string `json:"type"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	ObjectKind     string `json:"object_kind"`
	ObjectName     string `json:"object_name"`
	Count          int32  `json:"count"`
	FirstTimestamp string `json:"first_timestamp"`
	LastTimestamp  string `json:"last_timestamp"`
	Title          string `json:"title,omitempty"`
	Guidance       string `json:"guidance,omitempty"`
	// Severity categorizes a humanized event: "info" (normal progress),
	// "transient" (self-recovering), or "stuck" (needs user action). Empty for
	// events we have no copy for. The client's stuck banner triggers on "stuck".
	Severity string `json:"severity,omitempty"`
}

// DeploymentEventsResponse wraps Kubernetes events for a deployment namespace.
type DeploymentEventsResponse struct {
	Events []K8sEventItem `json:"events"`
}
