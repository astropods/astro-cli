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

// metaTTL is how long workflow metadata (description, tags, critical flag) is
// cached. This is static, environment-wide data (not per-user), so a modest TTL
// makes the settings page's per-workflow fetches effectively one-time across all
// viewers while still picking up dashboard edits within the window.
const metaTTL = 15 * time.Minute

type metaEntry struct {
	value  WorkflowMeta
	expiry time.Time
}

// Client talks to one Novu environment. A zero-value URL/key yields a client
// whose Configured() reports false; callers should select the no-op provider
// in that case rather than construct this.
type Client struct {
	apiURL    string
	secretKey string
	http      *http.Client

	metaMu    sync.Mutex
	metaCache map[string]metaEntry // workflowId -> cached metadata
}

// NewClient builds a Novu client. apiURL is the REST base without a trailing
// slash (e.g. https://api.novu.astroids.ai); a trailing slash is trimmed.
func NewClient(apiURL, secretKey string) *Client {
	return &Client{
		apiURL:    strings.TrimRight(apiURL, "/"),
		secretKey: secretKey,
		http:      &http.Client{Timeout: 15 * time.Second},
		metaCache: map[string]metaEntry{},
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
// the v2 preferences API reports them: the workflow identity plus effective
// per-channel state. WorkflowID is the workflow identifier (equal to our
// notify.Type); it addresses both the update endpoint and the metadata lookup.
// Description, tags, and the critical flag are not in this response — they come
// from GetWorkflowMeta.
type WorkflowPreference struct {
	WorkflowID string
	Name       string
	Channels   map[string]bool // e.g. {"email": true, "in_app": false}
}

// subscriberPreferenceDTO is one workflow entry of the v2 GET preferences
// response (data.workflows[]).
type subscriberPreferenceDTO struct {
	Channels map[string]bool `json:"channels"`
	Workflow struct {
		Identifier string `json:"identifier"`
		Name       string `json:"name"`
	} `json:"workflow"`
}

// GetSubscriberPreferences returns the subscriber's per-workflow channel
// preferences via GET /v2/subscribers/{id}/preferences. The response is
// workflow-driven — every active workflow appears with the subscriber's
// effective preference — so the list is complete even for a subscriber that has
// never customized anything (Novu falls back to the workflow default). This is
// why the settings page can render the full catalog from Novu alone. The v2
// response omits description/tags/critical; GetWorkflowMeta supplies those.
func (c *Client) GetSubscriberPreferences(ctx context.Context, subscriberID string) ([]WorkflowPreference, error) {
	var env struct {
		Data struct {
			Workflows []subscriberPreferenceDTO `json:"workflows"`
		} `json:"data"`
	}
	path := "/v2/subscribers/" + url.PathEscape(subscriberID) + "/preferences"
	if err := c.do(ctx, http.MethodGet, path, nil, &env); err != nil {
		return nil, err
	}
	out := make([]WorkflowPreference, 0, len(env.Data.Workflows))
	for _, d := range env.Data.Workflows {
		out = append(out, WorkflowPreference{
			WorkflowID: d.Workflow.Identifier,
			Name:       d.Workflow.Name,
			Channels:   d.Channels,
		})
	}
	return out, nil
}

// SetSubscriberPreference sets a workflow's channel preferences for a subscriber
// via PATCH /v2/subscribers/{id}/preferences, addressed by workflow identifier.
// Unlike the legacy per-channel endpoint, one call carries all changed channels;
// channels maps channel name (e.g. "email", "in_app") to the desired enabled
// state.
func (c *Client) SetSubscriberPreference(ctx context.Context, subscriberID, workflowID string, channels map[string]bool) error {
	path := "/v2/subscribers/" + url.PathEscape(subscriberID) + "/preferences"
	body := map[string]any{"workflowId": workflowID, "channels": channels}
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

// WorkflowMeta is a workflow's static, environment-wide metadata: the authored
// description, its tags (category), and whether it is critical (locked on). The
// v2 subscriber-preferences list omits all of these, so this is a per-workflow
// detail fetch.
type WorkflowMeta struct {
	Description string
	Tags        []string
	Critical    bool
}

// GetWorkflowMeta returns a workflow's metadata by its identifier via
// GET /v2/workflows/{id}. The critical flag is Novu's preferences.default.all.
// readOnly. Results are cached for metaTTL (metadata is static, env-wide) so the
// settings page pays the N fetches only on a cold cache.
func (c *Client) GetWorkflowMeta(ctx context.Context, workflowID string) (WorkflowMeta, error) {
	c.metaMu.Lock()
	if e, ok := c.metaCache[workflowID]; ok && time.Now().Before(e.expiry) {
		c.metaMu.Unlock()
		return e.value, nil
	}
	c.metaMu.Unlock()

	var env struct {
		Data struct {
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
			Preferences struct {
				Default struct {
					All struct {
						ReadOnly bool `json:"readOnly"`
					} `json:"all"`
				} `json:"default"`
			} `json:"preferences"`
		} `json:"data"`
	}
	path := "/v2/workflows/" + url.PathEscape(workflowID)
	if err := c.do(ctx, http.MethodGet, path, nil, &env); err != nil {
		return WorkflowMeta{}, err
	}
	meta := WorkflowMeta{
		Description: env.Data.Description,
		Tags:        env.Data.Tags,
		Critical:    env.Data.Preferences.Default.All.ReadOnly,
	}

	c.metaMu.Lock()
	c.metaCache[workflowID] = metaEntry{value: meta, expiry: time.Now().Add(metaTTL)}
	c.metaMu.Unlock()
	return meta, nil
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
