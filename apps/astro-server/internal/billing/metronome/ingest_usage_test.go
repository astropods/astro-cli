package metronome

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

func genEvents(n int) []billing.UsageEvent {
	events := make([]billing.UsageEvent, n)
	for i := range events {
		events[i] = billing.UsageEvent{TransactionID: "tx", AccountID: "acct_1", Type: "compute", Time: time.Now()}
	}
	return events
}

func TestIngestUsage_ChunksAtTheBatchLimit(t *testing.T) {
	var batchSizes []int
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		// V1UsageIngestParams marshals as a bare array, not {"usage": [...]}.
		body, _ := io.ReadAll(r.Body)
		var events []json.RawMessage
		if err := json.Unmarshal(body, &events); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		batchSizes = append(batchSizes, len(events))
		w.WriteHeader(http.StatusOK)
	})

	if err := p.IngestUsage(context.Background(), genEvents(250)); err != nil {
		t.Fatalf("IngestUsage() error = %v", err)
	}

	want := []int{100, 100, 50}
	if len(batchSizes) != len(want) {
		t.Fatalf("batch sizes = %v, want %v", batchSizes, want)
	}
	for i, size := range batchSizes {
		if size != want[i] {
			t.Errorf("batch %d size = %d, want %d", i, size, want[i])
		}
	}
}

func TestIngestUsage_ExactlyOneBatchIsOneRequest(t *testing.T) {
	var requests int
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	})

	if err := p.IngestUsage(context.Background(), genEvents(ingestBatchLimit)); err != nil {
		t.Fatalf("IngestUsage() error = %v", err)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want exactly 1 for a full single batch", requests)
	}
}

func TestIngestUsage_NoEventsSendsNoRequest(t *testing.T) {
	requests := 0
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	})

	if err := p.IngestUsage(context.Background(), nil); err != nil {
		t.Fatalf("IngestUsage() error = %v", err)
	}
	if requests != 0 {
		t.Errorf("requests = %d, want 0 for no events", requests)
	}
}
