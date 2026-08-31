package evaldataset

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

type EnsureOptions struct {
	DeploymentID string
	AccountID    string
	Description  string
}

func ExpectedName(deploymentID string) string {
	return "eval-" + deploymentID
}

// Ensure returns the canonical eval dataset row. It creates missing rows and
// heals legacy non-eval rows.
func Ensure(
	ctx context.Context,
	dsStore *evaldatasetstore.Store,
	client *langfuse.Client,
	opts EnsureOptions,
) (*evaldatasetstore.EvalDataset, error) {
	if opts.DeploymentID == "" {
		return nil, fmt.Errorf("deployment id is required")
	}

	existing, err := dsStore.GetByDeploymentID(opts.DeploymentID)
	if err != nil {
		return nil, fmt.Errorf("get dataset row: %w", err)
	}

	expected := ExpectedName(opts.DeploymentID)
	if existing != nil && existing.LangfuseDatasetName == expected {
		return existing, nil
	}

	if err := createDatasetIfAbsent(ctx, client, expected, opts.Description); err != nil {
		return nil, fmt.Errorf("create langfuse dataset: %w", err)
	}

	if existing == nil {
		record := &evaldatasetstore.EvalDataset{
			DeploymentID:        opts.DeploymentID,
			AccountID:           opts.AccountID,
			LangfuseDatasetName: expected,
		}
		if err := dsStore.Create(record); err != nil {
			return nil, fmt.Errorf("create dataset row: %w", err)
		}
		canonical, err := dsStore.GetByDeploymentID(opts.DeploymentID)
		if err != nil {
			return nil, fmt.Errorf("re-read dataset row: %w", err)
		}
		if canonical == nil {
			return nil, fmt.Errorf("re-read dataset row: not found after create")
		}
		return canonical, nil
	}

	// Older rows used the deployment-prefixed Langfuse dataset name. Move them
	// to the canonical eval-prefixed name after the target dataset exists.
	if err := dsStore.RepointByDeploymentID(opts.DeploymentID, expected); err != nil {
		return nil, fmt.Errorf("heal dataset row: repoint from %q to %q: %w", existing.LangfuseDatasetName, expected, err)
	}

	existing.LangfuseDatasetName = expected
	return existing, nil
}

func createDatasetIfAbsent(ctx context.Context, client *langfuse.Client, name, description string) error {
	err := client.CreateDataset(ctx, name, description)
	if err == nil || isConflict(err) {
		return nil
	}
	return err
}

func isConflict(err error) bool {
	var apiErr *langfuse.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}
