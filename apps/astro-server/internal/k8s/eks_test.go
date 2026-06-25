package k8s

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// Reproduces the EKS 401-retry bug: the first attempt consumes the request
// body, so a naive req.Clone on retry would re-send Content-Length bytes of
// nothing ("request declared a Content-Length of N but only wrote 0 bytes").
// cloneRequestForRetry must rewind the body via GetBody.
func TestCloneRequestForRetry_RewindsConsumedBody(t *testing.T) {
	const payload = "namespace-create-body"
	// http.NewRequest sets GetBody automatically for a strings.Reader body.
	req, err := http.NewRequest(http.MethodPost, "https://eks.example.com/api/v1/namespaces", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	// Simulate the first attempt fully consuming the body.
	if _, err := io.ReadAll(req.Body); err != nil {
		t.Fatalf("drain body: %v", err)
	}

	clone, ok := cloneRequestForRetry(req)
	if !ok {
		t.Fatal("expected retry clone to be allowed")
	}
	got, err := io.ReadAll(clone.Body)
	if err != nil {
		t.Fatalf("read clone body: %v", err)
	}
	if string(got) != payload {
		t.Errorf("retry body = %q, want %q (body was not rewound)", got, payload)
	}
	if clone.ContentLength != int64(len(payload)) {
		t.Errorf("retry ContentLength = %d, want %d", clone.ContentLength, len(payload))
	}
}

// A bodyless request (GET) is always safe to retry.
func TestCloneRequestForRetry_NoBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://eks.example.com/api/v1/namespaces", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if _, ok := cloneRequestForRetry(req); !ok {
		t.Fatal("expected bodyless request to be retryable")
	}
}

// A request with a body but no GetBody can't be rewound; the caller must not
// retry (better to surface the 401 than send a torn 0-byte request).
func TestCloneRequestForRetry_UnrewindableBodyNotRetried(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://eks.example.com/api/v1/namespaces", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.GetBody = nil // e.g. a streaming body that can't be replayed

	if _, ok := cloneRequestForRetry(req); ok {
		t.Fatal("expected retry to be refused when the body cannot be rewound")
	}
}
