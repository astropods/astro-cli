package org

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/workos/workos-go/v4/pkg/events"
)

const eventsBatchSize = 100

// EventsConsumer polls the WorkOS Events API and processes membership events.
type EventsConsumer struct {
	eventsClient *events.Client
	orgClient    *Client
	accountStore *account.AccountStore
	agentIdx     *agentindex.Index
	avatarStore  *avatar.Store
	db           *sql.DB
	log          *logger.Logger
}

// NewEventsConsumer creates a new events consumer.
func NewEventsConsumer(apiKey string, orgClient *Client, accountStore *account.AccountStore, agentIdx *agentindex.Index, avatarStore *avatar.Store, db *sql.DB, log *logger.Logger) *EventsConsumer {
	return &EventsConsumer{
		eventsClient: &events.Client{APIKey: apiKey},
		orgClient:    orgClient,
		accountStore: accountStore,
		agentIdx:     agentIdx,
		avatarStore:  avatarStore,
		db:           db,
		log:          log,
	}
}

func (ec *EventsConsumer) Poll(ctx context.Context) (int, error) {
	cursor, err := ec.getCursor(ctx)
	if err != nil {
		return 0, fmt.Errorf("get events cursor: %w", err)
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

	totalProcessed := 0
	for {
		resp, err := ec.eventsClient.ListEvents(ctx, events.ListEventsOpts{
			Events: eventTypes,
			After:  cursor,
			Limit:  eventsBatchSize,
		})
		if err != nil {
			return totalProcessed, fmt.Errorf("list WorkOS events: %w", err)
		}

		if len(resp.Data) == 0 {
			break
		}

		for _, event := range resp.Data {
			if err := ec.processEvent(ctx, event); err != nil {
				ec.recordEventError(ctx, event, err)
				ec.setStuck(ctx, event.ID)
				// Persist cursor up to the last successfully processed event.
				// The failed event will be retried on the next poll cycle.
				if cursor != "" {
					if cursorErr := ec.setCursor(ctx, cursor); cursorErr != nil {
						ec.log.Error("Failed to persist events cursor", "error", cursorErr)
					}
				}
				return totalProcessed, fmt.Errorf("process event %s (%s): %w", event.ID, event.Event, err)
			}
			ec.clearEventError(ctx, event.ID)
			cursor = event.ID
			totalProcessed++
		}

		ec.clearStuck(ctx)

		// Persist cursor after processing batch
		if err := ec.setCursor(ctx, cursor); err != nil {
			return totalProcessed, fmt.Errorf("persist events cursor: %w", err)
		}

		// If fewer events than limit, we've caught up
		if len(resp.Data) < eventsBatchSize {
			break
		}
	}

	return totalProcessed, nil
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

func (ec *EventsConsumer) processEvent(ctx context.Context, event events.Event) error {
	switch {
	case strings.HasPrefix(event.Event, "organization_membership."):
		return ec.processMembershipEvent(event)
	case strings.HasPrefix(event.Event, "organization."):
		return ec.processOrganizationEvent(ctx, event)
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
		ec.log.Debug("Skipping membership event — no local account",
			"event_type", event.Event, "workos_org_id", data.OrganizationID)
		return nil
	}

	switch event.Event {
	case "organization_membership.created", "organization_membership.updated":
		if err := ec.accountStore.UpsertMemberByWorkosMembershipID(acct.ID, data.UserID, data.ID); err != nil {
			return err
		}
		ec.log.Info("Upserted member from membership event",
			"event_type", event.Event, "account_id", acct.ID,
			"user_id", data.UserID, "membership_id", data.ID)

	case "organization_membership.deleted":
		member, err := ec.accountStore.GetMemberByWorkosMembershipID(data.ID)
		if err != nil {
			ec.log.Debug("Membership already removed locally",
				"membership_id", data.ID)
			return nil
		}
		if err := ec.accountStore.RemoveMember(member.AccountID, member.UserID); err != nil {
			return err
		}
		ec.log.Info("Removed member from membership deletion",
			"account_id", member.AccountID, "user_id", member.UserID,
			"membership_id", data.ID)
	}

	return nil
}

func (ec *EventsConsumer) processOrganizationEvent(ctx context.Context, event events.Event) error {
	var data organizationEventData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal organization event data: %w", err)
	}

	switch event.Event {
	case "organization.created":
		// Check if we already have a local account linked to this WorkOS org
		if _, err := ec.accountStore.GetByWorkOSOrganizationID(data.ID); err == nil {
			ec.log.Debug("Skipping org created — already linked",
				"workos_org_id", data.ID)
			return nil
		}

		// If the WorkOS org has an external_id, it was created by Astro and the
		// account should already exist. Link them.
		if data.ExternalID != "" {
			if _, err := ec.accountStore.GetByID(data.ExternalID); err == nil {
				if err := ec.accountStore.SetWorkOSOrganizationID(data.ExternalID, data.ID); err != nil {
					return err
				}
				ec.log.Info("Linked existing account to WorkOS organization",
					"account_id", data.ExternalID, "workos_org_id", data.ID)
				return nil
			}
		}

		// Externally-created WorkOS org — create a local account and link it.
		// The slug may already be taken (e.g. user deleted and recreated the
		// org quickly, and this stale event arrived late). On name collision
		// we create a suffixed account and immediately soft-delete it so it's
		// invisible to users. The subsequent organization.deleted event will
		// find it via the WorkOS org link and clean it up.
		slug := slugifyOrgName(data.Name)
		acct, corrupt, err := createAccountWithUniqueName(ec.accountStore, slug)
		if err != nil {
			return fmt.Errorf("create account for external org: %w", err)
		}
		if corrupt {
			if err := ec.accountStore.MarkDeleted(acct.ID); err != nil {
				_ = ec.accountStore.DeleteByID(acct.ID)
				return fmt.Errorf("mark corrupt account deleted: %w", err)
			}
			ec.log.Warn("Created corrupt account from stale org event (soft-deleted immediately)",
				"account_id", acct.ID, "workos_org_id", data.ID, "slug", slug)
		}
		if err := ec.accountStore.SetWorkOSOrganizationID(acct.ID, data.ID); err != nil {
			// Clean up on failure
			_ = ec.accountStore.DeleteByID(acct.ID)
			return fmt.Errorf("link external org: %w", err)
		}
		// Set external_id on the WorkOS org so the bidirectional link is complete
		if ec.orgClient != nil {
			if err := ec.orgClient.UpdateOrganizationExternalID(ctx, data.ID, acct.ID); err != nil {
				ec.log.Warn("Failed to set external_id on WorkOS organization",
					"workos_org_id", data.ID, "account_id", acct.ID, "error", err)
			}
		}

		if !corrupt {
			ec.log.Info("Created account for external WorkOS organization",
				"account_id", acct.ID, "workos_org_id", data.ID, "name", data.Name)
		}

	case "organization.updated":
		acct, err := ec.accountStore.GetByWorkOSOrganizationID(data.ID)
		if err != nil {
			return nil
		}
		newName := slugifyOrgName(data.Name)
		if acct.Name != newName {
			// Skip if the new name is already taken — don't block the consumer
			if _, err := ec.accountStore.GetByName(newName); err == nil {
				ec.log.Warn("Skipping org rename — name already taken",
					"account_id", acct.ID, "old_name", acct.Name, "new_name", newName, "workos_org_id", data.ID)
				return nil
			}
			oldName := acct.Name
			if err := ec.accountStore.Rename(acct.ID, newName); err != nil {
				return fmt.Errorf("rename account for org update: %w", err)
			}
			// Move avatars in storage to match the new account name
			if ec.avatarStore != nil {
				agentNames, _ := ec.agentIdx.AgentNamesWithAvatars(acct.ID)
				if err := ec.avatarStore.MoveAllForAccount(ctx, oldName, newName, acct.AvatarVersion, agentNames); err != nil {
					ec.log.Warn("Failed to move avatars during org rename", "error", err, "account_id", acct.ID)
				}
			}
		}

	case "organization.deleted":
		acct, err := ec.accountStore.GetByWorkOSOrganizationID(data.ID)
		if err != nil {
			ec.log.Debug("Skipping org deleted — no local account",
				"workos_org_id", data.ID)
			return nil
		}
		if err := ec.accountStore.MarkDeleted(acct.ID); errors.Is(err, account.ErrAlreadyDeleted) {
			ec.log.Warn("Skipping org deleted — account already deleted",
				"account_id", acct.ID, "workos_org_id", data.ID)
			return nil
		} else if err != nil {
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

// slugifyOrgName converts a free-form WorkOS organization name into a valid
// account name slug: lowercase, alphanumeric + hyphens, 4-39 chars.
// e.g. "Acme Corp" → "acme-corp", "My  Great--Org!" → "my-great-org"
func slugifyOrgName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))

	var b strings.Builder
	prevHyphen := false
	for _, ch := range name {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
			b.WriteRune(ch)
			prevHyphen = false
		default:
			// Replace any non-alphanumeric with a single hyphen
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}

	slug := strings.TrimRight(b.String(), "-")

	// Pad to minimum length if too short
	for len(slug) < 4 {
		slug += "-org"
	}

	// Truncate to max length, trim trailing hyphen
	if len(slug) > 39 {
		slug = strings.TrimRight(slug[:39], "-")
	}

	return slug
}

// createAccountWithUniqueName creates an organization account with the given
// slug. If the name is already taken (stale event collision), it appends
// _conflict_<unix_timestamp> and returns corrupt=true so the caller can
// soft-delete the account immediately.
func createAccountWithUniqueName(store *account.AccountStore, slug string) (acct *account.Account, corrupt bool, err error) {
	if _, err := store.GetByName(slug); err != nil {
		// Name is available — happy path
		acct, err := store.CreateWithoutOwner(slug, "organization")
		if err != nil {
			return nil, false, err
		}
		return acct, false, nil
	}

	// Name taken — create with a conflict suffix and mark as corrupt
	suffix := "-conflict-" + strconv.FormatInt(time.Now().Unix(), 10)
	maxBase := 39 - len(suffix)
	base := slug
	if len(base) > maxBase {
		base = strings.TrimRight(base[:maxBase], "-")
	}
	acct, err = store.CreateWithoutOwner(base+suffix, "organization")
	if err != nil {
		return nil, false, err
	}
	return acct, true, nil
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

func (ec *EventsConsumer) setStuck(ctx context.Context, eventID string) {
	_, err := ec.db.ExecContext(ctx,
		`UPDATE workos_event_cursor
		 SET stuck_event_id = $1, stuck_since = COALESCE(stuck_since, now()), updated_at = now()
		 WHERE id = 1`,
		eventID,
	)
	if err != nil {
		ec.log.Error("Failed to set stuck state", "event_id", eventID, "error", err)
	}
}

func (ec *EventsConsumer) clearStuck(ctx context.Context) {
	_, err := ec.db.ExecContext(ctx,
		`UPDATE workos_event_cursor
		 SET stuck_event_id = NULL, stuck_since = NULL, updated_at = now()
		 WHERE id = 1 AND stuck_event_id IS NOT NULL`,
	)
	if err != nil {
		ec.log.Error("Failed to clear stuck state", "error", err)
	}
}

func (ec *EventsConsumer) recordEventError(ctx context.Context, event events.Event, processErr error) {
	_, err := ec.db.ExecContext(ctx,
		`INSERT INTO workos_event_errors (event_id, event_type, event_data, error, attempts, first_failed_at, last_failed_at)
		 VALUES ($1, $2, $3, $4, 1, now(), now())
		 ON CONFLICT (event_id) DO UPDATE SET
			error = $4,
			attempts = workos_event_errors.attempts + 1,
			last_failed_at = now()`,
		event.ID, event.Event, string(event.Data), processErr.Error(),
	)
	if err != nil {
		ec.log.Error("Failed to record event error",
			"event_id", event.ID, "error", err)
	}
}

func (ec *EventsConsumer) clearEventError(ctx context.Context, eventID string) {
	_, err := ec.db.ExecContext(ctx,
		`DELETE FROM workos_event_errors WHERE event_id = $1`, eventID,
	)
	if err != nil {
		ec.log.Error("Failed to clear event error",
			"event_id", eventID, "error", err)
	}
}
