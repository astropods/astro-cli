package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/eventstream"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

// Builds are minutes apart, so an idle stream outlives a proxy read timeout.
const eventHeartbeat = 25 * time.Second

// StreamAccountEvents serves GET /api/v1/accounts/:account/events as SSE. Events
// name only the agent that changed, so a missed one costs a late refresh.
func StreamAccountEvents(log *logger.Logger, hub *eventstream.Hub, store *eventstream.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Membership-checked by middleware. Resolving the account here instead
		// would stream any account's agents to any signed-in user.
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "account not resolved"})
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		// The server WriteTimeout, 10s by default, would close the stream.
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})

		flusher, isFlusher := c.Writer.(http.Flusher)
		if !isFlusher {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
			return
		}

		// Subscribe before replaying, or a change published during catch-up is lost.
		events, unsubscribe := hub.Subscribe(acct.ID)
		defer unsubscribe()

		lastSent := parseLastEventID(c)
		write := func(e eventstream.Event) bool {
			payload, marshalErr := e.Encode()
			if marshalErr != nil {
				log.Warn("events: encode failed", "account_id", acct.ID, "error", marshalErr)
				return true
			}
			if _, writeErr := fmt.Fprintf(c.Writer, "id: %s\nevent: %s\ndata: %s\n\n", e.ID, e.Type, payload); writeErr != nil {
				log.Debug("events: write failed, client likely disconnected", "account_id", acct.ID, "error", writeErr)
				return false
			}
			if n, convErr := strconv.ParseInt(e.ID, 10, 64); convErr == nil && n > lastSent {
				lastSent = n
			}
			flusher.Flush()
			return true
		}

		truncated := false
		if lastSent > 0 {
			missed, more, replayErr := store.Since(c.Request.Context(), acct.ID, lastSent)
			if replayErr != nil {
				log.Warn("events: replay failed", "account_id", acct.ID, "error", replayErr)
			}
			truncated = more
			for _, e := range missed {
				if !write(e) {
					return
				}
			}
		}

		// resync tells the client its catch-up was capped, so it refetches instead.
		readyPayload := "{}"
		if truncated {
			readyPayload = `{"resync":true}`
		}
		fmt.Fprintf(c.Writer, "event: ready\ndata: %s\n\n", readyPayload) //nolint:errcheck
		flusher.Flush()

		heartbeat := time.NewTicker(eventHeartbeat)
		defer heartbeat.Stop()

		ctx := c.Request.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-events:
				if !write(e) {
					return
				}
			case <-heartbeat.C:
				maxID, maxErr := store.MaxID(c.Request.Context())
				if maxErr != nil {
					log.Warn("events: max id lookup failed", "account_id", acct.ID, "error", maxErr)
					maxID = lastSent
				}
				if _, writeErr := fmt.Fprintf(c.Writer, "event: heartbeat\ndata: {\"max_id\":%d}\n\n", maxID); writeErr != nil {
					log.Debug("events: heartbeat write failed, client likely disconnected", "account_id", acct.ID, "error", writeErr)
					return
				}
				flusher.Flush()
			}
		}
	}
}

// The query parameter serves a client that reconnects with a fresh EventSource,
// which cannot set the header a browser sends on its own retries.
func parseLastEventID(c *gin.Context) int64 {
	raw := c.GetHeader("Last-Event-ID")
	if raw == "" {
		raw = c.Query("last_event_id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}
