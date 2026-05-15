package admingrpc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	connectv1 "github.com/astropods/astro/packages/astro-proto/connect/v1"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Server implements adminv1.AdminServiceServer.
// CommandDispatcher sends commands to connected devices.
type CommandDispatcher interface {
	SendCommand(ctx context.Context, deviceID string, cmd *connectv1.ShellCommand) (*connectv1.CommandResult, error)
}

type Server struct {
	adminv1.UnimplementedAdminServiceServer

	log            *logger.Logger
	deployStore    *deploymentstore.Store
	k8sClient      k8s.ClusterClient
	lokiClient     *loki.Client
	db             *sql.DB
	openMeterURL   string
	cmdDispatch    CommandDispatcher
	httpHandler    http.Handler
	workosClientID string
	databaseURL    string
	queue          *riverqueue.Queue

	auditStore   *auditlog.Store
	workosClient *auth.WorkOSClient

	// Ingress domain config — needed by RepairNormalizedSpec to regenerate ingress rows
	ingressDomain          string
	ingestionIngressDomain string

	riverMu        sync.Mutex
	riverUIHandler http.Handler
	riverUICleanup func()
}

// SetHTTPHandler sets the HTTP handler (gin router) for proxying HTTP requests.
func (s *Server) SetHTTPHandler(h http.Handler) {
	s.httpHandler = h
}

// SetWorkOSClientID sets the WorkOS client ID for GetAuthConfig.
func (s *Server) SetWorkOSClientID(id string) {
	s.workosClientID = id
}

// SetWorkOSClient sets the WorkOS client for resolving user emails.
func (s *Server) SetWorkOSClient(c *auth.WorkOSClient) {
	s.workosClient = c
}

// New creates a new admin gRPC server.
func New(
	log *logger.Logger,
	deployStore *deploymentstore.Store,
	k8sClient k8s.ClusterClient,
	lokiClient *loki.Client,
	db *sql.DB,
	openMeterURL string,
	databaseURL string,
	queue *riverqueue.Queue,
	ingressDomain string,
	ingestionIngressDomain string,
	auditStore *auditlog.Store,
) *Server {
	return &Server{
		log:                    log,
		deployStore:            deployStore,
		k8sClient:              k8sClient,
		lokiClient:             lokiClient,
		db:                     db,
		openMeterURL:           strings.TrimRight(openMeterURL, "/"),
		databaseURL:            databaseURL,
		queue:                  queue,
		ingressDomain:          ingressDomain,
		ingestionIngressDomain: ingestionIngressDomain,
		auditStore:             auditStore,
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
		if d.DriftReportJSON != nil {
			var report struct {
				Summary adminv1.DriftSummary `json:"summary"`
			}
			if json.Unmarshal([]byte(*d.DriftReportJSON), &report) == nil {
				ad.DriftSummary = &report.Summary
			}
		}

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

	// Look up account name and owner
	var accountName, ownerUserID string
	_ = s.db.QueryRow(`
		SELECT a.name,
		       COALESCE((SELECT user_id FROM account_members WHERE account_id = a.id ORDER BY created_at ASC LIMIT 1), '')
		FROM accounts a WHERE a.id = $1`,
		dep.AccountID,
	).Scan(&accountName, &ownerUserID)

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
	}

	// Include adapters from the stored deployment spec (default to empty list)
	resp.Adapters = []string{}
	{
		var ds spec.AstroDeploymentSpec
		if err := json.Unmarshal([]byte(dep.DeploymentSpecJSON), &ds); err == nil && ds.Interfaces != nil && len(ds.Interfaces.Adapters) > 0 {
			resp.Adapters = ds.Interfaces.Adapters
		}
	}

	// Include stored drift report (cheap DB read, same row)
	if report, checkedAt, err := s.deployStore.GetDriftReport(dep.ID); err == nil && report != nil {
		resp.DriftReport = storeDriftReportToProto(report)
		if checkedAt != nil {
			resp.DriftCheckedAt = checkedAt.Format(time.RFC3339)
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
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	namespace := req.Namespace
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	clientset := s.k8sClient.Clientset()
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

			// Container statuses
			for _, cs := range p.Status.ContainerStatuses {
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
				})
			}

			// Container resources, security, mounts, envFrom
			for _, c := range p.Spec.Containers {
				cr := &adminv1.K8sContainerResources{
					Name:            c.Name,
					ImagePullPolicy: string(c.ImagePullPolicy),
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
	if err := s.deployStore.UpdateStatus(dep.ID, deploymentstore.StatusUndeploying, "", nil); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	// Enqueue async undeploy job
	if s.queue != nil {
		if err := s.queue.InsertUndeployJob(context.Background(), dep.ID); err != nil {
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
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}
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

	err = s.k8sClient.Clientset().CoreV1().Pods(dep.Namespace).Delete(ctx, req.Pod, metav1.DeleteOptions{})
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

	// Loki path: query the centralized log store.
	if s.lokiClient != nil {
		p := loki.QueryParams{
			Namespace: dep.Namespace,
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

		lines, err := s.lokiClient.QueryLogs(ctx, p)
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
	if s.k8sClient == nil {
		return nil, fmt.Errorf("log backend not configured")
	}
	if req.Pod == "" {
		return nil, fmt.Errorf("pod is required when Loki is not configured")
	}

	logOpts := &corev1.PodLogOptions{TailLines: &tailLines}
	if req.Container != "" {
		logOpts.Container = req.Container
	}
	stream, err := s.k8sClient.Clientset().CoreV1().Pods(dep.Namespace).GetLogs(req.Pod, logOpts).Stream(ctx)
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
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}
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

	pod, err := s.k8sClient.Clientset().CoreV1().Pods(dep.Namespace).Get(ctx, req.Pod, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod: %w", err)
	}

	clientset := s.k8sClient.Clientset()
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
			(a.openmeter_customer_id IS NOT NULL) AS has_openmeter,
			EXISTS(SELECT 1 FROM account_langfuse WHERE account_id = a.id) AS has_langfuse,
			a.deleted_at,
			a.created_at,
			a.updated_at
		FROM accounts a
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
		if err := rows.Scan(&acct.ID, &acct.Name, &acct.Type, &acct.OwnerUserID, &acct.MemberCount, &acct.HasOpenMeter, &acct.HasLangfuse, &deletedAt, &createdAt, &updatedAt); err != nil {
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

// ProxyOpenMeter forwards an HTTP request to the configured OpenMeter server.
func (s *Server) ProxyOpenMeter(ctx context.Context, req *adminv1.OpenMeterProxyRequest) (*adminv1.OpenMeterProxyResponse, error) {
	if s.openMeterURL == "" {
		return nil, fmt.Errorf("OPENMETER_URL not configured")
	}

	targetURL := s.openMeterURL + req.Path

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, fmt.Errorf("build openmeter request: %w", err)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(httpReq) //nolint:gosec // base URL is from trusted server config (OPENMETER_URL)
	if err != nil {
		return nil, fmt.Errorf("openmeter request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read openmeter response: %w", err)
	}

	headers := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	return &adminv1.OpenMeterProxyResponse{
		StatusCode: int32(resp.StatusCode), //nolint:gosec // HTTP status codes are bounded
		Headers:    headers,
		Body:       body,
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
	if dep.Status != deploymentstore.StatusScaledDown && dep.Status != deploymentstore.StatusStopped {
		return nil, fmt.Errorf("deployment is not stopped or scaled_down (current: %s)", dep.Status)
	}

	// Update status to pending
	if err := s.deployStore.UpdateStatus(dep.ID, deploymentstore.StatusPending, "Admin wakeup requested", nil); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	// Clear scaled-down tracking
	if err := s.deployStore.ClearScaledDown(dep.Namespace); err != nil {
		s.log.Warn("Failed to clear scaled-down tracking", "namespace", dep.Namespace, "error", err)
	}

	// Enqueue wakeup job
	if s.queue != nil {
		if err := s.queue.InsertWakeUpJob(context.Background(), dep.ID); err != nil {
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
	if dep.Status != deploymentstore.StatusActive && dep.Status != deploymentstore.StatusScaledDown {
		return nil, fmt.Errorf("deployment is not active or scaled_down (current: %s)", dep.Status)
	}

	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	if err := k8s.StopNamespaceWorkloads(ctx, s.k8sClient.Clientset(), dep.Namespace); err != nil {
		return nil, fmt.Errorf("stop workloads: %w", err)
	}

	if err := s.deployStore.UpdateStatus(dep.ID, deploymentstore.StatusStopped, "Admin stop requested", nil); err != nil {
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
		if err := s.queue.InsertDeployJob(context.Background(), dep.ID); err != nil {
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
// Works for active, failed, or scaled_down deployments.
func (s *Server) ReapplyDeployment(_ context.Context, req *adminv1.ReapplyDeploymentRequest) (*adminv1.ReapplyDeploymentResponse, error) {
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

	// Clear scaled-down tracking if applicable
	if dep.Status == deploymentstore.StatusScaledDown {
		if err := s.deployStore.ClearScaledDown(dep.Namespace); err != nil {
			s.log.Warn("Failed to clear scaled-down tracking", "namespace", dep.Namespace, "error", err)
		}
	}

	// Set status to pending (skip if already pending — just re-enqueue the job)
	if dep.Status != deploymentstore.StatusPending {
		if err := s.deployStore.UpdateStatus(dep.ID, deploymentstore.StatusPending, "Admin re-apply requested", nil); err != nil {
			return nil, fmt.Errorf("update status: %w", err)
		}
	}

	// Enqueue deploy job
	if s.queue != nil {
		if err := s.queue.InsertDeployJob(context.Background(), dep.ID); err != nil {
			return nil, fmt.Errorf("enqueue deploy job: %w", err)
		}
	}

	return &adminv1.ReapplyDeploymentResponse{Status: "reapplying"}, nil
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
	var storedDS spec.AstroDeploymentSpec
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

	// Read the live K8s Secret so repair can store correct value hashes.
	var liveSecretData map[string][]byte
	secretName := deployment.GenerateSecretName(dep.AgentName, dep.BuildID)
	secret, err := s.k8sClient.Clientset().CoreV1().Secrets(dep.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil {
		liveSecretData = secret.Data
	}

	workloads, services, ingresses, err := s.deployStore.RepairNormalizedSpec(req.DeploymentId, &deploymentstore.NormalizedSpecConfig{
		IngressDomain:          s.ingressDomain,
		IngestionIngressDomain: s.ingestionIngressDomain,
	}, liveSecretData)
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
func (s *Server) retemplateDeploymentSpec(dep *deploymentstore.Deployment, storedDS *spec.AstroDeploymentSpec) error {
	// Look up the package spec from agent_versions.
	var specJSON string
	var ecrNamespace string
	err := s.db.QueryRow(`
		SELECT spec_json, ecr_namespace FROM agent_versions
		WHERE account_id = $1 AND name = $2 AND build_id = $3
	`, dep.AccountID, dep.AgentName, dep.BuildID).Scan(&specJSON, &ecrNamespace)
	if err != nil {
		return fmt.Errorf("look up package spec: %w", err)
	}

	var astroSpec spec.AstroSpec
	if err := json.Unmarshal([]byte(specJSON), &astroSpec); err != nil {
		return fmt.Errorf("parse package spec: %w", err)
	}

	// Re-generate the template using the fixed code.
	newTemplate, err := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
		Spec:         &astroSpec,
		AgentName:    dep.AgentName,
		Account:      storedDS.Source.Account,
		ECRNamespace: ecrNamespace,
		BuildID:      dep.BuildID,
		RegistryURL:  storedDS.Source.Registry,
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

// SetAdapters updates the messaging adapters on a deployment's stored spec.
func (s *Server) SetAdapters(_ context.Context, req *adminv1.SetAdaptersRequest) (*adminv1.SetAdaptersResponse, error) {
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

	var ds spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(dep.DeploymentSpecJSON), &ds); err != nil {
		return nil, fmt.Errorf("parse deployment spec: %w", err)
	}

	if ds.Interfaces == nil {
		ds.Interfaces = &spec.DeploymentInterfaces{}
	}

	ds.Interfaces.Adapters = req.Adapters

	fixedJSON, err := json.Marshal(&ds)
	if err != nil {
		return nil, fmt.Errorf("marshal updated spec: %w", err)
	}
	if err := s.deployStore.UpdateDeploymentSpecJSON(dep.ID, string(fixedJSON)); err != nil {
		return nil, fmt.Errorf("update deployment spec: %w", err)
	}

	s.log.Info("Set adapters", "deployment_id", req.DeploymentId, "adapters", req.Adapters)

	return &adminv1.SetAdaptersResponse{
		Status:   "ok",
		Adapters: req.Adapters,
	}, nil
}

// GetDeploymentJobs returns River job history and last reconcile time for a deployment.
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
		SELECT kind, state, attempt, max_attempts, created_at, attempted_at, finalized_at, errors::text
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
		if err := rows.Scan(&j.Kind, &j.State, &j.Attempt, &j.MaxAttempt, &createdAt, &attemptedAt, &finalizedAt, &errorsStr); err != nil {
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

	// Get last reconcile run time from River job history.
	resp := &adminv1.GetDeploymentJobsResponse{Jobs: jobs}
	var finalizedAt sql.NullTime
	if err := s.db.QueryRowContext(ctx,
		`SELECT finalized_at FROM river.river_job WHERE kind = 'reconcile' AND state = 'completed' ORDER BY finalized_at DESC LIMIT 1`,
	).Scan(&finalizedAt); err == nil && finalizedAt.Valid {
		resp.LastReconcileAt = finalizedAt.Time.Format(time.RFC3339)
	}

	return resp, nil
}

// RefreshDriftReport runs drift detection on demand for a single deployment and returns the report.
func (s *Server) RefreshDriftReport(ctx context.Context, req *adminv1.RefreshDriftReportRequest) (*adminv1.RefreshDriftReportResponse, error) {
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

	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	// Build drift report using the same logic as the reconciler
	workloads, err := s.deployStore.GetWorkloads(dep.ID)
	if err != nil {
		return nil, fmt.Errorf("get workloads: %w", err)
	}
	services, _ := s.deployStore.GetServices(dep.ID)
	ingresses, _ := s.deployStore.GetIngresses(dep.ID)
	variables, _ := s.deployStore.GetDeploymentVariables(dep.ID)
	resolvedKeys, _ := s.deployStore.GetResolvedKeys(dep.ID)

	svcNameByID := map[int]string{}
	for _, svc := range services {
		svcNameByID[svc.ID] = svc.WorkloadName
	}

	report := riverqueue.BuildDriftReport(ctx, s.k8sClient.Clientset(), dep.Namespace, dep.AgentName, dep.BuildID, workloads, services, ingresses, svcNameByID, variables, resolvedKeys)
	if report == nil {
		return nil, fmt.Errorf("failed to build drift report")
	}

	// Save to DB
	if err := s.deployStore.SaveDriftReport(dep.ID, report); err != nil {
		s.log.Warn("Failed to save refreshed drift report", "error", err, "deployment_id", dep.ID)
	}

	// Read back to get the checked_at timestamp
	_, checkedAt, _ := s.deployStore.GetDriftReport(dep.ID)
	resp := &adminv1.RefreshDriftReportResponse{
		DriftReport: storeDriftReportToProto(report),
	}
	if checkedAt != nil {
		resp.DriftCheckedAt = checkedAt.Format(time.RFC3339)
	}

	return resp, nil
}

// storeDriftReportToProto converts a store DriftReport to the proto type.
func storeDriftReportToProto(report *deploymentstore.DriftReport) *adminv1.DriftReport {
	if report == nil {
		return nil
	}
	proto := &adminv1.DriftReport{
		DetectedAt: report.DetectedAt,
		Summary: &adminv1.DriftSummary{
			Total:   report.Summary.Total,
			Match:   report.Summary.Match,
			Missing: report.Summary.Missing,
			Extra:   report.Summary.Extra,
			Drift:   report.Summary.Drift,
		},
	}
	for _, item := range report.Workloads {
		proto.Workloads = append(proto.Workloads, &adminv1.DriftResourceItem{
			Name: item.Name, Type: item.Type, Status: item.Status,
			Expected: item.Expected, Actual: item.Actual,
		})
	}
	for _, item := range report.Services {
		proto.Services = append(proto.Services, &adminv1.DriftResourceItem{
			Name: item.Name, Type: item.Type, Status: item.Status,
			Expected: item.Expected, Actual: item.Actual,
		})
	}
	for _, item := range report.Ingresses {
		proto.Ingresses = append(proto.Ingresses, &adminv1.DriftResourceItem{
			Name: item.Name, Type: item.Type, Status: item.Status,
			Expected: item.Expected, Actual: item.Actual,
		})
	}
	for _, item := range report.EnvVars {
		proto.EnvVars = append(proto.EnvVars, &adminv1.DriftResourceItem{
			Name: item.Name, Type: item.Type, Status: item.Status,
			Expected: item.Expected, Actual: item.Actual,
		})
	}
	for _, item := range report.Secrets {
		proto.Secrets = append(proto.Secrets, &adminv1.DriftResourceItem{
			Name: item.Name, Type: item.Type, Status: item.Status,
			Expected: item.Expected, Actual: item.Actual,
		})
	}
	return proto
}

// BackfillResolvedKeys re-resolves the ConfigMap/Secret key sets for all active
// deployments and stores them in deployment_resolved_keys. This is a one-time
// migration for deployments that pre-date the resolved keys table.
func (s *Server) BackfillResolvedKeys(ctx context.Context, _ *adminv1.BackfillResolvedKeysRequest) (*adminv1.BackfillResolvedKeysResponse, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.agent_name, d.build_id, d.namespace, d.deployment_spec_json
		FROM deployments d
		WHERE d.status NOT IN ('undeployed', 'undeploying')
	`)
	if err != nil {
		return nil, fmt.Errorf("query deployments: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	type row struct {
		id, agentName, buildID, namespace, specJSON string
	}
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.agentName, &r.buildID, &r.namespace, &r.specJSON); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		todo = append(todo, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	var count int32
	for _, r := range todo {
		var ds spec.AstroDeploymentSpec
		if err := json.Unmarshal([]byte(r.specJSON), &ds); err != nil {
			s.log.Warn("BackfillResolvedKeys: bad spec JSON", "deployment_id", r.id, "error", err)
			continue
		}

		externalAgentHost := ""
		if ep := spec.ExposedEndpoint(ds.Agent.Endpoints); ep != nil {
			if ep.Expose != nil && ep.Expose.Domain != "" {
				externalAgentHost = ep.Expose.Domain
			} else if s.ingressDomain != "" {
				externalAgentHost = k8s.GenerateIngressHost(r.agentName, r.namespace, s.ingressDomain)
			}
		}
		rctx := deployment.ResolveContext{
			Namespace:         r.namespace,
			AgentName:         r.agentName,
			BuildID:           r.buildID,
			SecretName:        deployment.GenerateSecretName(r.agentName, r.buildID),
			ExternalAgentHost: externalAgentHost,
		}
		resolved := deployment.ResolveDeploymentSpecEnv(&ds, rctx)

		// Read live K8s secret to get correct secret hashes
		secretName := deployment.GenerateSecretName(r.agentName, r.buildID)
		liveSecret, err := s.k8sClient.Clientset().CoreV1().Secrets(r.namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err == nil {
			for key, val := range liveSecret.Data {
				resolved.SecretData[key] = string(val)
			}
		}

		cmKeys := make([]string, 0, len(resolved.ConfigMapData))
		cmHashes := make(map[string]string, len(resolved.ConfigMapData))
		for k, v := range resolved.ConfigMapData {
			cmKeys = append(cmKeys, k)
			h := sha256.Sum256([]byte(v))
			cmHashes[k] = hex.EncodeToString(h[:])
		}
		secKeys := make([]string, 0, len(resolved.SecretData))
		secHashes := make(map[string]string, len(resolved.SecretData))
		for k, v := range resolved.SecretData {
			secKeys = append(secKeys, k)
			h := sha256.Sum256([]byte(v))
			secHashes[k] = hex.EncodeToString(h[:])
		}
		cmHashJSON, err := json.Marshal(cmHashes)
		if err != nil {
			s.log.Warn("BackfillResolvedKeys: marshal configmap hashes", "error", err)
			continue
		}
		secHashJSON, err := json.Marshal(secHashes)
		if err != nil {
			s.log.Warn("BackfillResolvedKeys: marshal secret hashes", "error", err)
			continue
		}

		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO deployment_resolved_keys (deployment_id, configmap_keys, secret_keys, configmap_hashes, secret_hashes)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (deployment_id) DO UPDATE
			SET configmap_keys = EXCLUDED.configmap_keys, secret_keys = EXCLUDED.secret_keys,
			    configmap_hashes = EXCLUDED.configmap_hashes, secret_hashes = EXCLUDED.secret_hashes
		`, r.id, pq.Array(cmKeys), pq.Array(secKeys), cmHashJSON, secHashJSON); err != nil {
			s.log.Warn("BackfillResolvedKeys: insert failed", "deployment_id", r.id, "error", err)
			continue
		}
		count++
		s.log.Info("BackfillResolvedKeys: stored keys", "deployment_id", r.id,
			"configmap_keys", len(cmKeys), "secret_keys", len(secKeys))
	}

	return &adminv1.BackfillResolvedKeysResponse{BackfilledCount: count}, nil
}

// TriggerOpenMeterBackfill enqueues an immediate OpenMeter customer backfill job.
// This creates OpenMeter customers for any accounts that are missing one.
func (s *Server) TriggerOpenMeterBackfill(_ context.Context, _ *adminv1.TriggerOpenMeterBackfillRequest) (*adminv1.TriggerOpenMeterBackfillResponse, error) {
	if s.queue == nil {
		return nil, fmt.Errorf("river queue not available")
	}
	if err := s.queue.InsertOpenMeterBackfillJob(context.Background()); err != nil {
		return nil, fmt.Errorf("enqueue openmeter backfill: %w", err)
	}
	s.log.Info("Triggered OpenMeter customer backfill via admin RPC")
	return &adminv1.TriggerOpenMeterBackfillResponse{Status: "backfill_enqueued"}, nil
}
