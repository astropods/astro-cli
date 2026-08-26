package main

import (
	"database/sql"

	"github.com/astropods/astro/apps/astro-server/handlers"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountlifecycle"
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/appstore"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/connectapps"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/experiment"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/githubwebhook"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/metricsstore"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	"github.com/astropods/astro/apps/astro-server/internal/readmeassets"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
	"github.com/astropods/astro/apps/astro-server/internal/watcher"
)

// Deps bundles every shared dependency that route handlers need. Built once in
// runAPI and threaded through setupRoutes. Grouped into Stores and Clients so
// the top level stays small; new dependencies should join the right sub-struct
// rather than getting added as a positional argument anywhere.
type Deps struct {
	Log   *logger.Logger
	Cfg   *config.Config
	DB    *sql.DB
	Vault *envelope.Vault
	Ent   *middleware.Entitlements
	Quota *quota.DBChecker
	Probe *handlers.ProbeHandler

	Stores  Stores
	Clients Clients
}

type Stores struct {
	Account                   *account.AccountStore
	App                       *appstore.Store
	Deployment                *deploymentstore.Store
	AccountVars               *accountvars.Store
	Heart                     *heartstore.Store
	AgentMetrics              *metricsstore.Store
	Cluster                   *clusterstore.Store
	Audit                     *auditlog.Store
	Avatar                    *avatar.Store
	ReadmeAssets              *readmeassets.Store
	Knowledge                 *knowledgestore.Store
	GH                        *githubconnection.Store
	Webhook                   *githubwebhook.Store
	SlackID                   *slackidentity.Store
	Watcher                   *watcher.Store
	DeploymentFGASync         *authz.DeploymentFGASyncStore
	AuthorizationResourceSync *authz.AuthorizationResourceSyncStore
	ResourceAccessSync        *authz.ResourceAccessSyncStore
	Experiment                *experiment.Store
	// BillingStatus is nil for non-metronome backends; handlers treat that as
	// "every account active".
	BillingStatus *billing.StatusStore
}

type Clients struct {
	AgentIndex *agentindex.Index
	K8s        k8s.ClusterClient
	Registry   *k8s.Registry
	Loki       *loki.Client
	Org        *org.Client
	OrgSync    *org.Sync
	Billing    billing.BillingProvider
	Payment    payment.Provider
	Pipes      *pipes.Client
	Prom       *promquery.Client
	K8sCache   k8scache.Cache
	Preflight  *k8s.ImagePreflighter
	Queue      *riverqueue.Queue
	FGA        authz.FGA
	// ConnectApps is nil when no WorkOS API key is configured; handlers then
	// report apps unavailable.
	ConnectApps connectapps.Client

	// Observability provisioners, wired onto the admin server for the account
	// detail view's recover actions. Nil when their backends are unconfigured.
	AIGateway           *aigateway.Provisioner
	LangfuseProvisioner *langfuse.Provisioner

	// AccountDeleter is the account soft-delete sequence, shared by the
	// owner-facing delete route and the admin server's DeleteAccount.
	AccountDeleter *accountlifecycle.Deleter

	// AccountPurger is the hard-delete sequence behind the admin server's
	// PurgeAccount. The worker builds its own for the periodic sweep; this one
	// belongs to the API process, which is where the admin gRPC server runs.
	AccountPurger *accountlifecycle.Purger
}
