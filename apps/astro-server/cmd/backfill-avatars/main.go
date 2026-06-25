// backfill-avatars assigns a deterministic preset avatar for every existing
// account that doesn't already have one. Safe to re-run — it skips accounts
// whose avatar already exists.
//
// Local usage (filesystem):
//
//	DATABASE_URL=postgres://... ASSETS_LOCAL_DIR=../../assets go run ./cmd/backfill-avatars
//
// Production usage (S3):
//
//	DATABASE_URL=postgres://... ASSETS_BUCKET=prod-astro-assets go run ./cmd/backfill-avatars
//
// Optional env vars:
//
//	DRY_RUN=true     — log what would happen without writing
//	BATCH_SIZE=100   — accounts per query batch (default 100)
package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strconv"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/avatar"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	bucket := os.Getenv("ASSETS_BUCKET")
	localDir := os.Getenv("ASSETS_LOCAL_DIR")
	if bucket == "" && localDir == "" {
		log.Fatal("Either ASSETS_BUCKET or ASSETS_LOCAL_DIR is required")
	}

	dryRun := os.Getenv("DRY_RUN") == "true"
	batchSize := 100
	if v := os.Getenv("BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			batchSize = n
		}
	}

	ctx := context.Background()

	// Open database
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Initialize avatar store with appropriate backend
	var backend avatar.Backend
	if localDir != "" {
		backend = avatar.NewLocalBackend(localDir)
		log.Printf("Using local filesystem backend: %q", localDir) //nolint:gosec // env var, not user input
	} else {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
		if err != nil {
			log.Fatalf("Failed to load AWS config: %v", err)
		}
		backend = avatar.NewS3Backend(s3.NewFromConfig(awsCfg), bucket)
		log.Printf("Using S3 backend: %q", bucket) //nolint:gosec // env var, not user input
	}
	avatarStore := avatar.NewStore(backend, "")

	log.Printf("Starting avatar backfill (dry_run=%t, batch_size=%d)", dryRun, batchSize) //nolint:gosec // env-derived values, not user input

	var totalProcessed, totalSkipped, totalFailed int
	var lastID string

	for {
		query := `
			SELECT id, name FROM accounts
			WHERE ($1 = '' OR id > $1::uuid)
			ORDER BY id
			LIMIT $2
		`
		rows, err := db.QueryContext(ctx, query, lastID, batchSize)
		if err != nil {
			log.Fatalf("Failed to query accounts: %v", err)
		}

		var batchCount int
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				log.Printf("ERROR: failed to scan row: %v", err)
				continue
			}
			lastID = id
			batchCount++

			// Check if avatar already exists
			exists, err := avatarStore.AvatarExists(ctx, name)
			if err != nil {
				log.Printf("ERROR: failed to check avatar for %s: %v", name, err)
				totalFailed++
				continue
			}
			if exists {
				totalSkipped++
				continue
			}

			if dryRun {
				log.Printf("DRY RUN: would assign preset %d to %s (%s)", avatar.PresetIndex(name), name, id)
				totalProcessed++
				continue
			}

			if err := avatarStore.AssignPreset(ctx, name); err != nil {
				log.Printf("ERROR: failed to assign preset for %s (%s): %v", name, id, err)
				totalFailed++
				continue
			}
			if _, err := db.ExecContext(ctx, `UPDATE accounts SET avatar_updated_at = now() WHERE id = $1::uuid`, id); err != nil {
				log.Printf("WARN: failed to stamp avatar_updated_at for %s (%s): %v", name, id, err)
			}
			totalProcessed++
		}
		_ = rows.Close()

		if batchCount == 0 {
			break
		}

		log.Printf("Progress: processed=%d skipped=%d failed=%d (last_id=%s)",
			totalProcessed, totalSkipped, totalFailed, lastID)

		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("Done! processed=%d skipped=%d failed=%d", totalProcessed, totalSkipped, totalFailed)
}
