package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-queen/internal/client"
	"github.com/astropods/astro/apps/astro-queen/internal/config"
)

var (
	iconDomainDays  int32
	iconDomainLimit int32
	iconDomainJSON  bool
)

func newIconsCmd(env, addr string) *cobra.Command {
	iconsCmd := &cobra.Command{
		Use:   "icons",
		Short: "Brand-icon coverage tooling",
	}

	domainsCmd := &cobra.Command{
		Use:   "domains",
		Short: fmt.Sprintf("List outbound domains agents call on %s", env),
		Long: "Aggregates every destination agents call, fleet-wide, with how often it is\n" +
			"hit and how many deployments hit it. Pipe --json into the brand-icons\n" +
			"filter script to see which ones have no icon yet:\n\n" +
			"  queen " + env + " icons domains --json > /tmp/domains.json\n" +
			"  bun run --cwd packages/astro-brand-icons unresolved /tmp/domains.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg.Server = addr
			cfg.Insecure = env == "local"

			c, err := client.New(cfg)
			if err != nil {
				return fmt.Errorf("connect to %s: %w", addr, err)
			}
			defer c.Close() //nolint:errcheck

			// A fleet-wide aggregation over a month of high-cardinality series is
			// slow by nature; well past the default gRPC patience.
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()

			resp, err := c.AdminService().ListOutboundDomains(ctx, &adminv1.ListOutboundDomainsRequest{
				Days:  iconDomainDays,
				Limit: iconDomainLimit,
			})
			if err != nil {
				return fmt.Errorf("list outbound domains: %w", err)
			}

			if iconDomainJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(resp)
			}

			if len(resp.Domains) == 0 {
				fmt.Printf("No outbound traffic recorded over %s.\n", resp.Window)
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
			fmt.Fprintln(w, "DOMAIN\tREQUESTS\tDEPLOYMENTS\tHOSTS") //nolint:errcheck
			for _, d := range resp.Domains {
				fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", d.Domain, d.RequestCount, d.DeploymentCount, d.HostCount) //nolint:errcheck
			}
			if err := w.Flush(); err != nil {
				return err
			}
			fmt.Printf("\n%d domains over %s\n", len(resp.Domains), resp.Window)
			return nil
		},
	}

	domainsCmd.Flags().Int32Var(&iconDomainDays, "days", 30, "Lookback window in days")
	domainsCmd.Flags().Int32Var(&iconDomainLimit, "limit", 200, "Max domains to return, ranked by request count")
	domainsCmd.Flags().BoolVar(&iconDomainJSON, "json", false, "Emit JSON for piping into the filter script")

	iconsCmd.AddCommand(domainsCmd)
	return iconsCmd
}
