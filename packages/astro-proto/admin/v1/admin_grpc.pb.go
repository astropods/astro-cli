// Code generated manually to match proto/admin/v1/admin.proto.
// Hand-maintained: there is no buf.gen.yaml, so editing the .proto does not
// produce this file. Add or change an RPC in six places here (client
// interface, client method, server interface, Unimplemented stub, handler,
// ServiceDesc) to match.

package adminv1

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AdminServiceClient is the client API for AdminService.
type AdminServiceClient interface {
	ListDeployments(ctx context.Context, in *ListDeploymentsRequest, opts ...grpc.CallOption) (*ListDeploymentsResponse, error)
	GetDeployment(ctx context.Context, in *GetDeploymentRequest, opts ...grpc.CallOption) (*GetDeploymentResponse, error)
	GetDeploymentAccess(ctx context.Context, in *GetDeploymentAccessRequest, opts ...grpc.CallOption) (*GetDeploymentAccessResponse, error)
	ListAuthorizationResources(ctx context.Context, in *ListAuthorizationResourcesRequest, opts ...grpc.CallOption) (*ListAuthorizationResourcesResponse, error)
	ListAuthorizationOperations(ctx context.Context, in *ListAuthorizationOperationsRequest, opts ...grpc.CallOption) (*ListAuthorizationOperationsResponse, error)
	StartAuthorizationResourceReset(ctx context.Context, in *StartAuthorizationResourceResetRequest, opts ...grpc.CallOption) (*StartAuthorizationResourceResetResponse, error)
	StartAuthorizationResourceBackfill(ctx context.Context, in *StartAuthorizationResourceBackfillRequest, opts ...grpc.CallOption) (*StartAuthorizationResourceBackfillResponse, error)
	GetClusterStatus(ctx context.Context, in *GetClusterStatusRequest, opts ...grpc.CallOption) (*GetClusterStatusResponse, error)
	DeleteDeployment(ctx context.Context, in *DeleteDeploymentRequest, opts ...grpc.CallOption) (*DeleteDeploymentResponse, error)
	RestartDeployment(ctx context.Context, in *RestartDeploymentRequest, opts ...grpc.CallOption) (*RestartDeploymentResponse, error)
	ListAccounts(ctx context.Context, in *ListAccountsRequest, opts ...grpc.CallOption) (*ListAccountsResponse, error)
	GetAccount(ctx context.Context, in *GetAccountRequest, opts ...grpc.CallOption) (*GetAccountResponse, error)
	GetAccountMetronomeAliases(ctx context.Context, in *GetAccountMetronomeAliasesRequest, opts ...grpc.CallOption) (*MetronomeAliasStatus, error)
	RecoverAccountMetronomeAliases(ctx context.Context, in *RecoverAccountMetronomeAliasesRequest, opts ...grpc.CallOption) (*MetronomeAliasStatus, error)
	RegisterAccountMetronome(ctx context.Context, in *RegisterAccountMetronomeRequest, opts ...grpc.CallOption) (*RegisterAccountMetronomeResponse, error)
	GetAccountBillingDetail(ctx context.Context, in *GetAccountBillingDetailRequest, opts ...grpc.CallOption) (*GetAccountBillingDetailResponse, error)
	RetryBillingProvision(ctx context.Context, in *RetryBillingProvisionRequest, opts ...grpc.CallOption) (*RetryBillingProvisionResponse, error)
	ForceBillingResume(ctx context.Context, in *ForceBillingResumeRequest, opts ...grpc.CallOption) (*ForceBillingResumeResponse, error)
	SetAccountSpendLimit(ctx context.Context, in *SetAccountSpendLimitRequest, opts ...grpc.CallOption) (*SetAccountSpendLimitResponse, error)
	RecoverAccountLangfuse(ctx context.Context, in *RecoverAccountLangfuseRequest, opts ...grpc.CallOption) (*RecoverAccountLangfuseResponse, error)
	RecoverAccountBifrost(ctx context.Context, in *RecoverAccountBifrostRequest, opts ...grpc.CallOption) (*RecoverAccountBifrostResponse, error)
	RenameAccount(ctx context.Context, in *RenameAccountRequest, opts ...grpc.CallOption) (*RenameAccountResponse, error)
	DeleteAccount(ctx context.Context, in *DeleteAccountRequest, opts ...grpc.CallOption) (*DeleteAccountResponse, error)
	PurgeAccount(ctx context.Context, in *PurgeAccountRequest, opts ...grpc.CallOption) (*PurgeAccountResponse, error)
	GetPodLogs(ctx context.Context, in *GetPodLogsRequest, opts ...grpc.CallOption) (*GetPodLogsResponse, error)
	GetPodEnv(ctx context.Context, in *GetPodEnvRequest, opts ...grpc.CallOption) (*GetPodEnvResponse, error)
	ListAgents(ctx context.Context, in *ListAgentsRequest, opts ...grpc.CallOption) (*ListAgentsResponse, error)
	GetAgentBuilds(ctx context.Context, in *GetAgentBuildsRequest, opts ...grpc.CallOption) (*GetAgentBuildsResponse, error)
	ProxyHTTP(ctx context.Context, in *HTTPProxyRequest, opts ...grpc.CallOption) (*HTTPProxyResponse, error)
	GetAuthConfig(ctx context.Context, in *GetAuthConfigRequest, opts ...grpc.CallOption) (*GetAuthConfigResponse, error)
	StartRiverUI(ctx context.Context, in *StartRiverUIRequest, opts ...grpc.CallOption) (*StartRiverUIResponse, error)
	StopRiverUI(ctx context.Context, in *StopRiverUIRequest, opts ...grpc.CallOption) (*StopRiverUIResponse, error)
	GetRiverUIStatus(ctx context.Context, in *GetRiverUIStatusRequest, opts ...grpc.CallOption) (*GetRiverUIStatusResponse, error)
	ListQuotaIncreaseRequests(ctx context.Context, in *ListQuotaIncreaseRequestsRequest, opts ...grpc.CallOption) (*ListQuotaIncreaseRequestsResponse, error)
	ApproveQuotaIncrease(ctx context.Context, in *ApproveQuotaIncreaseRequest, opts ...grpc.CallOption) (*ApproveQuotaIncreaseResponse, error)
	DenyQuotaIncrease(ctx context.Context, in *DenyQuotaIncreaseRequest, opts ...grpc.CallOption) (*DenyQuotaIncreaseResponse, error)
	GetDeploymentEvents(ctx context.Context, in *GetDeploymentEventsRequest, opts ...grpc.CallOption) (*GetDeploymentEventsResponse, error)
	WakeUpDeployment(ctx context.Context, in *WakeUpDeploymentRequest, opts ...grpc.CallOption) (*WakeUpDeploymentResponse, error)
	RollbackDeployment(ctx context.Context, in *RollbackDeploymentRequest, opts ...grpc.CallOption) (*RollbackDeploymentResponse, error)
	ReapplyDeployment(ctx context.Context, in *ReapplyDeploymentRequest, opts ...grpc.CallOption) (*ReapplyDeploymentResponse, error)
	GetDeploymentJobs(ctx context.Context, in *GetDeploymentJobsRequest, opts ...grpc.CallOption) (*GetDeploymentJobsResponse, error)
	RepairNormalizedSpec(ctx context.Context, in *RepairNormalizedSpecRequest, opts ...grpc.CallOption) (*RepairNormalizedSpecResponse, error)
	ListFeedback(ctx context.Context, in *ListFeedbackRequest, opts ...grpc.CallOption) (*ListFeedbackResponse, error)
	StopDeployment(ctx context.Context, in *StopDeploymentRequest, opts ...grpc.CallOption) (*StopDeploymentResponse, error)
	RegisterCluster(ctx context.Context, in *RegisterClusterRequest, opts ...grpc.CallOption) (*RegisterClusterResponse, error)
	EnableCluster(ctx context.Context, in *EnableClusterRequest, opts ...grpc.CallOption) (*EnableClusterResponse, error)
	DisableCluster(ctx context.Context, in *DisableClusterRequest, opts ...grpc.CallOption) (*DisableClusterResponse, error)
	DeregisterCluster(ctx context.Context, in *DeregisterClusterRequest, opts ...grpc.CallOption) (*DeregisterClusterResponse, error)
	GetClusterBlockers(ctx context.Context, in *GetClusterBlockersRequest, opts ...grpc.CallOption) (*GetClusterBlockersResponse, error)
	ListClusters(ctx context.Context, in *ListClustersRequest, opts ...grpc.CallOption) (*ListClustersResponse, error)
	ListAccountClusters(ctx context.Context, in *ListAccountClustersRequest, opts ...grpc.CallOption) (*AccountClusterList, error)
	AddAccountCluster(ctx context.Context, in *AddAccountClusterRequest, opts ...grpc.CallOption) (*AccountClusterList, error)
	RemoveAccountCluster(ctx context.Context, in *RemoveAccountClusterRequest, opts ...grpc.CallOption) (*AccountClusterList, error)
	SetAccountDefaultCluster(ctx context.Context, in *SetAccountDefaultClusterRequest, opts ...grpc.CallOption) (*AccountClusterList, error)
	UpdateCluster(ctx context.Context, in *UpdateClusterRequest, opts ...grpc.CallOption) (*UpdateClusterResponse, error)
	CheckClusterHealth(ctx context.Context, in *CheckClusterHealthRequest, opts ...grpc.CallOption) (*CheckClusterHealthResponse, error)
	RefreshClusterPullSecrets(ctx context.Context, in *RefreshClusterPullSecretsRequest, opts ...grpc.CallOption) (*RefreshClusterPullSecretsResponse, error)
	ListEvaluators(ctx context.Context, in *ListEvaluatorsRequest, opts ...grpc.CallOption) (*ListEvaluatorsResponse, error)
	RunEvaluatorSweep(ctx context.Context, in *RunEvaluatorSweepRequest, opts ...grpc.CallOption) (*RunEvaluatorSweepResponse, error)
	ListEvaluatorDrift(ctx context.Context, in *ListEvaluatorDriftRequest, opts ...grpc.CallOption) (*ListEvaluatorDriftResponse, error)
	FixDeploymentDrift(ctx context.Context, in *FixDeploymentDriftRequest, opts ...grpc.CallOption) (*FixDeploymentDriftResponse, error)
	InvalidateAccountCaches(ctx context.Context, in *InvalidateAccountCachesRequest, opts ...grpc.CallOption) (*InvalidateCachesResponse, error)
	InvalidateAllCaches(ctx context.Context, in *InvalidateAllCachesRequest, opts ...grpc.CallOption) (*InvalidateCachesResponse, error)
	ListClusterMigrations(ctx context.Context, in *ListClusterMigrationsRequest, opts ...grpc.CallOption) (*ListClusterMigrationsResponse, error)
	ListJobKinds(ctx context.Context, in *ListJobKindsRequest, opts ...grpc.CallOption) (*ListJobKindsResponse, error)
	TriggerJob(ctx context.Context, in *TriggerJobRequest, opts ...grpc.CallOption) (*TriggerJobResponse, error)
	GetJobStates(ctx context.Context, in *GetJobStatesRequest, opts ...grpc.CallOption) (*GetJobStatesResponse, error)
	ListAdminQueues(ctx context.Context, in *ListAdminQueuesRequest, opts ...grpc.CallOption) (*ListAdminQueuesResponse, error)
	ListJobs(ctx context.Context, in *ListJobsRequest, opts ...grpc.CallOption) (*ListJobsResponse, error)
	GetJob(ctx context.Context, in *GetJobRequest, opts ...grpc.CallOption) (*GetJobResponse, error)
	CancelJobs(ctx context.Context, in *CancelJobsRequest, opts ...grpc.CallOption) (*CancelJobsResponse, error)
	RetryJobs(ctx context.Context, in *RetryJobsRequest, opts ...grpc.CallOption) (*RetryJobsResponse, error)
	PauseQueue(ctx context.Context, in *PauseQueueRequest, opts ...grpc.CallOption) (*PauseQueueResponse, error)
	ResumeQueue(ctx context.Context, in *ResumeQueueRequest, opts ...grpc.CallOption) (*ResumeQueueResponse, error)
	RefreshMessagingCache(ctx context.Context, in *RefreshMessagingCacheRequest, opts ...grpc.CallOption) (*RefreshMessagingCacheResponse, error)
	ListOutboundDomains(ctx context.Context, in *ListOutboundDomainsRequest, opts ...grpc.CallOption) (*ListOutboundDomainsResponse, error)
	ListAlerts(ctx context.Context, in *ListAlertsRequest, opts ...grpc.CallOption) (*ListAlertsResponse, error)
	ClearAlert(ctx context.Context, in *ClearAlertRequest, opts ...grpc.CallOption) (*ClearAlertResponse, error)
	MuteAlert(ctx context.Context, in *MuteAlertRequest, opts ...grpc.CallOption) (*MuteAlertResponse, error)
	UnmuteAlert(ctx context.Context, in *UnmuteAlertRequest, opts ...grpc.CallOption) (*UnmuteAlertResponse, error)
	ListAuditFindings(ctx context.Context, in *ListAuditFindingsRequest, opts ...grpc.CallOption) (*ListAuditFindingsResponse, error)
	AcknowledgeAuditFinding(ctx context.Context, in *AcknowledgeAuditFindingRequest, opts ...grpc.CallOption) (*AcknowledgeAuditFindingResponse, error)
}

type adminServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewAdminServiceClient(cc grpc.ClientConnInterface) AdminServiceClient {
	return &adminServiceClient{cc}
}

func (c *adminServiceClient) ListDeployments(ctx context.Context, in *ListDeploymentsRequest, opts ...grpc.CallOption) (*ListDeploymentsResponse, error) {
	out := new(ListDeploymentsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListDeployments", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetDeployment(ctx context.Context, in *GetDeploymentRequest, opts ...grpc.CallOption) (*GetDeploymentResponse, error) {
	out := new(GetDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetDeploymentAccess(ctx context.Context, in *GetDeploymentAccessRequest, opts ...grpc.CallOption) (*GetDeploymentAccessResponse, error) {
	out := new(GetDeploymentAccessResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetDeploymentAccess", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListAuthorizationResources(ctx context.Context, in *ListAuthorizationResourcesRequest, opts ...grpc.CallOption) (*ListAuthorizationResourcesResponse, error) {
	out := new(ListAuthorizationResourcesResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListAuthorizationResources", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListAuthorizationOperations(ctx context.Context, in *ListAuthorizationOperationsRequest, opts ...grpc.CallOption) (*ListAuthorizationOperationsResponse, error) {
	out := new(ListAuthorizationOperationsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListAuthorizationOperations", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) StartAuthorizationResourceReset(ctx context.Context, in *StartAuthorizationResourceResetRequest, opts ...grpc.CallOption) (*StartAuthorizationResourceResetResponse, error) {
	out := new(StartAuthorizationResourceResetResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/StartAuthorizationResourceReset", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) StartAuthorizationResourceBackfill(ctx context.Context, in *StartAuthorizationResourceBackfillRequest, opts ...grpc.CallOption) (*StartAuthorizationResourceBackfillResponse, error) {
	out := new(StartAuthorizationResourceBackfillResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/StartAuthorizationResourceBackfill", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetClusterStatus(ctx context.Context, in *GetClusterStatusRequest, opts ...grpc.CallOption) (*GetClusterStatusResponse, error) {
	out := new(GetClusterStatusResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetClusterStatus", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) DeleteDeployment(ctx context.Context, in *DeleteDeploymentRequest, opts ...grpc.CallOption) (*DeleteDeploymentResponse, error) {
	out := new(DeleteDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/DeleteDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RestartDeployment(ctx context.Context, in *RestartDeploymentRequest, opts ...grpc.CallOption) (*RestartDeploymentResponse, error) {
	out := new(RestartDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RestartDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListAccounts(ctx context.Context, in *ListAccountsRequest, opts ...grpc.CallOption) (*ListAccountsResponse, error) {
	out := new(ListAccountsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListAccounts", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetAccount(ctx context.Context, in *GetAccountRequest, opts ...grpc.CallOption) (*GetAccountResponse, error) {
	out := new(GetAccountResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetAccount", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetAccountMetronomeAliases(ctx context.Context, in *GetAccountMetronomeAliasesRequest, opts ...grpc.CallOption) (*MetronomeAliasStatus, error) {
	out := new(MetronomeAliasStatus)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetAccountMetronomeAliases", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RecoverAccountMetronomeAliases(ctx context.Context, in *RecoverAccountMetronomeAliasesRequest, opts ...grpc.CallOption) (*MetronomeAliasStatus, error) {
	out := new(MetronomeAliasStatus)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RecoverAccountMetronomeAliases", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RegisterAccountMetronome(ctx context.Context, in *RegisterAccountMetronomeRequest, opts ...grpc.CallOption) (*RegisterAccountMetronomeResponse, error) {
	out := new(RegisterAccountMetronomeResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RegisterAccountMetronome", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetAccountBillingDetail(ctx context.Context, in *GetAccountBillingDetailRequest, opts ...grpc.CallOption) (*GetAccountBillingDetailResponse, error) {
	out := new(GetAccountBillingDetailResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetAccountBillingDetail", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RetryBillingProvision(ctx context.Context, in *RetryBillingProvisionRequest, opts ...grpc.CallOption) (*RetryBillingProvisionResponse, error) {
	out := new(RetryBillingProvisionResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RetryBillingProvision", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ForceBillingResume(ctx context.Context, in *ForceBillingResumeRequest, opts ...grpc.CallOption) (*ForceBillingResumeResponse, error) {
	out := new(ForceBillingResumeResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ForceBillingResume", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) SetAccountSpendLimit(ctx context.Context, in *SetAccountSpendLimitRequest, opts ...grpc.CallOption) (*SetAccountSpendLimitResponse, error) {
	out := new(SetAccountSpendLimitResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/SetAccountSpendLimit", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RecoverAccountLangfuse(ctx context.Context, in *RecoverAccountLangfuseRequest, opts ...grpc.CallOption) (*RecoverAccountLangfuseResponse, error) {
	out := new(RecoverAccountLangfuseResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RecoverAccountLangfuse", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RecoverAccountBifrost(ctx context.Context, in *RecoverAccountBifrostRequest, opts ...grpc.CallOption) (*RecoverAccountBifrostResponse, error) {
	out := new(RecoverAccountBifrostResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RecoverAccountBifrost", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RenameAccount(ctx context.Context, in *RenameAccountRequest, opts ...grpc.CallOption) (*RenameAccountResponse, error) {
	out := new(RenameAccountResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RenameAccount", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) DeleteAccount(ctx context.Context, in *DeleteAccountRequest, opts ...grpc.CallOption) (*DeleteAccountResponse, error) {
	out := new(DeleteAccountResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/DeleteAccount", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) PurgeAccount(ctx context.Context, in *PurgeAccountRequest, opts ...grpc.CallOption) (*PurgeAccountResponse, error) {
	out := new(PurgeAccountResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/PurgeAccount", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetPodLogs(ctx context.Context, in *GetPodLogsRequest, opts ...grpc.CallOption) (*GetPodLogsResponse, error) {
	out := new(GetPodLogsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetPodLogs", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetPodEnv(ctx context.Context, in *GetPodEnvRequest, opts ...grpc.CallOption) (*GetPodEnvResponse, error) {
	out := new(GetPodEnvResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetPodEnv", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListAgents(ctx context.Context, in *ListAgentsRequest, opts ...grpc.CallOption) (*ListAgentsResponse, error) {
	out := new(ListAgentsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListAgents", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetAgentBuilds(ctx context.Context, in *GetAgentBuildsRequest, opts ...grpc.CallOption) (*GetAgentBuildsResponse, error) {
	out := new(GetAgentBuildsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetAgentBuilds", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ProxyHTTP(ctx context.Context, in *HTTPProxyRequest, opts ...grpc.CallOption) (*HTTPProxyResponse, error) {
	out := new(HTTPProxyResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ProxyHTTP", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetAuthConfig(ctx context.Context, in *GetAuthConfigRequest, opts ...grpc.CallOption) (*GetAuthConfigResponse, error) {
	out := new(GetAuthConfigResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetAuthConfig", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) StartRiverUI(ctx context.Context, in *StartRiverUIRequest, opts ...grpc.CallOption) (*StartRiverUIResponse, error) {
	out := new(StartRiverUIResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/StartRiverUI", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) StopRiverUI(ctx context.Context, in *StopRiverUIRequest, opts ...grpc.CallOption) (*StopRiverUIResponse, error) {
	out := new(StopRiverUIResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/StopRiverUI", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetRiverUIStatus(ctx context.Context, in *GetRiverUIStatusRequest, opts ...grpc.CallOption) (*GetRiverUIStatusResponse, error) {
	out := new(GetRiverUIStatusResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetRiverUIStatus", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListQuotaIncreaseRequests(ctx context.Context, in *ListQuotaIncreaseRequestsRequest, opts ...grpc.CallOption) (*ListQuotaIncreaseRequestsResponse, error) {
	out := new(ListQuotaIncreaseRequestsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListQuotaIncreaseRequests", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ApproveQuotaIncrease(ctx context.Context, in *ApproveQuotaIncreaseRequest, opts ...grpc.CallOption) (*ApproveQuotaIncreaseResponse, error) {
	out := new(ApproveQuotaIncreaseResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ApproveQuotaIncrease", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) DenyQuotaIncrease(ctx context.Context, in *DenyQuotaIncreaseRequest, opts ...grpc.CallOption) (*DenyQuotaIncreaseResponse, error) {
	out := new(DenyQuotaIncreaseResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/DenyQuotaIncrease", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetDeploymentEvents(ctx context.Context, in *GetDeploymentEventsRequest, opts ...grpc.CallOption) (*GetDeploymentEventsResponse, error) {
	out := new(GetDeploymentEventsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetDeploymentEvents", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) WakeUpDeployment(ctx context.Context, in *WakeUpDeploymentRequest, opts ...grpc.CallOption) (*WakeUpDeploymentResponse, error) {
	out := new(WakeUpDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/WakeUpDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RollbackDeployment(ctx context.Context, in *RollbackDeploymentRequest, opts ...grpc.CallOption) (*RollbackDeploymentResponse, error) {
	out := new(RollbackDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RollbackDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ReapplyDeployment(ctx context.Context, in *ReapplyDeploymentRequest, opts ...grpc.CallOption) (*ReapplyDeploymentResponse, error) {
	out := new(ReapplyDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ReapplyDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetDeploymentJobs(ctx context.Context, in *GetDeploymentJobsRequest, opts ...grpc.CallOption) (*GetDeploymentJobsResponse, error) {
	out := new(GetDeploymentJobsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetDeploymentJobs", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RepairNormalizedSpec(ctx context.Context, in *RepairNormalizedSpecRequest, opts ...grpc.CallOption) (*RepairNormalizedSpecResponse, error) {
	out := new(RepairNormalizedSpecResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RepairNormalizedSpec", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListFeedback(ctx context.Context, in *ListFeedbackRequest, opts ...grpc.CallOption) (*ListFeedbackResponse, error) {
	out := new(ListFeedbackResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListFeedback", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) StopDeployment(ctx context.Context, in *StopDeploymentRequest, opts ...grpc.CallOption) (*StopDeploymentResponse, error) {
	out := new(StopDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/StopDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RegisterCluster(ctx context.Context, in *RegisterClusterRequest, opts ...grpc.CallOption) (*RegisterClusterResponse, error) {
	out := new(RegisterClusterResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RegisterCluster", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) EnableCluster(ctx context.Context, in *EnableClusterRequest, opts ...grpc.CallOption) (*EnableClusterResponse, error) {
	out := new(EnableClusterResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/EnableCluster", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) DisableCluster(ctx context.Context, in *DisableClusterRequest, opts ...grpc.CallOption) (*DisableClusterResponse, error) {
	out := new(DisableClusterResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/DisableCluster", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) DeregisterCluster(ctx context.Context, in *DeregisterClusterRequest, opts ...grpc.CallOption) (*DeregisterClusterResponse, error) {
	out := new(DeregisterClusterResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/DeregisterCluster", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetClusterBlockers(ctx context.Context, in *GetClusterBlockersRequest, opts ...grpc.CallOption) (*GetClusterBlockersResponse, error) {
	out := new(GetClusterBlockersResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetClusterBlockers", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListClusters(ctx context.Context, in *ListClustersRequest, opts ...grpc.CallOption) (*ListClustersResponse, error) {
	out := new(ListClustersResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListClusters", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListAccountClusters(ctx context.Context, in *ListAccountClustersRequest, opts ...grpc.CallOption) (*AccountClusterList, error) {
	out := new(AccountClusterList)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListAccountClusters", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) AddAccountCluster(ctx context.Context, in *AddAccountClusterRequest, opts ...grpc.CallOption) (*AccountClusterList, error) {
	out := new(AccountClusterList)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/AddAccountCluster", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RemoveAccountCluster(ctx context.Context, in *RemoveAccountClusterRequest, opts ...grpc.CallOption) (*AccountClusterList, error) {
	out := new(AccountClusterList)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RemoveAccountCluster", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) SetAccountDefaultCluster(ctx context.Context, in *SetAccountDefaultClusterRequest, opts ...grpc.CallOption) (*AccountClusterList, error) {
	out := new(AccountClusterList)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/SetAccountDefaultCluster", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) UpdateCluster(ctx context.Context, in *UpdateClusterRequest, opts ...grpc.CallOption) (*UpdateClusterResponse, error) {
	out := new(UpdateClusterResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/UpdateCluster", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) CheckClusterHealth(ctx context.Context, in *CheckClusterHealthRequest, opts ...grpc.CallOption) (*CheckClusterHealthResponse, error) {
	out := new(CheckClusterHealthResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/CheckClusterHealth", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RefreshClusterPullSecrets(ctx context.Context, in *RefreshClusterPullSecretsRequest, opts ...grpc.CallOption) (*RefreshClusterPullSecretsResponse, error) {
	out := new(RefreshClusterPullSecretsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RefreshClusterPullSecrets", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListEvaluators(ctx context.Context, in *ListEvaluatorsRequest, opts ...grpc.CallOption) (*ListEvaluatorsResponse, error) {
	out := new(ListEvaluatorsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListEvaluators", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RunEvaluatorSweep(ctx context.Context, in *RunEvaluatorSweepRequest, opts ...grpc.CallOption) (*RunEvaluatorSweepResponse, error) {
	out := new(RunEvaluatorSweepResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RunEvaluatorSweep", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListEvaluatorDrift(ctx context.Context, in *ListEvaluatorDriftRequest, opts ...grpc.CallOption) (*ListEvaluatorDriftResponse, error) {
	out := new(ListEvaluatorDriftResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListEvaluatorDrift", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) FixDeploymentDrift(ctx context.Context, in *FixDeploymentDriftRequest, opts ...grpc.CallOption) (*FixDeploymentDriftResponse, error) {
	out := new(FixDeploymentDriftResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/FixDeploymentDrift", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) InvalidateAccountCaches(ctx context.Context, in *InvalidateAccountCachesRequest, opts ...grpc.CallOption) (*InvalidateCachesResponse, error) {
	out := new(InvalidateCachesResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/InvalidateAccountCaches", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) InvalidateAllCaches(ctx context.Context, in *InvalidateAllCachesRequest, opts ...grpc.CallOption) (*InvalidateCachesResponse, error) {
	out := new(InvalidateCachesResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/InvalidateAllCaches", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListClusterMigrations(ctx context.Context, in *ListClusterMigrationsRequest, opts ...grpc.CallOption) (*ListClusterMigrationsResponse, error) {
	out := new(ListClusterMigrationsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListClusterMigrations", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListJobKinds(ctx context.Context, in *ListJobKindsRequest, opts ...grpc.CallOption) (*ListJobKindsResponse, error) {
	out := new(ListJobKindsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListJobKinds", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) TriggerJob(ctx context.Context, in *TriggerJobRequest, opts ...grpc.CallOption) (*TriggerJobResponse, error) {
	out := new(TriggerJobResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/TriggerJob", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetJobStates(ctx context.Context, in *GetJobStatesRequest, opts ...grpc.CallOption) (*GetJobStatesResponse, error) {
	out := new(GetJobStatesResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetJobStates", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListAdminQueues(ctx context.Context, in *ListAdminQueuesRequest, opts ...grpc.CallOption) (*ListAdminQueuesResponse, error) {
	out := new(ListAdminQueuesResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListAdminQueues", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListJobs(ctx context.Context, in *ListJobsRequest, opts ...grpc.CallOption) (*ListJobsResponse, error) {
	out := new(ListJobsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListJobs", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetJob(ctx context.Context, in *GetJobRequest, opts ...grpc.CallOption) (*GetJobResponse, error) {
	out := new(GetJobResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetJob", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) CancelJobs(ctx context.Context, in *CancelJobsRequest, opts ...grpc.CallOption) (*CancelJobsResponse, error) {
	out := new(CancelJobsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/CancelJobs", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RetryJobs(ctx context.Context, in *RetryJobsRequest, opts ...grpc.CallOption) (*RetryJobsResponse, error) {
	out := new(RetryJobsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RetryJobs", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) PauseQueue(ctx context.Context, in *PauseQueueRequest, opts ...grpc.CallOption) (*PauseQueueResponse, error) {
	out := new(PauseQueueResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/PauseQueue", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ResumeQueue(ctx context.Context, in *ResumeQueueRequest, opts ...grpc.CallOption) (*ResumeQueueResponse, error) {
	out := new(ResumeQueueResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ResumeQueue", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RefreshMessagingCache(ctx context.Context, in *RefreshMessagingCacheRequest, opts ...grpc.CallOption) (*RefreshMessagingCacheResponse, error) {
	out := new(RefreshMessagingCacheResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RefreshMessagingCache", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListAlerts(ctx context.Context, in *ListAlertsRequest, opts ...grpc.CallOption) (*ListAlertsResponse, error) {
	out := new(ListAlertsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListAlerts", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ClearAlert(ctx context.Context, in *ClearAlertRequest, opts ...grpc.CallOption) (*ClearAlertResponse, error) {
	out := new(ClearAlertResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ClearAlert", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) MuteAlert(ctx context.Context, in *MuteAlertRequest, opts ...grpc.CallOption) (*MuteAlertResponse, error) {
	out := new(MuteAlertResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/MuteAlert", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) UnmuteAlert(ctx context.Context, in *UnmuteAlertRequest, opts ...grpc.CallOption) (*UnmuteAlertResponse, error) {
	out := new(UnmuteAlertResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/UnmuteAlert", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListAuditFindings(ctx context.Context, in *ListAuditFindingsRequest, opts ...grpc.CallOption) (*ListAuditFindingsResponse, error) {
	out := new(ListAuditFindingsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListAuditFindings", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) AcknowledgeAuditFinding(ctx context.Context, in *AcknowledgeAuditFindingRequest, opts ...grpc.CallOption) (*AcknowledgeAuditFindingResponse, error) {
	out := new(AcknowledgeAuditFindingResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/AcknowledgeAuditFinding", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminServiceServer is the server API for AdminService.
// Embed UnimplementedAdminServiceServer for forward compatibility.
type AdminServiceServer interface {
	ListDeployments(context.Context, *ListDeploymentsRequest) (*ListDeploymentsResponse, error)
	GetDeployment(context.Context, *GetDeploymentRequest) (*GetDeploymentResponse, error)
	GetDeploymentAccess(context.Context, *GetDeploymentAccessRequest) (*GetDeploymentAccessResponse, error)
	ListAuthorizationResources(context.Context, *ListAuthorizationResourcesRequest) (*ListAuthorizationResourcesResponse, error)
	ListAuthorizationOperations(context.Context, *ListAuthorizationOperationsRequest) (*ListAuthorizationOperationsResponse, error)
	StartAuthorizationResourceReset(context.Context, *StartAuthorizationResourceResetRequest) (*StartAuthorizationResourceResetResponse, error)
	StartAuthorizationResourceBackfill(context.Context, *StartAuthorizationResourceBackfillRequest) (*StartAuthorizationResourceBackfillResponse, error)
	GetClusterStatus(context.Context, *GetClusterStatusRequest) (*GetClusterStatusResponse, error)
	DeleteDeployment(context.Context, *DeleteDeploymentRequest) (*DeleteDeploymentResponse, error)
	RestartDeployment(context.Context, *RestartDeploymentRequest) (*RestartDeploymentResponse, error)
	ListAccounts(context.Context, *ListAccountsRequest) (*ListAccountsResponse, error)
	GetAccount(context.Context, *GetAccountRequest) (*GetAccountResponse, error)
	GetAccountMetronomeAliases(context.Context, *GetAccountMetronomeAliasesRequest) (*MetronomeAliasStatus, error)
	RecoverAccountMetronomeAliases(context.Context, *RecoverAccountMetronomeAliasesRequest) (*MetronomeAliasStatus, error)
	RegisterAccountMetronome(context.Context, *RegisterAccountMetronomeRequest) (*RegisterAccountMetronomeResponse, error)
	GetAccountBillingDetail(context.Context, *GetAccountBillingDetailRequest) (*GetAccountBillingDetailResponse, error)
	RetryBillingProvision(context.Context, *RetryBillingProvisionRequest) (*RetryBillingProvisionResponse, error)
	ForceBillingResume(context.Context, *ForceBillingResumeRequest) (*ForceBillingResumeResponse, error)
	SetAccountSpendLimit(context.Context, *SetAccountSpendLimitRequest) (*SetAccountSpendLimitResponse, error)
	RecoverAccountLangfuse(context.Context, *RecoverAccountLangfuseRequest) (*RecoverAccountLangfuseResponse, error)
	RecoverAccountBifrost(context.Context, *RecoverAccountBifrostRequest) (*RecoverAccountBifrostResponse, error)
	RenameAccount(context.Context, *RenameAccountRequest) (*RenameAccountResponse, error)
	DeleteAccount(context.Context, *DeleteAccountRequest) (*DeleteAccountResponse, error)
	PurgeAccount(context.Context, *PurgeAccountRequest) (*PurgeAccountResponse, error)
	GetPodLogs(context.Context, *GetPodLogsRequest) (*GetPodLogsResponse, error)
	GetPodEnv(context.Context, *GetPodEnvRequest) (*GetPodEnvResponse, error)
	ListAgents(context.Context, *ListAgentsRequest) (*ListAgentsResponse, error)
	GetAgentBuilds(context.Context, *GetAgentBuildsRequest) (*GetAgentBuildsResponse, error)
	ProxyHTTP(context.Context, *HTTPProxyRequest) (*HTTPProxyResponse, error)
	GetAuthConfig(context.Context, *GetAuthConfigRequest) (*GetAuthConfigResponse, error)
	StartRiverUI(context.Context, *StartRiverUIRequest) (*StartRiverUIResponse, error)
	StopRiverUI(context.Context, *StopRiverUIRequest) (*StopRiverUIResponse, error)
	GetRiverUIStatus(context.Context, *GetRiverUIStatusRequest) (*GetRiverUIStatusResponse, error)
	ListQuotaIncreaseRequests(context.Context, *ListQuotaIncreaseRequestsRequest) (*ListQuotaIncreaseRequestsResponse, error)
	ApproveQuotaIncrease(context.Context, *ApproveQuotaIncreaseRequest) (*ApproveQuotaIncreaseResponse, error)
	DenyQuotaIncrease(context.Context, *DenyQuotaIncreaseRequest) (*DenyQuotaIncreaseResponse, error)
	GetDeploymentEvents(context.Context, *GetDeploymentEventsRequest) (*GetDeploymentEventsResponse, error)
	WakeUpDeployment(context.Context, *WakeUpDeploymentRequest) (*WakeUpDeploymentResponse, error)
	RollbackDeployment(context.Context, *RollbackDeploymentRequest) (*RollbackDeploymentResponse, error)
	ReapplyDeployment(context.Context, *ReapplyDeploymentRequest) (*ReapplyDeploymentResponse, error)
	GetDeploymentJobs(context.Context, *GetDeploymentJobsRequest) (*GetDeploymentJobsResponse, error)
	RepairNormalizedSpec(context.Context, *RepairNormalizedSpecRequest) (*RepairNormalizedSpecResponse, error)
	ListFeedback(context.Context, *ListFeedbackRequest) (*ListFeedbackResponse, error)
	StopDeployment(context.Context, *StopDeploymentRequest) (*StopDeploymentResponse, error)
	RegisterCluster(context.Context, *RegisterClusterRequest) (*RegisterClusterResponse, error)
	EnableCluster(context.Context, *EnableClusterRequest) (*EnableClusterResponse, error)
	DisableCluster(context.Context, *DisableClusterRequest) (*DisableClusterResponse, error)
	DeregisterCluster(context.Context, *DeregisterClusterRequest) (*DeregisterClusterResponse, error)
	GetClusterBlockers(context.Context, *GetClusterBlockersRequest) (*GetClusterBlockersResponse, error)
	ListClusters(context.Context, *ListClustersRequest) (*ListClustersResponse, error)
	ListAccountClusters(context.Context, *ListAccountClustersRequest) (*AccountClusterList, error)
	AddAccountCluster(context.Context, *AddAccountClusterRequest) (*AccountClusterList, error)
	RemoveAccountCluster(context.Context, *RemoveAccountClusterRequest) (*AccountClusterList, error)
	SetAccountDefaultCluster(context.Context, *SetAccountDefaultClusterRequest) (*AccountClusterList, error)
	UpdateCluster(context.Context, *UpdateClusterRequest) (*UpdateClusterResponse, error)
	CheckClusterHealth(context.Context, *CheckClusterHealthRequest) (*CheckClusterHealthResponse, error)
	RefreshClusterPullSecrets(context.Context, *RefreshClusterPullSecretsRequest) (*RefreshClusterPullSecretsResponse, error)
	ListEvaluators(context.Context, *ListEvaluatorsRequest) (*ListEvaluatorsResponse, error)
	RunEvaluatorSweep(context.Context, *RunEvaluatorSweepRequest) (*RunEvaluatorSweepResponse, error)
	ListEvaluatorDrift(context.Context, *ListEvaluatorDriftRequest) (*ListEvaluatorDriftResponse, error)
	FixDeploymentDrift(context.Context, *FixDeploymentDriftRequest) (*FixDeploymentDriftResponse, error)
	InvalidateAccountCaches(context.Context, *InvalidateAccountCachesRequest) (*InvalidateCachesResponse, error)
	InvalidateAllCaches(context.Context, *InvalidateAllCachesRequest) (*InvalidateCachesResponse, error)
	ListClusterMigrations(context.Context, *ListClusterMigrationsRequest) (*ListClusterMigrationsResponse, error)
	ListJobKinds(context.Context, *ListJobKindsRequest) (*ListJobKindsResponse, error)
	TriggerJob(context.Context, *TriggerJobRequest) (*TriggerJobResponse, error)
	GetJobStates(context.Context, *GetJobStatesRequest) (*GetJobStatesResponse, error)
	ListAdminQueues(context.Context, *ListAdminQueuesRequest) (*ListAdminQueuesResponse, error)
	ListJobs(context.Context, *ListJobsRequest) (*ListJobsResponse, error)
	GetJob(context.Context, *GetJobRequest) (*GetJobResponse, error)
	CancelJobs(context.Context, *CancelJobsRequest) (*CancelJobsResponse, error)
	RetryJobs(context.Context, *RetryJobsRequest) (*RetryJobsResponse, error)
	PauseQueue(context.Context, *PauseQueueRequest) (*PauseQueueResponse, error)
	ResumeQueue(context.Context, *ResumeQueueRequest) (*ResumeQueueResponse, error)
	RefreshMessagingCache(context.Context, *RefreshMessagingCacheRequest) (*RefreshMessagingCacheResponse, error)
	ListOutboundDomains(context.Context, *ListOutboundDomainsRequest) (*ListOutboundDomainsResponse, error)
	ListAlerts(context.Context, *ListAlertsRequest) (*ListAlertsResponse, error)
	ClearAlert(context.Context, *ClearAlertRequest) (*ClearAlertResponse, error)
	MuteAlert(context.Context, *MuteAlertRequest) (*MuteAlertResponse, error)
	UnmuteAlert(context.Context, *UnmuteAlertRequest) (*UnmuteAlertResponse, error)
	ListAuditFindings(context.Context, *ListAuditFindingsRequest) (*ListAuditFindingsResponse, error)
	AcknowledgeAuditFinding(context.Context, *AcknowledgeAuditFindingRequest) (*AcknowledgeAuditFindingResponse, error)
	mustEmbedUnimplementedAdminServiceServer()
}

// UnimplementedAdminServiceServer must be embedded to have forward compatible implementations.
type UnimplementedAdminServiceServer struct{}

func (UnimplementedAdminServiceServer) ListDeployments(context.Context, *ListDeploymentsRequest) (*ListDeploymentsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListDeployments not implemented")
}

func (UnimplementedAdminServiceServer) GetDeployment(context.Context, *GetDeploymentRequest) (*GetDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDeployment not implemented")
}

func (UnimplementedAdminServiceServer) GetDeploymentAccess(context.Context, *GetDeploymentAccessRequest) (*GetDeploymentAccessResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDeploymentAccess not implemented")
}

func (UnimplementedAdminServiceServer) ListAuthorizationResources(context.Context, *ListAuthorizationResourcesRequest) (*ListAuthorizationResourcesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListAuthorizationResources not implemented")
}

func (UnimplementedAdminServiceServer) ListAuthorizationOperations(context.Context, *ListAuthorizationOperationsRequest) (*ListAuthorizationOperationsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListAuthorizationOperations not implemented")
}

func (UnimplementedAdminServiceServer) StartAuthorizationResourceReset(context.Context, *StartAuthorizationResourceResetRequest) (*StartAuthorizationResourceResetResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StartAuthorizationResourceReset not implemented")
}

func (UnimplementedAdminServiceServer) StartAuthorizationResourceBackfill(context.Context, *StartAuthorizationResourceBackfillRequest) (*StartAuthorizationResourceBackfillResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StartAuthorizationResourceBackfill not implemented")
}

func (UnimplementedAdminServiceServer) GetClusterStatus(context.Context, *GetClusterStatusRequest) (*GetClusterStatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetClusterStatus not implemented")
}

func (UnimplementedAdminServiceServer) DeleteDeployment(context.Context, *DeleteDeploymentRequest) (*DeleteDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteDeployment not implemented")
}

func (UnimplementedAdminServiceServer) RestartDeployment(context.Context, *RestartDeploymentRequest) (*RestartDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RestartDeployment not implemented")
}

func (UnimplementedAdminServiceServer) ListAccounts(context.Context, *ListAccountsRequest) (*ListAccountsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListAccounts not implemented")
}
func (UnimplementedAdminServiceServer) GetAccount(context.Context, *GetAccountRequest) (*GetAccountResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAccount not implemented")
}
func (UnimplementedAdminServiceServer) GetAccountMetronomeAliases(context.Context, *GetAccountMetronomeAliasesRequest) (*MetronomeAliasStatus, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAccountMetronomeAliases not implemented")
}
func (UnimplementedAdminServiceServer) RecoverAccountMetronomeAliases(context.Context, *RecoverAccountMetronomeAliasesRequest) (*MetronomeAliasStatus, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RecoverAccountMetronomeAliases not implemented")
}
func (UnimplementedAdminServiceServer) RegisterAccountMetronome(context.Context, *RegisterAccountMetronomeRequest) (*RegisterAccountMetronomeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RegisterAccountMetronome not implemented")
}
func (UnimplementedAdminServiceServer) GetAccountBillingDetail(context.Context, *GetAccountBillingDetailRequest) (*GetAccountBillingDetailResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAccountBillingDetail not implemented")
}
func (UnimplementedAdminServiceServer) RetryBillingProvision(context.Context, *RetryBillingProvisionRequest) (*RetryBillingProvisionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RetryBillingProvision not implemented")
}
func (UnimplementedAdminServiceServer) ForceBillingResume(context.Context, *ForceBillingResumeRequest) (*ForceBillingResumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ForceBillingResume not implemented")
}

func (UnimplementedAdminServiceServer) SetAccountSpendLimit(context.Context, *SetAccountSpendLimitRequest) (*SetAccountSpendLimitResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SetAccountSpendLimit not implemented")
}
func (UnimplementedAdminServiceServer) RecoverAccountLangfuse(context.Context, *RecoverAccountLangfuseRequest) (*RecoverAccountLangfuseResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RecoverAccountLangfuse not implemented")
}
func (UnimplementedAdminServiceServer) RecoverAccountBifrost(context.Context, *RecoverAccountBifrostRequest) (*RecoverAccountBifrostResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RecoverAccountBifrost not implemented")
}

func (UnimplementedAdminServiceServer) RenameAccount(context.Context, *RenameAccountRequest) (*RenameAccountResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RenameAccount not implemented")
}

func (UnimplementedAdminServiceServer) DeleteAccount(context.Context, *DeleteAccountRequest) (*DeleteAccountResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteAccount not implemented")
}

func (UnimplementedAdminServiceServer) PurgeAccount(context.Context, *PurgeAccountRequest) (*PurgeAccountResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PurgeAccount not implemented")
}

func (UnimplementedAdminServiceServer) GetPodLogs(context.Context, *GetPodLogsRequest) (*GetPodLogsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetPodLogs not implemented")
}

func (UnimplementedAdminServiceServer) GetPodEnv(context.Context, *GetPodEnvRequest) (*GetPodEnvResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetPodEnv not implemented")
}

func (UnimplementedAdminServiceServer) ListAgents(context.Context, *ListAgentsRequest) (*ListAgentsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListAgents not implemented")
}

func (UnimplementedAdminServiceServer) GetAgentBuilds(context.Context, *GetAgentBuildsRequest) (*GetAgentBuildsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAgentBuilds not implemented")
}

func (UnimplementedAdminServiceServer) ProxyHTTP(context.Context, *HTTPProxyRequest) (*HTTPProxyResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ProxyHTTP not implemented")
}

func (UnimplementedAdminServiceServer) GetAuthConfig(context.Context, *GetAuthConfigRequest) (*GetAuthConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAuthConfig not implemented")
}

func (UnimplementedAdminServiceServer) StartRiverUI(context.Context, *StartRiverUIRequest) (*StartRiverUIResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StartRiverUI not implemented")
}

func (UnimplementedAdminServiceServer) StopRiverUI(context.Context, *StopRiverUIRequest) (*StopRiverUIResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StopRiverUI not implemented")
}

func (UnimplementedAdminServiceServer) GetRiverUIStatus(context.Context, *GetRiverUIStatusRequest) (*GetRiverUIStatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRiverUIStatus not implemented")
}

func (UnimplementedAdminServiceServer) ListQuotaIncreaseRequests(context.Context, *ListQuotaIncreaseRequestsRequest) (*ListQuotaIncreaseRequestsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListQuotaIncreaseRequests not implemented")
}

func (UnimplementedAdminServiceServer) ApproveQuotaIncrease(context.Context, *ApproveQuotaIncreaseRequest) (*ApproveQuotaIncreaseResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ApproveQuotaIncrease not implemented")
}

func (UnimplementedAdminServiceServer) DenyQuotaIncrease(context.Context, *DenyQuotaIncreaseRequest) (*DenyQuotaIncreaseResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DenyQuotaIncrease not implemented")
}

func (UnimplementedAdminServiceServer) GetDeploymentEvents(context.Context, *GetDeploymentEventsRequest) (*GetDeploymentEventsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDeploymentEvents not implemented")
}

func (UnimplementedAdminServiceServer) WakeUpDeployment(context.Context, *WakeUpDeploymentRequest) (*WakeUpDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method WakeUpDeployment not implemented")
}

func (UnimplementedAdminServiceServer) RollbackDeployment(context.Context, *RollbackDeploymentRequest) (*RollbackDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RollbackDeployment not implemented")
}

func (UnimplementedAdminServiceServer) ReapplyDeployment(context.Context, *ReapplyDeploymentRequest) (*ReapplyDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReapplyDeployment not implemented")
}

func (UnimplementedAdminServiceServer) GetDeploymentJobs(context.Context, *GetDeploymentJobsRequest) (*GetDeploymentJobsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDeploymentJobs not implemented")
}

func (UnimplementedAdminServiceServer) RepairNormalizedSpec(context.Context, *RepairNormalizedSpecRequest) (*RepairNormalizedSpecResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RepairNormalizedSpec not implemented")
}

func (UnimplementedAdminServiceServer) ListFeedback(context.Context, *ListFeedbackRequest) (*ListFeedbackResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListFeedback not implemented")
}

func (UnimplementedAdminServiceServer) StopDeployment(context.Context, *StopDeploymentRequest) (*StopDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StopDeployment not implemented")
}

func (UnimplementedAdminServiceServer) RegisterCluster(context.Context, *RegisterClusterRequest) (*RegisterClusterResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RegisterCluster not implemented")
}

func (UnimplementedAdminServiceServer) EnableCluster(context.Context, *EnableClusterRequest) (*EnableClusterResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method EnableCluster not implemented")
}

func (UnimplementedAdminServiceServer) DisableCluster(context.Context, *DisableClusterRequest) (*DisableClusterResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DisableCluster not implemented")
}

func (UnimplementedAdminServiceServer) DeregisterCluster(context.Context, *DeregisterClusterRequest) (*DeregisterClusterResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeregisterCluster not implemented")
}
func (UnimplementedAdminServiceServer) GetClusterBlockers(context.Context, *GetClusterBlockersRequest) (*GetClusterBlockersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetClusterBlockers not implemented")
}

func (UnimplementedAdminServiceServer) ListClusters(context.Context, *ListClustersRequest) (*ListClustersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListClusters not implemented")
}

func (UnimplementedAdminServiceServer) ListAccountClusters(context.Context, *ListAccountClustersRequest) (*AccountClusterList, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListAccountClusters not implemented")
}
func (UnimplementedAdminServiceServer) AddAccountCluster(context.Context, *AddAccountClusterRequest) (*AccountClusterList, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AddAccountCluster not implemented")
}
func (UnimplementedAdminServiceServer) RemoveAccountCluster(context.Context, *RemoveAccountClusterRequest) (*AccountClusterList, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RemoveAccountCluster not implemented")
}
func (UnimplementedAdminServiceServer) SetAccountDefaultCluster(context.Context, *SetAccountDefaultClusterRequest) (*AccountClusterList, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SetAccountDefaultCluster not implemented")
}

func (UnimplementedAdminServiceServer) UpdateCluster(context.Context, *UpdateClusterRequest) (*UpdateClusterResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateCluster not implemented")
}

func (UnimplementedAdminServiceServer) CheckClusterHealth(context.Context, *CheckClusterHealthRequest) (*CheckClusterHealthResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CheckClusterHealth not implemented")
}
func (UnimplementedAdminServiceServer) RefreshClusterPullSecrets(context.Context, *RefreshClusterPullSecretsRequest) (*RefreshClusterPullSecretsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RefreshClusterPullSecrets not implemented")
}
func (UnimplementedAdminServiceServer) ListEvaluators(context.Context, *ListEvaluatorsRequest) (*ListEvaluatorsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListEvaluators not implemented")
}
func (UnimplementedAdminServiceServer) RunEvaluatorSweep(context.Context, *RunEvaluatorSweepRequest) (*RunEvaluatorSweepResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RunEvaluatorSweep not implemented")
}
func (UnimplementedAdminServiceServer) ListEvaluatorDrift(context.Context, *ListEvaluatorDriftRequest) (*ListEvaluatorDriftResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListEvaluatorDrift not implemented")
}
func (UnimplementedAdminServiceServer) FixDeploymentDrift(context.Context, *FixDeploymentDriftRequest) (*FixDeploymentDriftResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method FixDeploymentDrift not implemented")
}

func (UnimplementedAdminServiceServer) InvalidateAccountCaches(context.Context, *InvalidateAccountCachesRequest) (*InvalidateCachesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InvalidateAccountCaches not implemented")
}

func (UnimplementedAdminServiceServer) InvalidateAllCaches(context.Context, *InvalidateAllCachesRequest) (*InvalidateCachesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InvalidateAllCaches not implemented")
}

func (UnimplementedAdminServiceServer) ListClusterMigrations(context.Context, *ListClusterMigrationsRequest) (*ListClusterMigrationsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListClusterMigrations not implemented")
}

func (UnimplementedAdminServiceServer) ListJobKinds(context.Context, *ListJobKindsRequest) (*ListJobKindsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListJobKinds not implemented")
}

func (UnimplementedAdminServiceServer) TriggerJob(context.Context, *TriggerJobRequest) (*TriggerJobResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method TriggerJob not implemented")
}

func (UnimplementedAdminServiceServer) GetJobStates(context.Context, *GetJobStatesRequest) (*GetJobStatesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetJobStates not implemented")
}

func (UnimplementedAdminServiceServer) ListAdminQueues(context.Context, *ListAdminQueuesRequest) (*ListAdminQueuesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListAdminQueues not implemented")
}

func (UnimplementedAdminServiceServer) ListJobs(context.Context, *ListJobsRequest) (*ListJobsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListJobs not implemented")
}

func (UnimplementedAdminServiceServer) GetJob(context.Context, *GetJobRequest) (*GetJobResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetJob not implemented")
}

func (UnimplementedAdminServiceServer) CancelJobs(context.Context, *CancelJobsRequest) (*CancelJobsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CancelJobs not implemented")
}

func (UnimplementedAdminServiceServer) RetryJobs(context.Context, *RetryJobsRequest) (*RetryJobsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RetryJobs not implemented")
}

func (UnimplementedAdminServiceServer) PauseQueue(context.Context, *PauseQueueRequest) (*PauseQueueResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PauseQueue not implemented")
}

func (UnimplementedAdminServiceServer) ResumeQueue(context.Context, *ResumeQueueRequest) (*ResumeQueueResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ResumeQueue not implemented")
}

func (UnimplementedAdminServiceServer) RefreshMessagingCache(context.Context, *RefreshMessagingCacheRequest) (*RefreshMessagingCacheResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RefreshMessagingCache not implemented")
}

func (UnimplementedAdminServiceServer) ListOutboundDomains(context.Context, *ListOutboundDomainsRequest) (*ListOutboundDomainsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListOutboundDomains not implemented")
}
func (UnimplementedAdminServiceServer) ListAlerts(context.Context, *ListAlertsRequest) (*ListAlertsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListAlerts not implemented")
}
func (UnimplementedAdminServiceServer) ClearAlert(context.Context, *ClearAlertRequest) (*ClearAlertResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ClearAlert not implemented")
}
func (UnimplementedAdminServiceServer) MuteAlert(context.Context, *MuteAlertRequest) (*MuteAlertResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method MuteAlert not implemented")
}
func (UnimplementedAdminServiceServer) UnmuteAlert(context.Context, *UnmuteAlertRequest) (*UnmuteAlertResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UnmuteAlert not implemented")
}

func (UnimplementedAdminServiceServer) ListAuditFindings(context.Context, *ListAuditFindingsRequest) (*ListAuditFindingsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListAuditFindings not implemented")
}

func (UnimplementedAdminServiceServer) AcknowledgeAuditFinding(context.Context, *AcknowledgeAuditFindingRequest) (*AcknowledgeAuditFindingResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AcknowledgeAuditFinding not implemented")
}

func (UnimplementedAdminServiceServer) mustEmbedUnimplementedAdminServiceServer() {}

// UnsafeAdminServiceServer may be embedded to opt out of forward compatibility.
type UnsafeAdminServiceServer interface {
	mustEmbedUnimplementedAdminServiceServer()
}

// RegisterAdminServiceServer registers the server implementation.
func RegisterAdminServiceServer(s grpc.ServiceRegistrar, srv AdminServiceServer) {
	s.RegisterService(&AdminService_ServiceDesc, srv)
}

func _AdminService_ListDeployments_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListDeploymentsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListDeployments(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListDeployments"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListDeployments(ctx, req.(*ListDeploymentsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetDeployment(ctx, req.(*GetDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetDeploymentAccess_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetDeploymentAccessRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetDeploymentAccess(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetDeploymentAccess"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetDeploymentAccess(ctx, req.(*GetDeploymentAccessRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListAuthorizationResources_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListAuthorizationResourcesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListAuthorizationResources(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListAuthorizationResources"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListAuthorizationResources(ctx, req.(*ListAuthorizationResourcesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListAuthorizationOperations_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListAuthorizationOperationsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListAuthorizationOperations(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListAuthorizationOperations"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListAuthorizationOperations(ctx, req.(*ListAuthorizationOperationsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_StartAuthorizationResourceReset_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StartAuthorizationResourceResetRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).StartAuthorizationResourceReset(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/StartAuthorizationResourceReset"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).StartAuthorizationResourceReset(ctx, req.(*StartAuthorizationResourceResetRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_StartAuthorizationResourceBackfill_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StartAuthorizationResourceBackfillRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).StartAuthorizationResourceBackfill(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/StartAuthorizationResourceBackfill"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).StartAuthorizationResourceBackfill(ctx, req.(*StartAuthorizationResourceBackfillRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetClusterStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetClusterStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetClusterStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetClusterStatus"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetClusterStatus(ctx, req.(*GetClusterStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_DeleteDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).DeleteDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/DeleteDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).DeleteDeployment(ctx, req.(*DeleteDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RestartDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RestartDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RestartDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RestartDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RestartDeployment(ctx, req.(*RestartDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListAccounts_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListAccountsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListAccounts(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListAccounts"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListAccounts(ctx, req.(*ListAccountsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetAccount_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetAccountRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetAccount(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetAccount"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetAccount(ctx, req.(*GetAccountRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetAccountMetronomeAliases_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetAccountMetronomeAliasesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetAccountMetronomeAliases(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetAccountMetronomeAliases"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetAccountMetronomeAliases(ctx, req.(*GetAccountMetronomeAliasesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RecoverAccountMetronomeAliases_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RecoverAccountMetronomeAliasesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RecoverAccountMetronomeAliases(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RecoverAccountMetronomeAliases"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RecoverAccountMetronomeAliases(ctx, req.(*RecoverAccountMetronomeAliasesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RegisterAccountMetronome_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RegisterAccountMetronomeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RegisterAccountMetronome(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RegisterAccountMetronome"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RegisterAccountMetronome(ctx, req.(*RegisterAccountMetronomeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetAccountBillingDetail_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetAccountBillingDetailRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetAccountBillingDetail(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetAccountBillingDetail"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetAccountBillingDetail(ctx, req.(*GetAccountBillingDetailRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RetryBillingProvision_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RetryBillingProvisionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RetryBillingProvision(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RetryBillingProvision"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RetryBillingProvision(ctx, req.(*RetryBillingProvisionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ForceBillingResume_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ForceBillingResumeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ForceBillingResume(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ForceBillingResume"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ForceBillingResume(ctx, req.(*ForceBillingResumeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_SetAccountSpendLimit_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SetAccountSpendLimitRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).SetAccountSpendLimit(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/SetAccountSpendLimit"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).SetAccountSpendLimit(ctx, req.(*SetAccountSpendLimitRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RecoverAccountLangfuse_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RecoverAccountLangfuseRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RecoverAccountLangfuse(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RecoverAccountLangfuse"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RecoverAccountLangfuse(ctx, req.(*RecoverAccountLangfuseRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RecoverAccountBifrost_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RecoverAccountBifrostRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RecoverAccountBifrost(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RecoverAccountBifrost"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RecoverAccountBifrost(ctx, req.(*RecoverAccountBifrostRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RenameAccount_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RenameAccountRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RenameAccount(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RenameAccount"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RenameAccount(ctx, req.(*RenameAccountRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_DeleteAccount_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteAccountRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).DeleteAccount(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/DeleteAccount"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).DeleteAccount(ctx, req.(*DeleteAccountRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_PurgeAccount_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PurgeAccountRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).PurgeAccount(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/PurgeAccount"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).PurgeAccount(ctx, req.(*PurgeAccountRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetPodLogs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetPodLogsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetPodLogs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetPodLogs"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetPodLogs(ctx, req.(*GetPodLogsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetPodEnv_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetPodEnvRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetPodEnv(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetPodEnv"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetPodEnv(ctx, req.(*GetPodEnvRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListAgents_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListAgentsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListAgents(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListAgents"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListAgents(ctx, req.(*ListAgentsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetAgentBuilds_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetAgentBuildsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetAgentBuilds(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetAgentBuilds"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetAgentBuilds(ctx, req.(*GetAgentBuildsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ProxyHTTP_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HTTPProxyRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ProxyHTTP(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ProxyHTTP"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ProxyHTTP(ctx, req.(*HTTPProxyRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetAuthConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetAuthConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetAuthConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetAuthConfig"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetAuthConfig(ctx, req.(*GetAuthConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_StartRiverUI_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StartRiverUIRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).StartRiverUI(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/StartRiverUI"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).StartRiverUI(ctx, req.(*StartRiverUIRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_StopRiverUI_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StopRiverUIRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).StopRiverUI(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/StopRiverUI"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).StopRiverUI(ctx, req.(*StopRiverUIRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetRiverUIStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRiverUIStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetRiverUIStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetRiverUIStatus"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetRiverUIStatus(ctx, req.(*GetRiverUIStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListQuotaIncreaseRequests_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListQuotaIncreaseRequestsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListQuotaIncreaseRequests(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListQuotaIncreaseRequests"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListQuotaIncreaseRequests(ctx, req.(*ListQuotaIncreaseRequestsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ApproveQuotaIncrease_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ApproveQuotaIncreaseRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ApproveQuotaIncrease(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ApproveQuotaIncrease"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ApproveQuotaIncrease(ctx, req.(*ApproveQuotaIncreaseRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_DenyQuotaIncrease_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DenyQuotaIncreaseRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).DenyQuotaIncrease(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/DenyQuotaIncrease"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).DenyQuotaIncrease(ctx, req.(*DenyQuotaIncreaseRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetDeploymentEvents_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetDeploymentEventsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetDeploymentEvents(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetDeploymentEvents"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetDeploymentEvents(ctx, req.(*GetDeploymentEventsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_WakeUpDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(WakeUpDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).WakeUpDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/WakeUpDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).WakeUpDeployment(ctx, req.(*WakeUpDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RollbackDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RollbackDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RollbackDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RollbackDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RollbackDeployment(ctx, req.(*RollbackDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ReapplyDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReapplyDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ReapplyDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ReapplyDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ReapplyDeployment(ctx, req.(*ReapplyDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetDeploymentJobs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetDeploymentJobsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetDeploymentJobs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetDeploymentJobs"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetDeploymentJobs(ctx, req.(*GetDeploymentJobsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RepairNormalizedSpec_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RepairNormalizedSpecRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RepairNormalizedSpec(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RepairNormalizedSpec"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RepairNormalizedSpec(ctx, req.(*RepairNormalizedSpecRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListFeedback_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListFeedbackRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListFeedback(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListFeedback"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListFeedback(ctx, req.(*ListFeedbackRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_StopDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StopDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).StopDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/StopDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).StopDeployment(ctx, req.(*StopDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RegisterCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RegisterClusterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RegisterCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RegisterCluster"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RegisterCluster(ctx, req.(*RegisterClusterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_EnableCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(EnableClusterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).EnableCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/EnableCluster"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).EnableCluster(ctx, req.(*EnableClusterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_DisableCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DisableClusterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).DisableCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/DisableCluster"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).DisableCluster(ctx, req.(*DisableClusterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_DeregisterCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeregisterClusterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).DeregisterCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/DeregisterCluster"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).DeregisterCluster(ctx, req.(*DeregisterClusterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetClusterBlockers_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetClusterBlockersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetClusterBlockers(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetClusterBlockers"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetClusterBlockers(ctx, req.(*GetClusterBlockersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListClusters_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListClustersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListClusters(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListClusters"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListClusters(ctx, req.(*ListClustersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListAccountClusters_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListAccountClustersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListAccountClusters(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListAccountClusters"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListAccountClusters(ctx, req.(*ListAccountClustersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_AddAccountCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AddAccountClusterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).AddAccountCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/AddAccountCluster"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).AddAccountCluster(ctx, req.(*AddAccountClusterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RemoveAccountCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RemoveAccountClusterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RemoveAccountCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RemoveAccountCluster"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RemoveAccountCluster(ctx, req.(*RemoveAccountClusterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_SetAccountDefaultCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SetAccountDefaultClusterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).SetAccountDefaultCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/SetAccountDefaultCluster"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).SetAccountDefaultCluster(ctx, req.(*SetAccountDefaultClusterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_UpdateCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateClusterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).UpdateCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/UpdateCluster"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).UpdateCluster(ctx, req.(*UpdateClusterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_CheckClusterHealth_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CheckClusterHealthRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).CheckClusterHealth(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/CheckClusterHealth"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).CheckClusterHealth(ctx, req.(*CheckClusterHealthRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RefreshClusterPullSecrets_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RefreshClusterPullSecretsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RefreshClusterPullSecrets(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RefreshClusterPullSecrets"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RefreshClusterPullSecrets(ctx, req.(*RefreshClusterPullSecretsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListEvaluators_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListEvaluatorsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListEvaluators(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListEvaluators"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListEvaluators(ctx, req.(*ListEvaluatorsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RunEvaluatorSweep_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RunEvaluatorSweepRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RunEvaluatorSweep(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RunEvaluatorSweep"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RunEvaluatorSweep(ctx, req.(*RunEvaluatorSweepRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListEvaluatorDrift_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListEvaluatorDriftRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListEvaluatorDrift(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListEvaluatorDrift"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListEvaluatorDrift(ctx, req.(*ListEvaluatorDriftRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_FixDeploymentDrift_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(FixDeploymentDriftRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).FixDeploymentDrift(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/FixDeploymentDrift"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).FixDeploymentDrift(ctx, req.(*FixDeploymentDriftRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_InvalidateAccountCaches_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InvalidateAccountCachesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).InvalidateAccountCaches(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/InvalidateAccountCaches"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).InvalidateAccountCaches(ctx, req.(*InvalidateAccountCachesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_InvalidateAllCaches_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InvalidateAllCachesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).InvalidateAllCaches(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/InvalidateAllCaches"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).InvalidateAllCaches(ctx, req.(*InvalidateAllCachesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListClusterMigrations_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListClusterMigrationsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListClusterMigrations(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListClusterMigrations"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListClusterMigrations(ctx, req.(*ListClusterMigrationsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListJobKinds_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListJobKindsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListJobKinds(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListJobKinds"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListJobKinds(ctx, req.(*ListJobKindsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_TriggerJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TriggerJobRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).TriggerJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/TriggerJob"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).TriggerJob(ctx, req.(*TriggerJobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetJobStates_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetJobStatesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetJobStates(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetJobStates"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetJobStates(ctx, req.(*GetJobStatesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListAdminQueues_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListAdminQueuesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListAdminQueues(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListAdminQueues"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListAdminQueues(ctx, req.(*ListAdminQueuesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListJobs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListJobsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListJobs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListJobs"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListJobs(ctx, req.(*ListJobsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetJobRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetJob"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetJob(ctx, req.(*GetJobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_CancelJobs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CancelJobsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).CancelJobs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/CancelJobs"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).CancelJobs(ctx, req.(*CancelJobsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RetryJobs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RetryJobsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RetryJobs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RetryJobs"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RetryJobs(ctx, req.(*RetryJobsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_PauseQueue_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PauseQueueRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).PauseQueue(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/PauseQueue"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).PauseQueue(ctx, req.(*PauseQueueRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ResumeQueue_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ResumeQueueRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ResumeQueue(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ResumeQueue"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ResumeQueue(ctx, req.(*ResumeQueueRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RefreshMessagingCache_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RefreshMessagingCacheRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RefreshMessagingCache(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RefreshMessagingCache"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RefreshMessagingCache(ctx, req.(*RefreshMessagingCacheRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListAlerts_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListAlertsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListAlerts(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListAlerts"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListAlerts(ctx, req.(*ListAlertsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ClearAlert_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ClearAlertRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ClearAlert(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ClearAlert"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ClearAlert(ctx, req.(*ClearAlertRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_MuteAlert_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MuteAlertRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).MuteAlert(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/MuteAlert"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).MuteAlert(ctx, req.(*MuteAlertRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_UnmuteAlert_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UnmuteAlertRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).UnmuteAlert(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/UnmuteAlert"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).UnmuteAlert(ctx, req.(*UnmuteAlertRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListAuditFindings_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListAuditFindingsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListAuditFindings(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListAuditFindings"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListAuditFindings(ctx, req.(*ListAuditFindingsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_AcknowledgeAuditFinding_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AcknowledgeAuditFindingRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).AcknowledgeAuditFinding(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/AcknowledgeAuditFinding"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).AcknowledgeAuditFinding(ctx, req.(*AcknowledgeAuditFindingRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// AdminService_ServiceDesc is the grpc.ServiceDesc for AdminService service.
var AdminService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "admin.v1.AdminService",
	HandlerType: (*AdminServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "ListDeployments", Handler: _AdminService_ListDeployments_Handler},
		{MethodName: "GetDeployment", Handler: _AdminService_GetDeployment_Handler},
		{MethodName: "GetDeploymentAccess", Handler: _AdminService_GetDeploymentAccess_Handler},
		{MethodName: "ListAuthorizationResources", Handler: _AdminService_ListAuthorizationResources_Handler},
		{MethodName: "ListAuthorizationOperations", Handler: _AdminService_ListAuthorizationOperations_Handler},
		{MethodName: "StartAuthorizationResourceReset", Handler: _AdminService_StartAuthorizationResourceReset_Handler},
		{MethodName: "StartAuthorizationResourceBackfill", Handler: _AdminService_StartAuthorizationResourceBackfill_Handler},
		{MethodName: "GetClusterStatus", Handler: _AdminService_GetClusterStatus_Handler},
		{MethodName: "DeleteDeployment", Handler: _AdminService_DeleteDeployment_Handler},
		{MethodName: "RestartDeployment", Handler: _AdminService_RestartDeployment_Handler},
		{MethodName: "ListAccounts", Handler: _AdminService_ListAccounts_Handler},
		{MethodName: "GetAccount", Handler: _AdminService_GetAccount_Handler},
		{MethodName: "GetAccountMetronomeAliases", Handler: _AdminService_GetAccountMetronomeAliases_Handler},
		{MethodName: "RecoverAccountMetronomeAliases", Handler: _AdminService_RecoverAccountMetronomeAliases_Handler},
		{MethodName: "RegisterAccountMetronome", Handler: _AdminService_RegisterAccountMetronome_Handler},
		{MethodName: "GetAccountBillingDetail", Handler: _AdminService_GetAccountBillingDetail_Handler},
		{MethodName: "RetryBillingProvision", Handler: _AdminService_RetryBillingProvision_Handler},
		{MethodName: "ForceBillingResume", Handler: _AdminService_ForceBillingResume_Handler},
		{MethodName: "SetAccountSpendLimit", Handler: _AdminService_SetAccountSpendLimit_Handler},
		{MethodName: "RecoverAccountLangfuse", Handler: _AdminService_RecoverAccountLangfuse_Handler},
		{MethodName: "RecoverAccountBifrost", Handler: _AdminService_RecoverAccountBifrost_Handler},
		{MethodName: "RenameAccount", Handler: _AdminService_RenameAccount_Handler},
		{MethodName: "DeleteAccount", Handler: _AdminService_DeleteAccount_Handler},
		{MethodName: "PurgeAccount", Handler: _AdminService_PurgeAccount_Handler},
		{MethodName: "GetPodLogs", Handler: _AdminService_GetPodLogs_Handler},
		{MethodName: "GetPodEnv", Handler: _AdminService_GetPodEnv_Handler},
		{MethodName: "ListAgents", Handler: _AdminService_ListAgents_Handler},
		{MethodName: "GetAgentBuilds", Handler: _AdminService_GetAgentBuilds_Handler},
		{MethodName: "ProxyHTTP", Handler: _AdminService_ProxyHTTP_Handler},
		{MethodName: "GetAuthConfig", Handler: _AdminService_GetAuthConfig_Handler},
		{MethodName: "StartRiverUI", Handler: _AdminService_StartRiverUI_Handler},
		{MethodName: "StopRiverUI", Handler: _AdminService_StopRiverUI_Handler},
		{MethodName: "GetRiverUIStatus", Handler: _AdminService_GetRiverUIStatus_Handler},
		{MethodName: "ListQuotaIncreaseRequests", Handler: _AdminService_ListQuotaIncreaseRequests_Handler},
		{MethodName: "ApproveQuotaIncrease", Handler: _AdminService_ApproveQuotaIncrease_Handler},
		{MethodName: "DenyQuotaIncrease", Handler: _AdminService_DenyQuotaIncrease_Handler},
		{MethodName: "GetDeploymentEvents", Handler: _AdminService_GetDeploymentEvents_Handler},
		{MethodName: "WakeUpDeployment", Handler: _AdminService_WakeUpDeployment_Handler},
		{MethodName: "RollbackDeployment", Handler: _AdminService_RollbackDeployment_Handler},
		{MethodName: "ReapplyDeployment", Handler: _AdminService_ReapplyDeployment_Handler},
		{MethodName: "GetDeploymentJobs", Handler: _AdminService_GetDeploymentJobs_Handler},
		{MethodName: "RepairNormalizedSpec", Handler: _AdminService_RepairNormalizedSpec_Handler},
		{MethodName: "ListFeedback", Handler: _AdminService_ListFeedback_Handler},
		{MethodName: "StopDeployment", Handler: _AdminService_StopDeployment_Handler},
		{MethodName: "RegisterCluster", Handler: _AdminService_RegisterCluster_Handler},
		{MethodName: "EnableCluster", Handler: _AdminService_EnableCluster_Handler},
		{MethodName: "DisableCluster", Handler: _AdminService_DisableCluster_Handler},
		{MethodName: "DeregisterCluster", Handler: _AdminService_DeregisterCluster_Handler},
		{MethodName: "GetClusterBlockers", Handler: _AdminService_GetClusterBlockers_Handler},
		{MethodName: "ListClusters", Handler: _AdminService_ListClusters_Handler},
		{MethodName: "ListAccountClusters", Handler: _AdminService_ListAccountClusters_Handler},
		{MethodName: "AddAccountCluster", Handler: _AdminService_AddAccountCluster_Handler},
		{MethodName: "RemoveAccountCluster", Handler: _AdminService_RemoveAccountCluster_Handler},
		{MethodName: "SetAccountDefaultCluster", Handler: _AdminService_SetAccountDefaultCluster_Handler},
		{MethodName: "UpdateCluster", Handler: _AdminService_UpdateCluster_Handler},
		{MethodName: "CheckClusterHealth", Handler: _AdminService_CheckClusterHealth_Handler},
		{MethodName: "RefreshClusterPullSecrets", Handler: _AdminService_RefreshClusterPullSecrets_Handler},
		{MethodName: "ListEvaluators", Handler: _AdminService_ListEvaluators_Handler},
		{MethodName: "RunEvaluatorSweep", Handler: _AdminService_RunEvaluatorSweep_Handler},
		{MethodName: "ListEvaluatorDrift", Handler: _AdminService_ListEvaluatorDrift_Handler},
		{MethodName: "FixDeploymentDrift", Handler: _AdminService_FixDeploymentDrift_Handler},
		{MethodName: "InvalidateAccountCaches", Handler: _AdminService_InvalidateAccountCaches_Handler},
		{MethodName: "InvalidateAllCaches", Handler: _AdminService_InvalidateAllCaches_Handler},
		{MethodName: "ListClusterMigrations", Handler: _AdminService_ListClusterMigrations_Handler},
		{MethodName: "ListJobKinds", Handler: _AdminService_ListJobKinds_Handler},
		{MethodName: "TriggerJob", Handler: _AdminService_TriggerJob_Handler},
		{MethodName: "GetJobStates", Handler: _AdminService_GetJobStates_Handler},
		{MethodName: "ListAdminQueues", Handler: _AdminService_ListAdminQueues_Handler},
		{MethodName: "ListJobs", Handler: _AdminService_ListJobs_Handler},
		{MethodName: "GetJob", Handler: _AdminService_GetJob_Handler},
		{MethodName: "CancelJobs", Handler: _AdminService_CancelJobs_Handler},
		{MethodName: "RetryJobs", Handler: _AdminService_RetryJobs_Handler},
		{MethodName: "PauseQueue", Handler: _AdminService_PauseQueue_Handler},
		{MethodName: "ResumeQueue", Handler: _AdminService_ResumeQueue_Handler},
		{MethodName: "RefreshMessagingCache", Handler: _AdminService_RefreshMessagingCache_Handler},
		{MethodName: "ListOutboundDomains", Handler: _AdminService_ListOutboundDomains_Handler},
		{MethodName: "ListAlerts", Handler: _AdminService_ListAlerts_Handler},
		{MethodName: "ClearAlert", Handler: _AdminService_ClearAlert_Handler},
		{MethodName: "MuteAlert", Handler: _AdminService_MuteAlert_Handler},
		{MethodName: "UnmuteAlert", Handler: _AdminService_UnmuteAlert_Handler},
		{MethodName: "ListAuditFindings", Handler: _AdminService_ListAuditFindings_Handler},
		{MethodName: "AcknowledgeAuditFinding", Handler: _AdminService_AcknowledgeAuditFinding_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/admin/v1/admin.proto",
}

func (c *adminServiceClient) ListOutboundDomains(ctx context.Context, in *ListOutboundDomainsRequest, opts ...grpc.CallOption) (*ListOutboundDomainsResponse, error) {
	out := new(ListOutboundDomainsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListOutboundDomains", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func _AdminService_ListOutboundDomains_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListOutboundDomainsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListOutboundDomains(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListOutboundDomains"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListOutboundDomains(ctx, req.(*ListOutboundDomainsRequest))
	}
	return interceptor(ctx, in, info, handler)
}
