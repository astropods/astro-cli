package riverqueue

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"riverqueue.com/riverui"
)

// UIHandler creates an internal River UI HTTP handler.
// It opens its own pgxpool and creates a read-only River client (no workers).
// The returned cleanup function closes the pool.
func UIHandler(ctx context.Context, databaseURL string, logger *slog.Logger) (http.Handler, func(), error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("riverui: pgxpool: %w", err)
	}

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: "river",
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("riverui: river client: %w", err)
	}

	endpoints := riverui.NewEndpoints(riverClient, nil)

	handler, err := riverui.NewHandler(&riverui.HandlerOpts{
		Endpoints: endpoints,
		Logger:    logger,
		Prefix:    "/riverui",
	})
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("riverui: handler: %w", err)
	}

	if err := handler.Start(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("riverui: start: %w", err)
	}

	return handler, func() { pool.Close() }, nil
}
