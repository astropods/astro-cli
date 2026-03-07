package org

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/workos/workos-go/v4/pkg/events"
)

const eventsBatchSize = 100

// EventsConsumer polls the WorkOS Events API and processes membership events.
type EventsConsumer struct {
	eventsClient *events.Client
	accountStore *account.AccountStore
	db           *sql.DB
	log          *logger.Logger
	interval     time.Duration
}

// NewEventsConsumer creates a new events consumer.
func NewEventsConsumer(apiKey string, accountStore *account.AccountStore, db *sql.DB, log *logger.Logger, interval time.Duration) *EventsConsumer {
	return &EventsConsumer{
		eventsClient: &events.Client{APIKey: apiKey},
		accountStore: accountStore,
		db:           db,
		log:          log,
		interval:     interval,
	}
}

// Start polls the WorkOS Events API in a loop until the context is cancelled.
func (ec *EventsConsumer) Start(ctx context.Context) {
	ec.log.Info("WorkOS events consumer started", "interval", ec.interval.String())

	// Run immediately on start, then on interval
	ec.poll(ctx)

	ticker := time.NewTicker(ec.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ec.log.Info("WorkOS events consumer stopping")
			return
		case <-ticker.C:
			ec.poll(ctx)
		}
	}
}

func (ec *EventsConsumer) poll(ctx context.Context) {
	cursor, err := ec.getCursor(ctx)
	if err != nil {
		ec.log.Error("Failed to get events cursor", "error", err)
		return
	}

	eventTypes := []string{
		"organization_membership.created",
		"organization_membership.updated",
		"organization_membership.deleted",
	}

	for {
		resp, err := ec.eventsClient.ListEvents(ctx, events.ListEventsOpts{
			Events: eventTypes,
			After:  cursor,
			Limit:  eventsBatchSize,
		})
		if err != nil {
			ec.log.Error("Failed to list WorkOS events", "error", err)
			return
		}

		if len(resp.Data) == 0 {
			return
		}

		for _, event := range resp.Data {
			if err := ec.processEvent(ctx, event); err != nil {
				ec.log.Error("Failed to process event",
					"event_id", event.ID,
					"event_type", event.Event,
					"error", err,
				)
				// Continue processing — don't block on a single event failure
			}
			cursor = event.ID
		}

		// Persist cursor after processing batch
		if err := ec.setCursor(ctx, cursor); err != nil {
			ec.log.Error("Failed to persist events cursor", "error", err)
			return
		}

		// If fewer events than limit, we've caught up
		if len(resp.Data) < eventsBatchSize {
			return
		}
	}
}

// membershipEventData represents the data payload for organization_membership events.
type membershipEventData struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Role           struct {
		Slug string `json:"slug"`
	} `json:"role"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

func (ec *EventsConsumer) processEvent(_ context.Context, event events.Event) error {
	var data membershipEventData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal event data: %w", err)
	}

	// Look up local account by WorkOS org ID
	acct, err := ec.accountStore.GetByWorkOSOrganizationID(data.OrganizationID)
	if err != nil {
		// No local account for this org — skip silently
		return nil
	}

	switch event.Event {
	case "organization_membership.created", "organization_membership.updated":
		return ec.accountStore.UpsertMemberByWorkosMembershipID(
			acct.ID, data.UserID, data.ID,
		)

	case "organization_membership.deleted":
		member, err := ec.accountStore.GetMemberByWorkosMembershipID(data.ID)
		if err != nil {
			// Already deleted locally — idempotent
			return nil
		}
		return ec.accountStore.RemoveMember(member.AccountID, member.UserID)
	}

	return nil
}

func (ec *EventsConsumer) getCursor(ctx context.Context) (string, error) {
	var cursor string
	err := ec.db.QueryRowContext(ctx, `SELECT cursor_id FROM workos_event_cursor WHERE id = 1`).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to read cursor: %w", err)
	}
	return cursor, nil
}

func (ec *EventsConsumer) setCursor(ctx context.Context, cursor string) error {
	_, err := ec.db.ExecContext(ctx,
		`INSERT INTO workos_event_cursor (id, cursor_id, updated_at) VALUES (1, $1, now())
		 ON CONFLICT (id) DO UPDATE SET cursor_id = $1, updated_at = now()`,
		cursor,
	)
	if err != nil {
		return fmt.Errorf("failed to write cursor: %w", err)
	}
	return nil
}
