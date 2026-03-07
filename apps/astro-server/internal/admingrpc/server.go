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
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	connectv1 "github.com/astropods/astro/packages/astro-proto/connect/v1"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Server implements adminv1.AdminServiceServer.
// CommandDispatcher sends commands to connected devices.
type CommandDispatcher interface {
	SendCommand(ctx context.Context, deviceID string, cmd *connectv1.ShellCommand) (*connectv1.CommandResult, error)
}

// TokenRefresher exchanges a refresh token for a new access token.
type TokenRefresher interface {
	AuthenticateWithRefreshToken(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, err error)
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
	tokenRefresher TokenRefresher
	workosClientID string
}

// SetHTTPHandler sets the HTTP handler (gin router) for proxying HTTP requests.
func (s *Server) SetHTTPHandler(h http.Handler) {
	s.httpHandler = h
}

// SetTokenRefresher sets the token refresher for GetAuthToken.
func (s *Server) SetTokenRefresher(t TokenRefresher) {
	s.tokenRefresher = t
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
) *Server {
	return &Server{
		log:          log,
		deployStore:  deployStore,
		k8sClient:    k8sClient,
		db:           db,
		openMeterURL: strings.TrimRight(openMeterURL, "/"),
	}
}

// SetCommandDispatcher sets the dispatcher for sending commands to connected devices.
func (s *Server) SetCommandDispatcher(d CommandDispatcher) {
	s.cmdDispatch = d
}

// ListDeployments returns all active deployments across all accounts.
func (s *Server) ListDeployments(_ context.Context, req *adminv1.ListDeploymentsRequest) (*adminv1.ListDeploymentsResponse, error) {
	dbDeps, err := s.deployStore.ListAllActive()
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	var results []*adminv1.AdminDeployment
	for _, d := range dbDeps {
		if req.Namespace != "" && d.Namespace != req.Namespace {
			continue
		}
		results = append(results, &adminv1.AdminDeployment{
			Name:        d.AgentName,
			BuildID:     d.BuildID,
			Namespace:   d.Namespace,
			Status:      d.Status,
			CreatedAt:   d.DeployedAt.Format(time.RFC3339),
			AccountName: d.AccountName,
			Components:  []string{},
		})
	}

	return &adminv1.ListDeploymentsResponse{
		Deployments: results,
		Count:       int32(len(results)), //nolint:gosec // bounded by cluster size
	}, nil
}

// GetDeployment returns a single deployment with its spec and cluster status.
func (s *Server) GetDeployment(ctx context.Context, req *adminv1.GetDeploymentRequest) (*adminv1.GetDeploymentResponse, error) {
	if req.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	dbDeps, err := s.deployStore.ListAllActive()
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	var found *adminv1.AdminDeployment
	var specJSON string
	for _, d := range dbDeps {
		if d.Namespace == req.Namespace {
			found = &adminv1.AdminDeployment{
				Name:        d.AgentName,
				BuildID:     d.BuildID,
				Namespace:   d.Namespace,
				Status:      d.Status,
				CreatedAt:   d.DeployedAt.Format(time.RFC3339),
				AccountName: d.AccountName,
				Components:  []string{},
			}
			specJSON = d.DeploymentSpecJSON
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("deployment not found for namespace %q", req.Namespace)
	}

	clusterStatus, err := s.GetClusterStatus(ctx, &adminv1.GetClusterStatusRequest{Namespace: req.Namespace})
	if err != nil {
		s.log.Warn("Failed to get cluster status for deployment detail", "namespace", req.Namespace, "error", err)
		clusterStatus = &adminv1.GetClusterStatusResponse{}
	}

	return &adminv1.GetDeploymentResponse{
		Deployment:    found,
		SpecJSON:      specJSON,
		ClusterStatus: clusterStatus,
	}, nil
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

// DeleteDeployment deletes all k8s resources for a namespace.
func (s *Server) DeleteDeployment(ctx context.Context, req *adminv1.DeleteDeploymentRequest) (*adminv1.DeleteDeploymentResponse, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}
	if req.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	// Find the deployment record to get agent name and account ID
	dbDeps, err := s.deployStore.ListAllActive()
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	var agentName, accountID string
	for _, d := range dbDeps {
		if d.Namespace == req.Namespace {
			agentName = d.AgentName
			accountID = d.AccountID
			break
		}
	}

	// Delete k8s resources
	deleter := k8s.NewDeleter(s.k8sClient, req.Namespace)
	if _, err := deleter.Delete(ctx, agentName, ""); err != nil {
		return nil, fmt.Errorf("delete k8s resources: %w", err)
	}

	// Mark as undeployed in the DB if we found a record
	if agentName != "" && accountID != "" {
		if err := s.deployStore.MarkUndeployed(accountID, agentName); err != nil {
			s.log.Warn("Failed to mark deployment as undeployed", "namespace", req.Namespace, "error", err)
		}
	}

	return &adminv1.DeleteDeploymentResponse{Status: "deleted"}, nil
}

// RestartDeployment deletes a pod so Kubernetes recreates it.
func (s *Server) RestartDeployment(ctx context.Context, req *adminv1.RestartDeploymentRequest) (*adminv1.RestartDeploymentResponse, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}
	if req.Namespace == "" || req.Pod == "" {
		return nil, fmt.Errorf("namespace and pod are required")
	}

	err := s.k8sClient.Clientset().CoreV1().Pods(req.Namespace).Delete(ctx, req.Pod, metav1.DeleteOptions{})
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
	if req.Namespace == "" || req.Pod == "" {
		return nil, fmt.Errorf("namespace and pod are required")
	}

	tailLines := int64(req.TailLines)
	if tailLines <= 0 {
		tailLines = 100
	}

	stream, err := s.k8sClient.Clientset().CoreV1().Pods(req.Namespace).GetLogs(req.Pod, &corev1.PodLogOptions{
		TailLines: &tailLines,
	}).Stream(ctx)
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
	if req.Namespace == "" || req.Pod == "" {
		return nil, fmt.Errorf("namespace and pod are required")
	}

	pod, err := s.k8sClient.Clientset().CoreV1().Pods(req.Namespace).Get(ctx, req.Pod, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod: %w", err)
	}

	var containers []*adminv1.ContainerEnv
	for _, c := range pod.Spec.Containers {
		ce := &adminv1.ContainerEnv{Container: c.Name}
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

// GetAuthToken exchanges a refresh token for a fresh access token via WorkOS.
func (s *Server) GetAuthToken(ctx context.Context, req *adminv1.GetAuthTokenRequest) (*adminv1.GetAuthTokenResponse, error) {
	if s.tokenRefresher == nil {
		return nil, fmt.Errorf("token refresher not configured")
	}
	if req.RefreshToken == "" {
		return nil, fmt.Errorf("refresh_token is required")
	}

	accessToken, refreshToken, err := s.tokenRefresher.AuthenticateWithRefreshToken(ctx, req.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}

	return &adminv1.GetAuthTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// ProxyHTTP dispatches an HTTP request to the gin router running in the same process.
func (s *Server) ProxyHTTP(_ context.Context, req *adminv1.HTTPProxyRequest) (*adminv1.HTTPProxyResponse, error) {
	if s.httpHandler == nil {
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
	s.httpHandler.ServeHTTP(rec, httpReq)

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
