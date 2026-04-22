package riverqueue

import (
	"context"
	"encoding/json"

	"github.com/astropods/astro/apps/astro-server/internal/colorextract"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// colorBackfillItem represents a single entity that needs color extraction.
type colorBackfillItem struct {
	readAvatar  func(ctx context.Context) ([]byte, error)
	storeColors func(ctx context.Context, colorsJSON []byte) error
	logAttrs    []any
	// skipOnReadError treats a read failure as "skipped" (avatar may not exist
	// yet) rather than "failed" (avatar should exist).
	skipOnReadError bool
}

// backfillColors runs a paginated color extraction loop. fetchPage returns the
// next batch of items; an empty slice signals completion.
func backfillColors(
	ctx context.Context,
	log *logger.Logger,
	label string,
	fetchPage func(ctx context.Context) ([]colorBackfillItem, error),
) (processed, skipped, failed int) {
	for {
		page, err := fetchPage(ctx)
		if err != nil {
			log.Error(label+": fetch page", "error", err)
			return
		}
		if len(page) == 0 {
			break
		}
		for _, item := range page {
			data, err := item.readAvatar(ctx)
			if err != nil {
				if item.skipOnReadError {
					skipped++
				} else {
					log.Warn(label+": read avatar", append([]any{"error", err}, item.logAttrs...)...)
					failed++
				}
				continue
			}
			colors, err := colorextract.ExtractFromJPEG(data)
			if err != nil {
				log.Warn(label+": extract colors", append([]any{"error", err}, item.logAttrs...)...)
				failed++
				continue
			}
			colorsJSON, _ := json.Marshal(colors)
			if err := item.storeColors(ctx, colorsJSON); err != nil {
				log.Warn(label+": store colors", append([]any{"error", err}, item.logAttrs...)...)
				failed++
				continue
			}
			processed++
		}
	}
	return
}
