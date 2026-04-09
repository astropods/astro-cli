// cleanup-pending-members removes local account_members rows for organization
// accounts where the WorkOS membership is still pending (invited but not
// accepted). These stale rows were created by the event consumer before it
// learned to skip pending memberships.
//
// Usage:
//
//	DATABASE_URL=postgres://... WORKOS_API_KEY=sk_... go run ./cmd/cleanup-pending-members
//	DATABASE_URL=postgres://... WORKOS_API_KEY=sk_... DRY_RUN=true go run ./cmd/cleanup-pending-members
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
	"github.com/workos/workos-go/v6/pkg/usermanagement"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	apiKey := os.Getenv("WORKOS_API_KEY")
	dryRun := os.Getenv("DRY_RUN") == "true"

	if dbURL == "" || apiKey == "" {
		log.Fatal("DATABASE_URL and WORKOS_API_KEY are required")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	um := usermanagement.NewClient(apiKey)
	ctx := context.Background()

	// Find all org account members that have a WorkOS membership ID
	rows, err := db.QueryContext(ctx, `
		SELECT am.account_id, am.user_id, amw.workos_membership_id, a.name AS account_name
		FROM account_members am
		JOIN accounts a ON a.id = am.account_id
		JOIN account_member_workos amw ON amw.account_id = am.account_id AND amw.user_id = am.user_id
		WHERE a.type = 'organization'
	`)
	if err != nil {
		log.Fatalf("Failed to query members: %v", err)
	}
	defer rows.Close()

	type candidate struct {
		AccountID    string
		UserID       string
		MembershipID string
		AccountName  string
	}

	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.AccountID, &c.UserID, &c.MembershipID, &c.AccountName); err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("Row iteration error: %v", err)
	}

	log.Printf("Found %d org members with WorkOS memberships to check", len(candidates))

	removed := 0
	for _, c := range candidates {
		m, err := um.GetOrganizationMembership(ctx, usermanagement.GetOrganizationMembershipOpts{
			OrganizationMembership: c.MembershipID,
		})
		if err != nil {
			log.Printf("  WARN: could not fetch membership %s (account=%s, user=%s): %v",
				c.MembershipID, c.AccountName, c.UserID, err)
			continue
		}

		if m.Status != usermanagement.Active {
			if dryRun {
				log.Printf("  DRY RUN: would remove user %s from %s (membership %s, status=%s)",
					c.UserID, c.AccountName, c.MembershipID, m.Status)
			} else {
				// Delete from account_member_workos first (FK), then account_members
				if _, err := db.ExecContext(ctx,
					`DELETE FROM account_member_workos WHERE account_id = $1 AND user_id = $2`,
					c.AccountID, c.UserID); err != nil {
					log.Printf("  ERROR: failed to delete workos link for user %s in %s: %v",
						c.UserID, c.AccountName, err)
					continue
				}
				if _, err := db.ExecContext(ctx,
					`DELETE FROM account_members WHERE account_id = $1 AND user_id = $2`,
					c.AccountID, c.UserID); err != nil {
					log.Printf("  ERROR: failed to delete member %s from %s: %v",
						c.UserID, c.AccountName, err)
					continue
				}
				log.Printf("  REMOVED: user %s from %s (membership %s, status=%s)",
					c.UserID, c.AccountName, c.MembershipID, m.Status)
			}
			removed++
		}
	}

	action := "Removed"
	if dryRun {
		action = "Would remove"
	}
	fmt.Printf("\n%s %d stale pending members out of %d checked.\n", action, removed, len(candidates))
}
