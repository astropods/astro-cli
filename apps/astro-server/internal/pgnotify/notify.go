// Package pgnotify is a thin wrapper over PostgreSQL LISTEN/NOTIFY for
// cross-replica nudges. It lets one astro-server replica signal another through
// the shared database instead of an in-process call — used so the DeployWorker
// (which may run on any replica) can nudge the deployment controller (which runs
// only on the elected leader) to reconcile a namespace immediately.
//
// The listener side is github.com/lib/pq's Listener, which is already the
// Postgres driver in use and reconnects automatically.
package pgnotify

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// DeployReconcileChannel carries deployment namespaces that need an immediate
// controller reconcile. The DeployWorker publishes on it after marking a
// deployment "deploying"; the elected controller listens and enqueues. Routing
// through Postgres means the nudge reaches the leader even when the DeployWorker
// ran on a different, non-leader replica.
const DeployReconcileChannel = "deploy_reconcile"

// pingInterval bounds how long a silently-dead listener connection can go
// unnoticed: pq.Listener only reconnects once it detects the drop, and an idle
// connection produces no traffic to detect it with.
const pingInterval = 90 * time.Second

// Notify publishes payload on channel. Any Listen on the same channel — on any
// replica connected to this database — receives it. It is fire-and-forget from
// the caller's perspective; a lost notification is recovered by the listener's
// own backstop, so callers generally log and move on rather than fail.
func Notify(ctx context.Context, db *sql.DB, channel, payload string) error {
	_, err := db.ExecContext(ctx, "SELECT pg_notify($1, $2)", channel, payload)
	return err
}

// Listen subscribes to channel and calls handle for each notification payload
// until ctx is cancelled. Notifications emitted while the listener is briefly
// disconnected are dropped — this is a nudge, not a durable queue, so callers
// must have an independent backstop (the controller's periodic resync) rather
// than relying on every notification arriving.
func Listen(ctx context.Context, dsn, channel string, log *logger.Logger, handle func(payload string)) {
	l := pq.NewListener(dsn, 10*time.Second, time.Minute, func(_ pq.ListenerEventType, err error) {
		if err != nil {
			log.Warn("pgnotify: listener connection event", "channel", channel, "error", err)
		}
	})
	defer func() { _ = l.Close() }()

	if err := l.Listen(channel); err != nil {
		log.Error("pgnotify: subscribe failed", "channel", channel, "error", err)
		return
	}
	log.Info("pgnotify: listening", "channel", channel)

	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-l.Notify:
			// A nil notification is pq's signal that the connection was
			// re-established; anything sent while it was down was missed (the
			// resync backstop recovers it), so there is nothing to enqueue.
			if n == nil {
				continue
			}
			handle(n.Extra)
		case <-ping.C:
			if err := l.Ping(); err != nil {
				log.Warn("pgnotify: ping failed", "channel", channel, "error", err)
			}
		}
	}
}
