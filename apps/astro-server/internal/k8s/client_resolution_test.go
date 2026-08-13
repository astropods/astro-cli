package k8s

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/smithy-go"
)

type stubAPIError struct {
	code    string
	message string
}

func (e stubAPIError) Error() string                 { return e.message }
func (e stubAPIError) ErrorCode() string             { return e.code }
func (e stubAPIError) ErrorMessage() string          { return e.message }
func (e stubAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultUnknown }

func TestIsPermanentClientResolutionError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not found", ErrClusterNotFound, true},
		{"wrapped not found", fmt.Errorf("registry.Get: %w", ErrClusterNotFound), true},
		{"access denied api", stubAPIError{code: "AccessDeniedException", message: "denied"}, true},
		{"resource not found api", stubAPIError{code: "ResourceNotFoundException", message: "gone"}, true},
		{"throttling api", stubAPIError{code: "ThrottlingException", message: "slow down"}, false},
		{"service unavailable api", stubAPIError{code: "ServiceUnavailable", message: "try again"}, false},
		{"describe access denied string", fmt.Errorf(`failed to describe EKS cluster "x": AccessDeniedException`), true},
		{"timeout string", errors.New("context deadline exceeded"), false},
		{"unknown", errors.New("something unexpected"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPermanentClientResolutionError(tt.err); got != tt.want {
				t.Errorf("IsPermanentClientResolutionError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestPublicClusterHealthDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"not found", ErrClusterNotFound, "cluster is not registered"},
		{"access denied", errors.New(`AccessDeniedException: arn:aws:iam::123:role/foo`), "unable to authenticate to cluster"},
		{"connection refused", errors.New("connection refused"), "unable to connect to cluster"},
		{"generic", errors.New("unexpected internal failure at https://10.0.0.1"), "cluster health check failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := PublicClusterHealthDetail(tt.err); got != tt.want {
				t.Errorf("PublicClusterHealthDetail() = %q, want %q", got, tt.want)
			}
		})
	}
}
