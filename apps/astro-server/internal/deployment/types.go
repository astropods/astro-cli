package deployment

import "time"

// DeployRequest represents a deployment request
type DeployRequest struct {
	Name            string            `json:"name" binding:"required"`
	Version         string            `json:"version" binding:"required"`
	UserCredentials map[string]string `json:"user_credentials,omitempty"`
}

// DeployResponse represents a deployment response
type DeployResponse struct {
	Status           string             `json:"status"`
	Name             string             `json:"name"`
	Version          string             `json:"version"`
	K8sNamespace     string             `json:"k8s_namespace"`
	DeployedAt       time.Time          `json:"deployed_at"`
	Resources        []ResourceStatus   `json:"resources"`
	ServiceEndpoints []ServiceEndpoint  `json:"service_endpoints,omitempty"`
	Errors           []DeploymentError  `json:"errors,omitempty"`
}

// ResourceStatus represents the status of a deployed resource
type ResourceStatus struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// ServiceEndpoint represents an exposed service endpoint
type ServiceEndpoint struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URL  string `json:"url"`
	Port int32  `json:"port,omitempty"`
}

// DeploymentError represents an error during deployment
type DeploymentError struct {
	Resource string `json:"resource"`
	Kind     string `json:"kind"`
	Error    string `json:"error"`
}

// ValidationError represents a spec validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationResult holds validation results
type ValidationResult struct {
	Valid              bool              `json:"valid"`
	Errors             []ValidationError `json:"errors,omitempty"`
	MissingCredentials []string          `json:"missing_credentials,omitempty"`
}

// UndeployRequest represents an undeploy request
type UndeployRequest struct {
	Name    string `json:"name" binding:"required"`
	Version string `json:"version" binding:"required"`
}

// UndeployResponse represents an undeploy response
type UndeployResponse struct {
	Status       string            `json:"status"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	K8sNamespace string            `json:"k8s_namespace"`
	UndeployedAt time.Time         `json:"undeployed_at"`
	Resources    []ResourceStatus  `json:"resources"`
	Errors       []DeploymentError `json:"errors,omitempty"`
}
