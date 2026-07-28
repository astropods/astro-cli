package riverqueue

const evalJudgeMaxWorkers = 3

// Queue names. A job is routed to a queue by its Args' InsertOpts().Queue;
// per-queue worker-pool sizes are configured in New (client.go). Queues group
// jobs by domain so a backlog in one class of work can't starve another — e.g.
// a burst of builds can't consume the workers that deploys or billing need.
//
// The default queue (river.QueueDefault) is the fallback for anything unrouted.
const (
	queueDeploy      = "deploy"      // deployment lifecycle: deploy/undeploy/wakeup/migrate_cluster
	queueBuild       = "build"       // container image builds
	queueBilling     = "billing"     // dunning sweep, suspend/resume
	queueMetering    = "metering"    // usage heartbeat, message-count sync
	queueInsights    = "insights"    // insights + observability summary refresh
	queueMaintenance = "maintenance" // periodic backfills, reconciles, purges, privatelink
	queueEvalJudge   = "eval-judge"  // eval-dataset judgment prediction generation
)
