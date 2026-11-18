//go:build windows

package cmd

import "golang.org/x/term"

// watchTerminalSize sends the terminal's size once: Windows has no SIGWINCH.
func watchTerminalSize(fd int, resize func(cols, rows int)) (stop func()) {
	if cols, rows, err := term.GetSize(fd); err == nil {
		resize(cols, rows)
	}
	return func() {}
}
