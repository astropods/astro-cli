package admingrpc

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"

	"github.com/postman/astro/apps/astro-server/internal/deploymentstore"
	"github.com/postman/astro/apps/astro-server/internal/k8s"
	"github.com/postman/astro/apps/astro-server/internal/logger"
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
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Namespace:   namespace,
		Deployments: []*adminv1.K8sDeploymentInfo{},
		Pods:        []*adminv1.K8sPodInfo{},
		Services:    []*adminv1.K8sServiceInfo{},
		Summary:     &adminv1.ClusterSummary{},
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
			resp.Pods = append(resp.Pods, &adminv1.K8sPodInfo{
				Name:      p.Name,
				Namespace: p.Namespace,
				Phase:     string(p.Status.Phase),
				NodeName:  p.Spec.NodeName,
				PodIP:     p.Status.PodIP,
				Labels:    p.Labels,
				CreatedAt: p.CreationTimestamp.Format(time.RFC3339),
			})
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

	// Summary
	resp.Summary = &adminv1.ClusterSummary{
		TotalDeployments: int32(len(resp.Deployments)), //nolint:gosec // bounded by cluster size
		TotalPods:        int32(len(resp.Pods)),        //nolint:gosec // bounded by cluster size
		TotalServices:    int32(len(resp.Services)),    //nolint:gosec // bounded by cluster size
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
