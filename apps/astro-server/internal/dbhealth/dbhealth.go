// Package dbhealth reports database reachability through the log stream.
//
// Readiness must not answer this: every replica shares one instance, so a probe
// that checks it empties the Service at once.
package dbhealth

import (
	"context"
	"time"
)

// Pinger is the subset of *sql.DB this needs.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// Logger is the subset of the server logger this needs.
type Logger interface {
	Warn(msg string, args ...any)
	Info(msg string, args ...any)
}

const (
	// DefaultInterval keeps an outage inside one scrape window.
	DefaultInterval = 15 * time.Second
	pingTimeout     = 5 * time.Second
)

// Monitor logs database reachability on every change of state.
type Monitor struct {
	db       Pinger
	log      Logger
	interval time.Duration
	healthy  bool
}

func New(db Pinger, log Logger, interval time.Duration) *Monitor {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Monitor{db: db, log: log, interval: interval, healthy: true}
}

// Run blocks until ctx is done.
func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *Monitor) check(ctx context.Context) {
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	err := m.db.PingContext(pingCtx)
	cancel()

	switch {
	case err != nil && m.healthy:
		m.healthy = false
		m.log.Warn("database: unreachable", "error", err)
	case err == nil && !m.healthy:
		m.healthy = true
		m.log.Info("database: reachable again")
	}
}
