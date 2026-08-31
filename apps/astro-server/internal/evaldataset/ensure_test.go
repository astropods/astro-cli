package evaldataset

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore/datasetstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

func TestEnsureCreatesWhenMissing(t *testing.T) {
	srv, called := datasetstoretest.LangfuseStatusServer(t, http.StatusOK)
	dsMock, dsStore := datasetstoretest.NewMock(t)

	datasetstoretest.ExpectMissing(dsMock, "dep-1")
	datasetstoretest.ExpectCreate(dsMock, "dep-1", "acct-1")
	datasetstoretest.ExpectExists(dsMock, "dep-1")

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	record, err := Ensure(context.Background(), dsStore, client, EnsureOptions{
		DeploymentID: "dep-1",
		AccountID:    "acct-1",
		Description:  "test-agent",
	})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if record == nil || record.LangfuseDatasetName != "eval-dep-1" {
		t.Fatalf("record = %+v, want eval-dep-1", record)
	}
	if !*called {
		t.Error("expected Langfuse CreateDataset API to be called")
	}
	if err := dsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureCreateMissingAfterInsertReturnsError(t *testing.T) {
	srv, _ := datasetstoretest.LangfuseStatusServer(t, http.StatusOK)
	dsMock, dsStore := datasetstoretest.NewMock(t)

	datasetstoretest.ExpectMissing(dsMock, "dep-1")
	datasetstoretest.ExpectCreate(dsMock, "dep-1", "acct-1")
	datasetstoretest.ExpectMissing(dsMock, "dep-1")

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	record, err := Ensure(context.Background(), dsStore, client, EnsureOptions{
		DeploymentID: "dep-1",
		AccountID:    "acct-1",
	})
	if err == nil {
		t.Fatal("Ensure returned nil error, want missing-after-create")
	}
	if record != nil {
		t.Fatalf("record = %+v, want nil", record)
	}
	if !strings.Contains(err.Error(), "not found after create") {
		t.Fatalf("error = %v, want missing-after-create message", err)
	}
	if err := dsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureSkipsWhenAlreadyCanonical(t *testing.T) {
	srv, called := datasetstoretest.LangfuseStatusServer(t, http.StatusOK)
	dsMock, dsStore := datasetstoretest.NewMock(t)

	datasetstoretest.ExpectExists(dsMock, "dep-1")

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	record, err := Ensure(context.Background(), dsStore, client, EnsureOptions{
		DeploymentID: "dep-1",
	})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if record == nil || record.LangfuseDatasetName != "eval-dep-1" {
		t.Fatalf("record = %+v, want eval-dep-1", record)
	}
	if *called {
		t.Error("Langfuse API should not be called when dataset is already canonical")
	}
	if err := dsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureHealsLegacyRow(t *testing.T) {
	srv, called := datasetstoretest.LangfuseStatusServer(t, http.StatusOK)
	dsMock, dsStore := datasetstoretest.NewMock(t)

	datasetstoretest.ExpectLegacyExists(dsMock, "dep-1")
	datasetstoretest.ExpectRepoint(dsMock, "dep-1")

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	record, err := Ensure(context.Background(), dsStore, client, EnsureOptions{
		DeploymentID: "dep-1",
		Description:  "test-agent",
	})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if record.LangfuseDatasetName != "eval-dep-1" {
		t.Fatalf("record = %+v, want healed eval-dep-1", record)
	}
	if !*called {
		t.Error("expected Langfuse CreateDataset to be called for heal")
	}
	if err := dsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureHealFailureReturnsError(t *testing.T) {
	srv, _ := datasetstoretest.LangfuseStatusServer(t, http.StatusInternalServerError)
	dsMock, dsStore := datasetstoretest.NewMock(t)

	datasetstoretest.ExpectLegacyExists(dsMock, "dep-1")

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	record, err := Ensure(context.Background(), dsStore, client, EnsureOptions{
		DeploymentID: "dep-1",
		Description:  "test-agent",
	})
	if err == nil {
		t.Fatal("Ensure returned nil error, want heal failure")
	}
	if record != nil {
		t.Fatalf("record = %+v, want nil on heal failure", record)
	}
	if !strings.Contains(err.Error(), "create langfuse dataset") {
		t.Fatalf("error = %v, want create dataset failure", err)
	}
	if err := dsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureTreatsConflictAsExistingDataset(t *testing.T) {
	srv, _ := datasetstoretest.LangfuseStatusServer(t, http.StatusConflict)
	dsMock, dsStore := datasetstoretest.NewMock(t)

	datasetstoretest.ExpectLegacyExists(dsMock, "dep-1")
	datasetstoretest.ExpectRepoint(dsMock, "dep-1")

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	record, err := Ensure(context.Background(), dsStore, client, EnsureOptions{
		DeploymentID: "dep-1",
	})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if record.LangfuseDatasetName != "eval-dep-1" {
		t.Fatalf("LangfuseDatasetName = %q, want eval-dep-1", record.LangfuseDatasetName)
	}
	if err := dsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
