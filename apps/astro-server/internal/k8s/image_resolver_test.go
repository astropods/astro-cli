package k8s

import (
	"testing"
)

func TestImageResolver_ResolveImage(t *testing.T) {
	tests := []struct {
		name              string
		proxyRegistryHost string
		ecrRegistryURL    string
		inputImage        string
		expectedImage     string
		expectError       bool
	}{
		{
			name:              "resolve proxy image to ECR",
			proxyRegistryHost: "registry.odesdaz.com",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.odesdaz.com/user_01kggdgfrw46qcsnxeqbr1hr1z/engineering-assistant:latest",
			expectedImage:     "123456789.dkr.ecr.us-east-1.amazonaws.com/tenant-user_01kggdgfrw46qcsnxeqbr1hr1z/engineering-assistant:latest",
			expectError:       false,
		},
		{
			name:              "resolve proxy image with version tag",
			proxyRegistryHost: "registry.odesdaz.com",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.odesdaz.com/user_abc123/myagent:v1.0.0",
			expectedImage:     "123456789.dkr.ecr.us-east-1.amazonaws.com/tenant-user_abc123/myagent:v1.0.0",
			expectError:       false,
		},
		{
			name:              "pass through non-proxy image",
			proxyRegistryHost: "registry.odesdaz.com",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "docker.io/nginx:latest",
			expectedImage:     "docker.io/nginx:latest",
			expectError:       false,
		},
		{
			name:              "pass through when no proxy host configured",
			proxyRegistryHost: "",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.odesdaz.com/user_xxx/image:latest",
			expectedImage:     "registry.odesdaz.com/user_xxx/image:latest",
			expectError:       false,
		},
		{
			name:              "error on invalid image format",
			proxyRegistryHost: "registry.odesdaz.com",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.odesdaz.com/invalid",
			expectedImage:     "",
			expectError:       true,
		},
		{
			name:              "resolve image with digest",
			proxyRegistryHost: "registry.odesdaz.com",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.odesdaz.com/user_123/image@sha256:abcdef123456",
			expectedImage:     "123456789.dkr.ecr.us-east-1.amazonaws.com/tenant-user_123/image@sha256:abcdef123456",
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewImageResolver(tt.proxyRegistryHost, tt.ecrRegistryURL)
			result, err := resolver.ResolveImage(tt.inputImage)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expectedImage {
				t.Errorf("expected %s, got %s", tt.expectedImage, result)
			}
		})
	}
}
