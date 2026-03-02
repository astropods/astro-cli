package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"text/tabwriter"

	"github.com/postman/astro/apps/astro-queen/internal/devenv"
	"github.com/spf13/cobra"
)

var (
	devenvOwner string
	devenvRepo  string
)

var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,19}$`)

var devenvCmd = &cobra.Command{
	Use:   "devenv",
	Short: "Manage Astropod developer environments",
}

func init() {
	devenvCmd.PersistentFlags().StringVar(&devenvOwner, "owner", envOrDefault("DEVENV_OWNER", "astropods"), "GitHub owner")
	devenvCmd.PersistentFlags().StringVar(&devenvRepo, "repo", envOrDefault("DEVENV_REPO", "astro-infra"), "GitHub repo")

	devenvCmd.AddCommand(devenvShowCmd())
	devenvCmd.AddCommand(devenvDispatchCmd("create", "Create a new dev environment"))
	devenvCmd.AddCommand(devenvDispatchCmd("destroy", "Destroy a dev environment"))
	devenvCmd.AddCommand(devenvDispatchCmd("reset-images", "Refresh ECR images for a dev environment"))
	devenvCmd.AddCommand(devenvDispatchCmd("reset-db", "Reset databases for a dev environment"))
	devenvCmd.AddCommand(devenvDispatchCmd("restart", "Restart (scale nodes up) for a dev environment"))
	devenvCmd.AddCommand(devenvDispatchCmd("plan", "Terraform plan (dry run) for a dev environment"))
	devenvCmd.AddCommand(devenvStatusCmd())

	rootCmd.AddCommand(devenvCmd)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func newDevenvClient() (*devenv.Client, error) {
	return devenv.NewClient(devenvOwner, devenvRepo)
}

func devenvShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show your dev environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newDevenvClient()
			if err != nil {
				return err
			}
			ctx := context.Background()

			user, err := client.CurrentUser(ctx)
			if err != nil {
				return err
			}

			registry, err := client.FetchRegistry(ctx)
			if err != nil {
				return err
			}

			idx, exists := registry[user]
			if !exists {
				fmt.Printf("No dev environment found for %s.\n", user)
				return nil
			}

			envs := devenv.RegistryToEnvList(map[string]int{user: idx})
			e := envs[0]

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "Name:\t%s\n", e.Name)
			fmt.Fprintf(w, "Index:\t%d\n", e.Index)
			fmt.Fprintf(w, "Primary CIDR:\t%s\n", e.PrimaryCIDR)
			fmt.Fprintf(w, "Managed CIDR:\t%s\n", e.ManagedCIDR)
			fmt.Fprintf(w, "Domain:\t%s\n", e.Domain)
			return w.Flush()
		},
	}
}

func devenvDispatchCmd(action, description string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <name>",
		Short: description,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !nameRegex.MatchString(name) {
				return fmt.Errorf("invalid name %q: must match ^[a-z][a-z0-9-]{0,19}$", name)
			}

			client, err := newDevenvClient()
			if err != nil {
				return err
			}
			ctx := context.Background()

			registry, err := client.FetchRegistry(ctx)
			if err != nil {
				return err
			}
			_, exists := registry[name]
			if action == "create" && exists {
				return fmt.Errorf("environment %q already exists", name)
			}
			if action != "create" && !exists {
				return fmt.Errorf("environment %q does not exist", name)
			}

			fmt.Printf("Dispatching %s for %q...\n", action, name)
			if err := client.DispatchWorkflow(ctx, name, action); err != nil {
				return fmt.Errorf("dispatching workflow: %w", err)
			}

			fmt.Println("Finding workflow run...")
			run, err := client.FindWorkflowRun(ctx, name, action)
			if err != nil {
				return err
			}
			fmt.Printf("Workflow run: %s\n\n", run.HTMLURL)

			return client.PollWorkflowRun(ctx, run.ID, func(rs devenv.RunStatus) {
				fmt.Print("\r\033[K")
				for _, j := range rs.Jobs {
					icon := statusIcon(j.Status, j.Conclusion)
					fmt.Printf("  %s %s", icon, j.Name)
					if j.Conclusion != "" && j.Conclusion != "success" {
						fmt.Printf(" (%s)", j.Conclusion)
					}
					fmt.Println()
				}
				if rs.Status != "completed" {
					fmt.Printf("\033[%dA", len(rs.Jobs))
				}
			})
		},
	}
}

func devenvStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <run-id>",
		Short: "Check a workflow run status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var runID int64
			if _, err := fmt.Sscanf(args[0], "%d", &runID); err != nil {
				return fmt.Errorf("invalid run ID %q: must be a number", args[0])
			}

			client, err := newDevenvClient()
			if err != nil {
				return err
			}

			return client.PollWorkflowRun(context.Background(), runID, func(rs devenv.RunStatus) {
				fmt.Print("\r\033[K")
				for _, j := range rs.Jobs {
					icon := statusIcon(j.Status, j.Conclusion)
					fmt.Printf("  %s %s", icon, j.Name)
					if j.Conclusion != "" && j.Conclusion != "success" {
						fmt.Printf(" (%s)", j.Conclusion)
					}
					fmt.Println()
				}
				if rs.Status != "completed" {
					fmt.Printf("\033[%dA", len(rs.Jobs))
				}
			})
		},
	}
}

func statusIcon(status, conclusion string) string {
	switch {
	case status == "completed" && conclusion == "success":
		return "ok"
	case status == "completed" && conclusion == "skipped":
		return "--"
	case status == "completed":
		return "FAIL"
	case status == "in_progress":
		return ".."
	default:
		return "  "
	}
}
