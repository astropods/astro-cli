package registry

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
)

// ECRAuthProvider provides authentication tokens for ECR registry access.
// Tokens are cached since ECR tokens are valid for 12 hours.
type ECRAuthProvider struct {
	region string

	mu          sync.RWMutex
	cachedToken string
	expiresAt   time.Time
}

// NewECRAuthProvider creates a new ECR auth provider for the given region.
func NewECRAuthProvider(region string) *ECRAuthProvider {
	return &ECRAuthProvider{
		region: region,
	}
}

// GetAuthToken returns a base64-encoded Basic auth token for ECR.
// The token is cached and refreshed when it expires or is close to expiring.
func (p *ECRAuthProvider) GetAuthToken(ctx context.Context) (string, error) {
	p.mu.RLock()
	if p.cachedToken != "" && time.Now().Add(30*time.Minute).Before(p.expiresAt) {
		token := p.cachedToken
		p.mu.RUnlock()
		return token, nil
	}
	p.mu.RUnlock()

	return p.refreshToken(ctx)
}

// refreshToken fetches a new authorization token from ECR.
func (p *ECRAuthProvider) refreshToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if p.cachedToken != "" && time.Now().Add(30*time.Minute).Before(p.expiresAt) {
		return p.cachedToken, nil
	}

	// Load AWS config
	opts := []func(*config.LoadOptions) error{}
	if p.region != "" {
		opts = append(opts, config.WithRegion(p.region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create ECR client
	client := ecr.NewFromConfig(cfg)

	// Get authorization token
	result, err := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get ECR authorization token: %w", err)
	}

	if len(result.AuthorizationData) == 0 {
		return "", fmt.Errorf("no authorization data returned from ECR")
	}

	authData := result.AuthorizationData[0]
	if authData.AuthorizationToken == nil {
		return "", fmt.Errorf("authorization token is nil")
	}

	// ECR returns a base64-encoded "AWS:<token>" string
	// We need to decode it and re-encode for Basic auth
	decoded, err := base64.StdEncoding.DecodeString(*authData.AuthorizationToken)
	if err != nil {
		return "", fmt.Errorf("failed to decode ECR token: %w", err)
	}

	// The decoded token is "AWS:<password>", which is already in Basic auth format
	// We can use it directly as the Basic auth value
	p.cachedToken = base64.StdEncoding.EncodeToString(decoded)

	if authData.ExpiresAt != nil {
		p.expiresAt = *authData.ExpiresAt
	} else {
		// Default to 12 hours if not provided
		p.expiresAt = time.Now().Add(12 * time.Hour)
	}

	return p.cachedToken, nil
}

// GetUsername returns the username for ECR authentication (always "AWS").
func (p *ECRAuthProvider) GetUsername() string {
	return "AWS"
}

// GetPassword returns the password portion of the ECR auth token.
func (p *ECRAuthProvider) GetPassword(ctx context.Context) (string, error) {
	token, err := p.GetAuthToken(ctx)
	if err != nil {
		return "", err
	}

	// Decode the base64 token to get "AWS:password"
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("failed to decode token: %w", err)
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid token format")
	}

	return parts[1], nil
}

// ClearCache clears the cached token, forcing a refresh on next request.
func (p *ECRAuthProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cachedToken = ""
	p.expiresAt = time.Time{}
}

// EnsureRepository ensures an ECR repository exists, creating it if necessary.
// The repositoryName should include the full path (e.g., "tenant-user123/myapp").
func (p *ECRAuthProvider) CreateRepository(ctx context.Context, repositoryName string) error {
	// Load AWS config
	opts := []func(*config.LoadOptions) error{}
	if p.region != "" {
		opts = append(opts, config.WithRegion(p.region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := ecr.NewFromConfig(cfg)

	// Check if repository already exists
	_, err = client.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
		RepositoryNames: []string{repositoryName},
	})
	if err == nil {
		return nil // Repository already exists
	}

	// Create the repository
	_, err = client.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: &repositoryName,
	})
	if err != nil {
		// Handle race condition where repo was created between check and create
		if strings.Contains(err.Error(), "RepositoryAlreadyExistsException") {
			return nil
		}
		return fmt.Errorf("failed to create ECR repository: %w", err)
	}

	return nil
}
