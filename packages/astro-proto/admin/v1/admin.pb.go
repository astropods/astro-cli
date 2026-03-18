// Code generated manually to match proto/admin/v1/admin.proto.
// Uses JSON-over-gRPC encoding (overrides default proto codec).
// Regenerate by running: buf generate (requires buf CLI)

package adminv1

import (
	"encoding/json"

	"google.golang.org/grpc/encoding"
)

func init() {
	encoding.RegisterCodec(jsonCodec{})
}

// jsonCodec overrides the default proto codec with JSON encoding.
// This allows plain Go structs to be used as gRPC message types.
type jsonCodec struct{}

func (jsonCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (jsonCodec) Name() string { return "proto" }

// Message types — mirrors proto/admin/v1/admin.proto

type ListDeploymentsRequest struct {
	Namespace string `json:"namespace,omitempty"`
}

type AdminDeployment struct {
	Name            string        `json:"name,omitempty"`
	BuildID         string        `json:"build_id,omitempty"`
	Namespace       string        `json:"namespace,omitempty"`
	Status          string        `json:"status,omitempty"`
	CreatedAt       string        `json:"created_at,omitempty"`
	AccountName     string        `json:"account_name,omitempty"`
	Components      []string      `json:"components,omitempty"`
	DeploymentID    string        `json:"deployment_id,omitempty"`
	ErrorMessage    string        `json:"error_message,omitempty"`
	StatusChangedAt string        `json:"status_changed_at,omitempty"`
	CurrentRevision int32         `json:"current_revision,omitempty"`
	DriftSummary    *DriftSummary `json:"drift_summary,omitempty"`
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
	Timestamp       string                  `json:"timestamp,omitempty"`
	Namespace       string                  `json:"namespace,omitempty"`
	Deployments     []*K8sDeploymentInfo    `json:"deployments,omitempty"`
	StatefulSets    []*K8sDeploymentInfo    `json:"statefulsets,omitempty"`
	Pods            []*K8sPodInfo           `json:"pods,omitempty"`
	Services        []*K8sServiceInfo       `json:"services,omitempty"`
	Ingresses       []*K8sIngressInfo       `json:"ingresses,omitempty"`
	NetworkPolicies []*K8sNetworkPolicyInfo `json:"network_policies,omitempty"`
	Events          []*K8sEventInfo         `json:"events,omitempty"`
	Summary         *ClusterSummary         `json:"summary,omitempty"`
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
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	OwnerUserID string `json:"owner_user_id,omitempty"`
	MemberCount int32  `json:"member_count,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type ListAccountsResponse struct {
	Accounts []*AdminAccount `json:"accounts,omitempty"`
	Count    int32           `json:"count,omitempty"`
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

type DriftResourceItem struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`     // deployment, statefulset, service, ingress
	Status   string            `json:"status"`   // match, missing, extra, drift
	Expected map[string]string `json:"expected"` // e.g. {Image: "foo:v1", Replicas: "2"}
	Actual   map[string]string `json:"actual"`   // e.g. {Image: "foo:v1", Replicas: "1/2"}
}

type DriftSummary struct {
	Total   int `json:"total"`
	Match   int `json:"match"`
	Missing int `json:"missing"`
	Extra   int `json:"extra"`
	Drift   int `json:"drift"`
}

type DriftReport struct {
	DetectedAt string               `json:"detected_at"`
	Workloads  []*DriftResourceItem `json:"workloads"`
	Services   []*DriftResourceItem `json:"services"`
	Ingresses  []*DriftResourceItem `json:"ingresses"`
	EnvVars    []*DriftResourceItem `json:"env_vars,omitempty"`
	Secrets    []*DriftResourceItem `json:"secrets,omitempty"`
	Summary    *DriftSummary        `json:"summary"`
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
	DriftReport       *DriftReport               `json:"drift_report,omitempty"`
	DriftCheckedAt    string                     `json:"drift_checked_at,omitempty"`
	Variables         []*AdminVariable           `json:"variables,omitempty"`
	Adapters          []string                   `json:"adapters,omitempty"`
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

type OpenMeterProxyRequest struct {
	Method  string            `json:"method,omitempty"`
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
}

type OpenMeterProxyResponse struct {
	StatusCode int32             `json:"status_code,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"`
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
	Status string `json:"status,omitempty"`
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
}

type GetDeploymentJobsResponse struct {
	Jobs            []*DeploymentJob `json:"jobs,omitempty"`
	LastReconcileAt string           `json:"last_reconcile_at,omitempty"`
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

type RefreshDriftReportRequest struct {
	DeploymentId string `json:"deployment_id,omitempty"`
}

type RefreshDriftReportResponse struct {
	DriftReport    *DriftReport `json:"drift_report,omitempty"`
	DriftCheckedAt string       `json:"drift_checked_at,omitempty"`
}

type BackfillResolvedKeysRequest struct{}

type BackfillResolvedKeysResponse struct {
	BackfilledCount int32 `json:"backfilled_count"`
}

type SetAdaptersRequest struct {
	DeploymentId string   `json:"deployment_id,omitempty"`
	Adapters     []string `json:"adapters"`
}

type SetAdaptersResponse struct {
	Status   string   `json:"status,omitempty"`
	Adapters []string `json:"adapters"`
}
