package systemaudit

import (
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/accountlifecycle"
)

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
		// The purge sweep skips an account it cannot finish and reports success
		// anyway, so a permanently blocked purge is invisible in the job history.
		// This is the only thing that surfaces one. The grace day past the
		// retention window keeps the hourly sweep's normal lag out of the results.
		Name:     "account.purge_overdue",
		Severity: SeverityError,
		Title:    "Soft-deleted account not purged",
		Query: fmt.Sprintf(`
			SELECT a.id::text AS subject_id, a.name AS subject_label,
			       jsonb_build_object(
			         'type', a.type,
			         'deleted_at', a.deleted_at,
			         'days_deleted', floor(extract(epoch FROM now() - a.deleted_at) / 86400),
			         'retention_days', %[1]d,
			         'pending_deployments', (SELECT count(*) FROM deployments d
			                                  WHERE d.account_id = a.id AND d.status <> 'undeployed'),
				         'pending_authorization', (
				           (SELECT count(*) FROM authorization_resource_sync s
				             WHERE s.account_id = a.id
				               AND (s.synced_state IS DISTINCT FROM s.desired_state
				                    OR s.synced_version IS DISTINCT FROM s.desired_version))
				           +
				           (SELECT count(*) FROM deployment_fga_sync s
				              JOIN deployments d ON d.id = s.deployment_id
				             WHERE d.account_id = a.id
				               AND NOT EXISTS (
				                 SELECT 1 FROM authorization_resource_sync r
				                  WHERE r.resource_type = 'deployment' AND r.resource_id = d.id
				               )
				               AND (s.synced_state IS DISTINCT FROM s.desired_state
				                    OR s.synced_version IS DISTINCT FROM s.desired_version))
				         ),
			         'has_langfuse_project', EXISTS (SELECT 1 FROM account_langfuse l
			                                          WHERE l.account_id = a.id)
			       )
			  FROM accounts a
			 WHERE a.deleted_at IS NOT NULL
			   AND a.deleted_at < now() - interval '%[1]d days' - interval '1 day'`,
			accountlifecycle.RetentionDays),
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
