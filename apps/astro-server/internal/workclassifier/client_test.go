package workclassifier

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientEmptyBaseURLIsNil(t *testing.T) {
	if NewClient("", "v1") != nil {
		t.Fatal("empty baseURL should yield a nil client")
	}
}

// Retry behaviour is exercised without paying its wait.
func testClient(baseURL string) *Client {
	c := NewClient(baseURL, "v")
	c.retryWait = 0
	return c
}

func TestClassifyAlignsPredictionsToInputs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/models/work-classifier-purpose:predict"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		var req predictRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		preds := make([]Prediction, len(req.Instances))
		for i, in := range req.Instances {
			preds[i] = Prediction{Label: in.Text, Score: 0.5}
		}
		_ = json.NewEncoder(w).Encode(predictResponse{Predictions: preds})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "ver-1")
	got, err := c.Classify(context.Background(), AxisPurpose, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d predictions, want 3", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Label != want {
			t.Errorf("prediction %d = %q, want %q", i, got[i].Label, want)
		}
	}
}

func TestClassifyChunksAboveMaxBatch(t *testing.T) {
	var sizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req predictRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		sizes = append(sizes, len(req.Instances))
		preds := make([]Prediction, len(req.Instances))
		for i := range preds {
			preds[i] = Prediction{Label: "work", Score: 1}
		}
		_ = json.NewEncoder(w).Encode(predictResponse{Predictions: preds})
	}))
	defer srv.Close()

	texts := make([]string, maxBatch+10)
	for i := range texts {
		texts[i] = fmt.Sprintf("t%d", i)
	}
	got, err := testClient(srv.URL).Classify(context.Background(), AxisTopic, texts)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(got) != len(texts) {
		t.Fatalf("got %d predictions, want %d", len(got), len(texts))
	}
	if want := []int{maxBatch, 10}; len(sizes) != 2 || sizes[0] != want[0] || sizes[1] != want[1] {
		t.Errorf("chunk sizes = %v, want %v", sizes, want)
	}
}

// The predictor answers 200 with an {"error": ...} body on tokenizer faults, so
// a bare status check would treat a failure as success. Refusing every text is
// the predictor's fault, not any one text's, so it still fails the call.
func TestClassifyRejectsErrorBodyOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(predictResponse{Error: "TypeError : TextEncodeInput must be ..."})
	}))
	defer srv.Close()

	_, err := testClient(srv.URL).Classify(context.Background(), AxisPurpose, []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "predictor error") {
		t.Fatalf("err = %v, want predictor error", err)
	}
}

func TestClassifyRejectsCountMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req predictRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		// One more prediction than inputs, at every batch size, so splitting
		// the batch cannot resolve it.
		preds := make([]Prediction, len(req.Instances)+1)
		_ = json.NewEncoder(w).Encode(predictResponse{Predictions: preds})
	}))
	defer srv.Close()

	_, err := testClient(srv.URL).Classify(context.Background(), AxisPurpose, []string{"a", "b"})
	if err == nil || !strings.Contains(err.Error(), "predictions for") {
		t.Fatalf("err = %v, want count mismatch", err)
	}
}

func TestClassifyPropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer srv.Close()

	_, err := testClient(srv.URL).Classify(context.Background(), AxisPurpose, []string{"a"})
	if err == nil || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("err = %v, want status 502", err)
	}
}

// A pod rolling under the call is the common failure, and the batch's inference
// is already paid for, so the call is retried rather than lost.
func TestPredictRetriesAnUnavailablePredictor(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < maxAttempts {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(predictResponse{Predictions: []Prediction{{Label: "work", Score: 1}}})
	}))
	defer srv.Close()

	got, err := testClient(srv.URL).Classify(context.Background(), AxisPurpose, []string{"a"})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if calls != maxAttempts {
		t.Errorf("attempts = %d, want %d", calls, maxAttempts)
	}
	if len(got) != 1 || got[0].Label != "work" {
		t.Errorf("got %v, want one 'work' prediction", got)
	}
}

// A text the predictor refuses would otherwise cost every text batched with it
// its label, every tick, forever.
func TestClassifyIsolatesARefusedText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req predictRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, in := range req.Instances {
			if in.Text == "poison" {
				_ = json.NewEncoder(w).Encode(predictResponse{Error: "TypeError : TextEncodeInput"})
				return
			}
		}
		preds := make([]Prediction, len(req.Instances))
		for i, in := range req.Instances {
			preds[i] = Prediction{Label: in.Text, Score: 1}
		}
		_ = json.NewEncoder(w).Encode(predictResponse{Predictions: preds})
	}))
	defer srv.Close()

	texts := []string{"a", "b", "poison", "d"}
	got, err := testClient(srv.URL).Classify(context.Background(), AxisPurpose, texts)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(got) != len(texts) {
		t.Fatalf("got %d predictions, want %d: alignment must survive the split", len(got), len(texts))
	}
	for i, want := range []string{"a", "b", "", "d"} {
		if got[i].Label != want {
			t.Errorf("prediction %d = %q, want %q", i, got[i].Label, want)
		}
	}
}

// A predictor refusing everything must not have its blanks recorded as labels,
// and must not be asked again once per split of every remaining batch.
func TestClassifyStopsWhenAWholeBatchIsRefused(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(predictResponse{Error: "TypeError : TextEncodeInput"})
	}))
	defer srv.Close()

	texts := make([]string, maxBatch*3)
	for i := range texts {
		texts[i] = fmt.Sprintf("t%d", i)
	}
	got, err := testClient(srv.URL).Classify(context.Background(), AxisPurpose, texts)
	if err == nil {
		t.Fatal("a wholesale refusal must fail the call")
	}
	if len(got) != 0 {
		t.Errorf("got %d predictions, want none recorded from a broken predictor", len(got))
	}
	// One batch split down to single texts, and no batch after it.
	if maxCalls := 2*maxBatch - 1; calls > maxCalls {
		t.Errorf("issued %d requests, want at most %d", calls, maxCalls)
	}
}

// Inference is billed per call, so the batches that did answer must survive the
// one that did not.
func TestClassifyReturnsCompletedBatchesWithTheError(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var req predictRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		preds := make([]Prediction, len(req.Instances))
		for i := range preds {
			preds[i] = Prediction{Label: "work", Score: 1}
		}
		_ = json.NewEncoder(w).Encode(predictResponse{Predictions: preds})
	}))
	defer srv.Close()

	texts := make([]string, maxBatch+5)
	for i := range texts {
		texts[i] = fmt.Sprintf("t%d", i)
	}
	got, err := testClient(srv.URL).Classify(context.Background(), AxisPurpose, texts)
	if err == nil {
		t.Fatal("Classify should report the failed batch")
	}
	if len(got) != maxBatch {
		t.Errorf("got %d predictions, want the %d from the batch that completed", len(got), maxBatch)
	}
}

func TestClassifyEmptyInputSkipsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not issue a request for empty input")
	}))
	defer srv.Close()

	got, err := testClient(srv.URL).Classify(context.Background(), AxisPurpose, nil)
	if err != nil || got != nil {
		t.Fatalf("got (%v, %v), want (nil, nil)", got, err)
	}
}

func TestNilClientErrorsRatherThanPanics(t *testing.T) {
	var c *Client
	if _, err := c.Classify(context.Background(), AxisPurpose, []string{"a"}); err == nil {
		t.Error("Classify on nil client should error")
	}
	if err := c.Ready(context.Background(), AxisPurpose); err == nil {
		t.Error("Ready on nil client should error")
	}
	if c.ModelVersion() != "" {
		t.Error("ModelVersion on nil client should be empty")
	}
}

func TestReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models/work-classifier-topic" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"name":"work-classifier-topic","ready":true}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "v")
	if err := c.Ready(context.Background(), AxisTopic); err != nil {
		t.Errorf("Ready(topic) = %v, want nil", err)
	}
	if err := c.Ready(context.Background(), AxisPurpose); err == nil {
		t.Error("Ready(purpose) should fail against a 404")
	}
}

// Stored labels are constrained by the DB column width and drive chart ordering.
func TestLabelsCoverConsumedAxes(t *testing.T) {
	for _, axis := range Axes {
		labels, ok := Labels[axis]
		if !ok || len(labels) == 0 {
			t.Errorf("axis %q has no labels", axis)
		}
		for _, l := range labels {
			if len(l) > 64 {
				t.Errorf("label %q exceeds the 64-char column", l)
			}
		}
	}
}
