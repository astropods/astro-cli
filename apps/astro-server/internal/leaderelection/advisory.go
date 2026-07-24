// Package leaderelection provides single-writer coordination across a fleet of
// worker replicas using a PostgreSQL session-level advisory lock. Exactly one
// replica holds the lock at a time; when the holder's process dies its backend
// connection drops and Postgres releases the lock automatically, so another
// replica takes over without any explicit fencing token or lease renewal.
//
// It exists so the event-driven deployment controller (the sole writer of the
// deployment read-model) can be run on more than one astro-worker replica
// safely: only the elected leader runs its informers and DB writes.
//
// The lock primitive itself is github.com/allisson/go-pglock, which owns the
// dedicated-connection pinning and the pg_advisory_lock SQL. This package adds
// the orchestration on top: acquire-with-retry, a liveness heartbeat, and
// step-down that unwinds the leader's work before another replica takes over.
package leaderelection

import (
	"context"
	"database/sql"
	"hash/fnv"
	"time"

	pglock "github.com/allisson/go-pglock/v3"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

const (
	defaultRetryInterval     = 5 * time.Second
	defaultHeartbeatInterval = 10 * time.Second
)

// Key derives a stable int64 advisory-lock key from a name, so callers name
// their lock instead of hard-coding a magic number. All replicas that must
// elect a single leader pass the same name.
func Key(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("leaderelection:" + name))
	return int64(h.Sum64()) //nolint:gosec // advisory lock key; overflow is harmless
}

// Config parameterizes Run.
type Config struct {
	// LockKey is the advisory-lock key contended for. Derive it with Key.
	LockKey int64
	// RetryInterval is how long a follower waits before re-attempting to
	// acquire the lock. Defaults to defaultRetryInterval.
	RetryInterval time.Duration
	// HeartbeatInterval is how often the leader verifies its lock connection is
	// still alive. Defaults to defaultHeartbeatInterval.
	HeartbeatInterval time.Duration
	// Name labels this election in logs (e.g. "deploy-controller").
	Name   string
	Logger *logger.Logger
}

// Run contends for leadership and blocks until ctx is cancelled. Each time this
// replica wins the lock it calls onElected with a leaderCtx that is cancelled
// when leadership is lost (the lock connection dropped) or when ctx is
// cancelled. onElected must block until leaderCtx is done and return promptly
// thereafter. After onElected returns Run releases the lock and re-contends,
// so a briefly-partitioned replica can reclaim leadership.
func Run(ctx context.Context, db *sql.DB, cfg Config, onElected func(leaderCtx context.Context)) {
	retry := cfg.RetryInterval
	if retry <= 0 {
		retry = defaultRetryInterval
	}
	heartbeat := cfg.HeartbeatInterval
	if heartbeat <= 0 {
		heartbeat = defaultHeartbeatInterval
	}

	for ctx.Err() == nil {
		if !tryLead(ctx, db, cfg, heartbeat, onElected) {
			// Not acquired (contended, or a transient error). Back off before
			// re-contending, bailing out if the parent context is cancelled.
			select {
			case <-ctx.Done():
				return
			case <-time.After(retry):
			}
		}
	}
}

// tryLead makes a single attempt to acquire the lock and, on success, runs
// onElected until leadership ends. It returns true iff it held the lock (so the
// caller can skip the follower back-off and immediately re-contend).
func tryLead(
	ctx context.Context,
	db *sql.DB,
	cfg Config,
	heartbeat time.Duration,
	onElected func(leaderCtx context.Context),
) bool {
	log := cfg.Logger

	// NewLock pins a dedicated connection from the pool — mandatory, because a
	// session-level advisory lock lives on the backend that took it.
	lock, err := pglock.NewLock(ctx, cfg.LockKey, db)
	if err != nil {
		if ctx.Err() == nil {
			log.Warn("leaderelection: acquire connection failed", "name", cfg.Name, "error", err)
		}
		return false
	}
	// Close returns the pinned connection to the pool. It does NOT release the
	// lock (the pooled backend stays alive) — the explicit Unlock below does
	// that; Close is only cleanup so we don't leak the connection.
	defer func() { _ = lock.Close() }()

	acquired, err := lock.Lock(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Warn("leaderelection: try-lock failed", "name", cfg.Name, "error", err)
		}
		return false
	}
	if !acquired {
		return false
	}
	log.Info("leaderelection: elected leader", "name", cfg.Name)

	// Cancelled when leadership ends for any reason; unblocks onElected.
	leaderCtx, stepDown := context.WithCancel(ctx)
	defer stepDown()

	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		heartbeatLoop(leaderCtx, &lock, heartbeat, cfg, stepDown)
	}()

	onElected(leaderCtx)

	// Stop the heartbeat and wait for it to stop touching the connection before
	// we use it again — *sql.Conn is not safe for concurrent use.
	stepDown()
	<-hbDone

	// Explicit release so a same-process re-contention (or another replica)
	// takes over immediately. A dead connection makes this fail harmlessly —
	// Postgres already freed the lock when the session dropped.
	if err := lock.Unlock(context.WithoutCancel(ctx)); err != nil {
		log.Warn("leaderelection: unlock failed (lock frees on connection close)", "name", cfg.Name, "error", err)
	}
	log.Info("leaderelection: released leadership", "name", cfg.Name)
	return true
}

// heartbeatLoop verifies the lock connection is still alive while we lead. The
// go-pglock handle hides its connection, so liveness is probed by re-taking the
// lock (a no-op that stacks within our own session) and immediately releasing
// the extra hold — any error means the connection died, so Postgres already
// released the lock and we must step down before a second replica takes over.
func heartbeatLoop(ctx context.Context, lock *pglock.Lock, interval time.Duration, cfg Config, stepDown func()) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := lock.Lock(ctx); err != nil {
				if ctx.Err() == nil {
					cfg.Logger.Warn("leaderelection: lost leadership (connection unhealthy)", "name", cfg.Name, "error", err)
					stepDown()
				}
				return
			}
			if err := lock.Unlock(ctx); err != nil {
				if ctx.Err() == nil {
					cfg.Logger.Warn("leaderelection: lost leadership (connection unhealthy)", "name", cfg.Name, "error", err)
					stepDown()
				}
				return
			}
		}
	}
}
