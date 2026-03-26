package auditlog

// Action constants follow the pattern "<resource>.<verb>".
const (
	AccountCreate = "account.create"
	AccountRename = "account.rename"
	AccountDelete = "account.delete"
	ProfileUpdate = "profile.update"

	MemberAdd        = "member.add"
	MemberUpdateRole = "member.update_role"
	MemberRemove     = "member.remove"

	InvitationCreate = "invitation.create"
	InvitationRevoke = "invitation.revoke"

	AgentRegister      = "agent.register"
	AgentArchive       = "agent.archive"
	AgentSetVisibility = "agent.set_visibility"
	AgentTransfer      = "agent.transfer"

	DeploymentDeploy           = "deployment.deploy"
	DeploymentUndeploy         = "deployment.undeploy"
	DeploymentStop             = "deployment.stop"
	DeploymentWakeup           = "deployment.wakeup"
	DeploymentRollback         = "deployment.rollback"
	DeploymentRestartPod       = "deployment.restart_pod"
	DeploymentDelete           = "deployment.delete"
	DeploymentRestart          = "deployment.restart"
	DeploymentTriggerIngestion = "deployment.trigger_ingestion"

	AvatarUpload = "avatar.upload"
	AvatarPreset = "avatar.preset"
	AvatarReset  = "avatar.reset"

	QuotaApprove = "quota.approve"
	QuotaDeny    = "quota.deny"
)
