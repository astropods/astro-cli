// backfill-owner-roles promotes every account owner whose WorkOS role cannot
// administer the account they own. Safe to re-run: an owner already holding
// admin is left alone.
//
// Usage:
//
//	DATABASE_URL=postgres://... WORKOS_API_KEY=sk_... go run ./cmd/backfill-owner-roles
//
// Optional env vars:
//
//	DRY_RUN=true     report what would change without writing to WorkOS
package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/org"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	apiKey := os.Getenv("WORKOS_API_KEY")
	if apiKey == "" {
		log.Fatal("WORKOS_API_KEY is required")
	}
	dryRun := os.Getenv("DRY_RUN") == "true"

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close() //nolint:errcheck

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to reach database: %v", err)
	}

	sync := org.NewSync(org.NewClient(apiKey), account.NewAccountStore(db), nil, db, logger.New("info", "text"))

	result, err := sync.BackfillOwnerRoles(context.Background(), dryRun)
	if err != nil {
		log.Fatalf("Backfill failed: %v", err)
	}

	verb := "repaired"
	if dryRun {
		verb = "would repair"
	}
	log.Printf("accounts=%d owners=%d %s=%d unchanged=%d no_membership=%d failed=%d",
		result.Accounts, result.Owners, verb, result.Repaired,
		result.Unchanged, result.NoMembership, result.Failed)
	if result.Failed > 0 {
		os.Exit(1)
	}
}
