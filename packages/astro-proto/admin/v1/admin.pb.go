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
	Name        string   `json:"name,omitempty"`
	BuildID     string   `json:"build_id,omitempty"`
	Namespace   string   `json:"namespace,omitempty"`
	Status      string   `json:"status,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	AccountName string   `json:"account_name,omitempty"`
	Components  []string `json:"components,omitempty"`
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

type K8sPodInfo struct {
	Name      string            `json:"name,omitempty"`
	Namespace string            `json:"namespace,omitempty"`
	Phase     string            `json:"phase,omitempty"`
	NodeName  string            `json:"node_name,omitempty"`
	PodIP     string            `json:"pod_ip,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt string            `json:"created_at,omitempty"`
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

type ClusterSummary struct {
	TotalDeployments int32 `json:"total_deployments,omitempty"`
	TotalPods        int32 `json:"total_pods,omitempty"`
	RunningPods      int32 `json:"running_pods,omitempty"`
	PendingPods      int32 `json:"pending_pods,omitempty"`
	FailedPods       int32 `json:"failed_pods,omitempty"`
	TotalServices    int32 `json:"total_services,omitempty"`
}

type GetClusterStatusResponse struct {
	Timestamp   string               `json:"timestamp,omitempty"`
	Namespace   string               `json:"namespace,omitempty"`
	Deployments []*K8sDeploymentInfo `json:"deployments,omitempty"`
	Pods        []*K8sPodInfo        `json:"pods,omitempty"`
	Services    []*K8sServiceInfo    `json:"services,omitempty"`
	Summary     *ClusterSummary      `json:"summary,omitempty"`
}

type ListImagesRequest struct{}

type ImageInfo struct {
	Repository string   `json:"repository,omitempty"`
	Namespace  string   `json:"namespace,omitempty"`
	Name       string   `json:"name,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

type ListImagesResponse struct {
	Images []*ImageInfo `json:"images,omitempty"`
	Count  int32        `json:"count,omitempty"`
}

type DeleteDeploymentRequest struct {
	Namespace string `json:"namespace,omitempty"`
}

type DeleteDeploymentResponse struct {
	Status string `json:"status,omitempty"`
}

type RestartDeploymentRequest struct {
	Namespace string `json:"namespace,omitempty"`
	Pod       string `json:"pod,omitempty"`
}

type RestartDeploymentResponse struct {
	Status string `json:"status,omitempty"`
}

type GetSchemaRequest struct{}

type ColumnInfo struct {
	TableName  string `json:"table_name,omitempty"`
	ColumnName string `json:"column_name,omitempty"`
	DataType   string `json:"data_type,omitempty"`
}

type GetSchemaResponse struct {
	Columns []*ColumnInfo `json:"columns,omitempty"`
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

type QueryDatabaseRequest struct {
	Query string `json:"query,omitempty"`
}

type Row struct {
	Values []string `json:"values,omitempty"`
}

type QueryDatabaseResponse struct {
	Columns []string `json:"columns,omitempty"`
	Rows    []*Row   `json:"rows,omitempty"`
}
