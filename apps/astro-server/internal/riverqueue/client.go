package riverqueue

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

// Config holds dependencies that River workers need.
type Config struct {
	DB       *sql.DB
	OMClient *openmeter.Client
	Logger   *logger.Logger
}

// Queue wraps a River client and its pgxpool connection.
type Queue struct {
	pool   *pgxpool.Pool
	client *river.Client[pgx.Tx]
	log    *logger.Logger
}

// New creates a Queue: opens a pgxpool, registers workers, and builds the River client.
// The River schema tables must already exist (managed via Bytebase).
func New(ctx context.Context, databaseURL string, cfg Config) (*Queue, error) {
	// River tables live in the "river" schema; set search_path so River finds
	// its tables there while workers can still query public tables.
	u, err := url.Parse(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("riverqueue: parse url: %w", err)
	}
	q := u.Query()
	q.Set("search_path", "river,public")
	u.RawQuery = q.Encode()

	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		return nil, fmt.Errorf("riverqueue: pgxpool: %w", err)
	}

	workers := river.NewWorkers()
	addWorkers(workers, cfg)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers:      workers,
		PeriodicJobs: periodicJobs(),
		Logger:       cfg.Logger.Logger,
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("riverqueue: client: %w", err)
	}

	return &Queue{
		pool:   pool,
		client: riverClient,
		log:    cfg.Logger,
	}, nil
}

// Start starts the River client (begins processing jobs).
func (q *Queue) Start(ctx context.Context) error {
	if err := q.client.Start(ctx); err != nil {
		return fmt.Errorf("riverqueue: start: %w", err)
	}
	q.log.Info("River queue started")
	return nil
}

// Stop gracefully drains in-flight jobs and closes the pool.
func (q *Queue) Stop(ctx context.Context) error {
	if err := q.client.Stop(ctx); err != nil {
		q.log.Error("River queue stop error", "error", err)
	}
	q.pool.Close()
	q.log.Info("River queue stopped")
	return nil
}
