package k8s

import (
	"fmt"
	"net/url"
	"strings"
)

// ImageResolver resolves proxy registry image references to ECR paths
type ImageResolver struct {
	proxyRegistryHost string // e.g., "registry.odesdaz.com"
	ecrRegistryURL    string // e.g., "https://123456789.dkr.ecr.us-east-1.amazonaws.com"
}

// NewImageResolver creates a new image resolver
func NewImageResolver(proxyRegistryHost, ecrRegistryURL string) *ImageResolver {
	return &ImageResolver{
		proxyRegistryHost: proxyRegistryHost,
		ecrRegistryURL:    ecrRegistryURL,
	}
}

// ResolveImage resolves a proxy registry image reference to an ECR path
// Input:  registry.odesdaz.com/user_xxx/image:tag
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

	// Parse ECR registry URL to get the host
	ecrURL, err := url.Parse(r.ecrRegistryURL)
	if err != nil {
		return "", fmt.Errorf("invalid ECR registry URL: %w", err)
	}

	// Build final ECR image path
	ecrImage := fmt.Sprintf("%s/%s/%s", ecrURL.Host, tenantNamespace, imageAndTag)

	return ecrImage, nil
}
