package dbhealth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubPinger struct{ err error }

func (s *stubPinger) PingContext(context.Context) error { return s.err }

type capturedLog struct {
	warns []string
	infos []string
}

func (c *capturedLog) Warn(msg string, _ ...any) { c.warns = append(c.warns, msg) }
func (c *capturedLog) Info(msg string, _ ...any) { c.infos = append(c.infos, msg) }

func TestSilentWhileHealthy(t *testing.T) {
	log := &capturedLog{}
	m := New(&stubPinger{}, log, time.Second)

	m.check(context.Background())
	m.check(context.Background())

	if len(log.warns)+len(log.infos) != 0 {
		t.Fatalf("expected no output, got warns=%v infos=%v", log.warns, log.infos)
	}
}

// One line per outage, not per tick, or the signal drowns the stream.
func TestWarnsOnceForAContinuousOutage(t *testing.T) {
	db := &stubPinger{err: errors.New("dial timeout")}
	log := &capturedLog{}
	m := New(db, log, time.Second)

	for range 5 {
		m.check(context.Background())
	}

	if len(log.warns) != 1 {
		t.Fatalf("expected 1 warn, got %d: %v", len(log.warns), log.warns)
	}
}

func TestReportsRecovery(t *testing.T) {
	db := &stubPinger{err: errors.New("dial timeout")}
	log := &capturedLog{}
	m := New(db, log, time.Second)

	m.check(context.Background())
	db.err = nil
	m.check(context.Background())
	m.check(context.Background())

	if len(log.warns) != 1 || len(log.infos) != 1 {
		t.Fatalf("expected 1 warn and 1 info, got warns=%v infos=%v", log.warns, log.infos)
	}
}

func TestRunStopsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := New(&stubPinger{}, &capturedLog{}, time.Millisecond)

	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestZeroIntervalFallsBackToTheDefault(t *testing.T) {
	if got := New(&stubPinger{}, &capturedLog{}, 0).interval; got != DefaultInterval {
		t.Fatalf("interval %v, want %v", got, DefaultInterval)
	}
}
