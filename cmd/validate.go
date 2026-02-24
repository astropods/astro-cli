package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	spec "github.com/postman/astro/packages/astro-spec"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate astroai.yml against the spec schema",
	Long: `Validate the astroai.yml spec file strictly against the JSON schema.
Reports all schema violations and semantic errors.

Example:
  ast validate
  ast validate -f custom-spec.yml`,
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	specFile, _ := cmd.Flags().GetString("file")

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath := specFile
	if !filepath.IsAbs(specFile) {
		specPath = filepath.Join(workingDir, specFile)
	}
	data, err := os.ReadFile(specPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", specFile, err)
	}

	fmt.Println()
	fmt.Printf("%s%sValidating %s...%s\n\n", colorBold, colorBlue, specFile, colorReset)

	var errs []string

	// YAML parse check
	var raw interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		fmt.Printf("%s✗%s YAML syntax error: %v\n\n", colorRed, colorReset, err)
		return fmt.Errorf("validation failed")
	}

	// JSON schema validation
	errs = append(errs, schemaValidationErrors(raw)...)

	// Semantic validation (required fields, mutual exclusions, etc.)
	if _, semErr := spec.ParseSpec(specPath); semErr != nil {
		errs = append(errs, semErr.Error())
	}

	if len(errs) == 0 {
		fmt.Printf("%s✓%s %s is valid\n\n", colorGreen, colorReset, specFile)
		return nil
	}

	for _, e := range errs {
		fmt.Printf("  %s✗%s %s\n", colorRed, colorReset, e)
	}
	fmt.Println()
	fmt.Printf("%s%d error(s) found%s\n\n", colorRed, len(errs), colorReset)
	return fmt.Errorf("validation failed")
}

func schemaValidationErrors(raw interface{}) []string {
	// YAML → JSON to get standard JSON types expected by the validator
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return []string{fmt.Sprintf("failed to convert spec to JSON: %v", err)}
	}
	var jsonVal interface{}
	if err := json.Unmarshal(jsonBytes, &jsonVal); err != nil {
		return []string{fmt.Sprintf("failed to re-parse spec as JSON: %v", err)}
	}

	var schemaDoc interface{}
	if err := json.Unmarshal(spec.Schema(), &schemaDoc); err != nil {
		return []string{fmt.Sprintf("failed to parse schema: %v", err)}
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("astroai.schema.json", schemaDoc); err != nil {
		return []string{fmt.Sprintf("failed to load schema: %v", err)}
	}
	schema, err := c.Compile("astroai.schema.json")
	if err != nil {
		return []string{fmt.Sprintf("failed to compile schema: %v", err)}
	}

	valErr := schema.Validate(jsonVal)
	if valErr == nil {
		return nil
	}

	var ve *jsonschema.ValidationError
	if !errors.As(valErr, &ve) {
		return []string{valErr.Error()}
	}

	return collectSchemaErrors(ve)
}

// collectSchemaErrors extracts individual error messages from the flattened basic output.
func collectSchemaErrors(ve *jsonschema.ValidationError) []string {
	basic := ve.BasicOutput()

	var results []string
	for _, unit := range basic.Errors {
		if unit.Error == nil {
			continue
		}
		msgBytes, err := json.Marshal(unit.Error)
		if err != nil {
			continue
		}
		msg := strings.Trim(string(msgBytes), `"`)
		if msg == "" {
			continue
		}
		loc := unit.InstanceLocation
		if loc == "" || loc == "/" {
			loc = "(root)"
		}
		results = append(results, fmt.Sprintf("%s: %s", loc, msg))
	}

	// Fall back to the top-level error string if no leaf errors were extracted
	if len(results) == 0 {
		results = append(results, ve.Error())
	}
	return results
}
