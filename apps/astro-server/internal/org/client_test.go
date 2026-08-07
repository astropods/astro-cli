package org

import (
	"errors"
	"net/http"
	"testing"

	"github.com/workos/workos-go/v6/pkg/workos_errors"
)

func TestClassifyOrganizationError(t *testing.T) {
	notFound := workos_errors.HTTPError{Code: http.StatusNotFound}
	if err := classifyOrganizationError(notFound); !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("classifyOrganizationError() = %v, want ErrOrganizationNotFound", err)
	}

	serverErr := workos_errors.HTTPError{Code: http.StatusInternalServerError}
	if err := classifyOrganizationError(serverErr); errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("classifyOrganizationError() = %v, did not want ErrOrganizationNotFound", err)
	}
}
