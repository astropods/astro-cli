// Code generated manually to match proto/admin/v1/admin.proto.
// Uses JSON-over-gRPC encoding registered under the "json" content-subtype.
// Clients must opt in via grpc.CallContentSubtype("json"); see
// apps/astro-queen/internal/client/client.go.
// Hand-maintained: there is no buf.gen.yaml, so editing the .proto does not
// produce this file. Keep the structs and their JSON tags in step by hand.

package adminv1

import (
	"encoding/json"

	"google.golang.org/grpc/encoding"
)

func init() {
	encoding.RegisterCodec(jsonCodec{})
}

// jsonCodec marshals gRPC messages as JSON. Registered under name "json" so it
// never shadows gRPC's default proto codec — that collision previously broke
// the BuildKit session's grpc_health_v1 server in Docker Desktop 4.77.0
// because every gRPC server in our binary inherited the JSON codec.
type jsonCodec struct{}

func (jsonCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (jsonCodec) Name() string { return "json" }

// Message types — mirrors proto/admin/v1/admin.proto

type ListDeploymentsRequest struct {
	Namespace string `json:"namespace,omitempty"`
}

type AdminDeployment struct {
	Name              string              `json:"name,omitempty"`
	BuildID           string              `json:"build_id,omitempty"`
	Namespace         string              `json:"namespace,omitempty"`
	Status            string              `json:"status,omitempty"`
	CreatedAt         string              `json:"created_at,omitempty"`
	AccountName       string              `json:"account_name,omitempty"`
	Components        []string            `json:"components,omitempty"`
	DeploymentID      string              `json:"deployment_id,omitempty"`
	ErrorMessage      string              `json:"error_message,omitempty"`
	ErrorDetails      []map[string]string `json:"error_details,omitempty"`
	StatusChangedAt   string              `json:"status_changed_at,omitempty"`
	CurrentRevision   int32               `json:"current_revision,omitempty"`
	OwnerEmail        string              `json:"owner_email,omitempty"`
	ClusterID         string              `json:"cluster_id,omitempty"`
	AccountClusterID  string              `json:"account_cluster_id,omitempty"`
	PlacementMismatch bool                `json:"placement_mismatch,omitempty"`
	AccountId         string              `json:"account_id,omitempty"`
}

type AdminDeploymentEvent struct {
	Status    string `json:"status,omitempty"`
	Message   string `json:"message,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type AdminDeploymentRevision struct {
	Revision  int32  `json:"revision,omitempty"`
	BuildID   string `json:"build_id,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type ListDeploymentsResponse struct {
	Deployments []*AdminDeployment `json:"deployments,omitempty"`
	Count       int32              `json:"count,omitempty"`
}

type GetClusterStatusRequest struct {
	Namespace string `json:"namespace,omitempty"`
}

type K8sDeploymentInfo struct {
	Name              string            `json:"name,omitempty"`
	Namespace         string            `json:"namespace,omitempty"`
	Replicas          int32             `json:"replicas,omitempty"`
	ReadyReplicas     int32             `json:"ready_replicas,omitempty"`
	AvailableReplicas int32             `json:"available_replicas,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	CreatedAt         string            `json:"created_at,omitempty"`
}

type K8sContainerStatus struct {
	Name         string `json:"name,omitempty"`
	Ready        bool   `json:"ready,omitempty"`
	RestartCount int32  `json:"restart_count,omitempty"`
	State        string `json:"state,omitempty"` // e.g. "Running", "Waiting: CrashLoopBackOff", "Terminated: OOMKilled"
	Image        string `json:"image,omitempty"`
	Init         bool   `json:"init,omitempty"` // from initContainers (includes native sidecars)
}

type K8sContainerResources struct {
	Name            string              `json:"name,omitempty"`
	RequestCPU      string              `json:"request_cpu,omitempty"`
	RequestMemory   string              `json:"request_memory,omitempty"`
	LimitCPU        string              `json:"limit_cpu,omitempty"`
	LimitMemory     string              `json:"limit_memory,omitempty"`
	Security        *K8sSecurityContext `json:"security,omitempty"`
	VolumeMounts    []*K8sVolumeMount   `json:"volume_mounts,omitempty"`
	EnvFrom         []string            `json:"env_from,omitempty"` // "configmap:name" or "secret:name"
	ImagePullPolicy string              `json:"image_pull_policy,omitempty"`
	Init            bool                `json:"init,omitempty"`    // from initContainers
	Sidecar         bool                `json:"sidecar,omitempty"` // init container with restartPolicy: Always
}

type K8sSecurityContext struct {
	RunAsUser                *int64   `json:"run_as_user,omitempty"`
	RunAsNonRoot             *bool    `json:"run_as_non_root,omitempty"`
	ReadOnlyRootFilesystem   *bool    `json:"read_only_root_filesystem,omitempty"`
	AllowPrivilegeEscalation *bool    `json:"allow_privilege_escalation,omitempty"`
	Privileged               *bool    `json:"privileged,omitempty"`
	Capabilities             []string `json:"capabilities,omitempty"`     // dropped capabilities
	AddCapabilities          []string `json:"add_capabilities,omitempty"` // added capabilities
	SeccompProfile           string   `json:"seccomp_profile,omitempty"`  // e.g. "RuntimeDefault"
}

type K8sPodSecurityContext struct {
	RunAsUser      *int64 `json:"run_as_user,omitempty"`
	RunAsGroup     *int64 `json:"run_as_group,omitempty"`
	FSGroup        *int64 `json:"fs_group,omitempty"`
	SeccompProfile string `json:"seccomp_profile,omitempty"`
}

type K8sVolumeMount struct {
	Name      string `json:"name,omitempty"`
	MountPath string `json:"mount_path,omitempty"`
	ReadOnly  bool   `json:"read_only,omitempty"`
	SubPath   string `json:"sub_path,omitempty"`
}

type K8sEventInfo struct {
	Name           string `json:"name,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	Type           string `json:"type,omitempty"` // Normal or Warning
	Reason         string `json:"reason,omitempty"`
	Message        string `json:"message,omitempty"`
	InvolvedObject string `json:"involved_object,omitempty"` // "Kind/Name"
	Count          int32  `json:"count,omitempty"`
	FirstSeen      string `json:"first_seen,omitempty"`
	LastSeen       string `json:"last_seen,omitempty"`
}

type K8sPodInfo struct {
	Name                  string                   `json:"name,omitempty"`
	Namespace             string                   `json:"namespace,omitempty"`
	Phase                 string                   `json:"phase,omitempty"`
	NodeName              string                   `json:"node_name,omitempty"`
	PodIP                 string                   `json:"pod_ip,omitempty"`
	Labels                map[string]string        `json:"labels,omitempty"`
	CreatedAt             string                   `json:"created_at,omitempty"`
	ContainerStatuses     []*K8sContainerStatus    `json:"container_statuses,omitempty"`
	Containers            []*K8sContainerResources `json:"containers,omitempty"`
	Conditions            []string                 `json:"conditions,omitempty"`
	PodSecurity           *K8sPodSecurityContext   `json:"pod_security,omitempty"`
	ServiceAccount        string                   `json:"service_account,omitempty"`
	AutomountServiceToken *bool                    `json:"automount_service_token,omitempty"`
	Volumes               []*K8sVolume             `json:"volumes,omitempty"`
}

type K8sVolume struct {
	Name   string `json:"name,omitempty"`
	Type   string `json:"type,omitempty"`   // "pvc", "configmap", "secret", "emptydir", "projected", etc.
	Source string `json:"source,omitempty"` // PVC name, ConfigMap name, Secret name, etc.
}

type K8sServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port,omitempty"`
	TargetPort string `json:"target_port,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

type K8sServiceInfo struct {
	Name       string            `json:"name,omitempty"`
	Namespace  string            `json:"namespace,omitempty"`
	Type       string            `json:"type,omitempty"`
	ClusterIP  string            `json:"cluster_ip,omitempty"`
	ExternalIP []string          `json:"external_ip,omitempty"`
	Ports      []*K8sServicePort `json:"ports,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	CreatedAt  string            `json:"created_at,omitempty"`
}

type K8sIngressPath struct {
	Path           string `json:"path,omitempty"`
	PathType       string `json:"path_type,omitempty"`
	BackendService string `json:"backend_service,omitempty"`
	BackendPort    string `json:"backend_port,omitempty"`
}

type K8sIngressRule struct {
	Host  string            `json:"host,omitempty"`
	Paths []*K8sIngressPath `json:"paths,omitempty"`
}

type K8sIngressTLS struct {
	Hosts      []string `json:"hosts,omitempty"`
	SecretName string   `json:"secret_name,omitempty"`
}

type K8sIngressInfo struct {
	Name             string            `json:"name,omitempty"`
	Namespace        string            `json:"namespace,omitempty"`
	IngressClassName string            `json:"ingress_class_name,omitempty"`
	Rules            []*K8sIngressRule `json:"rules,omitempty"`
	TLS              []*K8sIngressTLS  `json:"tls,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	CreatedAt        string            `json:"created_at,omitempty"`
}

type K8sNetworkPolicyInfo struct {
	Name              string            `json:"name,omitempty"`
	Namespace         string            `json:"namespace,omitempty"`
	PolicyTypes       []string          `json:"policy_types,omitempty"`
	IngressRuleCount  int32             `json:"ingress_rule_count,omitempty"`
	EgressRuleCount   int32             `json:"egress_rule_count,omitempty"`
	PodSelectorLabels map[string]string `json:"pod_selector_labels,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	CreatedAt         string            `json:"created_at,omitempty"`
}

type ClusterSummary struct {
	TotalDeployments     int32 `json:"total_deployments,omitempty"`
	TotalPods            int32 `json:"total_pods,omitempty"`
	RunningPods          int32 `json:"running_pods,omitempty"`
	PendingPods          int32 `json:"pending_pods,omitempty"`
	FailedPods           int32 `json:"failed_pods,omitempty"`
	TotalServices        int32 `json:"total_services,omitempty"`
	TotalIngresses       int32 `json:"total_ingresses,omitempty"`
	TotalNetworkPolicies int32 `json:"total_network_policies,omitempty"`
	TotalEvents          int32 `json:"total_events,omitempty"`
	WarningEvents        int32 `json:"warning_events,omitempty"`
}

type GetClusterStatusResponse struct {
	Timestamp         string                  `json:"timestamp,omitempty"`
	Namespace         string                  `json:"namespace,omitempty"`
	Deployments       []*K8sDeploymentInfo    `json:"deployments,omitempty"`
	StatefulSets      []*K8sDeploymentInfo    `json:"statefulsets,omitempty"`
	Pods              []*K8sPodInfo           `json:"pods,omitempty"`
	Services          []*K8sServiceInfo       `json:"services,omitempty"`
	Ingresses         []*K8sIngressInfo       `json:"ingresses,omitempty"`
	NetworkPolicies   []*K8sNetworkPolicyInfo `json:"network_policies,omitempty"`
	Events            []*K8sEventInfo         `json:"events,omitempty"`
	Summary           *ClusterSummary         `json:"summary,omitempty"`
	ResolvedClusterID string                  `json:"resolved_cluster_id,omitempty"`
}

type DeleteDeploymentRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
}

type DeleteDeploymentResponse struct {
	Status string `json:"status,omitempty"`
}

type RestartDeploymentRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
	Pod          string `json:"pod,omitempty"`
}

type RestartDeploymentResponse struct {
	Status string `json:"status,omitempty"`
}

type ListAccountsRequest struct{}

type AdminAccount struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	Type          string `json:"type,omitempty"`
	OwnerUserID   string `json:"owner_user_id,omitempty"`
	MemberCount   int32  `json:"member_count,omitempty"`
	HasLangfuse   bool   `json:"has_langfuse"`
	DeletedAt     string `json:"deleted_at,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	ClusterID     string `json:"cluster_id,omitempty"`
	BillingStatus string `json:"billing_status,omitempty"`
}

type ListAccountsResponse struct {
	Accounts []*AdminAccount `json:"accounts,omitempty"`
	Count    int32           `json:"count,omitempty"`
}

type GetAccountRequest struct {
	AccountID string `json:"account_id,omitempty"`
}

type AccountBillingInfo struct {
	Status              string `json:"status,omitempty"`
	Reason              string `json:"reason,omitempty"`
	DunningSince        string `json:"dunning_since,omitempty"`
	AlertActive         bool   `json:"alert_active"`
	UpdatedAt           string `json:"updated_at,omitempty"`
	MetronomeCustomerID string `json:"metronome_customer_id,omitempty"`
	StripeCustomerID    string `json:"stripe_customer_id,omitempty"`
	BifrostCustomerID   string `json:"bifrost_customer_id,omitempty"`
}

type AccountResourceLimit struct {
	Resource string `json:"resource,omitempty"`
	Used     int64  `json:"used"`
	Limit    int64  `json:"limit"`
}

type AccountMemberInfo struct {
	UserID    string `json:"user_id,omitempty"`
	Email     string `json:"email,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	IsOwner   bool   `json:"is_owner"`
}

type GetAccountResponse struct {
	Account           *AdminAccount           `json:"account,omitempty"`
	Billing           *AccountBillingInfo     `json:"billing,omitempty"`
	Limits            []*AccountResourceLimit `json:"limits,omitempty"`
	Members           []*AccountMemberInfo    `json:"members,omitempty"`
	LangfuseProjectID string                  `json:"langfuse_project_id,omitempty"`
}

type GetAccountMetronomeAliasesRequest struct {
	AccountID string `json:"account_id,omitempty"`
}

type RecoverAccountMetronomeAliasesRequest struct {
	AccountID string `json:"account_id,omitempty"`
}

type RegisterAccountMetronomeRequest struct {
	AccountID string `json:"account_id,omitempty"`
}

type RegisterAccountMetronomeResponse struct {
	MetronomeCustomerID string `json:"metronome_customer_id,omitempty"`
}

type RecoverAccountLangfuseRequest struct {
	AccountID string `json:"account_id,omitempty"`
}

type RecoverAccountLangfuseResponse struct {
	LangfuseProjectID string `json:"langfuse_project_id,omitempty"`
}

type RecoverAccountBifrostRequest struct {
	AccountID string `json:"account_id,omitempty"`
}

type RecoverAccountBifrostResponse struct {
	BifrostCustomerID string `json:"bifrost_customer_id,omitempty"`
}

type GetAccountBillingDetailRequest struct {
	AccountID string `json:"account_id,omitempty"`
}

type BillingContract struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	RateCardID   string `json:"rate_card_id,omitempty"`
	StartingAt   string `json:"starting_at,omitempty"`
	EndingBefore string `json:"ending_before,omitempty"`
}

type BillingProvisionJob struct {
	ID          int64  `json:"id,omitempty"`
	State       string `json:"state,omitempty"`
	Attempt     int32  `json:"attempt"`
	CreatedAt   string `json:"created_at,omitempty"`
	FinalizedAt string `json:"finalized_at,omitempty"`
	LastError   string `json:"last_error,omitempty"`
}

type BillingSpend struct {
	Currency         string  `json:"currency,omitempty"`
	CreditRemaining  float64 `json:"credit_remaining"`
	HasCredit        bool    `json:"has_credit"`
	CurrentSpend     float64 `json:"current_spend"`
	CurrentPeriodEnd string  `json:"current_period_end,omitempty"`
	HasCurrentSpend  bool    `json:"has_current_spend"`
	LastInvoiceTotal float64 `json:"last_invoice_total"`
	LastInvoiceAt    string  `json:"last_invoice_at,omitempty"`
	HasLastInvoice   bool    `json:"has_last_invoice"`
}

type BillingCard struct {
	Brand    string `json:"brand,omitempty"`
	Last4    string `json:"last4,omitempty"`
	ExpMonth int32  `json:"exp_month"`
	ExpYear  int32  `json:"exp_year"`
}

type GetAccountBillingDetailResponse struct {
	Billing            *AccountBillingInfo  `json:"billing,omitempty"`
	ProvisionedAt      string               `json:"provisioned_at,omitempty"`
	Enforced           bool                 `json:"enforced"`
	WorkloadsSuspended bool                 `json:"workloads_suspended"`
	Coverage           string               `json:"coverage,omitempty"`
	Contracts          []*BillingContract   `json:"contracts,omitempty"`
	ProvisionJob       *BillingProvisionJob `json:"provision_job,omitempty"`
	Card               *BillingCard         `json:"card,omitempty"`
	Spend              *BillingSpend        `json:"spend,omitempty"`
	MetronomeURL       string               `json:"metronome_url,omitempty"`
	StripeURL          string               `json:"stripe_url,omitempty"`
	Warnings           []string             `json:"warnings,omitempty"`
}

type RetryBillingProvisionRequest struct {
	AccountID string `json:"account_id,omitempty"`
	Force     bool   `json:"force,omitempty"`
}

type RetryBillingProvisionResponse struct {
	Status string `json:"status,omitempty"`
}

type ForceBillingResumeRequest struct {
	AccountID string `json:"account_id,omitempty"`
}

type ForceBillingResumeResponse struct {
	Status string `json:"status,omitempty"`
}

type MetronomeAliasStatus struct {
	Configured bool     `json:"configured"`
	OK         bool     `json:"ok"`
	Expected   []string `json:"expected,omitempty"`
	Actual     []string `json:"actual,omitempty"`
	Missing    []string `json:"missing,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type RenameAccountRequest struct {
	AccountID string `json:"account_id,omitempty"`
	NewName   string `json:"new_name,omitempty"`
}

type RenameAccountResponse struct {
	Status string `json:"status,omitempty"`
}

type GetDeploymentRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
}

type AdminWorkload struct {
	Name          string `json:"name,omitempty"`
	ComponentKind string `json:"component_kind,omitempty"` // agent, model, knowledge, tool, ingestion, interfaces, collector
	ComponentKey  string `json:"component_key,omitempty"`
	WorkloadType  string `json:"workload_type,omitempty"` // deployment, statefulset, cronjob, sidecar
	Image         string `json:"image,omitempty"`
	Replicas      int32  `json:"replicas,omitempty"`
	CPURequest    string `json:"cpu_request,omitempty"`
	MemoryRequest string `json:"memory_request,omitempty"`
	Persistent    bool   `json:"persistent,omitempty"`
}

type ExpectedService struct {
	Name         string `json:"name"`
	Port         int32  `json:"port"`
	TargetPort   int32  `json:"target_port"`
	Protocol     string `json:"protocol"`
	WorkloadName string `json:"workload_name,omitempty"`
}

type ExpectedIngress struct {
	Hostname string `json:"hostname"`
	Path     string `json:"path"`
	Service  string `json:"service"`
}

type AdminVariable struct {
	Name     string   `json:"name,omitempty"`
	Secret   bool     `json:"secret,omitempty"`
	Optional bool     `json:"optional,omitempty"`
	Value    string   `json:"value,omitempty"`
	Targets  []string `json:"targets,omitempty"`
}

type GetDeploymentResponse struct {
	Deployment        *AdminDeployment           `json:"deployment,omitempty"`
	SpecJSON          string                     `json:"spec_json,omitempty"`
	ClusterStatus     *GetClusterStatusResponse  `json:"cluster_status,omitempty"`
	Events            []*AdminDeploymentEvent    `json:"events,omitempty"`
	Revisions         []*AdminDeploymentRevision `json:"revisions,omitempty"`
	Workloads         []*AdminWorkload           `json:"workloads,omitempty"`
	ExpectedServices  []*ExpectedService         `json:"expected_services,omitempty"`
	ExpectedIngresses []*ExpectedIngress         `json:"expected_ingresses,omitempty"`
	Variables         []*AdminVariable           `json:"variables,omitempty"`
	Adapters          []string                   `json:"adapters,omitempty"`
	PlacementHint     string                     `json:"placement_hint,omitempty"`
}

type GetPodLogsRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
	Pod          string `json:"pod,omitempty"` // optional when Loki is configured
	Container    string `json:"container,omitempty"`
	TailLines    int32  `json:"tail_lines,omitempty"`
	SinceUnixNs  int64  `json:"since_unix_ns,omitempty"` // optional start time (Unix nanoseconds)
	UntilUnixNs  int64  `json:"until_unix_ns,omitempty"` // optional end time (Unix nanoseconds)
}

type GetPodLogsResponse struct {
	Logs string `json:"logs,omitempty"`
}

type GetPodEnvRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
	Pod          string `json:"pod,omitempty"`
}

type EnvVar struct {
	Name      string `json:"name,omitempty"`
	Value     string `json:"value,omitempty"`
	ValueFrom string `json:"value_from,omitempty"`
}

type ContainerEnv struct {
	Container string    `json:"container,omitempty"`
	Vars      []*EnvVar `json:"vars,omitempty"`
}

type GetPodEnvResponse struct {
	Containers []*ContainerEnv `json:"containers,omitempty"`
}

type ListAgentsRequest struct{}

type AdminAgent struct {
	AccountName   string `json:"account_name,omitempty"`
	Name          string `json:"name,omitempty"`
	BuildCount    int32  `json:"build_count,omitempty"`
	LatestBuildID string `json:"latest_build_id,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type ListAgentsResponse struct {
	Agents []*AdminAgent `json:"agents,omitempty"`
	Count  int32         `json:"count,omitempty"`
}

type GetAgentBuildsRequest struct {
	AccountName string `json:"account_name,omitempty"`
	AgentName   string `json:"agent_name,omitempty"`
}

type AgentBuild struct {
	BuildID     string `json:"build_id,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type GetAgentBuildsResponse struct {
	Builds []*AgentBuild `json:"builds,omitempty"`
	Count  int32         `json:"count,omitempty"`
}

type ListConnectedDevicesRequest struct{}

type ConnectedDevice struct {
	ID              string `json:"id,omitempty"`
	AccountID       string `json:"account_id,omitempty"`
	UserID          string `json:"user_id,omitempty"`
	DeviceID        string `json:"device_id,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	OS              string `json:"os,omitempty"`
	Arch            string `json:"arch,omitempty"`
	CLIVersion      string `json:"cli_version,omitempty"`
	Status          string `json:"status,omitempty"`
	LastHeartbeatAt string `json:"last_heartbeat_at,omitempty"`
	ConnectedAt     string `json:"connected_at,omitempty"`
	DisconnectedAt  string `json:"disconnected_at,omitempty"`
	AccountName     string `json:"account_name,omitempty"`
}

type ListConnectedDevicesResponse struct {
	Devices []*ConnectedDevice `json:"devices,omitempty"`
	Count   int32              `json:"count,omitempty"`
}

type SendCommandRequest struct {
	DeviceID       string            `json:"device_id,omitempty"`
	Command        string            `json:"command,omitempty"`
	Shell          string            `json:"shell,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutSeconds uint32            `json:"timeout_seconds,omitempty"`
}

type SendCommandResponse struct {
	CommandID string `json:"command_id,omitempty"`
	ExitCode  int32  `json:"exit_code,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
}

type HTTPProxyRequest struct {
	Method  string            `json:"method,omitempty"`
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
}

type HTTPProxyResponse struct {
	StatusCode int32             `json:"status_code,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"`
}

type GetAuthConfigRequest struct{}

type GetAuthConfigResponse struct {
	WorkOSClientID string `json:"workos_client_id,omitempty"`
	WorkOSBaseURL  string `json:"workos_base_url,omitempty"`
}

type StartRiverUIRequest struct{}

type StartRiverUIResponse struct {
	Status string `json:"status,omitempty"`
}

type StopRiverUIRequest struct{}

type StopRiverUIResponse struct {
	Status string `json:"status,omitempty"`
}

type GetRiverUIStatusRequest struct{}

type GetRiverUIStatusResponse struct {
	Running bool `json:"running"`
}

type ListJobKindsRequest struct{}

type JobKindInfo struct {
	Kind       string          `json:"kind"`
	ArgsSchema json.RawMessage `json:"args_schema"`
}

type ListJobKindsResponse struct {
	Kinds []JobKindInfo `json:"kinds"`
}

type TriggerJobRequest struct {
	Kind     string          `json:"kind"`
	ArgsJSON json.RawMessage `json:"args_json"`
}

type TriggerJobResponse struct {
	JobID int64 `json:"job_id"`
}

type GetJobStatesRequest struct{}

type GetJobStatesResponse struct {
	Available int64 `json:"available"`
	Cancelled int64 `json:"cancelled"`
	Completed int64 `json:"completed"`
	Discarded int64 `json:"discarded"`
	Pending   int64 `json:"pending"`
	Retryable int64 `json:"retryable"`
	Running   int64 `json:"running"`
	Scheduled int64 `json:"scheduled"`
}

type AdminJobError struct {
	At      string `json:"at"`
	Attempt int    `json:"attempt"`
	Error   string `json:"error"`
	Trace   string `json:"trace"`
}

type AdminRiverJob struct {
	ID          int64           `json:"id"`
	Args        json.RawMessage `json:"args"`
	Attempt     int             `json:"attempt"`
	AttemptedAt string          `json:"attempted_at,omitempty"`
	CreatedAt   string          `json:"created_at"`
	Errors      []AdminJobError `json:"errors,omitempty"`
	FinalizedAt string          `json:"finalized_at,omitempty"`
	Kind        string          `json:"kind"`
	MaxAttempts int             `json:"max_attempts"`
	Priority    int             `json:"priority"`
	Queue       string          `json:"queue"`
	ScheduledAt string          `json:"scheduled_at"`
	State       string          `json:"state"`
	Tags        []string        `json:"tags,omitempty"`
}

type ListJobsRequest struct {
	State    string   `json:"state,omitempty"`
	Kinds    []string `json:"kinds,omitempty"`
	Queue    string   `json:"queue,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	BeforeID int64    `json:"before_id,omitempty"`
	AnchorID int64    `json:"anchor_id,omitempty"`
}

type ListJobsResponse struct {
	Jobs         []*AdminRiverJob `json:"jobs"`
	NextBeforeID int64            `json:"next_before_id,omitempty"`
	HasMore      bool             `json:"has_more"`
}

type GetJobRequest struct {
	ID int64 `json:"id"`
}

type GetJobResponse struct {
	Job *AdminRiverJob `json:"job"`
}

type CancelJobsRequest struct {
	IDs []int64 `json:"ids"`
}

type CancelJobsResponse struct {
	Cancelled int `json:"cancelled"`
}

type RetryJobsRequest struct {
	IDs []int64 `json:"ids"`
}

type RetryJobsResponse struct {
	Retried int `json:"retried"`
}

type AdminQueue struct {
	Name           string `json:"name"`
	CountAvailable int64  `json:"count_available"`
	CountRunning   int64  `json:"count_running"`
	PausedAt       string `json:"paused_at,omitempty"`
	UpdatedAt      string `json:"updated_at"`
}

type ListAdminQueuesRequest struct{}

type ListAdminQueuesResponse struct {
	Queues []*AdminQueue `json:"queues"`
}

type PauseQueueRequest struct {
	Name string `json:"name"`
}

type PauseQueueResponse struct{}

type ResumeQueueRequest struct {
	Name string `json:"name"`
}

type ResumeQueueResponse struct{}

type QuotaIncreaseRequestProto struct {
	ID              string  `json:"id,omitempty"`
	AccountID       string  `json:"account_id,omitempty"`
	AccountName     string  `json:"account_name,omitempty"`
	FeatureKey      string  `json:"feature_key,omitempty"`
	CurrentUsage    float64 `json:"current_usage,omitempty"`
	CurrentQuota    float64 `json:"current_quota,omitempty"`
	RequestedAmount float64 `json:"requested_amount,omitempty"`
	Reason          string  `json:"reason,omitempty"`
	Status          string  `json:"status,omitempty"`
	RequestedBy     string  `json:"requested_by,omitempty"`
	ResolvedBy      string  `json:"resolved_by,omitempty"`
	ResolvedAt      string  `json:"resolved_at,omitempty"`
	ResolutionNote  string  `json:"resolution_note,omitempty"`
	GrantAmount     float64 `json:"grant_amount,omitempty"`
	CreatedAt       string  `json:"created_at,omitempty"`
}

type ListQuotaIncreaseRequestsRequest struct {
	Status string `json:"status,omitempty"`
}

type ListQuotaIncreaseRequestsResponse struct {
	Requests []*QuotaIncreaseRequestProto `json:"requests,omitempty"`
	Count    int32                        `json:"count,omitempty"`
}

type ApproveQuotaIncreaseRequest struct {
	RequestID   string  `json:"request_id,omitempty"`
	GrantAmount float64 `json:"grant_amount,omitempty"`
	Note        string  `json:"note,omitempty"`
}

type ApproveQuotaIncreaseResponse struct {
	Status string `json:"status,omitempty"`
}

type DenyQuotaIncreaseRequest struct {
	RequestID string `json:"request_id,omitempty"`
	Note      string `json:"note,omitempty"`
}

type DenyQuotaIncreaseResponse struct {
	Status string `json:"status,omitempty"`
}

type GetDeploymentEventsRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
}

type GetDeploymentEventsResponse struct {
	Events []*AdminDeploymentEvent `json:"events,omitempty"`
}

type WakeUpDeploymentRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
}

type WakeUpDeploymentResponse struct {
	Status string `json:"status,omitempty"`
}

type RollbackDeploymentRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
	Revision     int32  `json:"revision,omitempty"`
}

type RollbackDeploymentResponse struct {
	Status string `json:"status,omitempty"`
}

type ReapplyDeploymentRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
}

type ReapplyDeploymentResponse struct {
	Status                  string `json:"status,omitempty"`
	ClusterPlacementUpdated bool   `json:"cluster_placement_updated,omitempty"`
	Message                 string `json:"message,omitempty"`
}

type GetDeploymentJobsRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
}

type DeploymentJob struct {
	Kind        string `json:"kind,omitempty"`
	State       string `json:"state,omitempty"`
	Attempt     int32  `json:"attempt,omitempty"`
	MaxAttempt  int32  `json:"max_attempt,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	AttemptedAt string `json:"attempted_at,omitempty"`
	FinalizedAt string `json:"finalized_at,omitempty"`
	Errors      string `json:"errors,omitempty"`
	ClusterId   string `json:"cluster_id,omitempty"`
	JobId       int64  `json:"job_id,omitempty"`
}

type GetDeploymentJobsResponse struct {
	Jobs []*DeploymentJob `json:"jobs,omitempty"`
}

type ListClusterMigrationsRequest struct {
	MismatchesOnly bool `json:"mismatches_only,omitempty"`
}

type ClusterMigrationEvent struct {
	DeploymentId string `json:"deployment_id,omitempty"`
	AccountName  string `json:"account_name,omitempty"`
	AgentName    string `json:"agent_name,omitempty"`
	Status       string `json:"status,omitempty"`
	Message      string `json:"message,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
}

type ClusterMigrationJob struct {
	JobId           int64  `json:"job_id,omitempty"`
	Kind            string `json:"kind,omitempty"`
	State           string `json:"state,omitempty"`
	DeploymentId    string `json:"deployment_id,omitempty"`
	ArgsJson        string `json:"args_json,omitempty"`
	Errors          string `json:"errors,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	FinalizedAt     string `json:"finalized_at,omitempty"`
	Attempt         int32  `json:"attempt,omitempty"`
	MaxAttempt      int32  `json:"max_attempt,omitempty"`
	DurationMs      int64  `json:"duration_ms,omitempty"`
	AccountName     string `json:"account_name,omitempty"`
	AgentName       string `json:"agent_name,omitempty"`
	SourceClusterId string `json:"source_cluster_id,omitempty"`
	TargetClusterId string `json:"target_cluster_id,omitempty"`
	DeployClusterId string `json:"deploy_cluster_id,omitempty"`
}

type ListClusterMigrationsResponse struct {
	Events        []*ClusterMigrationEvent `json:"events,omitempty"`
	Jobs          []*ClusterMigrationJob   `json:"jobs,omitempty"`
	MismatchCount int32                    `json:"mismatch_count,omitempty"`
}

type RepairNormalizedSpecRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
}

type RepairNormalizedSpecResponse struct {
	Status    string `json:"status,omitempty"`
	Workloads int32  `json:"workloads"`
	Services  int32  `json:"services"`
	Ingresses int32  `json:"ingresses"`
}

type StopDeploymentRequest struct {
	Namespace    string `json:"namespace,omitempty"`
	DeploymentId string `json:"deployment_id,omitempty"`
}

type StopDeploymentResponse struct {
	Status string `json:"status,omitempty"`
}

type ListFeedbackRequest struct{}

type FeedbackSubmission struct {
	ID        string `json:"id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
	Message   string `json:"message,omitempty"`
	PageURL   string `json:"page_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type ListFeedbackResponse struct {
	Submissions []*FeedbackSubmission `json:"submissions,omitempty"`
	Count       int32                 `json:"count,omitempty"`
}

type RegisteredCluster struct {
	ID                     string `json:"id,omitempty"`
	Region                 string `json:"region,omitempty"`
	EKSClusterName         string `json:"eks_cluster_name,omitempty"`
	EKSClusterEndpoint     string `json:"eks_cluster_endpoint,omitempty"`
	Enabled                bool   `json:"enabled,omitempty"`
	IsPrimary              bool   `json:"is_primary,omitempty"`
	CreatedAt              string `json:"created_at,omitempty"`
	UpdatedAt              string `json:"updated_at,omitempty"`
	Healthy                bool   `json:"healthy,omitempty"`
	HealthError            string `json:"health_error,omitempty"`
	AgentIngressDomain     string `json:"agent_ingress_domain,omitempty"`
	IngestionIngressDomain string `json:"ingestion_ingress_domain,omitempty"`
	LangfuseBaseURLExt     string `json:"langfuse_base_url_ext,omitempty"`
	LangfuseVPCEIPs        string `json:"langfuse_vpce_ips,omitempty"`
	PodSubnetCIDRs         string `json:"pod_subnet_cidrs,omitempty"`
	// EKS API server CA in PEM. Captured at registration so per-cluster client
	// builds don't need cross-account DescribeCluster. Empty for is_primary.
	EKSClusterCA       []byte `json:"eks_cluster_ca,omitempty"`
	PodSubnetIPv6CIDRs string `json:"pod_subnet_ipv6_cidrs,omitempty"`
	// Optional per-cluster observability query endpoints. Empty means this
	// cluster is queried through the global LOKI_URL/PROMETHEUS_URL instead.
	LokiURL       string `json:"loki_url,omitempty"`
	PrometheusURL string `json:"prometheus_url,omitempty"`
}

type RegisterClusterRequest struct {
	ID                     string `json:"id,omitempty"`
	Region                 string `json:"region,omitempty"`
	EKSClusterName         string `json:"eks_cluster_name,omitempty"`
	EKSClusterEndpoint     string `json:"eks_cluster_endpoint,omitempty"`
	Enabled                *bool  `json:"enabled,omitempty"` // omitted defaults to true in handler
	AgentIngressDomain     string `json:"agent_ingress_domain,omitempty"`
	IngestionIngressDomain string `json:"ingestion_ingress_domain,omitempty"`
	LangfuseBaseURLExt     string `json:"langfuse_base_url_ext,omitempty"`
	LangfuseVPCEIPs        string `json:"langfuse_vpce_ips,omitempty"`
	PodSubnetCIDRs         string `json:"pod_subnet_cidrs,omitempty"`
	EKSClusterCA           []byte `json:"eks_cluster_ca,omitempty"` // PEM CA bytes; required
	PodSubnetIPv6CIDRs     string `json:"pod_subnet_ipv6_cidrs,omitempty"`
	// Optional — empty means this cluster is queried through the global
	// LOKI_URL/PROMETHEUS_URL instead of its own observability stack.
	LokiURL       string `json:"loki_url,omitempty"`
	PrometheusURL string `json:"prometheus_url,omitempty"`
}

type RegisterClusterResponse struct {
	Cluster *RegisteredCluster `json:"cluster,omitempty"`
}

type EnableClusterRequest struct {
	ID string `json:"id,omitempty"`
}

type EnableClusterResponse struct {
	Cluster *RegisteredCluster `json:"cluster,omitempty"`
}

type DisableClusterRequest struct {
	ID string `json:"id,omitempty"`
}

type DisableClusterResponse struct {
	Cluster *RegisteredCluster `json:"cluster,omitempty"`
}

type DeregisterClusterRequest struct {
	ID string `json:"id,omitempty"`
}

type DeregisterClusterResponse struct{}

// ClusterBlocker is a single account or deployment row still referencing a
// cluster via an ON DELETE RESTRICT foreign key, preventing deregistration.
type ClusterBlocker struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"` // empty for accounts; deployment status otherwise
}

type GetClusterBlockersRequest struct {
	ID string `json:"id,omitempty"`
}

type GetClusterBlockersResponse struct {
	// No omitempty: the frontend reads account_count/accounts/deployment_count/
	// deployments unconditionally, and the common case is one side being zero.
	AccountCount    int32             `json:"account_count"`
	Accounts        []*ClusterBlocker `json:"accounts"`
	DeploymentCount int32             `json:"deployment_count"`
	Deployments     []*ClusterBlocker `json:"deployments"`
}

type ListClustersRequest struct {
	EnabledOnly bool `json:"enabled_only,omitempty"`
}

type ListClustersResponse struct {
	Clusters []*RegisteredCluster `json:"clusters,omitempty"`
}

type SetAccountClusterRequest struct {
	AccountID string `json:"account_id,omitempty"`
	ClusterID string `json:"cluster_id,omitempty"`
}

type SetAccountClusterResponse struct {
	Status    string `json:"status,omitempty"`
	ClusterID string `json:"cluster_id,omitempty"`
}

type MigrateAccountDeploymentsRequest struct {
	AccountID string `json:"account_id,omitempty"`
}

type MigrateAccountDeploymentsResponse struct {
	MigrationsEnqueued int32    `json:"migrations_enqueued,omitempty"`
	DeploymentIds      []string `json:"deployment_ids,omitempty"`
}

type UpdateClusterRequest struct {
	ID                     string `json:"id,omitempty"`
	Region                 string `json:"region,omitempty"`
	EKSClusterName         string `json:"eks_cluster_name,omitempty"`
	EKSClusterEndpoint     string `json:"eks_cluster_endpoint,omitempty"`
	AgentIngressDomain     string `json:"agent_ingress_domain,omitempty"`
	IngestionIngressDomain string `json:"ingestion_ingress_domain,omitempty"`
	LangfuseBaseURLExt     string `json:"langfuse_base_url_ext,omitempty"`
	LangfuseVPCEIPs        string `json:"langfuse_vpce_ips,omitempty"`
	PodSubnetCIDRs         string `json:"pod_subnet_cidrs,omitempty"`
	EKSClusterCA           []byte `json:"eks_cluster_ca,omitempty"` // PEM CA bytes; required
	PodSubnetIPv6CIDRs     string `json:"pod_subnet_ipv6_cidrs,omitempty"`
	// Optional — empty means this cluster is queried through the global
	// LOKI_URL/PROMETHEUS_URL instead of its own observability stack.
	LokiURL       string `json:"loki_url,omitempty"`
	PrometheusURL string `json:"prometheus_url,omitempty"`
}

type UpdateClusterResponse struct {
	Cluster *RegisteredCluster `json:"cluster,omitempty"`
}

type CheckClusterHealthRequest struct {
	ID string `json:"id,omitempty"`
}

type CheckClusterHealthResponse struct {
	Cluster *RegisteredCluster `json:"cluster,omitempty"`
}

type InvalidateAccountCachesRequest struct {
	AccountID string `json:"account_id,omitempty"`
}

type InvalidateAllCachesRequest struct{}

type InvalidateCachesResponse struct {
	AccountsBusted    int32 `json:"accounts_busted,omitempty"`
	DeploymentsBusted int32 `json:"deployments_busted,omitempty"`
}

type ListOutboundDomainsRequest struct {
	Days  int32 `json:"days,omitempty"`
	Limit int32 `json:"limit,omitempty"`
}

type OutboundDomain struct {
	Domain          string   `json:"domain,omitempty"`
	RequestCount    int64    `json:"request_count,omitempty"`
	DeploymentCount int32    `json:"deployment_count,omitempty"`
	Hosts           []string `json:"hosts,omitempty"`
	HostCount       int32    `json:"host_count,omitempty"`
}

type ListOutboundDomainsResponse struct {
	Domains []*OutboundDomain `json:"domains,omitempty"`
	Window  string            `json:"window,omitempty"`
}

type RefreshMessagingCacheRequest struct{}

type RefreshMessagingCacheResponse struct {
	Image   string `json:"image,omitempty"`
	Message string `json:"message,omitempty"`
}

type AlertCondition struct {
	Name        string `json:"name,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Severity    string `json:"severity,omitempty"`
}

type ActiveAlert struct {
	DeploymentID string `json:"deployment_id,omitempty"`
	AgentName    string `json:"agent_name,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	AccountName  string `json:"account_name,omitempty"`
	Workload     string `json:"workload,omitempty"`
	Condition    string `json:"condition,omitempty"`
	Title        string `json:"title,omitempty"`
	Severity     string `json:"severity,omitempty"`
	State        string `json:"state,omitempty"`
	ActiveSince  string `json:"active_since,omitempty"`
	Muted        bool   `json:"muted,omitempty"`
	MutedUntil   string `json:"muted_until,omitempty"`
	LastNotified string `json:"last_notified,omitempty"`
}

type ListAlertsRequest struct{}

type ListAlertsResponse struct {
	Catalog []*AlertCondition `json:"catalog,omitempty"`
	Active  []*ActiveAlert    `json:"active,omitempty"`
}

type ClearAlertRequest struct {
	DeploymentID string `json:"deployment_id,omitempty"`
	Workload     string `json:"workload,omitempty"`
	Condition    string `json:"condition,omitempty"`
}

type ClearAlertResponse struct{}

type MuteAlertRequest struct {
	DeploymentID    string `json:"deployment_id,omitempty"`
	Condition       string `json:"condition,omitempty"`
	DurationSeconds int64  `json:"duration_seconds,omitempty"`
}

type MuteAlertResponse struct{}

type UnmuteAlertRequest struct {
	DeploymentID string `json:"deployment_id,omitempty"`
	Condition    string `json:"condition,omitempty"`
}

type UnmuteAlertResponse struct{}
