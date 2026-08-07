package handlers

import (
	"context"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/novu"
)

// notifyQueue is the narrow slice of the River queue the notification handlers
// need: emitting an alert for off-request delivery.
type notifyQueue interface {
	EmitNotify(ctx context.Context, ev notify.Event) error
}

// billingProvisionQueue enqueues signup billing provisioning. Satisfied by
// *riverqueue.Queue and discovered by assertion off notifyQueue.
type billingProvisionQueue interface {
	InsertBillingProvision(ctx context.Context, accountID string) error
}

// emitNotify enqueues an alert best-effort from a request handler: a nil queue
// (e.g. in tests) or an enqueue error is logged, never surfaced to the caller,
// so notifications can never fail the user's operation.
func emitNotify(c *gin.Context, log *logger.Logger, queue notifyQueue, ev notify.Event) {
	if queue == nil {
		return
	}
	if err := queue.EmitNotify(c.Request.Context(), ev); err != nil {
		log.Warn("notify: emit failed", "error", err, "type", ev.Type, "account_id", ev.AccountID)
	}
}

// NotificationPreference is one workflow's preferences for the current user, as
// reported by Novu: its identity, category (Novu tag), critical flag, and
// effective per-channel state. Novu owns the catalog, so the list is whatever
// workflows exist in the environment.
type NotificationPreference struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category"`
	Critical    bool   `json:"critical"` // locked on; the user cannot disable it
	Email       bool   `json:"email"`
	InApp       bool   `json:"in_app"`
}

// NotificationPreferencesResponse is the settings-page payload. DeliveryEnabled
// reports whether Novu is wired; when false the list is empty and the UI shows
// a "not configured" note.
type NotificationPreferencesResponse struct {
	DeliveryEnabled bool                     `json:"delivery_enabled"`
	Preferences     []NotificationPreference `json:"preferences"`
}

// UpdateNotificationPreferenceRequest sets the current user's channels for one
// workflow (identified by its trigger type).
type UpdateNotificationPreferenceRequest struct {
	Type  string `json:"type"`
	Email bool   `json:"email"`
	InApp bool   `json:"in_app"`
}

// NotificationInboxConfig is everything the browser Inbox component needs to
// connect to the self-hosted Novu. Enabled is false when the Inbox isn't
// configured, so the client renders nothing.
type NotificationInboxConfig struct {
	Enabled               bool   `json:"enabled"`
	ApplicationIdentifier string `json:"application_identifier,omitempty"`
	SubscriberID          string `json:"subscriber_id,omitempty"`
	SubscriberHash        string `json:"subscriber_hash,omitempty"`
	BackendURL            string `json:"backend_url,omitempty"`
	SocketURL             string `json:"socket_url,omitempty"`
}

// GetNotificationInboxConfig returns the browser Inbox connection config for the
// current user. The subscriber id is the WorkOS user id; the hash is an HMAC so
// the feed is tamper-proof when the Novu environment enables HMAC.
func GetNotificationInboxConfig(log *logger.Logger, novuClient *novu.Client, appIdentifier, backendURL, socketURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := middleware.GetUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if novuClient == nil || appIdentifier == "" || backendURL == "" {
			c.JSON(http.StatusOK, NotificationInboxConfig{Enabled: false})
			return
		}
		c.JSON(http.StatusOK, NotificationInboxConfig{
			Enabled:               true,
			ApplicationIdentifier: appIdentifier,
			SubscriberID:          user.ID,
			SubscriberHash:        novuClient.SubscriberHash(user.ID),
			BackendURL:            backendURL,
			SocketURL:             socketURL,
		})
	}
}

// SendTestNotification emits a `system.test` alert to the requesting user so
// they can verify their channels work. It goes through the normal delivery
// path. testWorkflowID, when set, overrides the Novu workflow triggered (local
// dev where system.test is not authored — e.g. point at test-02).
func SendTestNotification(log *logger.Logger, queue notifyQueue, testWorkflowID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		user, ok := middleware.GetUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		ev := notify.Event{
			Type:        notify.TypeSystemTest,
			AccountID:   acct.ID,
			Audience:    notify.AudienceActor,
			ActorUserID: user.ID,
			WorkflowID:  testWorkflowID,
			Payload:     map[string]any{"account": acct.Name},
		}
		if err := queue.EmitNotify(c.Request.Context(), ev); err != nil {
			log.Error("notify: emit test failed", "error", err, "account_id", acct.ID, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send test notification"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"message": "test notification queued"})
	}
}

// GetNotificationPreferences returns the current user's per-workflow channel
// preferences from Novu. Novu owns the catalog and the preferences (per
// subscriber = per user); the list is workflow-driven, so it is complete even
// for a user who has never customized anything. When Novu is unconfigured the
// response reports delivery_enabled=false with an empty list.
func GetNotificationPreferences(log *logger.Logger, novuClient *novu.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := middleware.GetUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if novuClient == nil {
			c.JSON(http.StatusOK, NotificationPreferencesResponse{DeliveryEnabled: false, Preferences: []NotificationPreference{}})
			return
		}
		prefs, err := novuClient.GetSubscriberPreferences(c.Request.Context(), user.ID)
		if err != nil {
			log.Error("notify: get preferences failed", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load notification preferences"})
			return
		}

		// The v2 preferences list omits description, category (tags), and the
		// critical flag, so fetch each workflow's metadata from Novu concurrently.
		// Best-effort: a failed fetch just leaves the row with defaults rather than
		// failing the page.
		metas := make([]novu.WorkflowMeta, len(prefs))
		var wg sync.WaitGroup
		for i := range prefs {
			if prefs[i].WorkflowID == "" {
				continue
			}
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				m, merr := novuClient.GetWorkflowMeta(c.Request.Context(), prefs[i].WorkflowID)
				if merr != nil {
					log.Warn("notify: fetch workflow metadata failed", "error", merr, "workflow", prefs[i].WorkflowID)
					return
				}
				metas[i] = m
			}(i)
		}
		wg.Wait()

		out := make([]NotificationPreference, 0, len(prefs))
		for i, p := range prefs {
			if p.WorkflowID == "" {
				continue // a workflow without an identifier can't be addressed
			}
			out = append(out, NotificationPreference{
				Type:        p.WorkflowID,
				Name:        p.Name,
				Description: metas[i].Description,
				Category:    categoryFromTags(metas[i].Tags),
				Critical:    metas[i].Critical,
				Email:       p.Channels["email"],
				InApp:       p.Channels["in_app"],
			})
		}
		c.JSON(http.StatusOK, NotificationPreferencesResponse{DeliveryEnabled: true, Preferences: out})
	}
}

// UpdateNotificationPreference sets the current user's channels for one
// workflow. Critical workflows are rejected — Novu locks them on. Only channels
// whose value changed are patched.
func UpdateNotificationPreference(log *logger.Logger, novuClient *novu.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := middleware.GetUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if novuClient == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "notifications delivery is not configured"})
			return
		}
		var req UpdateNotificationPreferenceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
			return
		}

		prefs, err := novuClient.GetSubscriberPreferences(c.Request.Context(), user.ID)
		if err != nil {
			log.Error("notify: load preferences for update failed", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update notification preference"})
			return
		}
		var target *novu.WorkflowPreference
		for i := range prefs {
			if prefs[i].WorkflowID == req.Type {
				target = &prefs[i]
				break
			}
		}
		if target == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown notification type"})
			return
		}

		// The v2 preferences list omits the critical flag, so fetch the workflow's
		// metadata to reject updates to a locked-on workflow. Novu also enforces
		// readOnly server-side, but this gives a clear error instead of a silent
		// no-op.
		meta, err := novuClient.GetWorkflowMeta(c.Request.Context(), target.WorkflowID)
		if err != nil {
			log.Error("notify: load workflow metadata for update failed", "error", err, "user_id", user.ID, "type", req.Type)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update notification preference"})
			return
		}
		if meta.Critical {
			c.JSON(http.StatusBadRequest, gin.H{"error": "this notification is required and cannot be disabled"})
			return
		}

		// Send only the channels whose value changed, in a single PATCH.
		desired := map[string]bool{"email": req.Email, "in_app": req.InApp}
		changed := map[string]bool{}
		for channel, want := range desired {
			if target.Channels[channel] != want {
				changed[channel] = want
			}
		}
		if len(changed) > 0 {
			if err := novuClient.SetSubscriberPreference(c.Request.Context(), user.ID, target.WorkflowID, changed); err != nil {
				log.Error("notify: set preference failed", "error", err, "user_id", user.ID, "type", req.Type)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update notification preference"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "preference updated"})
	}
}

// categoryFromTags maps a workflow's Novu tags to a settings-page category. The
// first tag wins; workflows are tagged by category (Billing, Security, …) when
// authored. Untagged workflows fall under "General".
func categoryFromTags(tags []string) string {
	if len(tags) > 0 && tags[0] != "" {
		return tags[0]
	}
	return "General"
}
