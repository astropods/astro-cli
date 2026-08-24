package auditlog

// Action constants follow the pattern "<resource>.<verb>".
const (
	AccountCreate           = "account.create"
	AccountRename           = "account.rename"
	AccountSetCluster       = "account.set_cluster"
	AccountDelete           = "account.delete"
	AccountPurge            = "account.purge"
	AccountUpdateExperiment = "account.update_experiment"

	BillingRecoverAliases        = "billing.recover_aliases"
	BillingRegisterMetronome     = "billing.register_metronome"
	BillingRetryProvision        = "billing.retry_provision"
	BillingForceResume           = "billing.force_resume"
	ObservabilityRecoverLangfuse = "observability.recover_langfuse"
	ObservabilityRecoverBifrost  = "observability.recover_bifrost"
	ProfileUpdate                = "profile.update"

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
	DeploymentCancel           = "deployment.cancel"
	DeploymentWakeup           = "deployment.wakeup"
	DeploymentRollback         = "deployment.rollback"
	DeploymentRestartPod       = "deployment.restart_pod"
	DeploymentDelete           = "deployment.delete"
	DeploymentRestart          = "deployment.restart"
	DeploymentTriggerIngestion = "deployment.trigger_ingestion"
	DeploymentGrantAccess      = "deployment.grant_access"
	DeploymentRevokeAccess     = "deployment.revoke_access"
	AccessGroupCreate          = "access_group.create"
	AccessGroupUpdate          = "access_group.update"
	AccessGroupDelete          = "access_group.delete"
	AccessGroupAddMember       = "access_group.add_member"
	AccessGroupRemoveMember    = "access_group.remove_member"

	AvatarUpload = "avatar.upload"
	AvatarPreset = "avatar.preset"
	AvatarReset  = "avatar.reset"

	QuotaApprove = "quota.approve"
	QuotaDeny    = "quota.deny"

	CacheInvalidateAccount = "cache.invalidate_account"
	CacheInvalidateAll     = "cache.invalidate_all"

	ImageCacheRefreshMessaging = "image_cache.refresh_messaging"
)
