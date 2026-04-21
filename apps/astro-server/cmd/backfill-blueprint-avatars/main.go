// backfill-blueprint-avatars ensures every agent blueprint has a generated
// placeholder avatar, and every deployment inherits its blueprint's avatar.
// Safe to re-run — skips anything already present.
//
// Local usage (filesystem):
//
//	DATABASE_URL=postgres://... ASSETS_LOCAL_DIR=../../assets go run ./cmd/backfill-blueprint-avatars
//
// Production usage (S3):
//
//	DATABASE_URL=postgres://... ASSETS_BUCKET=prod-astro-assets go run ./cmd/backfill-blueprint-avatars
//
// Optional env vars:
//
//	DRY_RUN=true     — log what would happen without writing
//	BATCH_SIZE=100   — rows per query batch (default 100)
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
	"github.com/astropods/astro/apps/astro-server/internal/identitygen"
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

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

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

	log.Printf("Starting blueprint avatar backfill (dry_run=%t, batch_size=%d)", dryRun, batchSize) //nolint:gosec // env-derived values, not user input

	bp := backfillBlueprints(ctx, db, avatarStore, batchSize, dryRun)
	dep := backfillDeployments(ctx, db, avatarStore, batchSize, dryRun)

	log.Printf("Blueprints: processed=%d skipped=%d failed=%d", bp.processed, bp.skipped, bp.failed)
	log.Printf("Deployments: processed=%d skipped=%d failed=%d", dep.processed, dep.skipped, dep.failed)
}

type counts struct{ processed, skipped, failed int }

func backfillBlueprints(ctx context.Context, db *sql.DB, store *avatar.Store, batchSize int, dryRun bool) counts {
	var c counts
	var (
		lastAccountID string
		lastName      string
	)
	for {
		rows, err := db.QueryContext(ctx, `
			SELECT a.account_id::text, a.name, acc.name
			FROM agents a
			JOIN accounts acc ON acc.id = a.account_id
			WHERE a.archived_at IS NULL
			  AND ($1 = '' OR (a.account_id, a.name) > ($1::uuid, $2))
			ORDER BY a.account_id, a.name
			LIMIT $3
		`, lastAccountID, lastName, batchSize)
		if err != nil {
			log.Fatalf("query agents: %v", err)
		}

		var batchCount int
		for rows.Next() {
			var accountID, agentName, accountName string
			if err := rows.Scan(&accountID, &agentName, &accountName); err != nil {
				log.Printf("ERROR: scan agent row: %v", err)
				continue
			}
			lastAccountID, lastName = accountID, agentName
			batchCount++

			exists, err := store.AgentAvatarExists(ctx, accountName, agentName)
			if err != nil {
				log.Printf("ERROR: exists check for %s/%s: %v", accountName, agentName, err)
				c.failed++
				continue
			}
			if exists {
				c.skipped++
				continue
			}

			if dryRun {
				log.Printf("DRY RUN: would generate avatar for %s/%s", accountName, agentName)
				c.processed++
				continue
			}

			jpegBytes, err := identitygen.GenerateIdentityJPEG(identitygen.IdentityOptions{
				Seed: accountName + "/" + agentName,
			})
			if err != nil {
				log.Printf("ERROR: generate %s/%s: %v", accountName, agentName, err)
				c.failed++
				continue
			}
			if err := store.WriteAgentAvatarJPEG(ctx, accountName, agentName, jpegBytes); err != nil {
				log.Printf("ERROR: upload %s/%s: %v", accountName, agentName, err)
				c.failed++
				continue
			}
			c.processed++
		}
		_ = rows.Close()

		if batchCount == 0 {
			break
		}
		log.Printf("Blueprint progress: processed=%d skipped=%d failed=%d (last=%s/%s)",
			c.processed, c.skipped, c.failed, lastAccountID, lastName)
		time.Sleep(100 * time.Millisecond)
	}
	return c
}

func backfillDeployments(ctx context.Context, db *sql.DB, store *avatar.Store, batchSize int, dryRun bool) counts {
	var c counts
	var lastID string
	for {
		rows, err := db.QueryContext(ctx, `
			SELECT d.id, d.agent_name, acc.name
			FROM deployments d
			JOIN accounts acc ON acc.id = d.account_id
			WHERE ($1 = '' OR d.id > $1)
			ORDER BY d.id
			LIMIT $2
		`, lastID, batchSize)
		if err != nil {
			log.Fatalf("query deployments: %v", err)
		}

		var batchCount int
		for rows.Next() {
			var id, agentName, accountName string
			if err := rows.Scan(&id, &agentName, &accountName); err != nil {
				log.Printf("ERROR: scan deployment row: %v", err)
				continue
			}
			lastID = id
			batchCount++

			exists, err := store.DeploymentAvatarExists(ctx, id)
			if err != nil {
				log.Printf("ERROR: deployment exists check %s: %v", id, err)
				c.failed++
				continue
			}
			if exists {
				c.skipped++
				continue
			}

			if dryRun {
				log.Printf("DRY RUN: would copy %s/%s → deployment %s", accountName, agentName, id)
				c.processed++
				continue
			}

			copied, err := store.CopyAgentToDeployment(ctx, accountName, agentName, id)
			if err != nil {
				log.Printf("ERROR: copy to deployment %s: %v", id, err)
				c.failed++
				continue
			}
			if !copied {
				log.Printf("WARN: blueprint avatar missing for deployment %s (%s/%s) — run blueprint pass first", id, accountName, agentName)
				c.failed++
				continue
			}
			c.processed++
		}
		_ = rows.Close()

		if batchCount == 0 {
			break
		}
		log.Printf("Deployment progress: processed=%d skipped=%d failed=%d (last=%s)",
			c.processed, c.skipped, c.failed, lastID)
		time.Sleep(100 * time.Millisecond)
	}
	return c
}
