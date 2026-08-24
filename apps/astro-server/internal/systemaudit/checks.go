package systemaudit

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

type Check struct {
	Name     string
	Severity string
	Title    string
	Query    string
}

var checks = []Check{
	{
		Name:     "account.no_members",
		Severity: SeverityWarning,
		Title:    "Account has no members",
		Query: `
			SELECT a.id::text AS subject_id, a.name AS subject_label,
			       jsonb_build_object(
			         'type', a.type,
			         'has_owner', a.owner_user_id IS NOT NULL,
			         'live_deployments', (SELECT count(*) FROM deployments d
			                               WHERE d.account_id = a.id AND d.undeployed_at IS NULL),
			         'has_billing_customer', coalesce(a.metronome_customer_id, '') <> ''
			                                 OR coalesce(a.stripe_customer_id, '') <> '',
			         'created_at', a.created_at
			       )
			  FROM accounts a
			 WHERE a.deleted_at IS NULL
			   AND NOT EXISTS (SELECT 1 FROM account_members m WHERE m.account_id = a.id)`,
	},
	{
		Name:     "account.no_owner",
		Severity: SeverityWarning,
		Title:    "Account has no owner recorded",
		Query: `
			SELECT a.id::text AS subject_id, a.name AS subject_label,
			       jsonb_build_object(
			         'type', a.type,
			         'members', (SELECT count(*) FROM account_members m WHERE m.account_id = a.id),
			         'workos_org_id', coalesce((SELECT ao.workos_org_id FROM account_organizations ao
			                                     WHERE ao.account_id = a.id), ''),
			         'created_at', a.created_at
			       )
			  FROM accounts a
			 WHERE a.deleted_at IS NULL
			   AND a.owner_user_id IS NULL`,
	},
	{
		Name:     "account.owner_not_member",
		Severity: SeverityError,
		Title:    "Recorded owner is not a member",
		Query: `
			SELECT a.id::text AS subject_id, a.name AS subject_label,
			       jsonb_build_object(
			         'type', a.type,
			         'owner_user_id', a.owner_user_id,
			         'members', (SELECT count(*) FROM account_members m WHERE m.account_id = a.id)
			       )
			  FROM accounts a
			 WHERE a.deleted_at IS NULL
			   AND a.owner_user_id IS NOT NULL
			   AND NOT EXISTS (
			         SELECT 1 FROM account_members m
			          WHERE m.account_id = a.id AND m.user_id = a.owner_user_id)`,
	},
	{
		Name:     "deployment.stuck_transition",
		Severity: SeverityError,
		Title:    "Deployment stuck mid-transition",
		Query: `
			SELECT d.id AS subject_id, a.name || '/' || d.agent_name AS subject_label,
			       jsonb_build_object(
			         'account', a.name,
			         'agent', d.agent_name,
			         'status', d.status,
			         'cluster_id', coalesce(d.cluster_id, ''),
			         'status_changed_at', d.status_changed_at
			       )
			  FROM deployments d
			  JOIN accounts a ON a.id = d.account_id
			 WHERE d.status IN ('pending', 'provisioning', 'deploying', 'undeploying')
			   AND d.status_changed_at < now() - interval '1 hour'`,
	},
	{
		Name:     "cluster.config_stale",
		Severity: SeverityError,
		Title:    "Cluster config not synced",
		Query: `
			SELECT c.id AS subject_id, c.id AS subject_label,
			       jsonb_build_object(
			         'region', c.region,
			         'eks_cluster_name', c.eks_cluster_name,
			         'config_synced_at', c.config_synced_at
			       )
			  FROM clusters c
			 WHERE c.config_synced_at IS NULL
			    OR c.config_synced_at < now() - interval '24 hours'`,
	},
	{
		Name:     "billing.unprovisioned",
		Severity: SeverityWarning,
		Title:    "Account never provisioned for billing",
		Query: `
			SELECT a.id::text AS subject_id, a.name AS subject_label,
			       jsonb_build_object(
			         'type', a.type,
			         'created_at', a.created_at,
			         'has_billing_customer', coalesce(a.metronome_customer_id, '') <> ''
			       )
			  FROM accounts a
			 WHERE a.deleted_at IS NULL
			   AND a.billing_provisioned_at IS NULL
			   AND a.created_at < now() - interval '24 hours'`,
	},
}

func Checks() []Check { return checks }
