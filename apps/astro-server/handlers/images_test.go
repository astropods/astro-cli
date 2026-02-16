package handlers

import (
	"testing"
)

func TestParseRepositoryName(t *testing.T) {
	tenantPrefix := "prod-tenant-"

	tests := []struct {
		name              string
		repoName          string
		expectedNamespace string
		expectedImage     string
	}{
		{
			name:              "with image name",
			repoName:          "prod-tenant-user123/myapp",
			expectedNamespace: "user123",
			expectedImage:     "myapp",
		},
		{
			name:              "with nested image name",
			repoName:          "prod-tenant-user123/myapp-model-gpt4",
			expectedNamespace: "user123",
			expectedImage:     "myapp-model-gpt4",
		},
		{
			name:              "namespace only",
			repoName:          "prod-tenant-user123",
			expectedNamespace: "user123",
			expectedImage:     "",
		},
		{
			name:              "org namespace with image",
			repoName:          "prod-tenant-org456/chatbot",
			expectedNamespace: "org456",
			expectedImage:     "chatbot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace, imageName := parseRepositoryName(tt.repoName, tenantPrefix)

			if namespace != tt.expectedNamespace {
				t.Errorf("expected namespace %q, got %q", tt.expectedNamespace, namespace)
			}

			if imageName != tt.expectedImage {
				t.Errorf("expected image %q, got %q", tt.expectedImage, imageName)
			}
		})
	}
}
