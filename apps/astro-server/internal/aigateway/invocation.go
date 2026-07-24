package aigateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// InvocationClient calls the public OpenAI-compatible Bifrost data plane. It
// is separate from Client, which uses admin authentication for governance.
type InvocationClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewInvocationClient(baseURL string) *InvocationClient {
	return newInvocationClient(baseURL, &http.Client{Timeout: 60 * time.Second})
}

func newInvocationClient(baseURL string, httpClient *http.Client) *InvocationClient {
	return &InvocationClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest contains the caller-owned model contract. The
// invocation transport always adds stream:false. ResponseFormat is deliberately
// flexible so evaljudge can supply its strict JSON schema in PR 3.
type ChatCompletionRequest struct {
	Model          string
	Messages       []ChatMessage
	ResponseFormat any
}

type chatCompletionWireRequest struct {
	Model          string        `json:"model"`
	Stream         bool          `json:"stream"`
	Messages       []ChatMessage `json:"messages"`
	ResponseFormat any           `json:"response_format,omitempty"`
}

type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type ChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   *ChatCompletionUsage   `json:"usage,omitempty"`
}

// InvocationError preserves the structured OpenAI/Bifrost error fields for
// later policy decisions. A 429 alone does not identify budget exhaustion.
type InvocationError struct {
	StatusCode int
	Code       string
	Type       string
	Message    string
	Body       string
}

func (e *InvocationError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("ai gateway invocation: HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("ai gateway invocation: HTTP %d: %s", e.StatusCode, e.Body)
}

type invocationErrorEnvelope struct {
	Error struct {
		Code    json.RawMessage `json:"code"`
		Type    string          `json:"type"`
		Message string          `json:"message"`
	} `json:"error"`
}

// ChatCompletion performs one non-streaming OpenAI-compatible model call.
func (c *InvocationClient) ChatCompletion(ctx context.Context, apiKey string, request ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ai gateway invocation: api key is required")
	}
	if request.Model == "" {
		return nil, fmt.Errorf("ai gateway invocation: model is required")
	}

	body, err := json.Marshal(chatCompletionWireRequest{
		Model:          request.Model,
		Stream:         false,
		Messages:       request.Messages,
		ResponseFormat: request.ResponseFormat,
	})
	if err != nil {
		return nil, fmt.Errorf("ai gateway invocation: marshal request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("ai gateway invocation: build request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+apiKey)

	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("ai gateway invocation: http: %w", err)
	}
	defer response.Body.Close() //nolint:errcheck

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("ai gateway invocation: read response: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, parseInvocationError(response.StatusCode, responseBody)
	}

	var out ChatCompletionResponse
	if err := json.Unmarshal(responseBody, &out); err != nil {
		return nil, fmt.Errorf("ai gateway invocation: unmarshal response: %w", err)
	}
	return &out, nil
}

func parseInvocationError(statusCode int, body []byte) *InvocationError {
	out := &InvocationError{StatusCode: statusCode, Body: string(body)}
	var envelope invocationErrorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return out
	}
	out.Type = envelope.Error.Type
	out.Message = envelope.Error.Message
	if len(envelope.Error.Code) > 0 && string(envelope.Error.Code) != "null" {
		if err := json.Unmarshal(envelope.Error.Code, &out.Code); err != nil {
			out.Code = string(envelope.Error.Code)
		}
	}
	return out
}
