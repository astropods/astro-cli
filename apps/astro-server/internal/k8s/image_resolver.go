package k8s

import (
	"fmt"
	"net/url"
	"strings"
)

// ImageResolver resolves proxy registry image references to ECR paths
type ImageResolver struct {
	proxyRegistryHost string // e.g., "registry.example.com"
	ecrRegistryURL    string // e.g., "123456789.dkr.ecr.us-east-1.amazonaws.com" (scheme optional)
}

// NewImageResolver creates a new image resolver
func NewImageResolver(proxyRegistryHost, ecrRegistryURL string) *ImageResolver {
	return &ImageResolver{
		proxyRegistryHost: proxyRegistryHost,
		ecrRegistryURL:    ecrRegistryURL,
	}
}

// ResolveImage resolves a proxy registry image reference to an ECR path
// Input:  registry.example.com/user_xxx/image:tag
// Output: 123456789.dkr.ecr.us-east-1.amazonaws.com/tenant-user_xxx/image:tag
func (r *ImageResolver) ResolveImage(image string) (string, error) {
	// If no proxy host configured, return image as-is
	if r.proxyRegistryHost == "" {
		return image, nil
	}

	// Check if image starts with proxy registry host
	if !strings.HasPrefix(image, r.proxyRegistryHost+"/") {
		// Not a proxy image, return as-is
		return image, nil
	}

	// Remove proxy host prefix
	pathWithTag := strings.TrimPrefix(image, r.proxyRegistryHost+"/")

	// Split into namespace/rest
	parts := strings.SplitN(pathWithTag, "/", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid image format: %s", image)
	}

	namespace := parts[0]
	imageAndTag := parts[1]

	// Add tenant- prefix to namespace (matching registry proxy behavior)
	tenantNamespace := "tenant-" + namespace

	// Get the registry host from the ECR URL
	// The URL may or may not include a scheme - handle both cases
	registryHost := r.ecrRegistryURL
	if registryHost == "" {
		return "", fmt.Errorf("ECR registry URL is empty")
	}

	// If it looks like a URL with scheme, parse it to extract the host
	if strings.Contains(registryHost, "://") {
		ecrURL, err := url.Parse(registryHost)
		if err != nil {
			return "", fmt.Errorf("invalid ECR registry URL: %w", err)
		}
		if ecrURL.Host == "" {
			return "", fmt.Errorf("invalid ECR registry URL %q: could not extract host", r.ecrRegistryURL)
		}
		registryHost = ecrURL.Host
	}

	// Build final ECR image path
	ecrImage := fmt.Sprintf("%s/%s/%s", registryHost, tenantNamespace, imageAndTag)

	return ecrImage, nil
}
