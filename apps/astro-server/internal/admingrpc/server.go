package admingrpc

import (
	"bytes"
	"context"
	"database/sql"
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
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	db             *sql.DB
	openMeterURL   string
	cmdDispatch    CommandDispatcher
	httpHandler    http.Handler
	workosClientID string
	databaseURL    string
	queue          *riverqueue.Queue

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

// New creates a new admin gRPC server.
func New(
	log *logger.Logger,
	deployStore *deploymentstore.Store,
	k8sClient k8s.ClusterClient,
	db *sql.DB,
	openMeterURL string,
	databaseURL string,
	queue *riverqueue.Queue,
) *Server {
	return &Server{
		log:          log,
		deployStore:  deployStore,
		k8sClient:    k8sClient,
		db:           db,
		openMeterURL: strings.TrimRight(openMeterURL, "/"),
		databaseURL:  databaseURL,
		queue:        queue,
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
func (s *Server) ListDeployments(_ context.Context, req *adminv1.ListDeploymentsRequest) (*adminv1.ListDeploymentsResponse, error) {
	dbDeps, err := s.deployStore.ListAllWithAccount()
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

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
		}
		if d.ErrorMessage != nil {
			ad.ErrorMessage = *d.ErrorMessage
		}
		if d.CurrentRevision != nil {
			ad.CurrentRevision = int32(*d.CurrentRevision) //nolint:gosec // revision numbers are small
		}

		results = append(results, ad)
	}

	return &adminv1.ListDeploymentsResponse{
		Deployments: results,
		Count:       int32(len(results)), //nolint:gosec // bounded by cluster size
	}, nil
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

	// Look up account name
	var accountName string
	_ = s.db.QueryRow("SELECT name FROM accounts WHERE id = $1", dep.AccountID).Scan(&accountName)

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
	}
	if dep.ErrorMessage != nil {
		ad.ErrorMessage = *dep.ErrorMessage
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

	// Include stored drift report (cheap DB read, same row)
	if report, checkedAt, err := s.deployStore.GetDriftReport(dep.ID); err == nil && report != nil {
		resp.DriftReport = storeDriftReportToProto(report)
		if checkedAt != nil {
			resp.DriftCheckedAt = checkedAt.Format(time.RFC3339)
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

			// Container resources
			for _, c := range p.Spec.Containers {
				cr := &adminv1.K8sContainerResources{Name: c.Name}
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
				pi.Containers = append(pi.Containers, cr)
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

	return &adminv1.RestartDeploymentResponse{Status: "restarting"}, nil
}

// GetPodLogs returns the tail of a pod's logs.
func (s *Server) GetPodLogs(ctx context.Context, req *adminv1.GetPodLogsRequest) (*adminv1.GetPodLogsResponse, error) {
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

	tailLines := int64(req.TailLines)
	if tailLines <= 0 {
		tailLines = 100
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
			a.created_at,
			a.updated_at
		FROM accounts a
		ORDER BY a.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var accounts []*adminv1.AdminAccount
	for rows.Next() {
		var acct adminv1.AdminAccount
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&acct.ID, &acct.Name, &acct.Type, &acct.OwnerUserID, &acct.MemberCount, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
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
	if dep.Status != deploymentstore.StatusScaledDown {
		return nil, fmt.Errorf("deployment is not scaled_down (current: %s)", dep.Status)
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

	return &adminv1.WakeUpDeploymentResponse{Status: "waking_up"}, nil
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

// BackfillDeployments creates revision 1 and sets current_revision for all deployments
// that were created before the async deploy architecture (missing revisions).
func (s *Server) BackfillDeployments(ctx context.Context, _ *adminv1.BackfillDeploymentsRequest) (*adminv1.BackfillDeploymentsResponse, error) {
	// Find deployments without revisions
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.build_id, d.deployment_spec_json, d.encrypted_data_key, d.kms_key_arn, d.deployed_at
		FROM deployments d
		WHERE d.status != 'undeployed'
		  AND d.current_revision IS NULL
		  AND d.deployment_spec_json IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM deployment_revisions r WHERE r.deployment_id = d.id)
	`)
	if err != nil {
		return nil, fmt.Errorf("query deployments to backfill: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	type backfillRow struct {
		id, buildID, specJSON string
		dataKey               []byte
		kmsArn                *string
		deployedAt            time.Time
	}
	var toBackfill []backfillRow
	for rows.Next() {
		var r backfillRow
		if err := rows.Scan(&r.id, &r.buildID, &r.specJSON, &r.dataKey, &r.kmsArn, &r.deployedAt); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		toBackfill = append(toBackfill, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deployments: %w", err)
	}

	var count int32
	for _, r := range toBackfill {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			s.log.Warn("Backfill: failed to begin tx", "deployment_id", r.id, "error", err)
			continue
		}

		// Create revision 1
		_, err = tx.ExecContext(ctx, `
			INSERT INTO deployment_revisions (deployment_id, revision, build_id, spec_json, kms_ciphertext, kms_key_id, created_at)
			VALUES ($1, 1, $2, $3::jsonb, $4, $5, $6)
			ON CONFLICT (deployment_id, revision) DO NOTHING
		`, r.id, r.buildID, r.specJSON, r.dataKey, r.kmsArn, r.deployedAt)
		if err != nil {
			tx.Rollback() //nolint:errcheck,gosec
			s.log.Warn("Backfill: failed to insert revision", "deployment_id", r.id, "error", err)
			continue
		}

		// Set current_revision and status_changed_at
		_, err = tx.ExecContext(ctx, `
			UPDATE deployments SET current_revision = 1, status_changed_at = COALESCE(status_changed_at, deployed_at)
			WHERE id = $1
		`, r.id)
		if err != nil {
			tx.Rollback() //nolint:errcheck,gosec
			s.log.Warn("Backfill: failed to update deployment", "deployment_id", r.id, "error", err)
			continue
		}

		// Create initial event if none exists
		_, _ = tx.ExecContext(ctx, `
			INSERT INTO deployment_events (deployment_id, status, message, created_at)
			SELECT $1, status, 'Backfilled from legacy deployment', deployed_at
			FROM deployments WHERE id = $1
			AND NOT EXISTS (SELECT 1 FROM deployment_events WHERE deployment_id = $1)
		`, r.id)

		if err := tx.Commit(); err != nil {
			s.log.Warn("Backfill: failed to commit", "deployment_id", r.id, "error", err)
			continue
		}
		count++
		s.log.Info("Backfill: created revision 1", "deployment_id", r.id, "build_id", r.buildID)
	}

	return &adminv1.BackfillDeploymentsResponse{BackfilledCount: count}, nil
}

// RepairNormalizedSpec re-parses the stored spec JSON and rebuilds the
// deployment_workloads, services, ingresses, volumes, and variables tables.
func (s *Server) RepairNormalizedSpec(_ context.Context, req *adminv1.RepairNormalizedSpecRequest) (*adminv1.RepairNormalizedSpecResponse, error) {
	if req.DeploymentId == "" {
		return nil, fmt.Errorf("deployment_id is required")
	}

	workloads, services, ingresses, err := s.deployStore.RepairNormalizedSpec(req.DeploymentId)
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

	// Get last reconcile scan time from namespace_ownership
	resp := &adminv1.GetDeploymentJobsResponse{Jobs: jobs}
	var scannedAt sql.NullTime
	if err := s.db.QueryRowContext(ctx,
		`SELECT scanned_at FROM namespace_ownership WHERE namespace = $1`, dep.Namespace,
	).Scan(&scannedAt); err == nil && scannedAt.Valid {
		resp.LastReconcileAt = scannedAt.Time.Format(time.RFC3339)
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

	svcNameByID := map[int]string{}
	for _, svc := range services {
		svcNameByID[svc.ID] = svc.WorkloadName
	}

	report := s.buildDriftReportFromK8s(ctx, dep.Namespace, workloads, services, ingresses, svcNameByID)
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

// buildDriftReportFromK8s is the admin server's version of the reconciler's buildDriftReport.
// It reuses the same core logic from the riverqueue package.
func (s *Server) buildDriftReportFromK8s(ctx context.Context, namespace string, workloads []*deploymentstore.Workload, services []*deploymentstore.Service, ingresses []*deploymentstore.Ingress, svcNameByID map[int]string) *deploymentstore.DriftReport {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	report := &deploymentstore.DriftReport{
		DetectedAt: time.Now().UTC().Format(time.RFC3339),
	}

	clientset := s.k8sClient.Clientset()

	// --- Workloads ---
	expectedWorkloadNames := map[string]bool{}
	for _, wl := range workloads {
		expectedWorkloadNames[wl.Name] = true
		item := deploymentstore.DriftResourceItem{
			Name:     wl.Name,
			Type:     wl.WorkloadType,
			Expected: map[string]string{"Image": wl.Image, "Replicas": fmt.Sprintf("%d", wl.Replicas)},
		}

		switch wl.WorkloadType {
		case "deployment":
			actual, err := clientset.AppsV1().Deployments(namespace).Get(ctx, wl.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					item.Status = "missing"
					item.Actual = map[string]string{}
				} else {
					continue
				}
			} else {
				actualReplicas := int32(1)
				if actual.Spec.Replicas != nil {
					actualReplicas = *actual.Spec.Replicas
				}
				actualImage := ""
				if len(actual.Spec.Template.Spec.Containers) > 0 {
					actualImage = actual.Spec.Template.Spec.Containers[0].Image
				}
				item.Actual = map[string]string{
					"Image":    actualImage,
					"Replicas": fmt.Sprintf("%d/%d", actual.Status.ReadyReplicas, actualReplicas),
				}
				if int(actualReplicas) != wl.Replicas || actualImage != wl.Image {
					item.Status = "drift"
				} else {
					item.Status = "match"
				}
			}

		case "statefulset":
			actual, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, wl.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					item.Status = "missing"
					item.Actual = map[string]string{}
				} else {
					continue
				}
			} else {
				actualReplicas := int32(1)
				if actual.Spec.Replicas != nil {
					actualReplicas = *actual.Spec.Replicas
				}
				actualImage := ""
				if len(actual.Spec.Template.Spec.Containers) > 0 {
					actualImage = actual.Spec.Template.Spec.Containers[0].Image
				}
				item.Actual = map[string]string{
					"Image":    actualImage,
					"Replicas": fmt.Sprintf("%d/%d", actual.Status.ReadyReplicas, actualReplicas),
				}
				if int(actualReplicas) != wl.Replicas || actualImage != wl.Image {
					item.Status = "drift"
				} else {
					item.Status = "match"
				}
			}

		case "cronjob":
			actual, err := clientset.BatchV1().CronJobs(namespace).Get(ctx, wl.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					item.Status = "missing"
					item.Actual = map[string]string{}
				} else {
					continue
				}
			} else {
				item.Expected = map[string]string{}
				if wl.TriggerSchedule != nil {
					item.Expected["Schedule"] = *wl.TriggerSchedule
				}
				item.Actual = map[string]string{"Schedule": actual.Spec.Schedule}
				if wl.TriggerSchedule != nil && actual.Spec.Schedule != *wl.TriggerSchedule {
					item.Status = "drift"
				} else {
					item.Status = "match"
				}
			}

		default:
			continue
		}

		report.Workloads = append(report.Workloads, item)
	}

	// --- Services ---
	checked := map[string]bool{}
	svcPortsByName := map[string]string{}
	for _, svc := range services {
		svcName := svc.WorkloadName
		if svcName == "" {
			svcName = svc.Name
		}
		if existing, ok := svcPortsByName[svcName]; ok {
			svcPortsByName[svcName] = existing + ", " + fmt.Sprintf("%d", svc.Port)
		} else {
			svcPortsByName[svcName] = fmt.Sprintf("%d", svc.Port)
		}
	}

	for _, svc := range services {
		svcName := svc.WorkloadName
		if svcName == "" {
			svcName = svc.Name
		}
		if checked[svcName] {
			continue
		}
		checked[svcName] = true

		item := deploymentstore.DriftResourceItem{
			Name:     svcName,
			Type:     "service",
			Expected: map[string]string{"Ports": svcPortsByName[svcName]},
		}

		_, err := clientset.CoreV1().Services(namespace).Get(ctx, svcName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				item.Status = "missing"
				item.Actual = map[string]string{}
			} else {
				continue
			}
		} else {
			item.Actual = map[string]string{"Ports": svcPortsByName[svcName]}
			item.Status = "match"
		}
		report.Services = append(report.Services, item)
	}

	// --- Ingresses ---
	for _, ing := range ingresses {
		item := deploymentstore.DriftResourceItem{
			Name: ing.Hostname,
			Type: "ingress",
			Expected: map[string]string{
				"Hostname": ing.Hostname,
				"Path":     ing.Path,
				"Service":  svcNameByID[ing.ServiceID],
			},
		}

		found := false
		if liveIngresses, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{}); err == nil {
			for _, li := range liveIngresses.Items {
				for _, rule := range li.Spec.Rules {
					if rule.Host == ing.Hostname {
						found = true
						paths := ""
						backend := ""
						if rule.HTTP != nil {
							for _, p := range rule.HTTP.Paths {
								paths = p.Path
								if p.Backend.Service != nil {
									backend = fmt.Sprintf("%s:%d", p.Backend.Service.Name, p.Backend.Service.Port.Number)
								}
							}
						}
						item.Actual = map[string]string{
							"Hostname": rule.Host,
							"Path":     paths,
							"Service":  backend,
						}
						item.Status = "match"
						break
					}
				}
				if found {
					break
				}
			}
		}
		if !found {
			item.Status = "missing"
			item.Actual = map[string]string{}
		}
		report.Ingresses = append(report.Ingresses, item)
	}

	// Compute summary
	allItems := append(append(report.Workloads, report.Services...), report.Ingresses...)
	report.Summary.Total = len(allItems)
	for _, item := range allItems {
		switch item.Status {
		case "match":
			report.Summary.Match++
		case "missing":
			report.Summary.Missing++
		case "extra":
			report.Summary.Extra++
		case "drift":
			report.Summary.Drift++
		}
	}

	return report
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
	return proto
}
