package evaldataset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

// ItemInput is the content of one Langfuse dataset item. Metadata is
// caller-owned so each admission path can record its own provenance.
type ItemInput struct {
	DatasetName    string
	TraceID        string
	Input          any
	ExpectedOutput any
	Metadata       map[string]any
}

// ItemID derives the deterministic Langfuse dataset item ID for a trace. The
// same (dataset, trace) pair must always hash to the same ID: it is the
// duplicate gate and the handle used to delete the item during compensation.
func ItemID(datasetName, traceID string) string {
	h := sha256.New()
	h.Write([]byte(datasetName))
	h.Write([]byte{0})
	h.Write([]byte(traceID))
	return hex.EncodeToString(h.Sum(nil))
}

// UpsertItem writes the Langfuse dataset item for a trace and returns its
// deterministic ID.
func UpsertItem(ctx context.Context, client *langfuse.Client, item ItemInput) (string, error) {
	id := ItemID(item.DatasetName, item.TraceID)
	if err := client.UpsertDatasetItem(ctx, langfuse.DatasetItemInput{
		ID:             id,
		DatasetName:    item.DatasetName,
		Input:          item.Input,
		ExpectedOutput: item.ExpectedOutput,
		SourceTraceID:  item.TraceID,
		Metadata:       item.Metadata,
	}); err != nil {
		return "", err
	}
	return id, nil
}

// DeleteItem removes a Langfuse dataset item, treating an already-absent item as
// success so compensation paths stay idempotent.
func DeleteItem(ctx context.Context, client *langfuse.Client, id string) error {
	if err := client.DeleteDatasetItem(ctx, id); err != nil && !errors.Is(err, langfuse.ErrNotFound) {
		return err
	}
	return nil
}
