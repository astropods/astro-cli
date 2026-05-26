package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	"github.com/astropods/astro/apps/astro-cli/internal/tui"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// secretsServerURLOverride is set in tests to redirect API calls to a test server.
var secretsServerURLOverride string

func secretsBaseURL() string {
	if secretsServerURLOverride != "" {
		return strings.TrimSuffix(secretsServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

func exactValidSecretName(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("this command expected exactly one argument <secret name>, but got %d", len(args))
	}
	if err := spec.ValidateVarName(args[0]); err != nil {
		return fmt.Errorf("secret name %q: %w", args[0], err)
	}
	return nil
}

var secretCmd = &cobra.Command{
	Use:     "secrets",
	Aliases: []string{"secret"},
	Short:   "Manage account secrets",
	Long:    "Create, list, update, and delete secrets in the account vault. Values are write-only once set.",
	Args:    cobra.NoArgs,
	RunE:    func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var secretListCmd = &cobra.Command{
	Use:   "list",
	Short: "List secrets in the active account vault",
	Args:  cobra.NoArgs,
	RunE:  runSecretList,
}

var secretCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new secret",
	Args:  exactValidSecretName,
	RunE:  runSecretCreate,
}

var secretUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update an existing secret's value",
	Args:  exactValidSecretName,
	RunE:  runSecretUpdate,
}

var secretGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get details for a secret or variable",
	Args:  exactValidSecretName,
	RunE:  runSecretGet,
}

var secretDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a secret",
	Args:  exactValidSecretName,
	RunE:  runSecretDelete,
}

var secretImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import variables from a file (e.g., .env)",
	Long: `Import variables from a file (e.g., .env) into the active account vault.

Lines of the form KEY=value are imported as secrets by default.
Use --plain to store all imported variables as plain text.
Use --plain-keys KEY1,KEY2 to mark specific keys as plain text.
Blank values (KEY=) are silently skipped.
Existing variables are skipped unless --overwrite is set.`,
	Args: cobra.NoArgs,
	RunE: runSecretImport,
}

func init() {
	secretCreateCmd.Flags().Bool("plain", false, "Store as plaintext instead of an encrypted secret")
	secretCreateCmd.Flags().String("value", "", "Value to set (skips interactive prompt)")
	secretCreateCmd.Flags().Bool("overwrite", false, "Overwrite if the variable already exists")
	secretCreateCmd.Flags().StringP("description", "d", "", "Optional description for the secret")
	secretUpdateCmd.Flags().String("value", "", "New value (skips interactive prompt)")
	secretUpdateCmd.Flags().Bool("plain", false, "Store as plaintext instead of an encrypted secret")
	secretUpdateCmd.Flags().StringP("description", "d", "", "Update the description")
	secretListCmd.Flags().Bool("values", false, "Show variable values")
	secretListCmd.Flags().Bool("json", false, "Output as JSON")
	secretGetCmd.Flags().Bool("json", false, "Output as JSON")
	secretImportCmd.Flags().StringP("file", "f", "", "Path to the file to import (e.g., .env)")
	secretImportCmd.Flags().Bool("plain", false, "Store all imported variables as plain text")
	secretImportCmd.Flags().String("plain-keys", "", "Comma-separated keys to store as plain text")
	secretImportCmd.Flags().Bool("overwrite", false, "Overwrite existing variables")
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
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	var result struct {
		Variables []secretVariableMetadata `json:"variables"`
	}
	if _, err := apiCallForAccount(
		cmd.Context(),
		http.MethodGet,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables"),
		nil,
		at.Account,
		verbose,
		&result); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)

	if len(result.Variables) == 0 {
		fmt.Fprintf(w, "%sNo secrets found in account %s%s\n", colorDim, at.Account, colorReset) //nolint:errcheck,gosec
		return nil
	}

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result.Variables)
	}

	showValues, _ := cmd.Flags().GetBool("values")

	nameWidth := len("Name")
	for _, v := range result.Variables {
		if len(v.Name) > nameWidth {
			nameWidth = len(v.Name)
		}
	}

	typeHeader := "Type"
	if showValues {
		typeHeader = "Value"
	}
	dim.Fprintf(w, "%-*s  %-*s  %s\n", tableTimeWidth, "Updated", nameWidth, "Name", typeHeader) //nolint:errcheck,gosec

	for _, v := range result.Variables {
		date := v.UpdatedAt.Format(tableTimeFmt)
		if v.UpdatedAt.IsZero() {
			date = strings.Repeat("—", tableTimeWidth)
		}

		var typeOrValue string
		if v.Secret {
			if showValues {
				typeOrValue = "******"
			} else {
				typeOrValue = "secret"
			}
		} else if showValues && v.Value != nil {
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
	plain, _ := cmd.Flags().GetBool("plain")
	overwrite, _ := cmd.Flags().GetBool("overwrite")

	if v, _ := cmd.Flags().GetString("value"); v != "" {
		return runSecretCreateWithValue(cmd, args, v, plain, overwrite)
	}

	echoMode := huh.EchoModePassword
	if plain {
		echoMode = huh.EchoModeNormal
	}

	var value string
	description, _ := cmd.Flags().GetString("description")
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
			huh.NewInput().
				Title("Description (optional)").
				Value(&description),
		),
	)
	if err := runForm(form); err != nil {
		if errors.Is(err, tui.ErrCancelled) {
			printCancelled(cmd.OutOrStdout())
			return nil
		}
		return err
	}
	_ = cmd.Flags().Set("description", strings.TrimSpace(description))

	return runSecretCreateWithValue(cmd, args, value, plain, overwrite)
}

func runSecretCreateWithValue(cmd *cobra.Command, args []string, value string, plain, overwrite bool) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	if !overwrite {
		if status, _, err := fetchVariableMeta(cmd.Context(), at.Account, name, verbose); err == nil {
			return fmt.Errorf("%q already exists; use '%s secrets update' to change its value", name, buildinfo.BinaryName)
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
	status, err := apiCallForAccount(
		cmd.Context(),
		http.MethodPost,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables"),
		map[string]any{"variables": []map[string]any{variable}},
		at.Account,
		verbose,
		&result)
	if status == http.StatusConflict {
		return fmt.Errorf("%q already exists; use '%s secrets update' to change its value", name, buildinfo.BinaryName)
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

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	plain, _ := cmd.Flags().GetBool("plain")

	// Fetch the variable's type to pick the right echo mode and success message.
	// Fall back to treating it as a secret if the fetch fails (safe default).
	isSecret := true
	if _, meta, err := fetchVariableMeta(cmd.Context(), at.Account, name, verbose); err == nil {
		isSecret = meta.Secret
	}
	if plain {
		isSecret = false
	}

	var value string
	if v, _ := cmd.Flags().GetString("value"); v != "" {
		value = v
	} else {
		echoMode := huh.EchoModePassword
		if !isSecret {
			echoMode = huh.EchoModeNormal
		}
		description, _ := cmd.Flags().GetString("description")
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
				huh.NewInput().
					Title("Description (optional)").
					Value(&description),
			),
		)
		if err := runForm(form); err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				printCancelled(cmd.OutOrStdout())
				return nil
			}
			return err
		}
		_ = cmd.Flags().Set("description", strings.TrimSpace(description))
	}

	return runSecretUpdateWithValue(cmd, args, value, isSecret, plain, verbose)
}

// fetchVariableMeta returns the status code and metadata for a single variable via GET /:varName.
func fetchVariableMeta(ctx context.Context, account string, name string, verbose bool) (int, *secretVariableMetadata, error) {
	var meta secretVariableMetadata
	status, err := apiCallForAccount(
		ctx,
		http.MethodGet,
		apiPath(secretsBaseURL(), account, "accounts", "variables", name),
		nil,
		account,
		verbose,
		&meta)
	if err != nil {
		return status, nil, err
	}
	return status, &meta, nil
}

func runSecretUpdateWithValue(cmd *cobra.Command, args []string, value string, isSecret, plain, verbose bool) error { //nolint:unparam
	name := args[0]

	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}

	payload := map[string]any{"value": value}
	if plain {
		payload["secret"] = false
	}
	if desc, _ := cmd.Flags().GetString("description"); desc != "" {
		payload["description"] = desc
	}

	status, err := apiCallForAccount(
		cmd.Context(),
		http.MethodPut,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables", name),
		payload,
		at.Account,
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

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	_, meta, err := fetchVariableMeta(cmd.Context(), at.Account, name, verbose)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, meta)
	}

	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)

	printField := func(label, val string) {
		dim.Fprintf(w, "%-12s", label+":") //nolint:errcheck,gosec
		cyan.Fprintln(w, " "+val)          //nolint:errcheck,gosec
	}

	printField("Name", meta.Name)
	printField("Created", meta.CreatedAt.Format(tableTimeFmt))
	printField("Updated", meta.UpdatedAt.Format(tableTimeFmt))

	if meta.Secret {
		printField("Value", "******")
	} else if meta.Value != nil {
		printField("Value", *meta.Value)
	}

	if meta.Description != "" {
		printField("Description", meta.Description)
	}

	return nil
}

func runSecretDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	status, err := apiCallForAccount(
		cmd.Context(),
		http.MethodDelete,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables", name),
		nil,
		at.Account,
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

func runSecretImport(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	filePath := flagString(cmd, "file")
	if filePath == "" {
		return fmt.Errorf("flag --file is required")
	}

	f, err := os.Open(filePath) //nolint:gosec
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
	if plainKeys, _ := cmd.Flags().GetString("plain-keys"); plainKeys != "" {
		for k := range strings.SplitSeq(plainKeys, ",") {
			plainSet[strings.TrimSpace(k)] = true
		}
	}

	overwrite, _ := cmd.Flags().GetBool("overwrite")

	// Unless --overwrite, fetch existing names and skip them.
	if !overwrite {
		existing, err := fetchExistingVarNames(cmd.Context(), at.Account, verbose)
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
	if _, err := apiCallForAccount(
		cmd.Context(),
		http.MethodPost,
		apiPath(secretsBaseURL(), at.Account, "accounts", "variables"),
		map[string]any{"variables": vars},
		at.Account,
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
func fetchExistingVarNames(ctx context.Context, account string, verbose bool) (map[string]bool, error) {
	var result struct {
		Variables []struct {
			Name string `json:"name"`
		} `json:"variables"`
	}
	if _, err := apiCallForAccount(
		ctx,
		http.MethodGet,
		apiPath(secretsBaseURL(), account, "accounts", "variables"),
		nil,
		account,
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
