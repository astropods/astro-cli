// Package tui exposes shared building blocks for interactive prompts.
package tui

import (
	"errors"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
)

// ErrCancelled is the canonical sentinel returned by every interactive surface
// (huh forms via cmd.runForm, and bubbletea TUIs in this directory) when the
// user presses esc or ctrl+c. Callers should branch with errors.Is so they can
// translate the cancellation into a clean exit instead of a command failure.
var ErrCancelled = errors.New("cancelled")

// Cancel is the canonical "esc cancel" footer hint binding shared by every
// interactive surface. esc and ctrl+c both abort.
var Cancel = key.NewBinding(
	key.WithKeys("esc", "ctrl+c"),
	key.WithHelp("esc", "cancel"),
)

// Help returns a bubbles/help.Model styled to match huh's theme so any hint
// rendered through it visually matches huh's own per-field footer. When theme
// is nil the help.New() defaults are used.
func Help(theme *huh.Theme) help.Model {
	h := help.New()
	if theme != nil {
		h.Styles = theme.Help
	}
	return h
}

// Hint renders bindings as a single-line short help string ("k1 d1 • k2 d2 …")
// using the given theme's help styles. Returns "" if no bindings are enabled.
func Hint(theme *huh.Theme, bindings ...key.Binding) string {
	return Help(theme).ShortHelpView(bindings)
}
