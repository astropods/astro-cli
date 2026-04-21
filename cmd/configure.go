package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astropods/astro/apps/astro-cli/internal/config"
	"github.com/astropods/astro/apps/astro-cli/internal/telemetry"
	"github.com/astropods/astro/apps/astro-cli/internal/utils"
	spec "github.com/astropods/astro/packages/astro-spec"
)

var configureCmd = &cobra.Command{
	Use:     "configure",
	Aliases: []string{"config"},
	Short:   "Configure project credentials and inputs",
	RunE:    runConfigure,
}

var configureSetCmd = &cobra.Command{
	Use:   "set <KEY> <VALUE>",
	Short: "Set a single config variable",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigureSet,
}

var configureUnsetCmd = &cobra.Command{
	Use:   "unset <KEY> [KEY...]",
	Short: "Remove one or more config variables",
	Long: `Explicitly remove stored values for the given keys.

Use this when you want to clear a value you previously set. Leaving a field
blank in the interactive form does NOT remove the value — blanks are treated
as "no change" so existing secrets are preserved across re-runs.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runConfigureUnset,
}

var configureTelemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Enable or disable anonymous telemetry",
	Long: `Control whether the CLI sends anonymous usage data.

Use --enable to opt in, --disable to opt out.
You can also set the ASTRO_NO_TELEMETRY environment variable to disable.`,
	RunE: runConfigureTelemetry,
}

func init() {
	rootCmd.AddCommand(configureCmd)
	configureCmd.AddCommand(configureSetCmd)
	configureCmd.AddCommand(configureUnsetCmd)
	configureCmd.AddCommand(configureTelemetryCmd)

	configureCmd.Long = fmt.Sprintf(`Interactively set credentials and input values for your agent project.

Reads astropods.yml to determine required variables, presents an interactive
form to fill them in, and stores values in ~/.%s/project-configs.json.

Stored values are automatically loaded by '%s dev', removing the need for
a .env file.`, binaryName, binaryName)

	configureSetCmd.Long = fmt.Sprintf(`Set a single configuration variable for the current project.

The value is stored in ~/.%s/project-configs.json and automatically
loaded by '%s dev'.`, binaryName, binaryName)
	configureTelemetryCmd.Flags().Bool("enable", false, "Enable anonymous telemetry")
	configureTelemetryCmd.Flags().Bool("disable", false, "Disable anonymous telemetry")

	// Also allow --no-telemetry / --telemetry directly on configure
	configureCmd.Flags().Bool("no-telemetry", false, "Disable anonymous telemetry")
	configureCmd.Flags().Bool("telemetry", false, "Enable anonymous telemetry")

	configureCmd.Flags().String("out", "", "Print stored config vars in the given format: env or json")
}

// varEntry describes one configurable variable.
type varEntry struct {
	key         string
	description string
	secret      bool
}

// advanceFieldMsg is sent after a form rebuild to restore focus by replaying Tab presses.
type advanceFieldMsg struct{}

// configureApp is a bubbletea model wrapping a huh form.
// ctrl+r toggles the reveal state of the focused secret field in place.
type configureApp struct {
	form           *huh.Form
	varPtrs        map[string]*string
	allVars        []varEntry
	existing       map[string]string
	formHeight     int
	focusedIdx     int
	revealed       map[string]bool
	pendingAdvance int // Tab presses to replay after a rebuild
	aborted        bool
}

func (m *configureApp) buildForm() {
	var fields []huh.Field
	for _, v := range m.allVars {
		desc := v.description
		if m.existing[v.key] != "" {
			if desc != "" {
				desc += " (already set)"
			} else {
				desc = "already set"
			}
		}
		inp := huh.NewInput().
			Title(v.key).
			Description(desc).
			Value(m.varPtrs[v.key])
		if v.secret && !m.revealed[v.key] {
			inp = inp.EchoMode(huh.EchoModePassword)
		}
		fields = append(fields, inp)
	}
	f := huh.NewForm(huh.NewGroup(fields...))
	if m.formHeight > 0 {
		f = f.WithHeight(m.formHeight - 1) // reserve 1 line for the hint
	}
	m.form = f
}

func (m *configureApp) Init() tea.Cmd {
	return m.form.Init()
}

func (m *configureApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Replay a Tab press to restore focus after a rebuild.
	if _, ok := msg.(advanceFieldMsg); ok {
		newForm, cmd := m.form.Update(tea.KeyMsg{Type: tea.KeyTab})
		if f, ok := newForm.(*huh.Form); ok {
			m.form = f
		}
		m.pendingAdvance--
		if m.pendingAdvance > 0 {
			return m, tea.Batch(cmd, func() tea.Msg { return advanceFieldMsg{} })
		}
		return m, cmd
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "ctrl+c", "esc":
			m.aborted = true
			return m, tea.Quit
		case "ctrl+r":
			if m.focusedIdx < len(m.allVars) && m.allVars[m.focusedIdx].secret {
				k := m.allVars[m.focusedIdx].key
				m.revealed[k] = !m.revealed[k]
				m.buildForm()
				if m.focusedIdx > 0 {
					m.pendingAdvance = m.focusedIdx
					return m, tea.Batch(m.form.Init(), func() tea.Msg { return advanceFieldMsg{} })
				}
				return m, m.form.Init()
			}
			return m, nil
		case "tab", "enter":
			m.focusedIdx = min(m.focusedIdx+1, len(m.allVars)-1)
		case "shift+tab":
			m.focusedIdx = max(m.focusedIdx-1, 0)
		}
	}

	newForm, cmd := m.form.Update(msg)
	if f, ok := newForm.(*huh.Form); ok {
		m.form = f
	}
	if m.form.State == huh.StateCompleted || m.form.State == huh.StateAborted {
		return m, tea.Quit
	}
	return m, cmd
}

var (
	hintKeyStyle  = lipgloss.NewStyle()
	hintDescStyle = lipgloss.NewStyle().Faint(true)
)

func (m *configureApp) View() string {
	k := hintKeyStyle.Render
	d := hintDescStyle.Render
	hint := k("  tab") + d(" / ") + k("shift+tab") + d("  navigate")
	if m.focusedIdx < len(m.allVars) && m.allVars[m.focusedIdx].secret {
		if m.revealed[m.allVars[m.focusedIdx].key] {
			hint += d("  ·  ") + k("ctrl+r") + d("  hide")
		} else {
			hint += d("  ·  ") + k("ctrl+r") + d("  reveal")
		}
	}
	return m.form.View() + hint
}

func runConfigureTelemetry(cmd *cobra.Command, _ []string) error {
	enable, _ := cmd.Flags().GetBool("enable")
	disable, _ := cmd.Flags().GetBool("disable")

	if !enable && !disable {
		// Show current status
		if telemetry.IsEnabled(binaryName) {
			fmt.Println("Telemetry is enabled.")
		} else {
			fmt.Println("Telemetry is disabled.")
		}
		fmt.Println("Use --enable or --disable to change.")
		return nil
	}
	if enable && disable {
		return fmt.Errorf("cannot use both --enable and --disable")
	}

	if err := telemetry.SetEnabled(binaryName, enable); err != nil {
		return fmt.Errorf("failed to update telemetry setting: %w", err)
	}
	if enable {
		fmt.Println("Telemetry enabled.")
	} else {
		fmt.Println("Telemetry disabled. No usage data will be sent.")
	}
	return nil
}

func formatVars(format string, vars map[string]string) (string, error) {
	switch format {
	case "env":
		return godotenv.Marshal(vars)
	case "json":
		out, err := json.MarshalIndent(vars, "", "  ")
		if err != nil {
			return "", err
		}
		return string(out), nil
	default:
		return "", fmt.Errorf("unknown format %q: use env or json", format)
	}
}

func runConfigureOut(format string) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	vars := config.GetProjectVars(binaryName, workingDir)
	if len(vars) == 0 {
		fmt.Fprintln(os.Stderr, "ℹ️  No stored config variables found for this project.")
		return nil
	}

	out, err := formatVars(format, vars)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func runConfigure(cmd *cobra.Command, args []string) error {
	if format, _ := cmd.Flags().GetString("out"); format != "" {
		return runConfigureOut(format)
	}

	noTelemetry, _ := cmd.Flags().GetBool("no-telemetry")
	enableTelemetry, _ := cmd.Flags().GetBool("telemetry")
	if noTelemetry || enableTelemetry {
		if err := telemetry.SetEnabled(binaryName, enableTelemetry); err != nil {
			return fmt.Errorf("failed to update telemetry setting: %w", err)
		}
		if enableTelemetry {
			fmt.Println("Telemetry enabled.")
		} else {
			fmt.Println("Telemetry disabled.")
		}
		if noTelemetry && !enableTelemetry {
			return nil // --no-telemetry is standalone
		}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath, err := resolveSpecPath(cmd, workingDir)
	if err != nil {
		return err
	}

	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	fmt.Printf("📄 Configuring: %s\n\n", astroSpec.Name)
	printedLines := 2

	// Load .env for migration (pre-populate fields with existing file values)
	envFilePath := filepath.Join(workingDir, utils.DefaultEnvFile)
	dotenvVars, err := utils.LoadEnvFile(workingDir, utils.DefaultEnvFile)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", utils.DefaultEnvFile, err)
	}
	hasDotenv := dotenvVars != nil
	if hasDotenv {
		fmt.Printf("📂 Found %s — values will be pre-filled for migration\n\n", utils.DefaultEnvFile)
		printedLines += 2
	}

	// Load existing stored values; merge .env under them (stored config wins).
	// `stored` is the raw view of the project store and is used below to
	// distinguish "preserved existing stored value" from "genuinely blank"
	// when reporting the save summary.
	stored := config.GetProjectVars(binaryName, workingDir)
	if stored == nil {
		stored = make(map[string]string)
	}
	existing := make(map[string]string, len(stored)+len(dotenvVars))
	for k, v := range stored {
		existing[k] = v
	}
	for k, v := range dotenvVars {
		if existing[k] == "" {
			existing[k] = v
		}
	}

	// Collect required variables
	var credVars []varEntry
	var messagingVars []varEntry
	var inputVars []varEntry

	// Credentials from cloud/custom providers
	credKeys := spec.AllCredentialKeys(astroSpec)
	sortedCredKeys := make([]string, 0, len(credKeys))
	for k := range credKeys {
		sortedCredKeys = append(sortedCredKeys, k)
	}
	sort.Strings(sortedCredKeys)
	for _, k := range sortedCredKeys {
		meta := credKeys[k]
		credVars = append(credVars, varEntry{
			key:         k,
			description: meta.Description,
			secret:      true,
		})
	}

	// Non-secret custom provider variables (injected as plain env vars)
	var providerPlainVars []varEntry
	for provName, cp := range referencedCustomProviders(astroSpec) {
		prefix := spec.SanitizeEnvName(provName)
		for _, v := range cp.Variables {
			if v.Secret {
				continue // already covered by AllCredentialKeys above
			}
			providerPlainVars = append(providerPlainVars, varEntry{
				key:         prefix + "_" + v.Name,
				description: v.Description,
				secret:      false,
			})
		}
	}
	sort.Slice(providerPlainVars, func(i, j int) bool { return providerPlainVars[i].key < providerPlainVars[j].key })
	credVars = append(credVars, providerPlainVars...)

	// Slack tokens from dev interfaces
	if slices.Contains(astroSpec.Dev.MessagingAdapters(), "slack") {
		messagingVars = append(messagingVars,
			varEntry{key: "SLACK_BOT_TOKEN", description: "Slack bot token (xoxb-...)", secret: true},
			varEntry{key: "SLACK_APP_TOKEN", description: "Slack app-level token (xapp-...)", secret: true},
		)
	}

	// Top-level inputs
	topInputKeys := make([]string, 0, len(astroSpec.Inputs))
	for k := range astroSpec.Inputs {
		topInputKeys = append(topInputKeys, k)
	}
	sort.Strings(topInputKeys)
	for _, k := range topInputKeys {
		inp := astroSpec.Inputs[k]
		inputVars = append(inputVars, varEntry{
			key:         inp.Name,
			description: inp.Description,
			secret:      inp.Secret,
		})
	}

	// Agent-level inputs
	for _, inp := range astroSpec.Agent.Inputs {
		inputVars = append(inputVars, varEntry{
			key:         inp.Name,
			description: inp.Description,
			secret:      inp.Secret,
		})
	}

	// If nothing to configure, bail early
	if len(credVars) == 0 && len(messagingVars) == 0 && len(inputVars) == 0 {
		fmt.Println("ℹ️  No configurable variables found in spec.")
		return nil
	}

	// Build one pointer per var (pre-populated with existing/dotenv values)
	allVars := append(append(credVars, messagingVars...), inputVars...)
	varPtrs := make(map[string]*string, len(allVars))
	for _, v := range allVars {
		val := existing[v.key]
		varPtrs[v.key] = &val
	}

	app := &configureApp{
		varPtrs:  varPtrs,
		allVars:  allVars,
		existing: existing,
		revealed: make(map[string]bool),
	}
	if _, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil { //nolint:gosec
		app.formHeight = h - printedLines
	}
	app.buildForm()

	result, err := tea.NewProgram(app).Run()
	if err != nil {
		return err
	}
	app = result.(*configureApp)
	if app.aborted || app.form.State == huh.StateAborted {
		fmt.Println("Cancelled.")
		return nil
	}

	// Collect results
	newVars := make(map[string]string, len(allVars))
	for _, v := range allVars {
		if ptr := varPtrs[v.key]; ptr != nil {
			newVars[v.key] = *ptr
		}
	}

	// MergeProjectVars skips empty values so a blank submission for a
	// pre-filled secret preserves it. Split the counters to mirror that:
	//   saved     — non-empty submission, will be written
	//   preserved — blank submission but an existing stored value survives
	//   blank     — blank submission with nothing previously stored
	saved, preserved, blank := 0, 0, 0
	for _, v := range allVars {
		if strings.TrimSpace(newVars[v.key]) != "" {
			saved++
			continue
		}
		if _, ok := stored[v.key]; ok {
			preserved++
		} else {
			blank++
		}
	}

	if err := config.MergeProjectVars(binaryName, workingDir, astroSpec.Name, newVars); err != nil {
		return fmt.Errorf("failed to save project config: %w", err)
	}

	fmt.Printf("\n✅ Saved %d variable(s)", saved)
	if preserved > 0 {
		fmt.Printf(", preserved %d existing value(s)", preserved)
	}
	if blank > 0 {
		fmt.Printf(", skipped %d (left blank)", blank)
	}
	fmt.Println()
	if preserved > 0 {
		fmt.Println("ℹ️  Use `ast configure unset <KEY>` to explicitly clear a stored value.")
	}
	fmt.Printf("📁 Stored in: %s\n", configsPathDisplay())

	// Offer to delete .env if one was found
	if hasDotenv {
		var deleteDotenv bool
		prompt := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Delete %s?", utils.DefaultEnvFile)).
					Description("Credentials are now stored in the config store. The .env file is no longer needed.").
					Affirmative("Yes, delete it").
					Negative("Keep it").
					Value(&deleteDotenv),
			),
		)
		if err := prompt.Run(); err == nil && deleteDotenv {
			if err := os.Remove(envFilePath); err != nil {
				fmt.Printf("⚠️  Could not delete %s: %v\n", utils.DefaultEnvFile, err)
			} else {
				fmt.Printf("🗑️  Deleted %s\n", utils.DefaultEnvFile)
			}
		}
	}

	return nil
}

func runConfigureSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], strings.TrimSpace(args[1])

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath, err := resolveSpecPath(cmd, workingDir)
	if err != nil {
		return err
	}

	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	/*
		`set KEY ""` used to clobber the stored value when MergeProjectVars
		wrote empty strings through. Now that empties are preserved, an empty
		VALUE is explicitly routed to unset to keep the observable "clear a
		var" behavior of `set` intact.
	*/
	if value == "" {
		if err := config.UnsetProjectVars(binaryName, workingDir, []string{key}); err != nil {
			return fmt.Errorf("failed to unset config: %w", err)
		}
		fmt.Printf("🗑️  Unset %s\n", key)
		fmt.Printf("📁 Stored in: %s\n", configsPathDisplay())
		return nil
	}

	if err := config.MergeProjectVars(binaryName, workingDir, astroSpec.Name, map[string]string{key: value}); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✅ Set %s\n", key)
	fmt.Printf("📁 Stored in: %s\n", configsPathDisplay())
	return nil
}

func runConfigureUnset(cmd *cobra.Command, args []string) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	/*
		Unset doesn't need the spec to resolve — stored vars are keyed by the
		project path, not by agent name. Parsing is still useful for nicer
		error messages if the user runs it in the wrong directory, but we
		skip it to keep `unset` usable even when the spec is temporarily
		broken.
	*/
	if err := config.UnsetProjectVars(binaryName, workingDir, args); err != nil {
		return fmt.Errorf("failed to unset config: %w", err)
	}
	for _, key := range args {
		fmt.Printf("🗑️  Unset %s\n", key)
	}
	fmt.Printf("📁 Stored in: %s\n", configsPathDisplay())
	return nil
}

// configsPathDisplay returns a user-friendly display path for project-configs.json.
func configsPathDisplay() string {
	path, err := config.ConfigsPath(binaryName)
	if err != nil {
		return fmt.Sprintf("~/.%s/project-configs.json", binaryName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil {
		return path
	}
	return "~/" + rel
}
