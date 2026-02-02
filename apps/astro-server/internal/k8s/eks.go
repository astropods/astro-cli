package k8s

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	clusterIDHeader = "x-k8s-aws-id"
	tokenPrefix     = "k8s-aws-v1."
	tokenExpiry     = 14 * time.Minute // Tokens valid for 15min, refresh at 14min
)

// EKSClientConfig holds configuration for EKS client
type EKSClientConfig struct {
	ClusterName     string
	ClusterEndpoint string
	Region          string
	Logger          *logger.Logger
}

// EKSClient wraps the Kubernetes clientset with EKS-specific auth
type EKSClient struct {
	clientset   *kubernetes.Clientset
	config      *rest.Config
	awsConfig   aws.Config
	clusterName string
	log         *logger.Logger
}

// NewEKSClient creates a new Kubernetes client authenticated via EKS/IRSA
func NewEKSClient(ctx context.Context, cfg EKSClientConfig) (*EKSClient, error) {
	log := cfg.Logger

	if cfg.ClusterName == "" {
		return nil, fmt.Errorf("EKS_CLUSTER_NAME is required")
	}
	if cfg.ClusterEndpoint == "" {
		return nil, fmt.Errorf("K8S_MASTER_URL is required")
	}

	// Load AWS config (IRSA credentials are loaded automatically)
	awsOpts := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		awsOpts = append(awsOpts, awsconfig.WithRegion(cfg.Region))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Validate AWS credentials
	creds, err := awsCfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS credentials: %w", err)
	}

	if log != nil {
		log.Debug("AWS credentials loaded",
			"source", creds.Source,
			"region", awsCfg.Region,
		)
	}

	// Fetch cluster CA from EKS API
	eksClient := eks.NewFromConfig(awsCfg)
	describeOutput, err := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: aws.String(cfg.ClusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe EKS cluster %q: %w", cfg.ClusterName, err)
	}
	if describeOutput.Cluster == nil {
		return nil, fmt.Errorf("EKS cluster %q not found", cfg.ClusterName)
	}

	caData, err := base64.StdEncoding.DecodeString(
		aws.ToString(describeOutput.Cluster.CertificateAuthority.Data),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to decode cluster CA: %w", err)
	}

	// Create token provider (handles token generation and caching)
	tokenProvider := &eksTokenProvider{
		awsConfig:   awsCfg,
		clusterName: cfg.ClusterName,
		log:         log,
	}

	// Generate initial token
	if err := tokenProvider.refresh(ctx); err != nil {
		return nil, fmt.Errorf("failed to generate initial EKS token: %w", err)
	}

	// Create rest config with token transport
	restConfig := &rest.Config{
		Host: cfg.ClusterEndpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caData,
		},
		WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
			return &eksTokenTransport{
				base:          rt,
				tokenProvider: tokenProvider,
			}
		},
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	if log != nil {
		log.Info("EKS client initialized",
			"cluster", cfg.ClusterName,
			"endpoint", cfg.ClusterEndpoint,
		)
	}

	return &EKSClient{
		clientset:   clientset,
		config:      restConfig,
		awsConfig:   awsCfg,
		clusterName: cfg.ClusterName,
		log:         log,
	}, nil
}

// Clientset returns the underlying Kubernetes clientset
func (c *EKSClient) Clientset() *kubernetes.Clientset {
	return c.clientset
}

// Config returns the REST config
func (c *EKSClient) Config() *rest.Config {
	return c.config
}

// CheckHealth verifies connectivity to the Kubernetes API server
func (c *EKSClient) CheckHealth() error {
	_, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return diagnoseEKSError(err)
	}
	return nil
}

// GetServerVersion returns the Kubernetes server version string
func (c *EKSClient) GetServerVersion() (string, error) {
	version, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return "", diagnoseEKSError(err)
	}
	return version.GitVersion, nil
}

// DiagnoseConnection returns diagnostic information about the EKS connection
func (c *EKSClient) DiagnoseConnection() map[string]string {
	diag := map[string]string{
		"cluster_name": c.clusterName,
		"aws_region":   c.awsConfig.Region,
	}

	if c.config != nil {
		diag["api_server"] = c.config.Host
	}

	if tokenFile := os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"); tokenFile != "" {
		diag["irsa_token_file"] = tokenFile
		if _, err := os.Stat(tokenFile); err != nil {
			diag["irsa_status"] = "token file not accessible"
		} else {
			diag["irsa_status"] = "configured"
		}
	}

	if roleArn := os.Getenv("AWS_ROLE_ARN"); roleArn != "" {
		diag["aws_role_arn"] = roleArn
	}

	if version, err := c.GetServerVersion(); err != nil {
		diag["connection"] = "failed: " + err.Error()
	} else {
		diag["connection"] = "ok"
		diag["server_version"] = version
	}

	return diag
}

// eksTokenProvider manages EKS token generation and caching
type eksTokenProvider struct {
	awsConfig   aws.Config
	clusterName string
	log         *logger.Logger

	mu        sync.RWMutex
	token     string
	expiresAt time.Time
}

// getToken returns a valid token, refreshing if necessary
func (p *eksTokenProvider) getToken(ctx context.Context) (string, error) {
	p.mu.RLock()
	if p.token != "" && time.Now().Before(p.expiresAt) {
		token := p.token
		p.mu.RUnlock()
		return token, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if p.token != "" && time.Now().Before(p.expiresAt) {
		return p.token, nil
	}

	return p.token, p.refreshLocked(ctx)
}

// refresh generates a new token (acquires lock)
func (p *eksTokenProvider) refresh(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.refreshLocked(ctx)
}

// refreshLocked generates a new token (caller must hold lock)
func (p *eksTokenProvider) refreshLocked(ctx context.Context) error {
	// Create STS presign client
	stsClient := sts.NewFromConfig(p.awsConfig)
	presignClient := sts.NewPresignClient(stsClient)

	// Presign GetCallerIdentity with cluster ID header (matches aws-iam-authenticator)
	presignedReq, err := presignClient.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, func(opts *sts.PresignOptions) {
		opts.ClientOptions = append(opts.ClientOptions, func(o *sts.Options) {
			o.APIOptions = append(o.APIOptions,
				// Add cluster ID header
				smithyhttp.SetHeaderValue(clusterIDHeader, p.clusterName),
				// Add X-Amz-Expires for compatibility
				smithyhttp.SetHeaderValue("X-Amz-Expires", "60"),
			)
		})
	})
	if err != nil {
		return fmt.Errorf("failed to presign GetCallerIdentity: %w", err)
	}

	p.token = tokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(presignedReq.URL))
	p.expiresAt = time.Now().Add(tokenExpiry)

	if p.log != nil {
		p.log.Debug("EKS token refreshed", "expires_in", tokenExpiry)
	}

	return nil
}

// eksTokenTransport injects EKS tokens and handles 401 refresh
type eksTokenTransport struct {
	base          http.RoundTripper
	tokenProvider *eksTokenProvider
}

func (t *eksTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Get current token
	token, err := t.tokenProvider.getToken(req.Context())
	if err != nil {
		return nil, fmt.Errorf("failed to get EKS token: %w", err)
	}

	// Clone request and set auth header
	reqCopy := req.Clone(req.Context())
	reqCopy.Header.Set("Authorization", "Bearer "+token)

	resp, err := t.base.RoundTrip(reqCopy)
	if err != nil {
		return resp, err
	}

	// On 401, refresh token and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		if t.tokenProvider.log != nil {
			t.tokenProvider.log.Debug("Got 401, refreshing EKS token", "path", req.URL.Path)
		}

		if refreshErr := t.tokenProvider.refresh(req.Context()); refreshErr != nil {
			if t.tokenProvider.log != nil {
				t.tokenProvider.log.Error("Failed to refresh EKS token", "error", refreshErr)
			}
			return resp, nil
		}

		newToken, _ := t.tokenProvider.getToken(req.Context())
		resp.Body.Close()

		retryCopy := req.Clone(req.Context())
		retryCopy.Header.Set("Authorization", "Bearer "+newToken)
		return t.base.RoundTrip(retryCopy)
	}

	return resp, nil
}

// diagnoseEKSError provides helpful error messages for common EKS issues
func diagnoseEKSError(err error) error {
	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "ExpiredToken"):
		return fmt.Errorf("EKS token expired (should auto-refresh): %w", err)

	case strings.Contains(errStr, "InvalidIdentityToken"):
		return fmt.Errorf("invalid IRSA token - check service account annotation and IAM trust policy: %w", err)

	case strings.Contains(errStr, "AccessDenied"), strings.Contains(errStr, "Unauthorized"),
		strings.Contains(errStr, "forbidden"):
		return fmt.Errorf("access denied - ensure IAM role has EKS Access Entry on target cluster: %w", err)

	case strings.Contains(errStr, "AssumeRoleWithWebIdentity"):
		return fmt.Errorf("IRSA AssumeRole failed - check token file and IAM trust policy: %w", err)

	case strings.Contains(errStr, "certificate"), strings.Contains(errStr, "x509"):
		return fmt.Errorf("TLS error - cluster CA may be incorrect: %w", err)

	case strings.Contains(errStr, "connection refused"), strings.Contains(errStr, "no such host"):
		return fmt.Errorf("cannot reach EKS API - check endpoint URL and network connectivity: %w", err)

	case strings.Contains(errStr, "timeout"), strings.Contains(errStr, "deadline exceeded"):
		return fmt.Errorf("connection timeout - check network connectivity: %w", err)

	default:
		return fmt.Errorf("EKS API error: %w", err)
	}
}
