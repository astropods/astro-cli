package eventstream

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// One row per transition, never one per recipient: a blueprint with a thousand
// downstream deployments would else write a thousand rows. Recipients resolve on read.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Pass tx when the event must land with the state change it describes.
func (s *Store) Record(ctx context.Context, tx *sql.Tx, accountID, agentName, eventType, buildID, status string) (Event, error) {
	const q = `
		INSERT INTO agent_events (account_id, agent_name, type, build_id, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`
	row := s.db.QueryRowContext(ctx, q, accountID, agentName, eventType, buildID, status)
	if tx != nil {
		row = tx.QueryRowContext(ctx, q, accountID, agentName, eventType, buildID, status)
	}
	var id int64
	if err := row.Scan(&id); err != nil {
		return Event{}, fmt.Errorf("record agent event: %w", err)
	}
	return Event{
		ID:        fmt.Sprint(id),
		AccountID: accountID,
		Type:      eventType,
		Agent:     agentName,
		BuildID:   buildID,
		Status:    status,
	}, nil
}

// Bounds one catch-up so a long absence cannot pull the whole table. Since
// reports when it bites, so the caller refetches instead.
const replayLimit = 200

// Covers the agents this account's deployments deploy from, not just its own.
// The bool reports more events beyond replayLimit.
func (s *Store) Since(ctx context.Context, accountID string, afterID int64) ([]Event, bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.agent_name, e.type, e.build_id, e.status
		FROM agent_events e
		WHERE e.id > $2
		  AND (
		    e.account_id = $1
		    OR EXISTS (
		      SELECT 1 FROM deployments d
		      WHERE d.agent_name = e.agent_name
		        AND d.account_id = $1
		        AND d.status <> 'undeployed'
		        AND (d.source_account_id = e.account_id
		             OR (d.source_account_id IS NULL AND d.account_id = e.account_id))
		    )
		  )
		ORDER BY e.id
		LIMIT $3`, accountID, afterID, replayLimit+1)
	if err != nil {
		return nil, false, fmt.Errorf("replay agent events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []Event
	for rows.Next() {
		var id int64
		var e Event
		if err := rows.Scan(&id, &e.Agent, &e.Type, &e.BuildID, &e.Status); err != nil {
			return nil, false, fmt.Errorf("scan agent event: %w", err)
		}
		e.ID = fmt.Sprint(id)
		e.AccountID = accountID
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate agent events: %w", err)
	}
	if len(out) > replayLimit {
		return out[:replayLimit], true, nil
	}
	return out, false, nil
}

// The heartbeat carries this, which is the only way a client detects a dropped
// notification when no further event arrives.
func (s *Store) MaxID(ctx context.Context) (int64, error) {
	var id sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT max(id) FROM agent_events`).Scan(&id); err != nil {
		return 0, fmt.Errorf("max agent event id: %w", err)
	}
	return id.Int64, nil
}

// Retention only covers the window a disconnected client can still catch up in.
func (s *Store) Trim(ctx context.Context, age time.Duration) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM agent_events WHERE created_at < now() - $1::interval`,
		fmt.Sprintf("%d seconds", int(age.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("trim agent events: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
