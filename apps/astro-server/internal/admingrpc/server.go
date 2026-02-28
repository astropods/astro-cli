package admingrpc

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"

	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/deploymentstore"
	"github.com/postman/astro/apps/astro-server/internal/k8s"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Server implements adminv1.AdminServiceServer.
type Server struct {
	adminv1.UnimplementedAdminServiceServer

	log         *logger.Logger
	deployStore *deploymentstore.Store
	k8sClient   k8s.ClusterClient
	db          *sql.DB
	awsRegion   string
	environment string
}

// New creates a new admin gRPC server.
func New(
	log *logger.Logger,
	deployStore *deploymentstore.Store,
	k8sClient k8s.ClusterClient,
	db *sql.DB,
	awsRegion string,
	environment string,
) *Server {
	return &Server{
		log:         log,
		deployStore: deployStore,
		k8sClient:   k8sClient,
		db:          db,
		awsRegion:   awsRegion,
		environment: environment,
	}
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

// ListImages returns container images from ECR with the tenant prefix.
func (s *Server) ListImages(ctx context.Context, _ *adminv1.ListImagesRequest) (*adminv1.ListImagesResponse, error) {
	tenantPrefix := s.environment + "-tenant-"

	opts := []func(*config.LoadOptions) error{}
	if s.awsRegion != "" {
		opts = append(opts, config.WithRegion(s.awsRegion))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := ecr.NewFromConfig(cfg)
	images, err := listECRRepositories(ctx, client, tenantPrefix)
	if err != nil {
		return nil, fmt.Errorf("list ECR repositories: %w", err)
	}

	var result []*adminv1.ImageInfo
	for _, img := range images {
		result = append(result, &adminv1.ImageInfo{
			Repository: img.repository,
			Namespace:  img.namespace,
			Name:       img.name,
			Tags:       img.tags,
		})
	}

	return &adminv1.ListImagesResponse{
		Images: result,
		Count:  int32(len(result)), //nolint:gosec // bounded by registry response
	}, nil
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

// QueryDatabase executes a raw SQL query and returns columns and rows.
func (s *Server) QueryDatabase(ctx context.Context, req *adminv1.QueryDatabaseRequest) (*adminv1.QueryDatabaseResponse, error) {
	if req.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	rows, err := s.db.QueryContext(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	var resultRows []*adminv1.Row
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		valPtrs := make([]interface{}, len(cols))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}
		if err := rows.Scan(valPtrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		row := &adminv1.Row{Values: make([]string, len(cols))}
		for i, v := range vals {
			switch val := v.(type) {
			case nil:
				row.Values[i] = "NULL"
			case []byte:
				row.Values[i] = string(val)
			default:
				row.Values[i] = fmt.Sprintf("%v", val)
			}
		}
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return &adminv1.QueryDatabaseResponse{
		Columns: cols,
		Rows:    resultRows,
	}, nil
}

// GetSchema returns column type information for all public tables.
func (s *Server) GetSchema(ctx context.Context, _ *adminv1.GetSchemaRequest) (*adminv1.GetSchemaResponse, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT table_name, column_name, data_type FROM information_schema.columns WHERE table_schema = 'public' ORDER BY table_name, ordinal_position",
	)
	if err != nil {
		return nil, fmt.Errorf("query schema: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var cols []*adminv1.ColumnInfo
	for rows.Next() {
		var c adminv1.ColumnInfo
		if err := rows.Scan(&c.TableName, &c.ColumnName, &c.DataType); err != nil {
			return nil, fmt.Errorf("scan schema row: %w", err)
		}
		cols = append(cols, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("schema rows error: %w", err)
	}

	return &adminv1.GetSchemaResponse{Columns: cols}, nil
}

// ListAccounts returns all accounts with owner and member count.
func (s *Server) ListAccounts(ctx context.Context, _ *adminv1.ListAccountsRequest) (*adminv1.ListAccountsResponse, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			a.id,
			a.name,
			a.type,
			COALESCE((SELECT user_id FROM account_members WHERE account_id = a.id AND role = 'owner' LIMIT 1), '') AS owner_user_id,
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

// ecrImage is an internal type for ECR image data.
type ecrImage struct {
	repository string
	namespace  string
	name       string
	tags       []string
}

func listECRRepositories(ctx context.Context, client *ecr.Client, tenantPrefix string) ([]ecrImage, error) {
	var images []ecrImage
	var nextToken *string

	for {
		out, err := client.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{NextToken: nextToken})
		if err != nil {
			return nil, err
		}
		for _, repo := range out.Repositories {
			name := *repo.RepositoryName
			if !strings.HasPrefix(name, tenantPrefix) {
				continue
			}
			ns, imgName := parseRepoName(name, tenantPrefix)
			tags, _ := getRepoTags(ctx, client, name)
			images = append(images, ecrImage{
				repository: name,
				namespace:  ns,
				name:       imgName,
				tags:       tags,
			})
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return images, nil
}

func parseRepoName(repoName, tenantPrefix string) (ns, imgName string) {
	without := strings.TrimPrefix(repoName, tenantPrefix)
	parts := strings.SplitN(without, "/", 2)
	ns = parts[0]
	if len(parts) > 1 {
		imgName = parts[1]
	}
	return
}

func getRepoTags(ctx context.Context, client *ecr.Client, repoName string) ([]string, error) {
	var tags []string
	var nextToken *string
	for {
		out, err := client.ListImages(ctx, &ecr.ListImagesInput{
			RepositoryName: &repoName,
			NextToken:      nextToken,
		})
		if err != nil {
			return nil, err
		}
		for _, id := range out.ImageIds {
			if id.ImageTag != nil {
				tags = append(tags, *id.ImageTag)
			}
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return tags, nil
}
