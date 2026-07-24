package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvocationClientChatCompletion(t *testing.T) {
	var captured chatCompletionWireRequest
	var auth, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		auth = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-1",
			"model": "claude-sonnet-4-6",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": `{"verdict_score":0.5}`},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30},
		})
	}))
	defer srv.Close()

	responseFormat := map[string]any{
		"type":        "json_schema",
		"json_schema": map[string]any{"name": "prediction", "strict": true},
	}
	response, err := NewInvocationClient(srv.URL+"/").ChatCompletion(context.Background(), "sk-bf-judge", ChatCompletionRequest{
		Model:          "claude-sonnet-4-6",
		Messages:       []ChatMessage{{Role: "system", Content: "judge"}, {Role: "user", Content: "trace"}},
		ResponseFormat: responseFormat,
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-bf-judge", auth)
	assert.Equal(t, "application/json", contentType)
	assert.False(t, captured.Stream)
	assert.Equal(t, "claude-sonnet-4-6", captured.Model)
	assert.Len(t, captured.Messages, 2)
	assert.NotNil(t, captured.ResponseFormat)
	require.Len(t, response.Choices, 1)
	assert.Equal(t, `{"verdict_score":0.5}`, response.Choices[0].Message.Content)
	require.NotNil(t, response.Usage)
	assert.Equal(t, 30, response.Usage.TotalTokens)
}

func TestInvocationClientAllowsMissingUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer srv.Close()

	response, err := NewInvocationClient(srv.URL).ChatCompletion(context.Background(), "key", ChatCompletionRequest{Model: "model"})
	require.NoError(t, err)
	assert.Nil(t, response.Usage)
}

func TestInvocationClientReturnsStructuredError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"rate_limit_exceeded","type":"rate_limit_error","message":"slow down"}}`))
	}))
	defer srv.Close()

	_, err := NewInvocationClient(srv.URL).ChatCompletion(context.Background(), "key", ChatCompletionRequest{Model: "model"})
	require.Error(t, err)
	var invocationErr *InvocationError
	require.True(t, errors.As(err, &invocationErr))
	assert.Equal(t, http.StatusTooManyRequests, invocationErr.StatusCode)
	assert.Equal(t, "rate_limit_exceeded", invocationErr.Code)
	assert.Equal(t, "rate_limit_error", invocationErr.Type)
	assert.Equal(t, "slow down", invocationErr.Message)
}

func TestInvocationClientPreservesUnstructuredErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gateway unavailable", http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := NewInvocationClient(srv.URL).ChatCompletion(context.Background(), "key", ChatCompletionRequest{Model: "model"})
	var invocationErr *InvocationError
	require.True(t, errors.As(err, &invocationErr))
	assert.Equal(t, http.StatusBadGateway, invocationErr.StatusCode)
	assert.Contains(t, invocationErr.Body, "gateway unavailable")
}

func TestInvocationClientRejectsMalformedSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	_, err := NewInvocationClient(srv.URL).ChatCompletion(context.Background(), "key", ChatCompletionRequest{Model: "model"})
	require.ErrorContains(t, err, "unmarshal response")
}

func TestInvocationClientValidatesRequiredFields(t *testing.T) {
	client := NewInvocationClient("http://unused")
	_, err := client.ChatCompletion(context.Background(), "", ChatCompletionRequest{Model: "model"})
	require.ErrorContains(t, err, "api key is required")
	_, err = client.ChatCompletion(context.Background(), "key", ChatCompletionRequest{})
	require.ErrorContains(t, err, "model is required")
}

func TestInvocationClientRespectsCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewInvocationClient(srv.URL).ChatCompletion(ctx, "key", ChatCompletionRequest{Model: "model"})
		done <- err
	}()
	<-started
	cancel()
	err := <-done
	close(release)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestInvocationClientTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer srv.Close()

	client := newInvocationClient(srv.URL, &http.Client{Timeout: 10 * time.Millisecond})
	_, err := client.ChatCompletion(context.Background(), "key", ChatCompletionRequest{Model: "model"})
	close(release)
	require.Error(t, err)
	var deadline interface{ Timeout() bool }
	require.True(t, errors.As(err, &deadline))
	assert.True(t, deadline.Timeout())
}
