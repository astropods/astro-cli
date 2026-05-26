package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
)

var specCmd = &cobra.Command{
	Use:   "spec",
	Short: "Validate and explain astropods.yml spec files",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var explainCmd = &cobra.Command{
	Use:    "explain",
	Hidden: true,
	Short:  "Explain the agent project based on its spec",
	Long: `Parse astropods.yml and display a human-readable explanation
of the agent project: its components, what variables each component injects into
the agent, and what secrets and inputs are required.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		specPath, workingDir, err := resolveSpecPathAndCwd(flagString(cmd, "file"))
		if err != nil {
			return err
		}
		return runExplain(specPath, workingDir)
	},
}

var repairCmd = &cobra.Command{
	Use:    "repair",
	Short:  "Check and repair project files against the template",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Best-effort: repair handles missing spec with its own fallback logic.
		// workingDir="" means os.Getwd failed — that is fatal even for repair.
		specPath, workingDir, _ := resolveSpecPathAndCwd(flagString(cmd, "file"))
		if workingDir == "" {
			return fmt.Errorf("failed to get working directory")
		}
		return runRepair(cmd.OutOrStdout(), specPath, workingDir, flagBool(cmd, "yes"))
	},
}

var specValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate astropods.yml against the spec schema",
	Long: `Validate astropods.yml against the spec schema.
Reports all schema violations and semantic errors.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		specPath, _, err := resolveSpecPathAndCwd(flagString(cmd, "file"))
		if err != nil {
			return err
		}
		return runValidate(specPath)
	},
}

func init() {
	rootCmd.AddCommand(specCmd)

	specCmd.PersistentFlags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")

	repairCmd.Flags().BoolP("yes", "y", false, "Update all outdated files without prompting")
	specCmd.AddCommand(repairCmd)

	specCmd.AddCommand(specValidateCmd)

	explainCmd.Example = fmt.Sprintf(`  %[1]s spec explain
  %[1]s spec explain -f /path/to/astropods.yml`, buildinfo.BinaryName)
	specCmd.AddCommand(explainCmd)
}
