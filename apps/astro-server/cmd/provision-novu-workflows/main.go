// provision-novu-workflows creates the notification workflows in Novu, one per
// notify.Type. Templates are generic and payload-driven (subject/body/ctaUrl
// come from the trigger payload), so each workflow differs only in identity,
// category (tag), the critical flag, and default channels. Copy is rendered by
// the backend; email bodies are skeletons meant to be refined in the dashboard.
//
// Safe to re-run — it skips workflows that already exist, so it never clobbers
// email templates polished in the dashboard.
//
// Usage:
//
//	NOVU_API_URL=https://api.novu.astroids.ai NOVU_SECRET_KEY=... go run ./cmd/provision-novu-workflows
//
// Optional:
//
//	DRY_RUN=true — log what would be created without calling Novu
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/novu"
)

// spec is a workflow to provision: its Type (used as the Novu workflow id),
// display name, description, category (tag), critical flag, and default channels.
type spec struct {
	Type        notify.Type
	Name        string
	Description string
	Category    string
	Critical    bool
	Email       bool
	InApp       bool
}

// workflows is the full set to provision, mirroring docs/01-spec/notifications-spec.md
// (§Novu workflow configuration). Defaults match the alert-source catalog.
var workflows = []spec{
	{notify.TypeSystemTest, "Test notification", "A test notification to verify your channels are working.", "System", true, true, true},

	{notify.TypeBuildFailed, "Build failed", "A container image build for one of your agents failed.", "Deployments", false, true, true},

	{notify.TypeBillingPaymentFailed, "Payment failed", "A payment could not be collected.", "Billing", true, true, true},
	{notify.TypeBillingActionRequired, "Payment action required", "A payment needs additional authentication to complete.", "Billing", true, true, true},
	{notify.TypeBillingSpendThreshold, "Spend threshold reached", "Account usage crossed a configured spend threshold.", "Billing", false, true, true},
	{notify.TypeBillingSuspended, "Account suspended", "The account was suspended after the payment grace period elapsed.", "Billing", true, true, true},
	{notify.TypeBillingRecovered, "Payment recovered", "A previously failed payment was collected and the account is active.", "Billing", false, false, true},

	{notify.TypeTeamMemberChanged, "Team membership changes", "A member was added, removed, or had their role changed.", "Team", false, false, true},
	{notify.TypeOwnershipTransferred, "Ownership transferred", "Account or agent ownership was transferred.", "Team", false, true, true},

	{notify.TypeSecurityKeyChanged, "API key changes", "An API key or token was created or revoked.", "Security", true, true, true},

	{notify.TypeObservationMemoryOverBudget, "Memory over budget", "An agent's memory use stayed over its budget.", "Observability", false, true, true},
	{notify.TypeObservationComputeOverBudget, "Compute over budget", "An agent's CPU use stayed at its limit.", "Observability", false, false, true},
	{notify.TypeObservationCrashLoop, "Crash loop / OOM", "An agent is repeatedly crashing or being OOM-killed.", "Observability", false, true, true},
	{notify.TypeObservationErrorSpike, "Error-rate spike", "An agent's error rate crossed its threshold.", "Observability", false, false, true},
}

func main() {
	apiURL := os.Getenv("NOVU_API_URL")
	secret := os.Getenv("NOVU_SECRET_KEY")
	dryRun := os.Getenv("DRY_RUN") == "true"

	if apiURL == "" || secret == "" {
		log.Fatal("NOVU_API_URL and NOVU_SECRET_KEY are required")
	}
	client := novu.NewClient(apiURL, secret)
	ctx := context.Background()

	var created, skipped int
	for _, w := range workflows {
		id := string(w.Type)
		props := notify.PayloadProperties(w.Type)
		exists, err := client.WorkflowExists(ctx, id)
		if err != nil {
			log.Fatalf("check %s: %v", id, err)
		}
		if dryRun {
			verb := "create"
			if exists {
				verb = "schema-only"
			}
			fmt.Printf("%s %s (dry-run) — %s [%s] props=%v\n", verb, id, w.Name, w.Category, props)
			continue
		}
		if exists {
			fmt.Printf("skip   %s (exists) — updating payload schema\n", id)
			skipped++
		} else {
			if err := client.CreateWorkflow(ctx, novu.WorkflowSpec{
				WorkflowID:   id,
				Name:         w.Name,
				Description:  w.Description,
				Category:     w.Category,
				Critical:     w.Critical,
				EmailDefault: w.Email,
				InAppDefault: w.InApp,
			}); err != nil {
				log.Fatalf("create %s: %v", id, err)
			}
			fmt.Printf("create %s — %s [%s]\n", id, w.Name, w.Category)
			created++
		}
		// Always (re)upload the payload schema — additive and template-safe.
		if err := client.SetWorkflowPayloadSchema(ctx, id, props); err != nil {
			log.Fatalf("set payload schema %s: %v", id, err)
		}
	}
	fmt.Printf("\ndone: %d created, %d schema-updated\n", created, skipped)
}
