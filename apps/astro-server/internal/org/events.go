package org

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
		"organization.created",
		"organization.updated",
		"organization.deleted",
		"user.created",
		"user.updated",
		"user.deleted",
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

// organizationEventData represents the data payload for organization events.
type organizationEventData struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ExternalID string `json:"external_id"`
}

// userEventData represents the data payload for user events.
type userEventData struct {
	ID string `json:"id"`
}

func (ec *EventsConsumer) processEvent(_ context.Context, event events.Event) error {
	switch {
	case strings.HasPrefix(event.Event, "organization_membership."):
		return ec.processMembershipEvent(event)
	case strings.HasPrefix(event.Event, "organization."):
		return ec.processOrganizationEvent(event)
	case strings.HasPrefix(event.Event, "user."):
		return ec.processUserEvent(event)
	}
	return nil
}

func (ec *EventsConsumer) processMembershipEvent(event events.Event) error {
	var data membershipEventData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal membership event data: %w", err)
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

func (ec *EventsConsumer) processOrganizationEvent(event events.Event) error {
	var data organizationEventData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal organization event data: %w", err)
	}

	switch event.Event {
	case "organization.created":
		// Check if we already have a local account linked to this WorkOS org
		if _, err := ec.accountStore.GetByWorkOSOrganizationID(data.ID); err == nil {
			return nil // already linked — created via Astro's own flow
		}

		// If the WorkOS org has an external_id, it was created by Astro and the
		// account should already exist. Link them.
		if data.ExternalID != "" {
			if _, err := ec.accountStore.GetByID(data.ExternalID); err == nil {
				return ec.accountStore.SetWorkOSOrganizationID(data.ExternalID, data.ID)
			}
		}

		// Externally-created WorkOS org — create a local account and link it
		acct, err := ec.accountStore.CreateWithoutOwner(data.Name, "organization")
		if err != nil {
			return fmt.Errorf("create account for external org: %w", err)
		}
		if err := ec.accountStore.SetWorkOSOrganizationID(acct.ID, data.ID); err != nil {
			// Clean up on failure
			_ = ec.accountStore.DeleteByID(acct.ID)
			return fmt.Errorf("link external org: %w", err)
		}
		ec.log.Info("Created account for external WorkOS organization",
			"account_id", acct.ID, "workos_org_id", data.ID, "name", data.Name)

	case "organization.updated":
		acct, err := ec.accountStore.GetByWorkOSOrganizationID(data.ID)
		if err != nil {
			return nil // no local account — skip
		}
		if acct.Name != data.Name {
			if err := ec.accountStore.Rename(acct.ID, data.Name); err != nil {
				return fmt.Errorf("rename account for org update: %w", err)
			}
			ec.log.Info("Renamed account from org update",
				"account_id", acct.ID, "old_name", acct.Name, "new_name", data.Name)
		}

	case "organization.deleted":
		acct, err := ec.accountStore.GetByWorkOSOrganizationID(data.ID)
		if err != nil {
			return nil // already gone — idempotent
		}
		if err := ec.accountStore.MarkDeleted(acct.ID); err != nil {
			return fmt.Errorf("mark account deleted for org deletion: %w", err)
		}
		ec.log.Info("Marked account for cleanup from org deletion",
			"account_id", acct.ID, "workos_org_id", data.ID)
	}

	return nil
}

func (ec *EventsConsumer) processUserEvent(event events.Event) error {
	var data userEventData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal user event data: %w", err)
	}

	switch event.Event {
	case "user.created", "user.updated":
		// No local user table — nothing to do
		return nil

	case "user.deleted":
		n, err := ec.accountStore.RemoveUserFromAllAccounts(data.ID)
		if err != nil {
			return fmt.Errorf("remove deleted user from accounts: %w", err)
		}
		if n > 0 {
			ec.log.Info("Removed deleted user from accounts",
				"user_id", data.ID, "memberships_removed", n)
		}
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
