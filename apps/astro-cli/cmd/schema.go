package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	spec "github.com/postman/astro/packages/astro-spec"
)

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Print the JSON Schema for astroai.yml",
	Long: `Print the JSON Schema for astroai.yml to stdout.

Use this to enable IDE autocomplete and validation:

  ast schema > .ast/schema.json

Then in VS Code settings (settings.json):

  { "yaml.schemas": { ".ast/schema.json": "astroai.yml" } }

Or add a comment to the top of your astroai.yml:

  # yaml-language-server: $schema=https://your-server/schema/astroai.json`,
	RunE: runSchema,
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}

func runSchema(cmd *cobra.Command, args []string) error {
	_, err := fmt.Fprint(os.Stdout, string(spec.Schema()))
	return err
}
