package admingrpc

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	connectv1 "github.com/astropods/astro/packages/astro-proto/connect/v1"

	spec "github.com/astropods/astro-spec"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/clustercfg"
	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/imagecache"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/observation"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Server implements adminv1.AdminServiceServer.
// CommandDispatcher sends commands to connected devices.
type CommandDispatcher interface {
	SendCommand(ctx context.Context, deviceID string, cmd *connectv1.ShellCommand) (*connectv1.CommandResult, error)
}

// adminJobQueue enqueues River worker jobs from admin gRPC handlers.
type adminJobQueue interface {
	InsertUndeployJob(ctx context.Context, deploymentID, clusterID string) error
	InsertWakeUpJob(ctx context.Context, deploymentID, clusterID string) error
	InsertDeployJob(ctx context.Context, deploymentID, clusterID string) error
	InsertMigrateDeploymentClusterJob(ctx context.Context, deploymentID, targetClusterID, sourceClusterID string) error
	InsertBillingProvision(ctx context.Context, accountID string) error
	InsertBillingResume(ctx context.Context, accountID string) error
	TriggerJob(ctx context.Context, kind string, argsJSON json.RawMessage) (int64, error)
	CancelJob(ctx context.Context, id int64) error
	RetryJob(ctx context.Context, id int64) (bool, error)
	PauseQueue(ctx context.Context, name string) error
	ResumeQueue(ctx context.Context, name string) error
}

type Server struct {
	adminv1.UnimplementedAdminServiceServer

	log            *logger.Logger
	deployStore    *deploymentstore.Store
	k8sClient      k8s.ClusterClient
	clusterStore   *clusterstore.Store
	k8sRegistry    *k8s.Registry
	lokiClient     *loki.Client
	db             *sql.DB
	cmdDispatch    CommandDispatcher
	httpHandler    http.Handler
	workosClientID string
	databaseURL    string
	queue          adminJobQueue
	cache          k8scache.Cache

	auditStore   *auditlog.Store
	workosClient *auth.WorkOSClient

	// alertStore backs the observation-alert admin surface (ListAlerts and the
	// clear/mute/unmute actions). Constructed in New from the shared db.
	alertStore *observation.Store

	// quotaReporter resolves per-account resource usage and effective limits for
	// the account detail view. Nil until SetQuotaReporter is called; GetAccount
	// then returns no limits rather than failing.
	quotaReporter quota.Reporter

	// billingProvider backs the Metronome ingest-alias health check on the account
	// detail view. Nil until SetBillingProvider is called; the check then reports
	// "not configured" rather than failing.
	billingProvider billing.BillingProvider

	// paymentProvider reads the saved card. Nil until set; the view then omits it.
	paymentProvider payment.Provider

	// billingEnforced mirrors BILLING_GATE_ENFORCE: acted on, or only observed.
	billingEnforced bool

	// metronomeDashboardEnv is the deep link's environment segment, empty for
	// the default environment.
	metronomeDashboardEnv string

	// Langfuse + Bifrost recovery for the account detail view. Each is nil until
	// its setter is called; the corresponding recover RPC then reports "not
	// configured" rather than failing.
	langfuseProvisioner  *langfuse.Provisioner
	kmsClient            envelope.KMSClient
	kmsKeyARN            string
	aiGatewayProvisioner *aigateway.Provisioner

	riverMu        sync.Mutex
	riverUIHandler http.Handler
	riverUICleanup func()

	// imageRefresher force-refreshes ECR pull-through cache images (e.g. the
	// messaging sidecar). Nil until SetImageRefresher is called.
	imageRefresher *imagecache.Refresher

	// promClient backs ListOutboundDomains. Nil until SetPrometheusClient is
	// called; the RPC then reports FailedPrecondition.
	promClient *promquery.Client
}

// SetHTTPHandler sets the HTTP handler (gin router) for proxying HTTP requests.
func (s *Server) SetHTTPHandler(h http.Handler) {
	s.httpHandler = h
}

// resolveIngressForCluster returns the ingress hostnames for clusterID. Used
// by admin operations (RepairNormalizedSpec) that need the same hostnames
// the deployer will write.
func (s *Server) resolveIngressForCluster(ctx context.Context, clusterID string) (clustercfg.Resolved, error) {
	return clustercfg.Resolve(ctx, s.k8sRegistry, config.DeploymentConfig{}, clusterID)
}

// SetWorkOSClientID sets the WorkOS client ID for GetAuthConfig.
func (s *Server) SetWorkOSClientID(id string) {
	s.workosClientID = id
}

// SetWorkOSClient sets the WorkOS client for resolving user emails.
func (s *Server) SetWorkOSClient(c *auth.WorkOSClient) {
	s.workosClient = c
}

// SetQuotaReporter sets the reporter used by GetAccount to resolve per-account
// resource usage and effective limits.
func (s *Server) SetQuotaReporter(r quota.Reporter) {
	s.quotaReporter = r
}

// SetBillingProvider sets the billing provider used to check a Metronome
// customer's ingest aliases from the account detail view.
func (s *Server) SetBillingProvider(p billing.BillingProvider) {
	s.billingProvider = p
}

// SetBillingView wires what the billing panel needs beyond the provider: the
// card vault, the enforcement flag, and the Metronome dashboard environment.
func (s *Server) SetBillingView(p payment.Provider, enforced bool, metronomeDashboardEnv string) {
	s.paymentProvider = p
	s.billingEnforced = enforced
	s.metronomeDashboardEnv = metronomeDashboardEnv
}

// SetLangfuseProvisioner wires the Langfuse project provisioner (and the KMS
// deps it needs) used by RecoverAccountLangfuse.
func (s *Server) SetLangfuseProvisioner(p *langfuse.Provisioner, kmsClient envelope.KMSClient, kmsKeyARN string) {
	s.langfuseProvisioner = p
	s.kmsClient = kmsClient
	s.kmsKeyARN = kmsKeyARN
}

// SetAIGatewayProvisioner wires the AI-gateway provisioner used by
// RecoverAccountBifrost to ensure a Bifrost customer.
func (s *Server) SetAIGatewayProvisioner(p *aigateway.Provisioner) {
	s.aiGatewayProvisioner = p
}

// New creates a new admin gRPC server.
func New(
	log *logger.Logger,
	deployStore *deploymentstore.Store,
	k8sClient k8s.ClusterClient,
	lokiClient *loki.Client,
	db *sql.DB,
	databaseURL string,
	queue *riverqueue.Queue,
	auditStore *auditlog.Store,
	clusterStore *clusterstore.Store,
	k8sRegistry *k8s.Registry,
	cache k8scache.Cache,
) *Server {
	return &Server{
		log:          log,
		deployStore:  deployStore,
		k8sClient:    k8sClient,
		clusterStore: clusterStore,
		k8sRegistry:  k8sRegistry,
		lokiClient:   lokiClient,
		db:           db,
		databaseURL:  databaseURL,
		queue:        queue,
		auditStore:   auditStore,
		cache:        cache,
		alertStore:   observation.NewStore(db),
	}
}

// StartRiverUI starts the River UI handler. No-op if already running.
func (s *Server) StartRiverUI(_ context.Context, _ *adminv1.StartRiverUIRequest) (*adminv1.StartRiverUIResponse, error) {
	s.riverMu.Lock()
	defer s.riverMu.Unlock()

	if s.riverUIHandler != nil {
		return &adminv1.StartRiverUIResponse{Status: "already_running"}, nil
	}

	handler, cleanup, err := riverqueue.UIHandler(context.Background(), s.databaseURL, s.log.Logger)
	if err != nil {
		return nil, fmt.Errorf("start river UI: %w", err)
	}
	s.riverUIHandler = handler
	s.riverUICleanup = cleanup
	s.log.Info("River UI started")

	return &adminv1.StartRiverUIResponse{Status: "started"}, nil
}

// StopRiverUI stops the River UI handler and frees resources. No-op if not running.
func (s *Server) StopRiverUI(_ context.Context, _ *adminv1.StopRiverUIRequest) (*adminv1.StopRiverUIResponse, error) {
	s.riverMu.Lock()
	defer s.riverMu.Unlock()

	if s.riverUIHandler == nil {
		return &adminv1.StopRiverUIResponse{Status: "not_running"}, nil
	}

	if s.riverUICleanup != nil {
		s.riverUICleanup()
	}
	s.riverUIHandler = nil
	s.riverUICleanup = nil
	s.log.Info("River UI stopped")

	return &adminv1.StopRiverUIResponse{Status: "stopped"}, nil
}

// GetRiverUIStatus returns whether River UI is currently running.
func (s *Server) GetRiverUIStatus(_ context.Context, _ *adminv1.GetRiverUIStatusRequest) (*adminv1.GetRiverUIStatusResponse, error) {
	s.riverMu.Lock()
	defer s.riverMu.Unlock()
	return &adminv1.GetRiverUIStatusResponse{Running: s.riverUIHandler != nil}, nil
}

func (s *Server) ListJobKinds(_ context.Context, _ *adminv1.ListJobKindsRequest) (*adminv1.ListJobKindsResponse, error) {
	infos := riverqueue.RegisteredJobKinds()
	kinds := make([]adminv1.JobKindInfo, len(infos))
	for i, info := range infos {
		kinds[i] = adminv1.JobKindInfo{Kind: info.Kind, ArgsSchema: info.ArgsSchema}
	}
	return &adminv1.ListJobKindsResponse{Kinds: kinds}, nil
}

func (s *Server) TriggerJob(ctx context.Context, req *adminv1.TriggerJobRequest) (*adminv1.TriggerJobResponse, error) {
	id, err := s.queue.TriggerJob(ctx, req.Kind, req.ArgsJSON)
	if err != nil {
		return nil, err
	}
	return &adminv1.TriggerJobResponse{JobID: id}, nil
}

func (s *Server) GetJobStates(ctx context.Context, _ *adminv1.GetJobStatesRequest) (*adminv1.GetJobStatesResponse, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT state, COUNT(*) FROM river.river_job GROUP BY state
	`)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get job states: %v", err)
	}
	defer rows.Close() //nolint:errcheck
	resp := &adminv1.GetJobStatesResponse{}
	for rows.Next() {
		var state string
		var count int64
		if err := rows.Scan(&state, &count); err != nil {
			continue
		}
		switch state {
		case "available":
			resp.Available = count
		case "cancelled":
			resp.Cancelled = count
		case "completed":
			resp.Completed = count
		case "discarded":
			resp.Discarded = count
		case "pending":
			resp.Pending = count
		case "retryable":
			resp.Retryable = count
		case "running":
			resp.Running = count
		case "scheduled":
			resp.Scheduled = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "get job states: %v", err)
	}
	return resp, nil
}

func (s *Server) ListAdminQueues(ctx context.Context, _ *adminv1.ListAdminQueuesRequest) (*adminv1.ListAdminQueuesResponse, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT q.name, q.paused_at, q.updated_at,
		       COALESCE(j.available, 0), COALESCE(j.running, 0)
		FROM river.river_queue q
		LEFT JOIN (
		    SELECT queue,
		           COUNT(*) FILTER (WHERE state = 'available') AS available,
		           COUNT(*) FILTER (WHERE state = 'running')   AS running
		    FROM river.river_job
		    WHERE state IN ('available', 'running')
		    GROUP BY queue
		) j ON j.queue = q.name
		ORDER BY q.name
	`)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list admin queues: %v", err)
	}
	defer rows.Close() //nolint:errcheck
	var queues []*adminv1.AdminQueue
	for rows.Next() {
		var q adminv1.AdminQueue
		var pausedAt sql.NullTime
		var updatedAt time.Time
		if err := rows.Scan(&q.Name, &pausedAt, &updatedAt, &q.CountAvailable, &q.CountRunning); err != nil {
			continue
		}
		q.UpdatedAt = updatedAt.Format(time.RFC3339)
		if pausedAt.Valid {
			q.PausedAt = pausedAt.Time.Format(time.RFC3339)
		}
		queues = append(queues, &q)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "list admin queues: %v", err)
	}
	return &adminv1.ListAdminQueuesResponse{Queues: queues}, nil
}

func (s *Server) ListJobs(ctx context.Context, req *adminv1.ListJobsRequest) (*adminv1.ListJobsResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	if req.AnchorID > 0 {
		return s.listJobsAroundAnchor(ctx, req, limit)
	}
	return s.listJobsBefore(ctx, req, limit)
}

func listJobsFilters(req *adminv1.ListJobsRequest) ([]string, []interface{}) {
	where := []string{}
	args := []interface{}{}
	if req.State != "" {
		where = append(where, fmt.Sprintf("state = $%d", len(args)+1))
		args = append(args, req.State)
	}
	if len(req.Kinds) > 0 {
		where = append(where, fmt.Sprintf("kind = ANY($%d)", len(args)+1))
		args = append(args, pq.Array(req.Kinds))
	}
	if req.Queue != "" {
		where = append(where, fmt.Sprintf("queue = $%d", len(args)+1))
		args = append(args, req.Queue)
	}
	return where, args
}

func listJobsSelect(where []string, limitArg int, orderBy string) string {
	q := `SELECT id, kind, queue, state, attempt, max_attempts,
	             created_at, attempted_at, finalized_at, scheduled_at,
	             args, errors::text, priority
	      FROM river.river_job`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += fmt.Sprintf(" ORDER BY %s LIMIT $%d", orderBy, limitArg) //nolint:gosec
	return q
}

func (s *Server) listJobsBefore(ctx context.Context, req *adminv1.ListJobsRequest, limit int) (*adminv1.ListJobsResponse, error) {
	where, args := listJobsFilters(req)
	if req.BeforeID > 0 {
		where = append(where, fmt.Sprintf("id < $%d", len(args)+1))
		args = append(args, req.BeforeID)
	}
	args = append(args, limit+1)
	jobs, err := s.queryRiverJobs(ctx, listJobsSelect(where, len(args), "id DESC"), args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list jobs: %v", err)
	}
	return pageJobs(jobs, limit), nil
}

func (s *Server) listJobsAroundAnchor(ctx context.Context, req *adminv1.ListJobsRequest, limit int) (*adminv1.ListJobsResponse, error) {
	where, args := listJobsFilters(req)
	anchorWhere := append(append([]string{}, where...), fmt.Sprintf("id = $%d", len(args)+1))
	anchorArgs := append(append([]interface{}{}, args...), req.AnchorID)
	anchorRow := s.db.QueryRowContext(ctx, listJobsSelect(anchorWhere, len(anchorArgs)+1, "id DESC"), append(anchorArgs, 1)...)
	anchor, err := scanRiverJobRow(anchorRow)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &adminv1.ListJobsResponse{Jobs: []*adminv1.AdminRiverJob{}}, nil
		}
		return nil, status.Errorf(codes.Internal, "list jobs: %v", err)
	}

	newerLimit := (limit - 1) / 2
	olderLimit := limit - 1 - newerLimit

	var newer []*adminv1.AdminRiverJob
	if newerLimit > 0 {
		newerWhere := append(append([]string{}, where...), fmt.Sprintf("id > $%d", len(args)+1))
		newerArgs := append(append([]interface{}{}, args...), req.AnchorID, newerLimit)
		newer, err = s.queryRiverJobs(ctx, listJobsSelect(newerWhere, len(newerArgs), "id ASC"), newerArgs...)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "list jobs: %v", err)
		}
		olderLimit += newerLimit - len(newer)
	}

	olderWhere := append(append([]string{}, where...), fmt.Sprintf("id < $%d", len(args)+1))
	olderArgs := append(append([]interface{}{}, args...), req.AnchorID, olderLimit+1)
	older, err := s.queryRiverJobs(ctx, listJobsSelect(olderWhere, len(olderArgs), "id DESC"), olderArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list jobs: %v", err)
	}

	jobs := append(reverseJobs(newer), anchor)
	jobs = append(jobs, older...)
	return pageJobs(jobs, limit), nil
}

func (s *Server) queryRiverJobs(ctx context.Context, q string, args ...interface{}) ([]*adminv1.AdminRiverJob, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var jobs []*adminv1.AdminRiverJob
	for rows.Next() {
		j, err := scanRiverJob(rows)
		if err != nil {
			s.log.Warn("ListJobs scan failed", "error", err)
			continue
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func pageJobs(jobs []*adminv1.AdminRiverJob, limit int) *adminv1.ListJobsResponse {
	resp := &adminv1.ListJobsResponse{Jobs: jobs}
	if len(jobs) > limit {
		resp.HasMore = true
		resp.Jobs = jobs[:limit]
	}
	if resp.HasMore && len(resp.Jobs) > 0 {
		resp.NextBeforeID = resp.Jobs[len(resp.Jobs)-1].ID
	}
	return resp
}

func reverseJobs(jobs []*adminv1.AdminRiverJob) []*adminv1.AdminRiverJob {
	for i, j := 0, len(jobs)-1; i < j; i, j = i+1, j-1 {
		jobs[i], jobs[j] = jobs[j], jobs[i]
	}
	return jobs
}

func (s *Server) GetJob(ctx context.Context, req *adminv1.GetJobRequest) (*adminv1.GetJobResponse, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, kind, queue, state, attempt, max_attempts,
		       created_at, attempted_at, finalized_at, scheduled_at,
		       args, errors::text, priority
		FROM river.river_job WHERE id = $1
	`, req.ID)
	j, err := scanRiverJobRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "job %d not found", req.ID)
		}
		return nil, status.Errorf(codes.Internal, "get job: %v", err)
	}
	return &adminv1.GetJobResponse{Job: j}, nil
}

func (s *Server) CancelJobs(ctx context.Context, req *adminv1.CancelJobsRequest) (*adminv1.CancelJobsResponse, error) {
	cancelled := 0
	for _, id := range req.IDs {
		if err := s.queue.CancelJob(ctx, id); err != nil {
			s.log.Warn("CancelJob failed", "job_id", id, "error", err)
			continue
		}
		cancelled++
	}
	return &adminv1.CancelJobsResponse{Cancelled: cancelled}, nil
}

func (s *Server) RetryJobs(ctx context.Context, req *adminv1.RetryJobsRequest) (*adminv1.RetryJobsResponse, error) {
	retried := 0
	for _, id := range req.IDs {
		ok, err := s.queue.RetryJob(ctx, id)
		if err != nil {
			s.log.Warn("RetryJob failed", "job_id", id, "error", err)
			continue
		}
		if ok {
			retried++
		}
	}
	return &adminv1.RetryJobsResponse{Retried: retried}, nil
}

func (s *Server) PauseQueue(ctx context.Context, req *adminv1.PauseQueueRequest) (*adminv1.PauseQueueResponse, error) {
	if err := s.queue.PauseQueue(ctx, req.Name); err != nil {
		return nil, fmt.Errorf("pause queue %q: %w", req.Name, err)
	}
	return &adminv1.PauseQueueResponse{}, nil
}

func (s *Server) ResumeQueue(ctx context.Context, req *adminv1.ResumeQueueRequest) (*adminv1.ResumeQueueResponse, error) {
	if err := s.queue.ResumeQueue(ctx, req.Name); err != nil {
		return nil, fmt.Errorf("resume queue %q: %w", req.Name, err)
	}
	return &adminv1.ResumeQueueResponse{}, nil
}

type riverJobScanner interface {
	Scan(dest ...interface{}) error
}

func scanRiverJob(row riverJobScanner) (*adminv1.AdminRiverJob, error) {
	var j adminv1.AdminRiverJob
	var createdAt, scheduledAt time.Time
	var attemptedAt, finalizedAt sql.NullTime
	var argsBytes []byte
	var errorsStr sql.NullString
	if err := row.Scan(&j.ID, &j.Kind, &j.Queue, &j.State, &j.Attempt, &j.MaxAttempts,
		&createdAt, &attemptedAt, &finalizedAt, &scheduledAt,
		&argsBytes, &errorsStr, &j.Priority); err != nil {
		return nil, err
	}
	j.CreatedAt = createdAt.Format(time.RFC3339)
	j.ScheduledAt = scheduledAt.Format(time.RFC3339)
	if attemptedAt.Valid {
		j.AttemptedAt = attemptedAt.Time.Format(time.RFC3339)
	}
	if finalizedAt.Valid {
		j.FinalizedAt = finalizedAt.Time.Format(time.RFC3339)
	}
	if argsBytes != nil {
		j.Args = json.RawMessage(argsBytes)
	}
	if errorsStr.Valid && errorsStr.String != "" && errorsStr.String != "null" {
		_ = json.Unmarshal([]byte(errorsStr.String), &j.Errors)
	}
	return &j, nil
}

func scanRiverJobRow(row *sql.Row) (*adminv1.AdminRiverJob, error) {
	return scanRiverJob(row)
}

// ShutdownRiverUI cleans up River UI resources during server shutdown.
func (s *Server) ShutdownRiverUI() {
	s.riverMu.Lock()
	defer s.riverMu.Unlock()
	if s.riverUICleanup != nil {
		s.riverUICleanup()
		s.riverUIHandler = nil
		s.riverUICleanup = nil
	}
}

// SetCommandDispatcher sets the dispatcher for sending commands to connected devices.
func (s *Server) SetCommandDispatcher(d CommandDispatcher) {
	s.cmdDispatch = d
}

// SetImageRefresher wires the ECR pull-through cache refresher used by
// RefreshMessagingCache.
func (s *Server) SetImageRefresher(r *imagecache.Refresher) {
	s.imageRefresher = r
}

// ListDeployments returns all non-undeployed deployments across all accounts.
func (s *Server) ListDeployments(ctx context.Context, req *adminv1.ListDeploymentsRequest) (*adminv1.ListDeploymentsResponse, error) {
	dbDeps, err := s.deployStore.ListAllWithAccount()
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	// Collect unique owner user IDs for batch email resolution.
	ownerUserIDs := map[string]struct{}{}
	for _, d := range dbDeps {
		if d.OwnerUserID != "" {
			ownerUserIDs[d.OwnerUserID] = struct{}{}
		}
	}
	emailsByUserID := s.resolveEmails(ctx, ownerUserIDs)

	var results []*adminv1.AdminDeployment
	for _, d := range dbDeps {
		if req.Namespace != "" && d.Namespace != req.Namespace {
			continue
		}

		// Populate components from normalized workloads + sidecars tables
		components := []string{}
		if summaries, err := s.deployStore.GetWorkloadSummaries(d.ID); err == nil && len(summaries) > 0 {
			for _, ws := range summaries {
				name := ws.ComponentKind
				if ws.ComponentKey != "" {
					name += "/" + ws.ComponentKey
				}
				components = append(components, name)
			}
		}
		if sidecars, err := s.deployStore.GetSidecars(d.ID); err == nil {
			for _, sc := range sidecars {
				components = append(components, sc.ComponentKind)
			}
		}

		ad := &adminv1.AdminDeployment{
			Name:            d.AgentName,
			BuildID:         d.BuildID,
			Namespace:       d.Namespace,
			Status:          d.Status,
			CreatedAt:       d.DeployedAt.Format(time.RFC3339),
			AccountName:     d.AccountName,
			AccountId:       d.AccountID,
			Components:      components,
			DeploymentID:    d.ID,
			StatusChangedAt: d.StatusChangedAt.Format(time.RFC3339),
			OwnerEmail:      emailsByUserID[d.OwnerUserID],
		}
		if d.ErrorMessage != nil {
			ad.ErrorMessage = *d.ErrorMessage
		}
		if d.CurrentRevision != nil {
			ad.CurrentRevision = int32(*d.CurrentRevision) //nolint:gosec // revision numbers are small
		}
		populateAdminDeploymentPlacement(ad, d.EffectiveClusterID(), d.AccountClusterID, d.Status)

		results = append(results, ad)
	}

	return &adminv1.ListDeploymentsResponse{
		Deployments: results,
		Count:       int32(len(results)), //nolint:gosec // bounded by cluster size
	}, nil
}

// resolveEmails batch-resolves WorkOS user IDs to email addresses.
// Returns a map of userID → email. Failures are silently skipped.
func (s *Server) resolveEmails(ctx context.Context, userIDs map[string]struct{}) map[string]string {
	emails := make(map[string]string, len(userIDs))
	if s.workosClient == nil {
		return emails
	}
	for uid := range userIDs {
		user, err := s.workosClient.GetUser(ctx, uid)
		if err != nil {
			continue
		}
		emails[uid] = user.Email
	}
	return emails
}

// GetDeployment returns a single deployment with its spec, cluster status, events, and revisions.
func (s *Server) GetDeployment(ctx context.Context, req *adminv1.GetDeploymentRequest) (*adminv1.GetDeploymentResponse, error) {
	if req.DeploymentId == "" {
		return nil, fmt.Errorf("deployment_id is required")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}

	// Look up account name, placement, and owner
	var accountName, accountClusterID, ownerUserID string
	_ = s.db.QueryRow(`
		SELECT a.name, COALESCE(a.cluster_id, ''),
		       COALESCE((SELECT user_id FROM account_members WHERE account_id = a.id ORDER BY created_at ASC LIMIT 1), '')
		FROM accounts a WHERE a.id = $1`,
		dep.AccountID,
	).Scan(&accountName, &accountClusterID, &ownerUserID)

	var ownerEmail string
	if ownerUserID != "" && s.workosClient != nil {
		if user, err := s.workosClient.GetUser(ctx, ownerUserID); err == nil {
			ownerEmail = user.Email
		}
	}

	ad := &adminv1.AdminDeployment{
		Name:            dep.AgentName,
		BuildID:         dep.BuildID,
		Namespace:       dep.Namespace,
		Status:          dep.Status,
		CreatedAt:       dep.DeployedAt.Format(time.RFC3339),
		AccountName:     accountName,
		AccountId:       dep.AccountID,
		Components:      []string{},
		DeploymentID:    dep.ID,
		StatusChangedAt: dep.StatusChangedAt.Format(time.RFC3339),
		OwnerEmail:      ownerEmail,
	}
	if dep.ErrorMessage != nil {
		ad.ErrorMessage = *dep.ErrorMessage
	}
	if len(dep.ErrorDetails) > 0 {
		_ = json.Unmarshal(dep.ErrorDetails, &ad.ErrorDetails)
	}
	if dep.CurrentRevision != nil {
		ad.CurrentRevision = int32(*dep.CurrentRevision) //nolint:gosec // revision numbers are small
	}
	populateAdminDeploymentPlacement(ad, dep.EffectiveClusterID(), accountClusterID, dep.Status)

	// Fetch events
	var protoEvents []*adminv1.AdminDeploymentEvent
	events, evErr := s.deployStore.GetDeploymentEvents(dep.ID, 50)
	if evErr != nil {
		s.log.Warn("Failed to fetch deployment events", "deployment_id", dep.ID, "error", evErr)
	}
	for _, ev := range events {
		protoEvents = append(protoEvents, &adminv1.AdminDeploymentEvent{
			Status:    ev.Status,
			Message:   ev.Message,
			CreatedAt: ev.CreatedAt.Format(time.RFC3339),
		})
	}

	// Fetch revisions
	var protoRevisions []*adminv1.AdminDeploymentRevision
	if revisions, err := s.deployStore.GetRevisions(dep.ID); err == nil {
		for _, rev := range revisions {
			protoRevisions = append(protoRevisions, &adminv1.AdminDeploymentRevision{
				Revision:  int32(rev.Revision), //nolint:gosec // revision numbers are small
				BuildID:   rev.BuildID,
				CreatedAt: rev.CreatedAt.Format(time.RFC3339),
			})
		}
	}

	clusterStatus, err := s.GetClusterStatus(ctx, &adminv1.GetClusterStatusRequest{Namespace: dep.Namespace})
	if err != nil {
		s.log.Warn("Failed to get cluster status for deployment detail", "namespace", dep.Namespace, "error", err)
		clusterStatus = &adminv1.GetClusterStatusResponse{}
	}

	// Fetch expected workloads from normalized table
	var protoWorkloads []*adminv1.AdminWorkload
	if summaries, err := s.deployStore.GetWorkloadSummaries(dep.ID); err == nil {
		for _, w := range summaries {
			protoWorkloads = append(protoWorkloads, &adminv1.AdminWorkload{
				Name:          w.Name,
				ComponentKind: w.ComponentKind,
				ComponentKey:  w.ComponentKey,
				WorkloadType:  w.WorkloadType,
				Image:         w.Image,
				Replicas:      int32(w.Replicas), //nolint:gosec
				CPURequest:    w.CPURequest,
				MemoryRequest: w.MemoryRequest,
				Persistent:    w.Persistent,
			})
		}
	}
	// Include sidecars (messaging, collector) in workloads list for display
	if sidecars, err := s.deployStore.GetSidecars(dep.ID); err == nil {
		for _, sc := range sidecars {
			protoWorkloads = append(protoWorkloads, &adminv1.AdminWorkload{
				Name:          sc.Name,
				ComponentKind: sc.ComponentKind,
				WorkloadType:  "sidecar",
				Image:         sc.Image,
				Replicas:      1,
				CPURequest:    sc.CPURequest,
				MemoryRequest: sc.MemoryRequest,
			})
		}
	}

	// Fetch expected services from normalized table
	var protoServices []*adminv1.ExpectedService
	if services, err := s.deployStore.GetServices(dep.ID); err == nil {
		for _, svc := range services {
			protoServices = append(protoServices, &adminv1.ExpectedService{
				Name:         svc.Name,
				Port:         int32(svc.Port),       //nolint:gosec
				TargetPort:   int32(svc.TargetPort), //nolint:gosec
				Protocol:     svc.Protocol,
				WorkloadName: svc.WorkloadName,
			})
		}
	}

	// Fetch expected ingresses from normalized table
	var protoIngresses []*adminv1.ExpectedIngress
	if ingresses, err := s.deployStore.GetIngresses(dep.ID); err == nil {
		// Build service ID -> "workload_name:port" map for display (matches K8s ingress backend format)
		svcDisplayByID := map[int]string{}
		if services, err := s.deployStore.GetServices(dep.ID); err == nil {
			for _, svc := range services {
				svcDisplayByID[svc.ID] = fmt.Sprintf("%s:%d", svc.WorkloadName, svc.Port)
			}
		}
		for _, ing := range ingresses {
			protoIngresses = append(protoIngresses, &adminv1.ExpectedIngress{
				Hostname: ing.Hostname,
				Path:     ing.Path,
				Service:  svcDisplayByID[ing.ServiceID],
			})
		}
	}

	resp := &adminv1.GetDeploymentResponse{
		Deployment:        ad,
		SpecJSON:          dep.DeploymentSpecJSON,
		ClusterStatus:     clusterStatus,
		Events:            protoEvents,
		Revisions:         protoRevisions,
		Workloads:         protoWorkloads,
		ExpectedServices:  protoServices,
		ExpectedIngresses: protoIngresses,
		PlacementHint:     placementHintMessage(accountClusterID, dep.EffectiveClusterID()),
	}

	// Include adapters from the stored deployment spec (default to empty list)
	resp.Adapters = []string{}
	{
		var ds deployment.AstroDeploymentSpec
		if err := json.Unmarshal([]byte(dep.DeploymentSpecJSON), &ds); err == nil && ds.Interfaces != nil && len(ds.Interfaces.Adapters) > 0 {
			resp.Adapters = ds.Interfaces.Adapters
		}
	}

	// Include deployment variables
	if vars, err := s.deployStore.GetDeploymentVariables(dep.ID); err == nil {
		for _, v := range vars {
			av := &adminv1.AdminVariable{
				Name:     v.Name,
				Secret:   v.Secret,
				Optional: v.Optional,
				Targets:  v.Targets,
			}
			if v.Secret {
				av.Value = "***"
			} else {
				av.Value = v.Value
			}
			resp.Variables = append(resp.Variables, av)
		}
	}

	return resp, nil
}

// GetClusterStatus returns current cluster resource status.
func (s *Server) GetClusterStatus(ctx context.Context, req *adminv1.GetClusterStatusRequest) (*adminv1.GetClusterStatusResponse, error) {
	namespace := req.Namespace
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	kc, err := s.clusterClientForNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}
	clientset := kc.Clientset()
	resp := &adminv1.GetClusterStatusResponse{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		Namespace:       namespace,
		Deployments:     []*adminv1.K8sDeploymentInfo{},
		StatefulSets:    []*adminv1.K8sDeploymentInfo{},
		Pods:            []*adminv1.K8sPodInfo{},
		Services:        []*adminv1.K8sServiceInfo{},
		Ingresses:       []*adminv1.K8sIngressInfo{},
		NetworkPolicies: []*adminv1.K8sNetworkPolicyInfo{},
		Events:          []*adminv1.K8sEventInfo{},
		Summary:         &adminv1.ClusterSummary{},
	}
	if namespace != "" {
		if dep, err := s.deployStore.GetDeploymentByNamespace(namespace); err == nil && dep != nil {
			resp.ResolvedClusterID = dep.EffectiveClusterID()
		}
	}

	// Deployments
	deps, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.log.Warn("Failed to list k8s deployments", "error", err)
	} else {
		for _, d := range deps.Items {
			replicas := int32(0)
			if d.Spec.Replicas != nil {
				replicas = *d.Spec.Replicas
			}
			resp.Deployments = append(resp.Deployments, &adminv1.K8sDeploymentInfo{
				Name:              d.Name,
				Namespace:         d.Namespace,
				Replicas:          replicas,
				ReadyReplicas:     d.Status.ReadyReplicas,
				AvailableReplicas: d.Status.AvailableReplicas,
				Labels:            d.Labels,
				CreatedAt:         d.CreationTimestamp.Format(time.RFC3339),
			})
		}
	}

	// StatefulSets
	ssets, err := clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.log.Warn("Failed to list k8s statefulsets", "error", err)
	} else {
		for _, ss := range ssets.Items {
			replicas := int32(0)
			if ss.Spec.Replicas != nil {
				replicas = *ss.Spec.Replicas
			}
			resp.StatefulSets = append(resp.StatefulSets, &adminv1.K8sDeploymentInfo{
				Name:              ss.Name,
				Namespace:         ss.Namespace,
				Replicas:          replicas,
				ReadyReplicas:     ss.Status.ReadyReplicas,
				AvailableReplicas: ss.Status.ReadyReplicas,
				Labels:            ss.Labels,
				CreatedAt:         ss.CreationTimestamp.Format(time.RFC3339),
			})
		}
	}

	// Pods
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.log.Warn("Failed to list pods", "error", err)
	} else {
		for _, p := range pods.Items {
			pi := &adminv1.K8sPodInfo{
				Name:      p.Name,
				Namespace: p.Namespace,
				Phase:     string(p.Status.Phase),
				NodeName:  p.Spec.NodeName,
				PodIP:     p.Status.PodIP,
				Labels:    p.Labels,
				CreatedAt: p.CreationTimestamp.Format(time.RFC3339),
			}

			// Container statuses. Init containers (which include native sidecars
			// like the messaging container, added as restartPolicy: Always init
			// containers) live in a separate list, so fold both in and tag origin.
			appendStatus := func(cs corev1.ContainerStatus, init bool) {
				state := "Unknown"
				switch {
				case cs.State.Running != nil:
					state = "Running"
				case cs.State.Waiting != nil:
					state = "Waiting"
					if cs.State.Waiting.Reason != "" {
						state = "Waiting: " + cs.State.Waiting.Reason
					}
				case cs.State.Terminated != nil:
					state = "Terminated"
					if cs.State.Terminated.Reason != "" {
						state = "Terminated: " + cs.State.Terminated.Reason
					}
				}
				pi.ContainerStatuses = append(pi.ContainerStatuses, &adminv1.K8sContainerStatus{
					Name:         cs.Name,
					Ready:        cs.Ready,
					RestartCount: cs.RestartCount,
					State:        state,
					Image:        cs.Image,
					Init:         init,
				})
			}
			for _, cs := range p.Status.ContainerStatuses {
				appendStatus(cs, false)
			}
			for _, cs := range p.Status.InitContainerStatuses {
				appendStatus(cs, true)
			}

			// Container resources, security, mounts, envFrom — for regular and
			// init/sidecar containers alike.
			appendResources := func(c corev1.Container, init bool) {
				cr := &adminv1.K8sContainerResources{
					Name:            c.Name,
					ImagePullPolicy: string(c.ImagePullPolicy),
					Init:            init,
					Sidecar:         init && c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways,
				}
				if req := c.Resources.Requests; req != nil {
					if v, ok := req[corev1.ResourceCPU]; ok {
						cr.RequestCPU = v.String()
					}
					if v, ok := req[corev1.ResourceMemory]; ok {
						cr.RequestMemory = v.String()
					}
				}
				if lim := c.Resources.Limits; lim != nil {
					if v, ok := lim[corev1.ResourceCPU]; ok {
						cr.LimitCPU = v.String()
					}
					if v, ok := lim[corev1.ResourceMemory]; ok {
						cr.LimitMemory = v.String()
					}
				}
				if sc := c.SecurityContext; sc != nil {
					sec := &adminv1.K8sSecurityContext{
						RunAsUser:                sc.RunAsUser,
						RunAsNonRoot:             sc.RunAsNonRoot,
						ReadOnlyRootFilesystem:   sc.ReadOnlyRootFilesystem,
						AllowPrivilegeEscalation: sc.AllowPrivilegeEscalation,
						Privileged:               sc.Privileged,
					}
					if sc.Capabilities != nil {
						for _, cap := range sc.Capabilities.Drop {
							sec.Capabilities = append(sec.Capabilities, string(cap))
						}
						for _, cap := range sc.Capabilities.Add {
							sec.AddCapabilities = append(sec.AddCapabilities, string(cap))
						}
					}
					if sc.SeccompProfile != nil {
						sec.SeccompProfile = string(sc.SeccompProfile.Type)
					}
					cr.Security = sec
				}
				for _, vm := range c.VolumeMounts {
					cr.VolumeMounts = append(cr.VolumeMounts, &adminv1.K8sVolumeMount{
						Name: vm.Name, MountPath: vm.MountPath,
						ReadOnly: vm.ReadOnly, SubPath: vm.SubPath,
					})
				}
				for _, ef := range c.EnvFrom {
					if ef.ConfigMapRef != nil {
						cr.EnvFrom = append(cr.EnvFrom, "configmap:"+ef.ConfigMapRef.Name)
					}
					if ef.SecretRef != nil {
						cr.EnvFrom = append(cr.EnvFrom, "secret:"+ef.SecretRef.Name)
					}
				}
				pi.Containers = append(pi.Containers, cr)
			}
			for _, c := range p.Spec.Containers {
				appendResources(c, false)
			}
			for _, c := range p.Spec.InitContainers {
				appendResources(c, true)
			}

			// Pod-level security context
			if psc := p.Spec.SecurityContext; psc != nil {
				podSec := &adminv1.K8sPodSecurityContext{
					RunAsUser:  psc.RunAsUser,
					RunAsGroup: psc.RunAsGroup,
					FSGroup:    psc.FSGroup,
				}
				if psc.SeccompProfile != nil {
					podSec.SeccompProfile = string(psc.SeccompProfile.Type)
				}
				pi.PodSecurity = podSec
			}

			// Service account
			pi.ServiceAccount = p.Spec.ServiceAccountName
			pi.AutomountServiceToken = p.Spec.AutomountServiceAccountToken

			// Volumes
			for _, vol := range p.Spec.Volumes {
				v := &adminv1.K8sVolume{Name: vol.Name}
				switch {
				case vol.PersistentVolumeClaim != nil:
					v.Type = "pvc"
					v.Source = vol.PersistentVolumeClaim.ClaimName
				case vol.ConfigMap != nil:
					v.Type = "configmap"
					v.Source = vol.ConfigMap.Name
				case vol.Secret != nil:
					v.Type = "secret"
					v.Source = vol.Secret.SecretName
				case vol.EmptyDir != nil:
					v.Type = "emptydir"
				case vol.Projected != nil:
					v.Type = "projected"
				default:
					v.Type = "other"
				}
				pi.Volumes = append(pi.Volumes, v)
			}

			// Conditions (only true ones)
			for _, cond := range p.Status.Conditions {
				if cond.Status == corev1.ConditionTrue {
					pi.Conditions = append(pi.Conditions, string(cond.Type))
				}
			}

			resp.Pods = append(resp.Pods, pi)
		}
	}

	// Services
	svcs, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.log.Warn("Failed to list services", "error", err)
	} else {
		for _, svc := range svcs.Items {
			si := &adminv1.K8sServiceInfo{
				Name:      svc.Name,
				Namespace: svc.Namespace,
				Type:      string(svc.Spec.Type),
				ClusterIP: svc.Spec.ClusterIP,
				Labels:    svc.Labels,
				CreatedAt: svc.CreationTimestamp.Format(time.RFC3339),
			}
			for _, ing := range svc.Status.LoadBalancer.Ingress {
				if ing.IP != "" {
					si.ExternalIP = append(si.ExternalIP, ing.IP)
				}
				if ing.Hostname != "" {
					si.ExternalIP = append(si.ExternalIP, ing.Hostname)
				}
			}
			for _, port := range svc.Spec.Ports {
				si.Ports = append(si.Ports, &adminv1.K8sServicePort{
					Name:       port.Name,
					Port:       port.Port,
					TargetPort: port.TargetPort.String(),
					Protocol:   string(port.Protocol),
				})
			}
			resp.Services = append(resp.Services, si)
		}
	}

	// Ingresses
	ings, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.log.Warn("Failed to list ingresses", "error", err)
	} else {
		for _, ing := range ings.Items {
			ii := &adminv1.K8sIngressInfo{
				Name:      ing.Name,
				Namespace: ing.Namespace,
				Labels:    ing.Labels,
				CreatedAt: ing.CreationTimestamp.Format(time.RFC3339),
			}
			if ing.Spec.IngressClassName != nil {
				ii.IngressClassName = *ing.Spec.IngressClassName
			}
			for _, rule := range ing.Spec.Rules {
				r := &adminv1.K8sIngressRule{Host: rule.Host}
				if rule.HTTP != nil {
					for _, path := range rule.HTTP.Paths {
						p := &adminv1.K8sIngressPath{
							Path:     path.Path,
							PathType: string(*path.PathType),
						}
						if path.Backend.Service != nil {
							p.BackendService = path.Backend.Service.Name
							port := path.Backend.Service.Port
							if port.Name != "" {
								p.BackendPort = port.Name
							} else {
								p.BackendPort = fmt.Sprintf("%d", port.Number)
							}
						}
						r.Paths = append(r.Paths, p)
					}
				}
				ii.Rules = append(ii.Rules, r)
			}
			for _, tls := range ing.Spec.TLS {
				ii.TLS = append(ii.TLS, &adminv1.K8sIngressTLS{
					Hosts:      tls.Hosts,
					SecretName: tls.SecretName,
				})
			}
			resp.Ingresses = append(resp.Ingresses, ii)
		}
	}

	// NetworkPolicies
	netpols, err := clientset.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.log.Warn("Failed to list network policies", "error", err)
	} else {
		for _, np := range netpols.Items {
			policyTypes := make([]string, len(np.Spec.PolicyTypes))
			for i, pt := range np.Spec.PolicyTypes {
				policyTypes[i] = string(pt)
			}
			resp.NetworkPolicies = append(resp.NetworkPolicies, &adminv1.K8sNetworkPolicyInfo{
				Name:              np.Name,
				Namespace:         np.Namespace,
				PolicyTypes:       policyTypes,
				IngressRuleCount:  int32(len(np.Spec.Ingress)), //nolint:gosec // bounded by cluster size
				EgressRuleCount:   int32(len(np.Spec.Egress)),  //nolint:gosec // bounded by cluster size
				PodSelectorLabels: np.Spec.PodSelector.MatchLabels,
				Labels:            np.Labels,
				CreatedAt:         np.CreationTimestamp.Format(time.RFC3339),
			})
		}
	}

	// Events
	events, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.log.Warn("Failed to list events", "error", err)
	} else {
		for _, ev := range events.Items {
			resp.Events = append(resp.Events, &adminv1.K8sEventInfo{
				Name:           ev.Name,
				Namespace:      ev.Namespace,
				Type:           ev.Type,
				Reason:         ev.Reason,
				Message:        ev.Message,
				InvolvedObject: ev.InvolvedObject.Kind + "/" + ev.InvolvedObject.Name,
				Count:          ev.Count,
				FirstSeen:      ev.FirstTimestamp.Format(time.RFC3339),
				LastSeen:       ev.LastTimestamp.Format(time.RFC3339),
			})
		}
	}

	// Summary
	resp.Summary = &adminv1.ClusterSummary{
		TotalDeployments:     int32(len(resp.Deployments)),     //nolint:gosec // bounded by cluster size
		TotalPods:            int32(len(resp.Pods)),            //nolint:gosec // bounded by cluster size
		TotalServices:        int32(len(resp.Services)),        //nolint:gosec // bounded by cluster size
		TotalIngresses:       int32(len(resp.Ingresses)),       //nolint:gosec // bounded by cluster size
		TotalNetworkPolicies: int32(len(resp.NetworkPolicies)), //nolint:gosec // bounded by cluster size
		TotalEvents:          int32(len(resp.Events)),          //nolint:gosec // bounded by cluster size
	}
	for _, ev := range resp.Events {
		if ev.Type == "Warning" {
			resp.Summary.WarningEvents++
		}
	}
	for _, p := range resp.Pods {
		switch p.Phase {
		case "Running":
			resp.Summary.RunningPods++
		case "Pending":
			resp.Summary.PendingPods++
		case "Failed":
			resp.Summary.FailedPods++
		}
	}

	if namespace == "" {
		resp.Namespace = "all"
	}

	return resp, nil
}

// DeleteDeployment sets status to undeploying and enqueues an async undeploy job.
func (s *Server) DeleteDeployment(_ context.Context, req *adminv1.DeleteDeploymentRequest) (*adminv1.DeleteDeploymentResponse, error) {
	if req.DeploymentId == "" {
		return nil, fmt.Errorf("deployment_id is required")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}

	// Set status to undeploying
	if err := s.deployStore.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusUndeploying}); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	// Enqueue async undeploy job
	if s.queue != nil {
		if err := s.queue.InsertUndeployJob(context.Background(), dep.ID, dep.EffectiveClusterID()); err != nil {
			s.log.Warn("Failed to enqueue undeploy job", "deployment_id", dep.ID, "error", err)
		}
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(dep.AccountID, "grpc")
		evt.Action = auditlog.DeploymentDelete
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Admin deleted deployment " + dep.AgentName
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.DeleteDeploymentResponse{Status: "undeploying"}, nil
}

// RestartDeployment deletes a pod so Kubernetes recreates it.
func (s *Server) RestartDeployment(ctx context.Context, req *adminv1.RestartDeploymentRequest) (*adminv1.RestartDeploymentResponse, error) {
	if req.DeploymentId == "" || req.Pod == "" {
		return nil, fmt.Errorf("deployment_id and pod are required")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}

	kc, err := s.deploymentClusterClient(ctx, dep)
	if err != nil {
		return nil, err
	}

	err = kc.Clientset().CoreV1().Pods(dep.Namespace).Delete(ctx, req.Pod, metav1.DeleteOptions{})
	if err != nil {
		return nil, fmt.Errorf("delete pod: %w", err)
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(dep.AccountID, "grpc")
		evt.Action = auditlog.DeploymentRestart
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Admin restarted pod " + req.Pod
		evt.Metadata = map[string]any{"pod": req.Pod, "namespace": dep.Namespace}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.RestartDeploymentResponse{Status: "restarting"}, nil
}

// GetPodLogs returns the tail of a deployment's logs.
// Uses Loki when configured; falls back to direct K8s pod log streaming otherwise.
func (s *Server) GetPodLogs(ctx context.Context, req *adminv1.GetPodLogsRequest) (*adminv1.GetPodLogsResponse, error) {
	if req.DeploymentId == "" {
		return nil, fmt.Errorf("deployment_id is required")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}

	tailLines := int64(req.TailLines)
	if tailLines <= 0 {
		tailLines = 100
	}

	// lokiClient may differ from s.lokiClient when dep's cluster has its own
	// loki_url override; see k8s.Registry.LokiClientFor.
	lokiClient := s.k8sRegistry.LokiClientFor(ctx, dep.EffectiveClusterID(), s.lokiClient)

	// Loki path: query the centralized log store.
	if lokiClient != nil {
		p := loki.QueryParams{
			Namespace: dep.Namespace,
			Cluster:   s.k8sRegistry.LokiClusterName(ctx, dep.EffectiveClusterID()),
			Pod:       req.Pod,
			Container: req.Container,
			Limit:     tailLines,
		}
		if req.SinceUnixNs > 0 {
			p.Start = time.Unix(0, req.SinceUnixNs)
		}
		if req.UntilUnixNs > 0 {
			p.End = time.Unix(0, req.UntilUnixNs)
		}

		lines, err := lokiClient.QueryLogs(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("query loki logs: %w", err)
		}

		var sb strings.Builder
		for _, l := range lines {
			sb.WriteString(l.Line)
			sb.WriteByte('\n')
		}
		return &adminv1.GetPodLogsResponse{Logs: sb.String()}, nil
	}

	// K8s fallback: direct pod log stream.
	if req.Pod == "" {
		return nil, fmt.Errorf("pod is required when Loki is not configured")
	}

	kc, err := s.deploymentClusterClient(ctx, dep)
	if err != nil {
		return nil, err
	}

	logOpts := &corev1.PodLogOptions{TailLines: &tailLines}
	if req.Container != "" {
		logOpts.Container = req.Container
	}
	stream, err := kc.Clientset().CoreV1().Pods(dep.Namespace).GetLogs(req.Pod, logOpts).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("get pod logs: %w", err)
	}
	defer stream.Close() //nolint:errcheck

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(stream); err != nil {
		return nil, fmt.Errorf("read pod logs: %w", err)
	}

	return &adminv1.GetPodLogsResponse{Logs: buf.String()}, nil
}

// GetPodEnv returns environment variables for all containers in a pod.
func (s *Server) GetPodEnv(ctx context.Context, req *adminv1.GetPodEnvRequest) (*adminv1.GetPodEnvResponse, error) {
	if req.DeploymentId == "" || req.Pod == "" {
		return nil, fmt.Errorf("deployment_id and pod are required")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}

	kc, err := s.deploymentClusterClient(ctx, dep)
	if err != nil {
		return nil, err
	}

	pod, err := kc.Clientset().CoreV1().Pods(dep.Namespace).Get(ctx, req.Pod, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod: %w", err)
	}

	clientset := kc.Clientset()
	var containers []*adminv1.ContainerEnv
	for _, c := range pod.Spec.Containers {
		ce := &adminv1.ContainerEnv{Container: c.Name}

		// Resolve envFrom (ConfigMap/Secret bulk injection) first
		for _, ef := range c.EnvFrom {
			prefix := ef.Prefix
			if ef.ConfigMapRef != nil {
				cm, err := clientset.CoreV1().ConfigMaps(dep.Namespace).Get(ctx, ef.ConfigMapRef.Name, metav1.GetOptions{})
				if err != nil {
					// ConfigMap may not exist or be inaccessible — note it
					ce.Vars = append(ce.Vars, &adminv1.EnvVar{
						Name:      prefix + "*",
						ValueFrom: fmt.Sprintf("configmap:%s (error: %v)", ef.ConfigMapRef.Name, err),
					})
					continue
				}
				for k, v := range cm.Data {
					ce.Vars = append(ce.Vars, &adminv1.EnvVar{
						Name:      prefix + k,
						Value:     v,
						ValueFrom: fmt.Sprintf("configmap:%s", ef.ConfigMapRef.Name),
					})
				}
			}
			if ef.SecretRef != nil {
				sec, err := clientset.CoreV1().Secrets(dep.Namespace).Get(ctx, ef.SecretRef.Name, metav1.GetOptions{})
				if err != nil {
					ce.Vars = append(ce.Vars, &adminv1.EnvVar{
						Name:      prefix + "*",
						ValueFrom: fmt.Sprintf("secret:%s (error: %v)", ef.SecretRef.Name, err),
					})
					continue
				}
				for k := range sec.Data {
					ce.Vars = append(ce.Vars, &adminv1.EnvVar{
						Name:      prefix + k,
						Value:     "***",
						ValueFrom: fmt.Sprintf("secret:%s", ef.SecretRef.Name),
					})
				}
			}
		}

		// Individual env entries
		for _, e := range c.Env {
			ev := &adminv1.EnvVar{Name: e.Name, Value: e.Value}
			if e.ValueFrom != nil {
				switch {
				case e.ValueFrom.SecretKeyRef != nil:
					ev.ValueFrom = fmt.Sprintf("secret:%s/%s", e.ValueFrom.SecretKeyRef.Name, e.ValueFrom.SecretKeyRef.Key)
				case e.ValueFrom.ConfigMapKeyRef != nil:
					ev.ValueFrom = fmt.Sprintf("configmap:%s/%s", e.ValueFrom.ConfigMapKeyRef.Name, e.ValueFrom.ConfigMapKeyRef.Key)
				case e.ValueFrom.FieldRef != nil:
					ev.ValueFrom = fmt.Sprintf("fieldRef:%s", e.ValueFrom.FieldRef.FieldPath)
				default:
					ev.ValueFrom = "ref"
				}
			}
			ce.Vars = append(ce.Vars, ev)
		}
		containers = append(containers, ce)
	}

	return &adminv1.GetPodEnvResponse{Containers: containers}, nil
}

// ListAccounts returns all accounts with owner and member count.
func (s *Server) ListAccounts(ctx context.Context, _ *adminv1.ListAccountsRequest) (*adminv1.ListAccountsResponse, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			a.id,
			a.name,
			a.type,
			COALESCE((SELECT user_id FROM account_members WHERE account_id = a.id ORDER BY created_at ASC LIMIT 1), '') AS owner_user_id,
			(SELECT COUNT(*) FROM account_members WHERE account_id = a.id) AS member_count,
			EXISTS(SELECT 1 FROM account_langfuse WHERE account_id = a.id) AS has_langfuse,
			a.deleted_at,
			a.created_at,
			a.updated_at,
			COALESCE(a.cluster_id, '') AS cluster_id,
			COALESCE(bs.status, '') AS billing_status
		FROM accounts a
		LEFT JOIN account_billing_status bs ON bs.account_id = a.id
		ORDER BY a.deleted_at NULLS FIRST, a.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var accounts []*adminv1.AdminAccount
	for rows.Next() {
		var acct adminv1.AdminAccount
		var deletedAt sql.NullTime
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&acct.ID, &acct.Name, &acct.Type, &acct.OwnerUserID, &acct.MemberCount, &acct.HasLangfuse, &deletedAt, &createdAt, &updatedAt, &acct.ClusterID, &acct.BillingStatus); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		if deletedAt.Valid {
			acct.DeletedAt = deletedAt.Time.Format(time.RFC3339)
		}
		acct.CreatedAt = createdAt.Format(time.RFC3339)
		acct.UpdatedAt = updatedAt.Format(time.RFC3339)
		accounts = append(accounts, &acct)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("accounts rows error: %w", err)
	}

	return &adminv1.ListAccountsResponse{
		Accounts: accounts,
		Count:    int32(len(accounts)), //nolint:gosec // bounded by DB rows
	}, nil
}

// GetAccount returns one account with its billing status, per-resource usage and
// limits, and member roster. It aggregates the DB-owned account facts an admin
// needs on the account detail page; live cluster state is not consulted here.
func (s *Server) GetAccount(ctx context.Context, req *adminv1.GetAccountRequest) (*adminv1.GetAccountResponse, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}

	acct := &adminv1.AdminAccount{}
	billing := &adminv1.AccountBillingInfo{}
	var deletedAt sql.NullTime
	var createdAt, updatedAt time.Time
	var langfuseProjectID string
	err := s.db.QueryRowContext(ctx, `
		SELECT
			a.id,
			a.name,
			a.type,
			COALESCE((SELECT user_id FROM account_members WHERE account_id = a.id ORDER BY created_at ASC LIMIT 1), '') AS owner_user_id,
			(SELECT COUNT(*) FROM account_members WHERE account_id = a.id) AS member_count,
			EXISTS(SELECT 1 FROM account_langfuse WHERE account_id = a.id) AS has_langfuse,
			a.deleted_at,
			a.created_at,
			a.updated_at,
			COALESCE(a.cluster_id, '') AS cluster_id,
			COALESCE(a.metronome_customer_id, '') AS metronome_customer_id,
			COALESCE(a.stripe_customer_id, '') AS stripe_customer_id,
			COALESCE(a.bifrost_customer_id, '') AS bifrost_customer_id,
			COALESCE((SELECT langfuse_project_id FROM account_langfuse WHERE account_id = a.id), '') AS langfuse_project_id
		FROM accounts a
		WHERE a.id = $1
	`, req.AccountID).Scan(
		&acct.ID, &acct.Name, &acct.Type, &acct.OwnerUserID, &acct.MemberCount, &acct.HasLangfuse,
		&deletedAt, &createdAt, &updatedAt, &acct.ClusterID,
		&billing.MetronomeCustomerID, &billing.StripeCustomerID, &billing.BifrostCustomerID,
		&langfuseProjectID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "account not found: %s", req.AccountID)
	}
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	if deletedAt.Valid {
		acct.DeletedAt = deletedAt.Time.Format(time.RFC3339)
	}
	acct.CreatedAt = createdAt.Format(time.RFC3339)
	acct.UpdatedAt = updatedAt.Format(time.RFC3339)

	// Billing status row is optional — an account that has never been billed has
	// no row, which we surface as an empty status rather than an error.
	var reason sql.NullString
	var dunningSince, billingUpdatedAt sql.NullTime
	err = s.db.QueryRowContext(ctx,
		`SELECT status, reason, dunning_since, alert_active, updated_at
		 FROM account_billing_status WHERE account_id = $1`,
		req.AccountID,
	).Scan(&billing.Status, &reason, &dunningSince, &billing.AlertActive, &billingUpdatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get billing status: %w", err)
	}
	if reason.Valid {
		billing.Reason = reason.String
	}
	if dunningSince.Valid {
		billing.DunningSince = dunningSince.Time.Format(time.RFC3339)
	}
	if billingUpdatedAt.Valid {
		billing.UpdatedAt = billingUpdatedAt.Time.Format(time.RFC3339)
	}
	acct.BillingStatus = billing.Status

	limits, err := s.accountLimits(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	members, err := s.accountMembers(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	return &adminv1.GetAccountResponse{
		Account:           acct,
		Billing:           billing,
		Limits:            limits,
		Members:           members,
		LangfuseProjectID: langfuseProjectID,
	}, nil
}

// accountLimits reports usage and effective limit for every quota-managed
// resource, in display order. Returns nil when no reporter is configured.
func (s *Server) accountLimits(ctx context.Context, accountID string) ([]*adminv1.AccountResourceLimit, error) {
	if s.quotaReporter == nil {
		return nil, nil
	}
	usage, err := s.quotaReporter.Report(ctx, accountID, quota.AllResources...)
	if err != nil {
		return nil, fmt.Errorf("report account limits: %w", err)
	}
	limits := make([]*adminv1.AccountResourceLimit, 0, len(quota.AllResources))
	for _, resource := range quota.AllResources {
		u := usage[resource]
		limits = append(limits, &adminv1.AccountResourceLimit{
			Resource: resource,
			Used:     u.Used,
			Limit:    u.Limit,
		})
	}
	return limits, nil
}

// accountMembers returns the member roster with a best-effort email (preferring
// verified addresses) and the earliest-joined member flagged as owner.
func (s *Server) accountMembers(ctx context.Context, accountID string) ([]*adminv1.AccountMemberInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			m.user_id,
			COALESCE((
				SELECT e.email FROM account_member_emails e
				WHERE e.user_id = m.user_id
				ORDER BY e.verified DESC, e.updated_at DESC
				LIMIT 1
			), '') AS email,
			m.created_at
		FROM account_members m
		WHERE m.account_id = $1
		ORDER BY m.created_at ASC
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list account members: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var members []*adminv1.AccountMemberInfo
	for rows.Next() {
		var m adminv1.AccountMemberInfo
		var createdAt time.Time
		if err := rows.Scan(&m.UserID, &m.Email, &createdAt); err != nil {
			return nil, fmt.Errorf("scan account member: %w", err)
		}
		m.CreatedAt = createdAt.Format(time.RFC3339)
		m.IsOwner = len(members) == 0 // earliest-joined is the owner
		members = append(members, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("account members rows error: %w", err)
	}
	return members, nil
}

// GetAccountMetronomeAliases checks, live against the Metronome API, whether the
// account's Metronome customer carries the ingest aliases it should. The expected
// set mirrors what CreateCustomer/SyncBifrostAlias write: the account ID, plus the
// Bifrost customer ID when the account has one.
func (s *Server) GetAccountMetronomeAliases(ctx context.Context, req *adminv1.GetAccountMetronomeAliasesRequest) (*adminv1.MetronomeAliasStatus, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}

	var metronomeCustomerID, bifrostCustomerID string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(metronome_customer_id, ''), COALESCE(bifrost_customer_id, '')
		 FROM accounts WHERE id = $1`,
		req.AccountID,
	).Scan(&metronomeCustomerID, &bifrostCustomerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "account not found: %s", req.AccountID)
	}
	if err != nil {
		return nil, fmt.Errorf("get account billing ids: %w", err)
	}

	// No hosted-billing customer (or no provider) means there is nothing to check.
	if s.billingProvider == nil || metronomeCustomerID == "" {
		return &adminv1.MetronomeAliasStatus{Configured: false}, nil
	}

	expected := []string{req.AccountID}
	if bifrostCustomerID != "" {
		expected = append(expected, bifrostCustomerID)
	}

	actual, err := s.billingProvider.GetIngestAliases(ctx, metronomeCustomerID)
	if err != nil {
		return &adminv1.MetronomeAliasStatus{Configured: true, Expected: expected, Error: err.Error()}, nil
	}

	var missing []string
	for _, want := range expected {
		if !slices.Contains(actual, want) {
			missing = append(missing, want)
		}
	}

	return &adminv1.MetronomeAliasStatus{
		Configured: true,
		OK:         len(missing) == 0,
		Expected:   expected,
		Actual:     actual,
		Missing:    missing,
	}, nil
}

// RecoverAccountMetronomeAliases writes the expected ingest aliases
// ({account_id, bifrost_customer_id}) onto the account's Metronome customer, then
// returns the re-checked status. Used from the admin panel to repair a customer
// whose aliases drifted or were never set.
func (s *Server) RecoverAccountMetronomeAliases(ctx context.Context, req *adminv1.RecoverAccountMetronomeAliasesRequest) (*adminv1.MetronomeAliasStatus, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}

	var metronomeCustomerID, bifrostCustomerID string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(metronome_customer_id, ''), COALESCE(bifrost_customer_id, '')
		 FROM accounts WHERE id = $1`,
		req.AccountID,
	).Scan(&metronomeCustomerID, &bifrostCustomerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "account not found: %s", req.AccountID)
	}
	if err != nil {
		return nil, fmt.Errorf("get account billing ids: %w", err)
	}
	if s.billingProvider == nil {
		return nil, status.Error(codes.FailedPrecondition, "billing provider not configured")
	}
	if metronomeCustomerID == "" {
		return nil, status.Error(codes.FailedPrecondition, "account has no Metronome customer to recover")
	}

	expected := []string{req.AccountID}
	if bifrostCustomerID != "" {
		expected = append(expected, bifrostCustomerID)
	}
	if err := s.billingProvider.SetIngestAliases(ctx, metronomeCustomerID, expected); err != nil {
		return nil, fmt.Errorf("set ingest aliases: %w", err)
	}

	s.log.Info("Recovered Metronome ingest aliases",
		"account_id", req.AccountID, "customer_id", metronomeCustomerID, "aliases", expected)

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.BillingRecoverAliases
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.Description = "Admin recovered Metronome ingest aliases"
		evt.Metadata = map[string]any{"customer_id": metronomeCustomerID, "aliases": expected}
		s.auditStore.LogAsync(s.log, evt)
	}

	// Return the freshly re-checked status.
	return s.GetAccountMetronomeAliases(ctx, &adminv1.GetAccountMetronomeAliasesRequest{AccountID: req.AccountID})
}

// RegisterAccountMetronome creates a Metronome customer for the account when it
// has none, persisting the returned customer id. Idempotent — returns the
// existing id if already registered. CreateCustomer seeds the ingest aliases
// ({account_id, bifrost_customer_id}) at creation.
func (s *Server) RegisterAccountMetronome(ctx context.Context, req *adminv1.RegisterAccountMetronomeRequest) (*adminv1.RegisterAccountMetronomeResponse, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if s.billingProvider == nil {
		return nil, status.Error(codes.FailedPrecondition, "billing provider not configured")
	}

	var name, existing, bifrost string
	err := s.db.QueryRowContext(ctx,
		`SELECT name, COALESCE(metronome_customer_id, ''), COALESCE(bifrost_customer_id, '')
		 FROM accounts WHERE id = $1`,
		req.AccountID,
	).Scan(&name, &existing, &bifrost)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "account not found: %s", req.AccountID)
	}
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	if existing != "" {
		return &adminv1.RegisterAccountMetronomeResponse{MetronomeCustomerID: existing}, nil
	}

	customerID, err := s.billingProvider.CreateCustomer(ctx, billing.Account{
		ID:                req.AccountID,
		Name:              name,
		BifrostCustomerID: bifrost,
	})
	if err != nil {
		return nil, fmt.Errorf("create metronome customer: %w", err)
	}
	if customerID == "" {
		return nil, status.Error(codes.FailedPrecondition, "billing provider returned no customer id (backend may be unmetered)")
	}

	if err := account.NewAccountStore(s.db).SetMetronomeCustomerID(req.AccountID, customerID); err != nil {
		return nil, fmt.Errorf("persist metronome customer id: %w", err)
	}

	s.log.Info("Registered Metronome customer", "account_id", req.AccountID, "customer_id", customerID)
	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.BillingRegisterMetronome
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.Description = "Admin registered Metronome customer"
		evt.Metadata = map[string]any{"customer_id": customerID}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.RegisterAccountMetronomeResponse{MetronomeCustomerID: customerID}, nil
}

// RecoverAccountLangfuse provisions the account's Langfuse project if missing
// (idempotent) and returns the project id. Mirrors the lazy provisioning the
// deploy and ingest-key paths perform, but on demand from the admin panel.
func (s *Server) RecoverAccountLangfuse(ctx context.Context, req *adminv1.RecoverAccountLangfuseRequest) (*adminv1.RecoverAccountLangfuseResponse, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if s.langfuseProvisioner == nil {
		return nil, status.Error(codes.FailedPrecondition, "langfuse provisioning not configured")
	}

	var accountName string
	err := s.db.QueryRowContext(ctx, `SELECT name FROM accounts WHERE id = $1`, req.AccountID).Scan(&accountName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "account not found: %s", req.AccountID)
	}
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	store := langfuse.NewStore(s.db)
	if _, _, err := s.langfuseProvisioner.EnsureProject(ctx, store, s.kmsKeyARN, s.kmsClient, req.AccountID, accountName); err != nil {
		return nil, fmt.Errorf("ensure langfuse project: %w", err)
	}

	var projectID string
	if row, err := store.Get(req.AccountID); err == nil && row != nil {
		projectID = row.LangfuseProjectID
	}

	s.log.Info("Recovered Langfuse project", "account_id", req.AccountID, "project_id", projectID)
	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.ObservabilityRecoverLangfuse
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.Description = "Admin recovered Langfuse project"
		evt.Metadata = map[string]any{"project_id": projectID}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.RecoverAccountLangfuseResponse{LangfuseProjectID: projectID}, nil
}

// RecoverAccountBifrost ensures the account's Bifrost customer exists (idempotent)
// and returns its id. Also re-syncs the Metronome ingest alias when a new customer
// is minted (handled inside EnsureCustomer).
func (s *Server) RecoverAccountBifrost(ctx context.Context, req *adminv1.RecoverAccountBifrostRequest) (*adminv1.RecoverAccountBifrostResponse, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if s.aiGatewayProvisioner == nil {
		return nil, status.Error(codes.FailedPrecondition, "ai gateway not configured")
	}

	customerID, err := s.aiGatewayProvisioner.EnsureCustomer(ctx, req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("ensure bifrost customer: %w", err)
	}

	s.log.Info("Recovered Bifrost customer", "account_id", req.AccountID, "customer_id", customerID)
	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.ObservabilityRecoverBifrost
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.Description = "Admin recovered Bifrost customer"
		evt.Metadata = map[string]any{"customer_id": customerID}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.RecoverAccountBifrostResponse{BifrostCustomerID: customerID}, nil
}

// ListAgents returns all agents across accounts with version counts.
func (s *Server) ListAgents(ctx context.Context, _ *adminv1.ListAgentsRequest) (*adminv1.ListAgentsResponse, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			ac.name AS account_name,
			a.name,
			(SELECT COUNT(*) FROM agent_versions av WHERE av.account_id = a.account_id AND av.name = a.name) AS build_count,
			COALESCE((SELECT av2.build_id FROM agent_versions av2 WHERE av2.account_id = a.account_id AND av2.name = a.name ORDER BY av2.published_at DESC LIMIT 1), '') AS latest_build_id,
			a.created_at,
			a.updated_at
		FROM agents a
		JOIN accounts ac ON ac.id = a.account_id
		ORDER BY a.updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var agents []*adminv1.AdminAgent
	for rows.Next() {
		var agent adminv1.AdminAgent
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&agent.AccountName, &agent.Name,
			&agent.BuildCount, &agent.LatestBuildID,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agent.CreatedAt = createdAt.Format(time.RFC3339)
		agent.UpdatedAt = updatedAt.Format(time.RFC3339)
		agents = append(agents, &agent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agents rows error: %w", err)
	}

	return &adminv1.ListAgentsResponse{
		Agents: agents,
		Count:  int32(len(agents)), //nolint:gosec // bounded by DB rows
	}, nil
}

// GetAgentBuilds returns all builds for a specific agent.
func (s *Server) GetAgentBuilds(ctx context.Context, req *adminv1.GetAgentBuildsRequest) (*adminv1.GetAgentBuildsResponse, error) {
	if req.AccountName == "" || req.AgentName == "" {
		return nil, fmt.Errorf("account_name and agent_name are required")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT av.build_id, av.published_at, av.updated_at
		FROM agent_versions av
		WHERE av.account_id = (SELECT id FROM accounts WHERE name = $1)
			AND av.name = $2
		ORDER BY av.published_at DESC
	`, req.AccountName, req.AgentName)
	if err != nil {
		return nil, fmt.Errorf("get agent builds: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var builds []*adminv1.AgentBuild
	for rows.Next() {
		var b adminv1.AgentBuild
		var publishedAt, updatedAt time.Time
		if err := rows.Scan(&b.BuildID, &publishedAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan agent build: %w", err)
		}
		b.PublishedAt = publishedAt.Format(time.RFC3339)
		b.UpdatedAt = updatedAt.Format(time.RFC3339)
		builds = append(builds, &b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agent builds rows error: %w", err)
	}

	return &adminv1.GetAgentBuildsResponse{
		Builds: builds,
		Count:  int32(len(builds)), //nolint:gosec // bounded by DB rows
	}, nil
}

// RenameAccount validates and updates an account's name.
func (s *Server) RenameAccount(ctx context.Context, req *adminv1.RenameAccountRequest) (*adminv1.RenameAccountResponse, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}
	if req.NewName == "" {
		return nil, fmt.Errorf("new_name is required")
	}
	if err := account.ValidateAccountName(req.NewName); err != nil {
		return nil, fmt.Errorf("invalid name: %w", err)
	}

	result, err := s.db.ExecContext(ctx, "UPDATE accounts SET name = $1, updated_at = NOW() WHERE id = $2", req.NewName, req.AccountID)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return nil, fmt.Errorf("account name %q is already taken", req.NewName)
		}
		return nil, fmt.Errorf("rename account: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, fmt.Errorf("account %q not found", req.AccountID)
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.AccountRename
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.ResourceName = req.NewName
		evt.Description = "Admin renamed account to " + req.NewName
		evt.Metadata = map[string]any{"new_name": req.NewName}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.RenameAccountResponse{Status: "renamed"}, nil
}

// ListConnectedDevices returns all devices that have connected via ast connect.
func (s *Server) ListConnectedDevices(ctx context.Context, _ *adminv1.ListConnectedDevicesRequest) (*adminv1.ListConnectedDevicesResponse, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.account_id, d.user_id, d.device_id, d.hostname, d.os, d.arch,
		       d.cli_version, d.status, d.last_heartbeat_at, d.connected_at, d.disconnected_at,
		       COALESCE(a.name, '') as account_name
		FROM connected_devices d
		LEFT JOIN accounts a ON a.id = d.account_id
		ORDER BY d.status ASC, d.connected_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list connected devices: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var devices []*adminv1.ConnectedDevice
	for rows.Next() {
		var (
			d               adminv1.ConnectedDevice
			lastHeartbeatAt sql.NullTime
			disconnectedAt  sql.NullTime
			connectedAt     sql.NullTime
		)
		if err := rows.Scan(
			&d.ID, &d.AccountID, &d.UserID, &d.DeviceID, &d.Hostname, &d.OS, &d.Arch,
			&d.CLIVersion, &d.Status, &lastHeartbeatAt, &connectedAt, &disconnectedAt,
			&d.AccountName,
		); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		if lastHeartbeatAt.Valid {
			d.LastHeartbeatAt = lastHeartbeatAt.Time.Format("2006-01-02T15:04:05Z")
		}
		if connectedAt.Valid {
			d.ConnectedAt = connectedAt.Time.Format("2006-01-02T15:04:05Z")
		}
		if disconnectedAt.Valid {
			d.DisconnectedAt = disconnectedAt.Time.Format("2006-01-02T15:04:05Z")
		}
		devices = append(devices, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}

	return &adminv1.ListConnectedDevicesResponse{
		Devices: devices,
		Count:   int32(len(devices)), //nolint:gosec // bounded by DB result set
	}, nil
}

// SendCommand dispatches a shell command to a connected device and returns the result.
func (s *Server) SendCommand(ctx context.Context, req *adminv1.SendCommandRequest) (*adminv1.SendCommandResponse, error) {
	if s.cmdDispatch == nil {
		return nil, fmt.Errorf("command dispatch not available")
	}
	if req.DeviceID == "" {
		return nil, fmt.Errorf("device_id is required")
	}
	if req.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	result, err := s.cmdDispatch.SendCommand(ctx, req.DeviceID, &connectv1.ShellCommand{
		Shell:          req.Shell,
		Command:        req.Command,
		WorkingDir:     req.WorkingDir,
		Env:            req.Env,
		TimeoutSeconds: req.TimeoutSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("send command to device %q: %w", req.DeviceID, err)
	}

	return &adminv1.SendCommandResponse{
		CommandID: result.CommandID,
		ExitCode:  result.ExitCode,
		Stdout:    result.Stdout,
		Stderr:    result.Stderr,
	}, nil
}

// GetAuthConfig returns auth configuration needed for device authorization flow.
func (s *Server) GetAuthConfig(_ context.Context, _ *adminv1.GetAuthConfigRequest) (*adminv1.GetAuthConfigResponse, error) {
	return &adminv1.GetAuthConfigResponse{
		WorkOSClientID: s.workosClientID,
		WorkOSBaseURL:  "https://api.workos.com",
	}, nil
}

// ProxyHTTP dispatches an HTTP request to the gin router running in the same process.
// Requests to /riverui/ are routed to the internal River UI handler (not exposed on the public HTTP port).
func (s *Server) ProxyHTTP(_ context.Context, req *adminv1.HTTPProxyRequest) (*adminv1.HTTPProxyResponse, error) {
	// Route /riverui/ requests to the River UI handler (must be started via StartRiverUI first).
	handler := s.httpHandler
	if strings.HasPrefix(req.Path, "/riverui/") || req.Path == "/riverui" {
		s.riverMu.Lock()
		h := s.riverUIHandler
		s.riverMu.Unlock()
		if h == nil {
			return nil, fmt.Errorf("river UI is not running — start it first")
		}
		handler = h
	}

	if handler == nil {
		return nil, fmt.Errorf("HTTP handler not configured")
	}

	httpReq, err := http.NewRequest(req.Method, req.Path, bytes.NewReader(req.Body))
	if err != nil {
		return nil, fmt.Errorf("build HTTP request: %w", err)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httpReq)

	result := rec.Result()
	defer result.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	headers := make(map[string]string, len(result.Header))
	for k := range result.Header {
		headers[k] = result.Header.Get(k)
	}

	return &adminv1.HTTPProxyResponse{
		StatusCode: int32(result.StatusCode), //nolint:gosec // HTTP status codes are bounded
		Headers:    headers,
		Body:       body,
	}, nil
}

// GetDeploymentEvents returns status events for a deployment identified by ID.
func (s *Server) GetDeploymentEvents(_ context.Context, req *adminv1.GetDeploymentEventsRequest) (*adminv1.GetDeploymentEventsResponse, error) {
	if req.DeploymentId == "" {
		return nil, fmt.Errorf("deployment_id is required")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}

	events, err := s.deployStore.GetDeploymentEvents(dep.ID, 50)
	if err != nil {
		return nil, fmt.Errorf("get deployment events: %w", err)
	}

	var protoEvents []*adminv1.AdminDeploymentEvent
	for _, ev := range events {
		protoEvents = append(protoEvents, &adminv1.AdminDeploymentEvent{
			Status:    ev.Status,
			Message:   ev.Message,
			CreatedAt: ev.CreatedAt.Format(time.RFC3339),
		})
	}

	return &adminv1.GetDeploymentEventsResponse{Events: protoEvents}, nil
}

// WakeUpDeployment wakes up a scaled-down deployment by setting status to pending and enqueuing a wakeup job.
func (s *Server) WakeUpDeployment(_ context.Context, req *adminv1.WakeUpDeploymentRequest) (*adminv1.WakeUpDeploymentResponse, error) {
	if req.DeploymentId == "" {
		return nil, fmt.Errorf("deployment_id is required")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}
	if dep.Status != deploymentstore.StatusStopped {
		return nil, fmt.Errorf("deployment is not stopped (current: %s)", dep.Status)
	}

	// Update status to pending
	if err := s.deployStore.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusPending, EventMsg: "Admin wakeup requested"}); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	// Enqueue wakeup job
	if s.queue != nil {
		if err := s.queue.InsertWakeUpJob(context.Background(), dep.ID, dep.EffectiveClusterID()); err != nil {
			return nil, fmt.Errorf("enqueue wakeup job: %w", err)
		}
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(dep.AccountID, "grpc")
		evt.Action = auditlog.DeploymentWakeup
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Admin woke up deployment " + dep.AgentName
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.WakeUpDeploymentResponse{Status: "waking_up"}, nil
}

// StopDeployment stops a deployment by scaling workloads to zero without deleting resources.
func (s *Server) StopDeployment(ctx context.Context, req *adminv1.StopDeploymentRequest) (*adminv1.StopDeploymentResponse, error) {
	var dep *deploymentstore.Deployment
	var err error
	if req.DeploymentId != "" {
		dep, err = s.deployStore.GetDeploymentByID(req.DeploymentId)
		if err != nil {
			return nil, fmt.Errorf("get deployment: %w", err)
		}
		if dep == nil {
			return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
		}
	} else if req.Namespace != "" {
		dep, err = s.deployStore.GetDeploymentByNamespace(req.Namespace)
		if err != nil {
			return nil, fmt.Errorf("get deployment: %w", err)
		}
		if dep == nil {
			return nil, fmt.Errorf("deployment not found for namespace %q", req.Namespace)
		}
	} else {
		return nil, fmt.Errorf("deployment_id or namespace is required")
	}
	if dep.Status != deploymentstore.StatusActive {
		return nil, fmt.Errorf("deployment is not active (current: %s)", dep.Status)
	}

	kc, err := s.deploymentClusterClient(ctx, dep)
	if err != nil {
		return nil, err
	}

	if err := k8s.StopNamespaceWorkloads(ctx, kc.Clientset(), dep.Namespace); err != nil {
		return nil, fmt.Errorf("stop workloads: %w", err)
	}

	if err := s.deployStore.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusStopped, EventMsg: "Admin stop requested"}); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(dep.AccountID, "grpc")
		evt.Action = auditlog.DeploymentStop
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = "Admin stopped deployment " + dep.AgentName
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.StopDeploymentResponse{Status: "stopped"}, nil
}

// RollbackDeployment rolls back to a previous revision by atomically setting the revision and enqueuing a deploy job.
func (s *Server) RollbackDeployment(_ context.Context, req *adminv1.RollbackDeploymentRequest) (*adminv1.RollbackDeploymentResponse, error) {
	if req.DeploymentId == "" {
		return nil, fmt.Errorf("deployment_id is required")
	}
	if req.Revision <= 0 {
		return nil, fmt.Errorf("revision must be positive")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}
	if dep.Status != deploymentstore.StatusActive && dep.Status != deploymentstore.StatusFailed {
		return nil, fmt.Errorf("deployment must be active or failed to rollback (current: %s)", dep.Status)
	}

	if err := s.deployStore.SetCurrentRevision(dep.ID, int(req.Revision), nil); err != nil {
		return nil, fmt.Errorf("set revision: %w", err)
	}

	// Enqueue deploy job (idempotent — safe outside transaction)
	if s.queue != nil {
		if err := s.queue.InsertDeployJob(context.Background(), dep.ID, dep.EffectiveClusterID()); err != nil {
			s.log.Warn("Failed to enqueue deploy job for rollback", "deployment_id", dep.ID, "error", err)
		}
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(dep.AccountID, "grpc")
		evt.Action = auditlog.DeploymentRollback
		evt.ResourceType = "deployment"
		evt.ResourceID = dep.ID
		evt.ResourceName = dep.AgentName
		evt.Description = fmt.Sprintf("Admin rolled back deployment to revision %d", req.Revision)
		evt.Metadata = map[string]any{"revision": req.Revision}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.RollbackDeploymentResponse{Status: "rolling_back"}, nil
}

// ReapplyDeployment re-applies the current revision by setting status to pending and enqueuing a deploy job.
// When account placement differs from deployment routing, enqueues a cross-cluster migration job instead.
// Works for active or failed deployments.
func (s *Server) ReapplyDeployment(ctx context.Context, req *adminv1.ReapplyDeploymentRequest) (*adminv1.ReapplyDeploymentResponse, error) {
	if req.DeploymentId == "" {
		return nil, fmt.Errorf("deployment_id is required")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}
	if dep.Status == deploymentstore.StatusUndeploying {
		return nil, fmt.Errorf("deployment is being undeployed")
	}

	var accountClusterID string
	if err := s.db.QueryRow(`SELECT COALESCE(cluster_id, '') FROM accounts WHERE id = $1`, dep.AccountID).
		Scan(&accountClusterID); err != nil {
		return nil, fmt.Errorf("load account cluster: %w", err)
	}

	routingClusterID := dep.EffectiveClusterID()

	if placementMismatch(accountClusterID, routingClusterID) {
		if s.queue == nil {
			return nil, fmt.Errorf("queue not configured; cannot migrate cluster placement")
		}
		if err := s.queue.InsertMigrateDeploymentClusterJob(ctx, dep.ID, accountClusterID, routingClusterID); err != nil {
			return nil, fmt.Errorf("enqueue cluster migration: %w", err)
		}
		msg := placementUpdateMessage(routingClusterID, accountClusterID)
		return &adminv1.ReapplyDeploymentResponse{
			Status:                  "reapplying",
			ClusterPlacementUpdated: true,
			Message:                 msg + ". Teardown on source cluster queued, then deploy to target.",
		}, nil
	}

	// Set status to pending (skip if already pending — just re-enqueue the job)
	if dep.Status != deploymentstore.StatusPending {
		if err := s.deployStore.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusPending, EventMsg: "Admin re-apply requested"}); err != nil {
			return nil, fmt.Errorf("update status: %w", err)
		}
	}

	// Enqueue deploy job
	if s.queue != nil {
		if err := s.queue.InsertDeployJob(ctx, dep.ID, routingClusterID); err != nil {
			return nil, fmt.Errorf("enqueue deploy job: %w", err)
		}
	}

	return &adminv1.ReapplyDeploymentResponse{
		Status: "reapplying",
	}, nil
}

// RepairNormalizedSpec re-generates the deployment template from the original
// package spec, merges existing variable values, updates the stored deployment
// spec JSON, and rebuilds normalized tables.
func (s *Server) RepairNormalizedSpec(ctx context.Context, req *adminv1.RepairNormalizedSpecRequest) (*adminv1.RepairNormalizedSpecResponse, error) {
	if req.DeploymentId == "" {
		return nil, fmt.Errorf("deployment_id is required")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found: %s", req.DeploymentId)
	}

	// Parse the currently stored deployment spec.
	var storedDS deployment.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(dep.DeploymentSpecJSON), &storedDS); err != nil {
		return nil, fmt.Errorf("parse stored deployment spec: %w", err)
	}

	// Re-generate the deployment template from the original package spec.
	// This picks up fixes to credential dedup, variable merging, etc.
	if err := s.retemplateDeploymentSpec(dep, &storedDS); err != nil {
		s.log.Warn("Re-template from package spec failed, falling back to stored spec",
			"deployment_id", req.DeploymentId, "error", err)
	} else {
		// Persist the fixed deployment spec JSON.
		fixedJSON, err := json.Marshal(&storedDS)
		if err != nil {
			return nil, fmt.Errorf("marshal fixed spec: %w", err)
		}
		if err := s.deployStore.UpdateDeploymentSpecJSON(dep.ID, string(fixedJSON)); err != nil {
			return nil, fmt.Errorf("update deployment spec JSON: %w", err)
		}
	}

	ingressCfg, ingressErr := s.resolveIngressForCluster(ctx, dep.EffectiveClusterID())
	if ingressErr != nil {
		return nil, fmt.Errorf("resolve cluster ingress config: %w", ingressErr)
	}
	workloads, services, ingresses, err := s.deployStore.RepairNormalizedSpec(req.DeploymentId, &deploymentstore.NormalizedSpecConfig{
		IngressDomain:          ingressCfg.AgentIngressDomain,
		IngestionIngressDomain: ingressCfg.IngestionIngressDomain,
	})
	if err != nil {
		return nil, fmt.Errorf("repair normalized spec: %w", err)
	}

	s.log.Info("Repaired normalized spec",
		"deployment_id", req.DeploymentId,
		"workloads", workloads,
		"services", services,
		"ingresses", ingresses,
	)

	return &adminv1.RepairNormalizedSpecResponse{
		Status:    "ok",
		Workloads: int32(workloads), //nolint:gosec
		Services:  int32(services),  //nolint:gosec
		Ingresses: int32(ingresses), //nolint:gosec
	}, nil
}

// retemplateDeploymentSpec re-generates the template from the original package
// spec and merges the new variables/environment into the stored deployment spec.
// Existing variable Values are preserved (they hold user-supplied secrets).
func (s *Server) retemplateDeploymentSpec(dep *deploymentstore.Deployment, storedDS *deployment.AstroDeploymentSpec) error {
	// Look up the package spec from agent_versions.
	var specJSON string
	err := s.db.QueryRow(`
		SELECT spec_json FROM agent_versions
		WHERE account_id = $1 AND name = $2 AND build_id = $3
	`, dep.AccountID, dep.AgentName, dep.BuildID).Scan(&specJSON)
	if err != nil {
		return fmt.Errorf("look up package spec: %w", err)
	}

	var astroSpec spec.AstroSpec
	if err := json.Unmarshal([]byte(specJSON), &astroSpec); err != nil {
		return fmt.Errorf("parse package spec: %w", err)
	}

	// Re-generate the template using the fixed code.
	newTemplate, err := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
		Spec:        &astroSpec,
		AgentName:   dep.AgentName,
		Account:     storedDS.Source.Account,
		BuildID:     dep.BuildID,
		RegistryURL: storedDS.Source.Registry,
	})
	if err != nil {
		return fmt.Errorf("generate template: %w", err)
	}

	// Collect existing variable Values from the stored spec (these contain
	// user-supplied or KMS-encrypted data that we must not lose).
	existingValues := make(map[string]string)
	for key, v := range storedDS.Variables {
		if v.Value != "" {
			existingValues[key] = v.Value
		}
	}

	// Also pull values from the DB variables table (they survive stripping).
	dbVars, err := s.deployStore.GetDeploymentVariables(dep.ID)
	if err == nil {
		for _, v := range dbVars {
			if v.Value != "" {
				existingValues[v.Name] = v.Value
			}
		}
	}

	// Preserve user-selected adapters from the stored interfaces block.
	var userAdapters []string
	if storedDS.Interfaces != nil {
		userAdapters = storedDS.Interfaces.Adapters
	}

	// Replace variables and agent environment with the re-generated template.
	storedDS.Variables = newTemplate.Variables
	storedDS.Agent.Environment = newTemplate.Agent.Environment
	storedDS.Interfaces = newTemplate.Interfaces

	// Restore user-selected adapters (the template generates empty adapters)
	// and strip variables for adapters the user didn't select (e.g. SLACK_CONFIG
	// when only "web" is enabled).
	if storedDS.Interfaces != nil && userAdapters != nil {
		storedDS.Interfaces.Adapters = userAdapters
		deployment.ApplyAdapterShaping(storedDS, userAdapters)
	}

	// Restore variable values from the existing deployment.
	for key, v := range storedDS.Variables {
		if val, ok := existingValues[key]; ok {
			v.Value = val
			storedDS.Variables[key] = v
		}
	}

	return nil
}

// GetDeploymentJobs returns River job history for a deployment.
func (s *Server) GetDeploymentJobs(ctx context.Context, req *adminv1.GetDeploymentJobsRequest) (*adminv1.GetDeploymentJobsResponse, error) {
	if req.DeploymentId == "" {
		return nil, fmt.Errorf("deployment_id is required")
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}

	// Query River job table for jobs related to this deployment.
	// errors is jsonb[] (Postgres array) — cast to text for scanning.
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, state, attempt, max_attempts, created_at, attempted_at, finalized_at, errors::text,
		       COALESCE(args->>'cluster_id', '')
		FROM river.river_job
		WHERE args->>'deployment_id' = $1
		ORDER BY created_at DESC
		LIMIT 25
	`, dep.ID)
	if err != nil {
		s.log.Warn("Failed to query river jobs", "error", err, "deployment_id", dep.ID)
		// Return empty jobs instead of failing — river schema may not exist
		return &adminv1.GetDeploymentJobsResponse{}, nil
	}
	defer rows.Close() //nolint:errcheck

	var jobs []*adminv1.DeploymentJob
	for rows.Next() {
		var j adminv1.DeploymentJob
		var createdAt time.Time
		var attemptedAt, finalizedAt sql.NullTime
		var errorsStr sql.NullString
		if err := rows.Scan(&j.JobId, &j.Kind, &j.State, &j.Attempt, &j.MaxAttempt, &createdAt, &attemptedAt, &finalizedAt, &errorsStr, &j.ClusterId); err != nil {
			s.log.Warn("Failed to scan river job row", "error", err)
			continue
		}
		j.CreatedAt = createdAt.Format(time.RFC3339)
		if attemptedAt.Valid {
			j.AttemptedAt = attemptedAt.Time.Format(time.RFC3339)
		}
		if finalizedAt.Valid {
			j.FinalizedAt = finalizedAt.Time.Format(time.RFC3339)
		}
		if errorsStr.Valid {
			j.Errors = errorsStr.String
		}
		jobs = append(jobs, &j)
	}
	if err := rows.Err(); err != nil {
		s.log.Warn("Error iterating river jobs", "error", err)
	}

	return &adminv1.GetDeploymentJobsResponse{Jobs: jobs}, nil
}
