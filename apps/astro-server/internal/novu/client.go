// Package novu is a thin HTTP client for the self-hosted Novu REST API. It
// covers the surface the notifications system needs: triggering workflows and
// reading/updating a subscriber's channel preferences. Auth is the environment
// API key sent as `Authorization: ApiKey <key>`.
package novu

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// APIError is a non-2xx response from the Novu API. Callers can inspect
// StatusCode to handle expected cases (e.g. 404 for an untriggered subscriber).
type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("novu: %s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// descTTL is how long a workflow description is cached. Descriptions are static,
// environment-wide data (not per-user), so a modest TTL makes the settings
// page's per-workflow fetches effectively one-time across all viewers while
// still picking up dashboard edits within the window.
const descTTL = 15 * time.Minute

type descEntry struct {
	value  string
	expiry time.Time
}

// Client talks to one Novu environment. A zero-value URL/key yields a client
// whose Configured() reports false; callers should select the no-op provider
// in that case rather than construct this.
type Client struct {
	apiURL    string
	secretKey string
	http      *http.Client

	descMu    sync.Mutex
	descCache map[string]descEntry // templateID -> cached description
}

// NewClient builds a Novu client. apiURL is the REST base without a trailing
// slash (e.g. https://api.novu.astroids.ai); a trailing slash is trimmed.
func NewClient(apiURL, secretKey string) *Client {
	return &Client{
		apiURL:    strings.TrimRight(apiURL, "/"),
		secretKey: secretKey,
		http:      &http.Client{Timeout: 15 * time.Second},
		descCache: map[string]descEntry{},
	}
}

// Configured reports whether the client has both a base URL and a key.
func (c *Client) Configured() bool {
	return c.apiURL != "" && c.secretKey != ""
}

// SubscriberHash is the HMAC-SHA256 of the subscriber id keyed by the secret
// key, hex-encoded. The browser Inbox sends it as the subscriber's proof of
// identity when the Novu environment has HMAC enabled.
func (c *Client) SubscriberHash(subscriberID string) string {
	mac := hmac.New(sha256.New, []byte(c.secretKey))
	mac.Write([]byte(subscriberID))
	return hex.EncodeToString(mac.Sum(nil))
}

// Subscriber is a trigger recipient. SubscriberID is the stable identity (the
// WorkOS user id); Email is required for the email channel. Novu upserts the
// subscriber from these fields on trigger, so no separate create call is needed.
type Subscriber struct {
	SubscriberID string `json:"subscriberId"`
	Email        string `json:"email,omitempty"`
	FirstName    string `json:"firstName,omitempty"`
	LastName     string `json:"lastName,omitempty"`
}

// TriggerRequest is one workflow trigger. TransactionID makes retries and
// provider redeliveries idempotent — Novu drops a duplicate.
type TriggerRequest struct {
	WorkflowID    string
	To            []Subscriber
	Payload       map[string]any
	TransactionID string
}

// Trigger fires a workflow via POST /v1/events/trigger. The `name` field is the
// workflow trigger identifier (equal to the workflowId for our workflows).
func (c *Client) Trigger(ctx context.Context, req TriggerRequest) error {
	if len(req.To) == 0 {
		return fmt.Errorf("novu: trigger %q has no recipients", req.WorkflowID)
	}
	payload := req.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	body := map[string]any{
		"name":    req.WorkflowID,
		"to":      req.To,
		"payload": payload,
	}
	if req.TransactionID != "" {
		body["transactionId"] = req.TransactionID
	}
	return c.do(ctx, http.MethodPost, "/v1/events/trigger", body, nil)
}

// WorkflowPreference is one workflow's channel preferences for a subscriber, as
// Novu reports them: the workflow identity plus effective per-channel state.
// WorkflowID is the trigger identifier (equal to our notify.Type); TemplateID is
// Novu's internal workflow `_id`, needed to address the update endpoint.
type WorkflowPreference struct {
	WorkflowID string
	TemplateID string
	Name       string
	Critical   bool
	Tags       []string
	Channels   map[string]bool // e.g. {"email": true, "in_app": false}
}

// subscriberPreferenceDTO is one element of the GET preferences response.
type subscriberPreferenceDTO struct {
	Preference struct {
		Channels map[string]bool `json:"channels"`
	} `json:"preference"`
	Template struct {
		ID       string   `json:"_id"`
		Name     string   `json:"name"`
		Critical bool     `json:"critical"`
		Tags     []string `json:"tags"`
		Triggers []struct {
			Identifier string `json:"identifier"`
		} `json:"triggers"`
	} `json:"template"`
}

// GetSubscriberPreferences returns the subscriber's per-workflow channel
// preferences. The response is workflow-driven — every active workflow appears
// with the subscriber's effective preference — so the list is complete even for
// a subscriber that has never customized anything (Novu falls back to the
// workflow default). This is why the settings page can render the full catalog
// from Novu alone.
func (c *Client) GetSubscriberPreferences(ctx context.Context, subscriberID string) ([]WorkflowPreference, error) {
	var env struct {
		Data []subscriberPreferenceDTO `json:"data"`
	}
	path := "/v1/subscribers/" + url.PathEscape(subscriberID) + "/preferences"
	if err := c.do(ctx, http.MethodGet, path, nil, &env); err != nil {
		return nil, err
	}
	out := make([]WorkflowPreference, 0, len(env.Data))
	for _, d := range env.Data {
		var id string
		if len(d.Template.Triggers) > 0 {
			id = d.Template.Triggers[0].Identifier
		}
		out = append(out, WorkflowPreference{
			WorkflowID: id,
			TemplateID: d.Template.ID,
			Name:       d.Template.Name,
			Critical:   d.Template.Critical,
			Tags:       d.Template.Tags,
			Channels:   d.Preference.Channels,
		})
	}
	return out, nil
}

// SetSubscriberPreferenceChannel enables/disables one channel (e.g. "email",
// "in_app") for one workflow, addressed by its Novu template id, for a
// subscriber. Novu updates a single channel per call.
func (c *Client) SetSubscriberPreferenceChannel(ctx context.Context, subscriberID, templateID, channel string, enabled bool) error {
	path := "/v1/subscribers/" + url.PathEscape(subscriberID) + "/preferences/" + url.PathEscape(templateID)
	body := map[string]any{"channel": map[string]any{"type": channel, "enabled": enabled}}
	return c.do(ctx, http.MethodPatch, path, body, nil)
}

// WorkflowSpec defines one notification workflow to provision in Novu: its
// identity, category (tag), critical flag, and default channels. The message
// wording is authored in Novu (not here); provisioning only creates the shell +
// placeholder templates and uploads the payload schema.
type WorkflowSpec struct {
	WorkflowID   string
	Name         string
	Description  string
	Category     string // Novu tag; groups the preferences UI
	Critical     bool   // locks the workflow on (preferences.all.readOnly)
	EmailDefault bool   // email channel enabled by default
	InAppDefault bool   // in-app channel enabled by default
}

// WorkflowExists reports whether a workflow with the given identifier exists.
func (c *Client) WorkflowExists(ctx context.Context, workflowID string) (bool, error) {
	err := c.do(ctx, http.MethodGet, "/v2/workflows/"+url.PathEscape(workflowID), nil, nil)
	if err == nil {
		return true, nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, err
}

// CreateWorkflow provisions a workflow (in-app + email steps) from a
// WorkflowSpec. The message wording lives in Novu — this creates neutral
// placeholder templates (a title + generic line, linked to {{payload.ctaUrl}})
// that are then authored per type in the Novu dashboard. The backend pushes
// data only. Channel gating is Novu's job (per-workflow defaults + per-subscriber
// overrides), so no send_* skip conditions are set.
func (c *Client) CreateWorkflow(ctx context.Context, spec WorkflowSpec) error {
	const placeholder = "You have a new notification from Astro."
	emailBodyDoc := `{"type":"doc","content":[{"type":"paragraph","attrs":{"textAlign":"left"},"content":[{"type":"text","text":"` + placeholder + `"}]}]}`
	body := map[string]any{
		"name":        spec.Name,
		"workflowId":  spec.WorkflowID,
		"description": spec.Description,
		"active":      true,
		"__source":    "editor",
		"tags":        []string{spec.Category},
		"steps": []map[string]any{
			{
				"name": "In-App Step",
				"type": "in_app",
				"controlValues": map[string]any{
					"subject":  spec.Name,
					"body":     placeholder,
					"redirect": map[string]any{"url": "{{payload.ctaUrl}}", "target": "_self"},
				},
			},
			{
				"name": "Email Step",
				"type": "email",
				"controlValues": map[string]any{
					"subject":    spec.Name,
					"editorType": "block",
					"body":       emailBodyDoc,
				},
			},
		},
		// The write shape is preferences.workflow (a.k.a. defaultPreferences); the
		// read shape echoes it back as preferences.default. All five channels must
		// be present even though we only define email + in-app steps.
		"preferences": map[string]any{
			"workflow": map[string]any{
				"all": map[string]any{"enabled": true, "readOnly": spec.Critical},
				"channels": map[string]any{
					"email":  map[string]any{"enabled": spec.EmailDefault},
					"in_app": map[string]any{"enabled": spec.InAppDefault},
					"sms":    map[string]any{"enabled": false},
					"chat":   map[string]any{"enabled": false},
					"push":   map[string]any{"enabled": false},
				},
			},
		},
	}
	return c.do(ctx, http.MethodPost, "/v2/workflows", body, nil)
}

// SetWorkflowPayloadSchema uploads the workflow's payload JSON Schema (all
// properties typed as strings) via PATCH. This documents the variables a
// template author can use and does not touch the step templates, so it is safe
// to run on workflows whose messaging has already been authored in the dashboard.
func (c *Client) SetWorkflowPayloadSchema(ctx context.Context, workflowID string, properties []string) error {
	props := make(map[string]any, len(properties))
	for _, p := range properties {
		props[p] = map[string]any{"type": "string"}
	}
	body := map[string]any{
		"payloadSchema": map[string]any{
			"type":                 "object",
			"properties":           props,
			"additionalProperties": true,
		},
	}
	return c.do(ctx, http.MethodPatch, "/v2/workflows/"+url.PathEscape(workflowID), body, nil)
}

// GetWorkflowDescription returns a workflow's authored description by its Novu
// template id, or the empty string if none. The subscriber-preferences list
// omits descriptions, and Novu has no bulk endpoint that includes them for
// dashboard (novu-cloud origin) workflows, so this is a per-workflow detail
// fetch. Results are cached for descTTL (descriptions are static, env-wide) so
// the settings page pays the N fetches only on a cold cache.
func (c *Client) GetWorkflowDescription(ctx context.Context, templateID string) (string, error) {
	c.descMu.Lock()
	if e, ok := c.descCache[templateID]; ok && time.Now().Before(e.expiry) {
		c.descMu.Unlock()
		return e.value, nil
	}
	c.descMu.Unlock()

	var env struct {
		Data struct {
			Description string `json:"description"`
		} `json:"data"`
	}
	path := "/v1/notification-templates/" + url.PathEscape(templateID)
	if err := c.do(ctx, http.MethodGet, path, nil, &env); err != nil {
		return "", err
	}

	c.descMu.Lock()
	c.descCache[templateID] = descEntry{value: env.Data.Description, expiry: time.Now().Add(descTTL)}
	c.descMu.Unlock()
	return env.Data.Description, nil
}

// do performs a request against the Novu API. When out is non-nil the response
// `data` envelope is decoded into it.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("novu: marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, c.apiURL+path, reader)
	if err != nil {
		return fmt.Errorf("novu: new request: %w", err)
	}
	httpReq.Header.Set("Authorization", "ApiKey "+c.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("novu: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Method: method, Path: path, Body: string(respBody)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("novu: decode %s %s: %w", method, path, err)
	}
	return nil
}
