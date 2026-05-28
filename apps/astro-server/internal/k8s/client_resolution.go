package k8s

import (
	"errors"
	"strings"

	"github.com/aws/smithy-go"
)

// IsPermanentClientResolutionError reports whether a registry.Get or clusterClient
// failure is unlikely to succeed on retry (cluster gone/disabled, IAM deny).
// Unclassified errors are treated as transient so undeploy workers retry rather
// than marking the deployment undeployed while K8s resources remain.
func IsPermanentClientResolutionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrClusterNotFound) || errors.Is(err, ErrClusterDisabled) {
		return true
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccessDeniedException", "UnauthorizedException", "ResourceNotFoundException",
			"InvalidClientTokenId", "UnrecognizedClientException":
			return true
		case "ThrottlingException", "TooManyRequestsException", "RequestLimitExceeded",
			"ServiceUnavailable", "InternalServerException":
			return false
		}
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "AccessDenied"),
		strings.Contains(errStr, "Unauthorized"),
		strings.Contains(errStr, "ResourceNotFound"),
		strings.Contains(errStr, "InvalidIdentityToken"),
		strings.Contains(errStr, "AssumeRoleWithWebIdentity"):
		return true
	case strings.Contains(errStr, "Throttling"),
		strings.Contains(errStr, "RequestLimitExceeded"),
		strings.Contains(errStr, "timeout"),
		strings.Contains(errStr, "deadline exceeded"),
		strings.Contains(errStr, "connection refused"):
		return false
	default:
		return false
	}
}

const (
	publicHealthDetailConnect       = "unable to connect to cluster"
	publicHealthDetailAuth          = "unable to authenticate to cluster"
	publicHealthDetailNotRegistered = "cluster is not registered"
	publicHealthDetailDisabled      = "cluster is disabled"
	publicHealthDetailFailed        = "cluster health check failed"
)

// PublicClusterHealthDetail returns a safe, generic message for API consumers.
// Full errors must be logged server-side only.
func PublicClusterHealthDetail(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrClusterNotFound):
		return publicHealthDetailNotRegistered
	case errors.Is(err, ErrClusterDisabled):
		return publicHealthDetailDisabled
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "AccessDenied"),
		strings.Contains(errStr, "Unauthorized"),
		strings.Contains(errStr, "InvalidIdentityToken"),
		strings.Contains(errStr, "AssumeRoleWithWebIdentity"),
		strings.Contains(errStr, "forbidden"):
		return publicHealthDetailAuth
	case strings.Contains(errStr, "connection refused"),
		strings.Contains(errStr, "no such host"),
		strings.Contains(errStr, "timeout"),
		strings.Contains(errStr, "deadline exceeded"),
		strings.Contains(errStr, "cannot reach"):
		return publicHealthDetailConnect
	default:
		return publicHealthDetailFailed
	}
}
