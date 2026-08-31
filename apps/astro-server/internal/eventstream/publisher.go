package eventstream

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/pgnotify"
)

// LineageLookup names the accounts deploying from a publisher's agent. They see
// the same builds, so they get the same events.
type LineageLookup interface {
	ListAccountIDsWithLineageAgent(ctx context.Context, accountID, agentName string) ([]string, error)
}

// Recipients ride along rather than being resolved per replica: one lineage
// query beats the same query on every replica holding a connection.
type Notification struct {
	Event      Event    `json:"event"`
	Recipients []string `json:"recipients"`
}

// The row is the durable part and the notification only the fast path, so a
// notification lost mid-reconnect costs latency, not the event.
type Publisher struct {
	store   *Store
	db      *sql.DB
	lineage LineageLookup
	log     *logger.Logger
}

func NewPublisher(db *sql.DB, lineage LineageLookup, log *logger.Logger) *Publisher {
	return &Publisher{store: NewStore(db), db: db, lineage: lineage, log: log}
}

// Best-effort by design: an unsent nudge must not fail the build that produced
// it. Pass tx to commit the event with the state change it describes.
func (p *Publisher) Publish(ctx context.Context, tx *sql.Tx, accountID, agentName, eventType, buildID, status string) {
	if p == nil || accountID == "" || agentName == "" {
		return
	}
	event, err := p.store.Record(ctx, tx, accountID, agentName, eventType, buildID, status)
	if err != nil {
		p.log.Warn("events: record failed", "account_id", accountID, "agent", agentName, "error", err)
		return
	}

	recipients := map[string]struct{}{accountID: {}}
	if p.lineage != nil {
		accts, lineageErr := p.lineage.ListAccountIDsWithLineageAgent(ctx, accountID, agentName)
		if lineageErr != nil {
			p.log.Warn("events: lineage lookup failed", "agent", agentName, "error", lineageErr)
		}
		for _, id := range accts {
			recipients[id] = struct{}{}
		}
	}
	list := make([]string, 0, len(recipients))
	for id := range recipients {
		list = append(list, id)
	}

	payload, err := json.Marshal(Notification{Event: event, Recipients: list})
	if err != nil {
		p.log.Warn("events: encode notification failed", "agent", agentName, "error", err)
		return
	}
	// Sent on the pool, never on tx: NOTIFY in a transaction holds an instance-wide
	// lock through COMMIT, serializing every build write. Replay covers the loss.
	if err := pgnotify.Notify(ctx, p.db, pgnotify.AgentEventChannel, string(payload)); err != nil {
		p.log.Warn("events: notify failed", "agent", agentName, "error", err)
	}
}

// The stored row is singular; this is where it becomes one event per account.
func Fanout(hub *Hub, n Notification) {
	for _, accountID := range n.Recipients {
		e := n.Event
		e.AccountID = accountID
		hub.Publish(e)
	}
}
