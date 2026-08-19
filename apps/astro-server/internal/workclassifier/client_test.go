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
	got, err := NewClient(srv.URL, "v").Classify(context.Background(), AxisTopic, texts)
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
// a bare status check would treat a failure as success.
func TestClassifyRejectsErrorBodyOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(predictResponse{Error: "TypeError : TextEncodeInput must be ..."})
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "v").Classify(context.Background(), AxisPurpose, []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "predictor error") {
		t.Fatalf("err = %v, want predictor error", err)
	}
}

func TestClassifyRejectsCountMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(predictResponse{Predictions: []Prediction{{Label: "work", Score: 1}}})
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "v").Classify(context.Background(), AxisPurpose, []string{"a", "b"})
	if err == nil || !strings.Contains(err.Error(), "for 2 inputs") {
		t.Fatalf("err = %v, want count mismatch", err)
	}
}

func TestClassifyPropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "v").Classify(context.Background(), AxisPurpose, []string{"a"})
	if err == nil || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("err = %v, want status 502", err)
	}
}

func TestClassifyEmptyInputSkipsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not issue a request for empty input")
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "v").Classify(context.Background(), AxisPurpose, nil)
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
