package add

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenName screen = iota
	screenOllamaModel
	screenPersistent
	screenImage
	screenTrigger
	screenScope
	screenVarName
	screenVarSecret
	screenAddAnother
	screenConfirm
)

// providerVar holds one variable collected during the provider wizard.
type providerVar struct {
	name   string
	secret bool
}

// ProviderVar is the exported form of a provider variable, returned in Result.
type ProviderVar struct {
	Name   string
	Secret bool
}

// Result holds the outcome of the TUI wizard.
type Result struct {
	Name         string
	Entry        map[string]any
	ProviderVars []ProviderVar // populated only for domain=="provider"
}

type model struct {
	domain   string
	provider string
	name     string // trimmed entry name after confirmation

	nameInput    textinput.Model
	imageInput   textinput.Model
	varNameInput textinput.Model

	cursor      int
	ollamaModel string
	persistent  bool
	triggerType string

	// provider domain fields
	selected       map[int]bool
	vars           []providerVar
	currentVarName string

	screen   screen
	done     bool
	quitting bool
	err      string
}

func initialModel(domain, provider string, existingNames map[string]bool) model {
	ni := textinput.New()
	ni.Placeholder = "my-" + domain
	ni.CharLimit = 64
	ni.Width = 40

	// Pre-fill the name with the provider name if it isn't already taken.
	if provider != "" && !existingNames[provider] {
		ni.SetValue(provider)
	}
	ni.Focus()

	ii := textinput.New()
	ii.Placeholder = "registry.example.com/my-image:latest"
	ii.CharLimit = 256
	ii.Width = 60

	vi := textinput.New()
	vi.Placeholder = "API_KEY"
	vi.CharLimit = 64
	vi.Width = 40

	m := model{
		domain:       domain,
		provider:     provider,
		nameInput:    ni,
		imageInput:   ii,
		varNameInput: vi,
		screen:       screenName,
		cursor:       0,
	}

	if domain == "provider" {
		m.name = provider // name comes from CLI arg, skip name screen
		m.screen = screenScope
		m.selected = make(map[int]bool)
	}

	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// Update uses a value receiver, so every mutation is made on a local copy of m.
// Every code path that mutates m MUST return the modified copy; forgetting to do
// so silently discards the change. The screen-specific helpers (updateName,
// updateRadio, etc.) all return (tea.Model, tea.Cmd), which satisfies this
// requirement. Any new branch added here must follow the same pattern.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q":
			// Only quit on 'q' outside of text input screens.
			// The if-body returns m explicitly; the else path falls through to the
			// screen switch below without mutating m, so no return is needed there.
			if m.screen != screenName && m.screen != screenImage && m.screen != screenVarName {
				m.quitting = true
				return m, tea.Quit
			}
		}
	}

	switch m.screen {
	case screenName:
		return m.updateName(msg)
	case screenOllamaModel:
		return m.updateRadio(msg, ollamaModelOptions())
	case screenPersistent:
		return m.updateRadio(msg, persistentOptions())
	case screenImage:
		return m.updateImage(msg)
	case screenTrigger:
		return m.updateRadio(msg, triggerOptions())
	case screenScope:
		return m.updateScopeMulti(msg)
	case screenVarName:
		return m.updateVarName(msg)
	case screenVarSecret:
		return m.updateRadio(msg, secretOptions())
	case screenAddAnother:
		return m.updateRadio(msg, addAnotherOptions())
	case screenConfirm:
		return m.updateConfirm(msg)
	}

	return m, nil
}

// Run launches the TUI for adding a resource and returns the result.
// existingNames is the set of names already present in the target section;
// it is used to decide whether to pre-fill the name input with the provider name.
func Run(domain, provider string, existingNames map[string]bool) (Result, error) {
	m := initialModel(domain, provider, existingNames)
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		return Result{}, err
	}

	final := result.(model)
	if final.quitting {
		return Result{}, fmt.Errorf("cancelled")
	}

	r := Result{
		Name:  final.name,
		Entry: final.buildEntry(),
	}
	for _, v := range final.vars {
		r.ProviderVars = append(r.ProviderVars, ProviderVar{Name: v.name, Secret: v.secret})
	}
	return r, nil
}
