package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
)

// secretsServerURLOverride is set in tests to redirect API calls to a test server.
var secretsServerURLOverride string

var secretNameRe = regexp.MustCompile(`^[A-Za-z0-9_]{4,}$`)

func validateSecretName(name string) error {
	if !secretNameRe.MatchString(name) {
		return fmt.Errorf("secret name must be at least 4 characters and contain only letters, digits, or underscores")
	}
	return nil
}

func secretsBaseURL() string {
	if secretsServerURLOverride != "" {
		return strings.TrimSuffix(secretsServerURLOverride, "/")
	}
	return strings.TrimSuffix(auth.DefaultServerURL, "/")
}

var secretCmd = &cobra.Command{
	Use:     "secrets",
	Aliases: []string{"secret"},
	Short:   "Manage account secrets",
	Long:    "Create, list, update, and delete secrets in the account vault. Values are write-only once set.",
}

var secretListCmd = &cobra.Command{
	Use:   "list",
	Short: "List secrets in the active account vault",
	RunE:  runSecretList,
}

var secretCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new secret",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretCreate,
}

var secretUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update an existing secret's value",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretUpdate,
}

var secretGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get details for a secret or variable",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretGet,
}

var secretDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a secret",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretDelete,
}

var secretImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import variables from a .env file",
	Long: `Import variables from a .env file into the active account vault.

Lines of the form KEY=value are imported as secrets by default.
Use --plain to store all imported variables as plain text.
Use --plain-keys KEY1,KEY2 to mark specific keys as plain text.
Blank values (KEY=) are silently skipped.
Existing variables are skipped unless --overwrite is set.`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretImport,
}

var (
	secretPlain           bool
	secretValues          bool
	secretCreateValue     string
	secretCreateOverwrite bool
	secretUpdateValue     string
	secretImportPlainKeys string
	secretImportOverwrite bool
)

func init() {
	secretCreateCmd.Flags().BoolVar(&secretPlain, "plain", false, "Store as plaintext instead of an encrypted secret")
	secretCreateCmd.Flags().StringVar(&secretCreateValue, "value", "", "Value to set (skips interactive prompt)")
	secretCreateCmd.Flags().BoolVar(&secretCreateOverwrite, "overwrite", false, "Overwrite if the variable already exists")
	secretCreateCmd.Flags().StringP("description", "d", "", "Optional description for the secret")
	secretUpdateCmd.Flags().StringVar(&secretUpdateValue, "value", "", "New value (skips interactive prompt)")
	secretUpdateCmd.Flags().StringP("description", "d", "", "Update the description")
	secretListCmd.Flags().BoolVar(&secretValues, "values", false, "Show variable values")
	secretListCmd.Flags().Bool("json", false, "Output as JSON")
	secretGetCmd.Flags().Bool("json", false, "Output as JSON")
	secretImportCmd.Flags().Bool("plain", false, "Store all imported variables as plain text")
	secretImportCmd.Flags().StringVar(&secretImportPlainKeys, "plain-keys", "", "Comma-separated keys to store as plain text")
	secretImportCmd.Flags().BoolVar(&secretImportOverwrite, "overwrite", false, "Overwrite existing variables")
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretGetCmd)
	secretCmd.AddCommand(secretCreateCmd)
	secretCmd.AddCommand(secretUpdateCmd)
	secretCmd.AddCommand(secretDeleteCmd)
	secretCmd.AddCommand(secretImportCmd)
	rootCmd.AddCommand(secretCmd)
}

// secretVariableMetadata mirrors the server's VariableMetadata response shape.
type secretVariableMetadata struct {
	Name        string    `json:"name"`
	Secret      bool      `json:"secret"`
	Description string    `json:"description"`
	Value       *string   `json:"value,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func runSecretList(cmd *cobra.Command, args []string) error {
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	var result struct {
		Variables []secretVariableMetadata `json:"variables"`
	}
	if _, err := apiCall(
		cmd.Context(),
		http.MethodGet,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables"),
		nil,
		at.Token,
		verbose,
		&result); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)

	if len(result.Variables) == 0 {
		fmt.Fprintf(w, "No secrets found in account %q.\n", at.Account) //nolint:errcheck,gosec
		return nil
	}

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Variables) //nolint:errcheck,gosec
	}

	const dateFmt = "2006-01-02T15:04:05"
	const dateWidth = len(dateFmt)
	nameWidth := len("Name")
	for _, v := range result.Variables {
		if len(v.Name) > nameWidth {
			nameWidth = len(v.Name)
		}
	}

	typeHeader := "Type"
	if secretValues {
		typeHeader = "Value"
	}
	dim.Fprintf(w, "%-*s  %-*s  %s\n", dateWidth, "Updated", nameWidth, "Name", typeHeader) //nolint:errcheck,gosec

	for _, v := range result.Variables {
		date := v.UpdatedAt.Format(dateFmt)
		if v.UpdatedAt.IsZero() {
			date = strings.Repeat("—", dateWidth)
		}

		var typeOrValue string
		if v.Secret {
			if secretValues {
				typeOrValue = "******"
			} else {
				typeOrValue = "secret"
			}
		} else if secretValues && v.Value != nil {
			typeOrValue = *v.Value
		} else {
			typeOrValue = "variable"
		}

		dim.Fprintf(w, "%s", date)                   //nolint:errcheck,gosec
		cyan.Fprintf(w, "  %-*s", nameWidth, v.Name) //nolint:errcheck,gosec
		dim.Fprintf(w, "  %s", typeOrValue)          //nolint:errcheck,gosec
		fmt.Fprintln(w)                              //nolint:errcheck,gosec
	}
	return nil
}

func runSecretCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	if secretCreateValue != "" {
		return runSecretCreateWithValue(cmd, args, secretCreateValue, secretPlain, secretCreateOverwrite)
	}

	echoMode := huh.EchoModePassword
	if secretPlain {
		echoMode = huh.EchoModeNormal
	}

	var value string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(fmt.Sprintf("Value for %q", name)).
				EchoMode(echoMode).
				Value(&value).
				Validate(func(v string) error {
					if v == "" {
						return fmt.Errorf("value cannot be empty")
					}
					return nil
				}),
		),
	)
	if err := form.Run(); err != nil {
		return err
	}

	return runSecretCreateWithValue(cmd, args, value, secretPlain, secretCreateOverwrite)
}

func runSecretCreateWithValue(cmd *cobra.Command, args []string, value string, plain, overwrite bool) error {
	name := args[0]
	if err := validateSecretName(name); err != nil {
		return err
	}

	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	if !overwrite {
		if status, _, err := fetchVariableMeta(cmd.Context(), at, name, verbose); err == nil {
			return fmt.Errorf("%q already exists; use 'ast secrets update' to change its value", name)
		} else if status != http.StatusNotFound {
			return err
		}
	}

	variable := map[string]any{"name": name, "value": value, "secret": !plain}
	if desc, _ := cmd.Flags().GetString("description"); desc != "" {
		variable["description"] = desc
	}

	var result struct {
		Results []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Error  string `json:"error,omitempty"`
		} `json:"results"`
	}
	status, err := apiCall(
		cmd.Context(),
		http.MethodPost,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables"),
		map[string]any{"variables": []map[string]any{variable}},
		at.Token,
		verbose,
		&result)
	if status == http.StatusConflict {
		return fmt.Errorf("%q already exists; use 'ast secrets update' to change its value", name)
	}
	if err != nil {
		return err
	}
	for _, r := range result.Results {
		if r.Status != "created" {
			return fmt.Errorf("failed to create %q: %s", r.Name, r.Error)
		}
	}

	w := cmd.OutOrStdout()
	green := color.New(color.FgGreen)
	green.Fprint(w, "✓ ") //nolint:errcheck,gosec
	if plain {
		_, _ = fmt.Fprintf(w, "Created variable %q\n", name)
	} else {
		_, _ = fmt.Fprintf(w, "Created secret %q\n", name)
	}
	return nil
}

func runSecretUpdate(cmd *cobra.Command, args []string) error {
	name := args[0]
	ctx := cmd.Context()

	at, err := getCurrentAccountToken(ctx)
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	// Fetch the variable's type to pick the right echo mode and success message.
	// Fall back to treating it as a secret if the fetch fails (safe default).
	isSecret := true
	if _, meta, err := fetchVariableMeta(ctx, at, name, verbose); err == nil {
		isSecret = meta.Secret
	}

	var value string
	if secretUpdateValue != "" {
		value = secretUpdateValue
	} else {
		echoMode := huh.EchoModePassword
		if !isSecret {
			echoMode = huh.EchoModeNormal
		}
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title(func() string {
						if isSecret {
							return fmt.Sprintf("New value for %q", name)
						}
						return fmt.Sprintf("New value for %q (plain text)", name)
					}()).
					EchoMode(echoMode).
					Value(&value).
					Validate(func(v string) error {
						if v == "" {
							return fmt.Errorf("value cannot be empty")
						}
						return nil
					}),
			),
		)
		if err := form.Run(); err != nil {
			return err
		}
	}

	return runSecretUpdateWithValue(cmd, args, value, isSecret, verbose)
}

// fetchVariableMeta returns the status code and metadata for a single variable via GET /:varName.
func fetchVariableMeta(ctx context.Context, at AccountToken, name string, verbose bool) (int, *secretVariableMetadata, error) {
	var meta secretVariableMetadata
	status, err := apiCall(
		ctx,
		http.MethodGet,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables", name),
		nil,
		at.Token,
		verbose,
		&meta)
	if err != nil {
		return status, nil, err
	}
	return status, &meta, nil
}

func runSecretUpdateWithValue(cmd *cobra.Command, args []string, value string, isSecret, verbose bool) error { //nolint:unparam
	name := args[0]

	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}

	payload := map[string]any{"value": value}
	if desc, _ := cmd.Flags().GetString("description"); desc != "" {
		payload["description"] = desc
	}

	status, err := apiCall(
		cmd.Context(),
		http.MethodPut,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables", name),
		payload,
		at.Token,
		verbose,
		nil)
	if status == http.StatusNotFound {
		return fmt.Errorf("secret %q not found", name)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	green := color.New(color.FgGreen)
	green.Fprint(w, "✓ ") //nolint:errcheck,gosec
	if isSecret {
		_, _ = fmt.Fprintf(w, "Updated secret %q\n", name)
	} else {
		_, _ = fmt.Fprintf(w, "Updated variable %q\n", name)
	}
	return nil
}

func runSecretGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	ctx := cmd.Context()

	at, err := getCurrentAccountToken(ctx)
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	_, meta, err := fetchVariableMeta(ctx, at, name, verbose)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(meta) //nolint:errcheck,gosec
	}

	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)

	const dateFmt = "2006-01-02T15:04:05"
	printField := func(label, val string) {
		dim.Fprintf(w, "%-12s", label+":") //nolint:errcheck,gosec
		cyan.Fprintln(w, " "+val)          //nolint:errcheck,gosec
	}

	printField("Name", meta.Name)
	if meta.Description != "" {
		printField("Description", meta.Description)
	}
	printField("Created", meta.CreatedAt.Format(dateFmt))
	printField("Updated", meta.UpdatedAt.Format(dateFmt))

	if meta.Secret {
		printField("Value", "******")
	} else if meta.Value != nil {
		printField("Value", *meta.Value)
	}

	return nil
}

func runSecretDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	status, err := apiCall(
		cmd.Context(),
		http.MethodDelete,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables", name),
		nil,
		at.Token,
		verbose,
		nil)
	if status == http.StatusNotFound {
		return fmt.Errorf("secret %q not found", name)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	green := color.New(color.FgGreen)
	green.Fprint(w, "✓ ") //nolint:errcheck,gosec
	_, _ = fmt.Fprintf(w, "Deleted secret %q\n", name)
	return nil
}

func runSecretImport(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	at, err := getCurrentAccountToken(ctx)
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	f, err := os.Open(args[0]) //nolint:gosec
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close() //nolint:errcheck,gosec

	envMap, err := godotenv.Parse(f)
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}
	for k, v := range envMap {
		if v == "" {
			delete(envMap, k)
		}
	}
	w := cmd.OutOrStdout()
	green := color.New(color.FgGreen)
	dim := color.New(color.Faint)

	if len(envMap) == 0 {
		_, _ = fmt.Fprintln(w, "No variables to import.")
		return nil
	}

	// Build set of keys to treat as plain text.
	// --plain marks all keys plain; --plain-keys marks specific ones.
	allPlain, _ := cmd.Flags().GetBool("plain")
	plainSet := map[string]bool{}
	if secretImportPlainKeys != "" {
		for _, k := range strings.Split(secretImportPlainKeys, ",") {
			plainSet[strings.TrimSpace(k)] = true
		}
	}

	// Unless --overwrite, fetch existing names and skip them.
	if !secretImportOverwrite {
		existing, err := fetchExistingVarNames(ctx, at, verbose)
		if err != nil {
			return fmt.Errorf("failed to fetch existing variables: %w", err)
		}
		for k := range envMap {
			if existing[k] {
				dim.Fprintf(w, "  Skipped %q (already exists)\n", k) //nolint:errcheck,gosec
				delete(envMap, k)
			}
		}
	}

	if len(envMap) == 0 {
		_, _ = fmt.Fprintln(w, "Nothing new to import.")
		return nil
	}

	// Build batch payload.
	vars := make([]map[string]any, 0, len(envMap))
	for k, v := range envMap {
		vars = append(vars, map[string]any{
			"name":   k,
			"value":  v,
			"secret": !allPlain && !plainSet[k],
		})
	}

	var result struct {
		Results []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Error  string `json:"error,omitempty"`
		} `json:"results"`
	}
	if _, err := apiCall(
		ctx,
		http.MethodPost,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables"),
		map[string]any{"variables": vars},
		at.Token,
		verbose,
		&result); err != nil {
		return err
	}

	for _, r := range result.Results {
		if r.Status != "created" {
			red := color.New(color.FgRed)
			red.Fprintf(w, "✗ Failed  %q: %s\n", r.Name, r.Error) //nolint:errcheck,gosec
		} else if allPlain || plainSet[r.Name] {
			green.Fprintf(w, "✓ Imported variable %q\n", r.Name) //nolint:errcheck,gosec
		} else {
			green.Fprintf(w, "✓ Imported secret   %q\n", r.Name) //nolint:errcheck,gosec
		}
	}
	return nil
}

// fetchExistingVarNames returns a set of variable names already in the account.
func fetchExistingVarNames(ctx context.Context, at AccountToken, verbose bool) (map[string]bool, error) {
	var result struct {
		Variables []struct {
			Name string `json:"name"`
		} `json:"variables"`
	}
	if _, err := apiCall(
		ctx,
		http.MethodGet,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables"),
		nil,
		at.Token,
		verbose,
		&result); err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(result.Variables))
	for _, v := range result.Variables {
		names[v.Name] = true
	}
	return names, nil
}
