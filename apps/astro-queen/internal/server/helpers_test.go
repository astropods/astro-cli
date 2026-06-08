package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWriteGRPCErrMapsStatusCode(t *testing.T) {
	w := httptest.NewRecorder()

	writeGRPCErr(w, status.Error(codes.NotFound, "job 99 not found"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "job 99 not found") {
		t.Fatalf("body = %q, want not found message", w.Body.String())
	}
}

func TestWriteGRPCErrFallsBackToBadGateway(t *testing.T) {
	w := httptest.NewRecorder()

	writeGRPCErr(w, errors.New("upstream failed"))

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
}

func TestWriteGRPCErrKeepsOtherGRPCErrorsAsBadGateway(t *testing.T) {
	w := httptest.NewRecorder()

	writeGRPCErr(w, status.Error(codes.Internal, "db down"))

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
}
