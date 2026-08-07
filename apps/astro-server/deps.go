package main

import (
	"database/sql"

	"github.com/astropods/astro/apps/astro-server/handlers"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
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
	Ent   *middleware.Entitlements
	Quota *quota.DBChecker
	Probe *handlers.ProbeHandler

	Stores  Stores
	Clients Clients
}

type Stores struct {
	Account           *account.AccountStore
	Deployment        *deploymentstore.Store
	AccountVars       *accountvars.Store
	Heart             *heartstore.Store
	AgentMetrics      *metricsstore.Store
	Cluster           *clusterstore.Store
	Audit             *auditlog.Store
	Avatar            *avatar.Store
	ReadmeAssets      *readmeassets.Store
	Knowledge         *knowledgestore.Store
	GH                *githubconnection.Store
	Webhook           *githubwebhook.Store
	SlackID           *slackidentity.Store
	Watcher           *watcher.Store
	DeploymentFGASync *authz.DeploymentFGASyncStore
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

	// Observability provisioners, wired onto the admin server for the account
	// detail view's recover actions. Nil when their backends are unconfigured.
	AIGateway           *aigateway.Provisioner
	LangfuseProvisioner *langfuse.Provisioner
	KMSClient           envelope.KMSClient
}
