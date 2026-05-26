package cmd

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/astropods/astro/apps/astro-cli/internal/tui"
)

// promptKeyMap returns the canonical huh keymap. esc and ctrl+c both abort.
// huh's footer only renders the focused field's KeyBinds — the form-level
// Quit binding is never shown there. The "esc cancel" hint is rendered by
// promptModel.View instead.
func promptKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = tui.Cancel
	return km
}

// promptModel wraps a huh.Form to append a uniform "esc cancel" hint on its
// own line below huh's per-field footer, and to translate completion/abort
// into tea.Quit without relying on Form.Run's SubmitCmd/CancelCmd plumbing.
type promptModel struct {
	form *huh.Form
}

func (m *promptModel) Init() tea.Cmd { return m.form.Init() }

func (m *promptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.form.Update(msg)
	if f, ok := next.(*huh.Form); ok {
		m.form = f
	}
	if m.form.State == huh.StateCompleted || m.form.State == huh.StateAborted {
		return m, tea.Quit
	}
	return m, cmd
}

func (m *promptModel) View() string {
	v := strings.TrimRight(m.form.View(), "\n")
	hint := tui.Hint(cliHuhTheme(), tui.Cancel)
	return v + "\n" + hint + "\n"
}

// runForm applies the shared theme and keymap to form, runs it inside a
// promptModel wrapper so every form gets the same "esc cancel" footer hint,
// and normalizes user cancellation (esc / ctrl+c) to tui.ErrCancelled.
func runForm(form *huh.Form) error {
	form = form.WithTheme(cliHuhTheme()).WithKeyMap(promptKeyMap())
	if _, err := tea.NewProgram(&promptModel{form: form}).Run(); err != nil {
		return err
	}
	if form.State == huh.StateAborted {
		return tui.ErrCancelled
	}
	return nil
}

// printCancelled writes a uniform "Cancelled." message to w. Use after a
// runForm or TUI returns tui.ErrCancelled when the command should exit 0.
func printCancelled(w io.Writer) {
	fmt.Fprintf(w, "%sCancelled.%s\n", colorDim, colorReset) //nolint:errcheck,gosec
}
