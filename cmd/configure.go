package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/config"
	"github.com/astropods/astro-cli/internal/tui"
	"github.com/astropods/astro-cli/internal/utils"
	spec "github.com/astropods/astro-spec"
)

var configureCmd = &cobra.Command{
	Use:     "configure",
	Aliases: []string{"config"},
	Short:   "Configure project credentials and inputs",
	Args:    cobra.NoArgs,
	RunE:    runConfigure,
}

func registerConfigureFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "", "Print stored config vars in the given format: env or json")
	cmd.Flags().String("vars-file", "", "Import variables from an env file")
	cmd.Flags().StringArray("var", nil, "Set a variable (KEY=VALUE, repeatable)")
	cmd.Flags().StringArray("rm-var", nil, "Remove a variable (KEY, repeatable)")
}

func init() {
	devCmd.AddCommand(configureCmd)

	configureCmd.Long = fmt.Sprintf(`Configure credentials and input values for your agent project.

Reads astropods.yml to determine required variables. '%[1]s project start' loads them automatically.

Run without flags for an interactive form.`, buildinfo.BinaryName)

	registerConfigureFlags(configureCmd)

	topLevelConfigureCmd := &cobra.Command{
		Use:    "configure",
		Short:  configureCmd.Short,
		Long:   configureCmd.Long,
		Args:   configureCmd.Args,
		RunE:   runConfigure,
		Hidden: true,
	}
	registerConfigureFlags(topLevelConfigureCmd)
	rootCmd.AddCommand(topLevelConfigureCmd)
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
	f := huh.NewForm(huh.NewGroup(fields...)).
		WithTheme(cliHuhTheme()).
		WithKeyMap(promptKeyMap()).
		WithShowHelp(false) // configureApp.View renders the full hint itself
	if m.formHeight > 0 {
		// Reserve 1 line below the form for the hint rendered by View.
		f = f.WithHeight(m.formHeight - 1)
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

func (m *configureApp) View() string {
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab/shift+tab", "navigate")),
	}
	if m.focusedIdx < len(m.allVars) && m.allVars[m.focusedIdx].secret {
		verb := "reveal"
		if m.revealed[m.allVars[m.focusedIdx].key] {
			verb = "hide"
		}
		bindings = append(bindings, key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", verb)))
	}
	bindings = append(bindings, tui.Cancel)
	v := strings.TrimRight(m.form.View(), "\n")
	return v + "\n" + tui.Hint(cliHuhTheme(), bindings...) + "\n"
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

	vars := config.GetProjectVars(buildinfo.BinaryName, workingDir)
	if len(vars) == 0 {
		fmt.Fprintf(os.Stderr, "%sNo stored config variables found for this project.%s\n", colorDim, colorReset)
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
	if format, _ := cmd.Flags().GetString("output"); format != "" {
		return runConfigureOut(format)
	}

	importFile, _ := cmd.Flags().GetString("vars-file")
	setVals, _ := cmd.Flags().GetStringArray("var")
	unsetKeys, _ := cmd.Flags().GetStringArray("rm-var")

	if importFile != "" || len(setVals) > 0 || len(unsetKeys) > 0 {
		return runConfigureFlags(cmd, importFile, setVals, unsetKeys)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath, err := resolveSpecPath(flagString(cmd, "file"), workingDir)
	if err != nil {
		return err
	}

	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	fmt.Printf("%s→%s Configuring: %s%s%s\n\n", colorCyan, colorReset, colorBold, astroSpec.Name, colorReset)
	printedLines := 2

	// Load .env for migration (pre-populate fields with existing file values)
	envFilePath := filepath.Join(workingDir, utils.DefaultEnvFile)
	dotenvVars, err := utils.LoadEnvFile(workingDir, utils.DefaultEnvFile)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", utils.DefaultEnvFile, err)
	}
	hasDotenv := dotenvVars != nil
	if hasDotenv {
		fmt.Printf("%s→%s Found %s — values will be pre-filled for migration\n\n", colorCyan, colorReset, utils.DefaultEnvFile)
		printedLines += 2
	}

	// Load existing stored values; merge .env under them (stored config wins).
	// `stored` is the raw view of the project store and is used below to
	// distinguish "preserved existing stored value" from "genuinely blank"
	// when reporting the save summary.
	stored := config.GetProjectVars(buildinfo.BinaryName, workingDir)
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
		fmt.Printf("%sNo configurable variables found in spec.%s\n", colorDim, colorReset)
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
		printCancelled(cmd.OutOrStdout())
		return nil
	}

	// Collect results
	newVars := make(map[string]string, len(allVars))
	for _, v := range allVars {
		if ptr := varPtrs[v.key]; ptr != nil {
			newVars[v.key] = *ptr
		}
	}

	// In the TUI path a blank submission means "leave existing value alone".
	// Filter out blank entries before calling MergeProjectVars, which skips
	// empty values. The --var flag path uses SetProjectVars to allow KEY="".
	//
	// Split counters:
	//   saved     — non-empty submission, will be written
	//   preserved — blank submission but an existing stored value survives
	//   blank     — blank submission with nothing previously stored
	saved, preserved, blank := 0, 0, 0
	filtered := make(map[string]string, len(newVars))
	for _, v := range allVars {
		if strings.TrimSpace(newVars[v.key]) != "" {
			saved++
			filtered[v.key] = newVars[v.key]
			continue
		}
		if _, ok := stored[v.key]; ok {
			preserved++
		} else {
			blank++
		}
	}

	if err := config.MergeProjectVars(buildinfo.BinaryName, workingDir, astroSpec.Name, filtered); err != nil {
		return fmt.Errorf("failed to save project config: %w", err)
	}

	fmt.Printf("\n%s✓%s Saved %d variable(s)", colorGreen, colorReset, saved)
	if preserved > 0 {
		fmt.Printf(", preserved %d existing value(s)", preserved)
	}
	if blank > 0 {
		fmt.Printf(", skipped %d (left blank)", blank)
	}
	fmt.Println()
	if preserved > 0 {
		fmt.Printf("%sUse --rm-var KEY to explicitly clear a stored value.%s\n", colorDim, colorReset)
	}
	fmt.Printf("%sStored in: %s%s\n", colorDim, configsPathDisplay(), colorReset)

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
		if err := runForm(prompt); err == nil && deleteDotenv {
			if err := os.Remove(envFilePath); err != nil {
				fmt.Printf("%s✗%s Could not delete %s: %v\n", colorRed, colorReset, utils.DefaultEnvFile, err)
			} else {
				fmt.Printf("%s✓%s Deleted %s\n", colorGreen, colorReset, utils.DefaultEnvFile)
			}
		}
	}

	return nil
}

func runConfigureFlags(cmd *cobra.Command, importFile string, setVals, unsetKeys []string) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Resolve spec for project name (needed by MergeProjectVars).
	var specName string
	if importFile != "" || len(setVals) > 0 {
		specPath, err := resolveSpecPath(flagString(cmd, "file"), workingDir)
		if err != nil {
			return err
		}
		astroSpec, err := spec.ParseSpec(specPath)
		if err != nil {
			return fmt.Errorf("failed to parse spec: %w", err)
		}
		specName = astroSpec.Name
	}

	if importFile != "" {
		vars, err := godotenv.Read(importFile)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", importFile, err)
		}
		if err := config.MergeProjectVars(buildinfo.BinaryName, workingDir, specName, vars); err != nil {
			return fmt.Errorf("failed to import config: %w", err)
		}
		fmt.Printf("Imported %d variable(s) from %s\n", len(vars), importFile)
	}

	if len(setVals) > 0 {
		vars := make(map[string]string, len(setVals))
		for _, kv := range setVals {
			idx := strings.IndexByte(kv, '=')
			if idx < 0 {
				return fmt.Errorf("--var %q: expected KEY=VALUE", kv)
			}
			vars[kv[:idx]] = kv[idx+1:]
		}
		if err := config.SetProjectVars(buildinfo.BinaryName, workingDir, specName, vars); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		for k := range vars {
			fmt.Printf("Set %s\n", k)
		}
	}

	if len(unsetKeys) > 0 {
		if err := config.UnsetProjectVars(buildinfo.BinaryName, workingDir, unsetKeys); err != nil {
			return fmt.Errorf("failed to unset config: %w", err)
		}
		for _, k := range unsetKeys {
			fmt.Printf("Unset %s\n", k)
		}
	}

	fmt.Printf("Stored in: %s\n", configsPathDisplay())
	return nil
}

// configsPathDisplay returns a user-friendly display path for project-configs.json.
func configsPathDisplay() string {
	path, err := config.ConfigsPath(buildinfo.BinaryName)
	if err != nil {
		return fmt.Sprintf("~/.%s/project-configs.json", buildinfo.BinaryName)
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
