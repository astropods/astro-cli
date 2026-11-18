//go:build !windows

package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

// watchTerminalSize sends the terminal's size now and on every SIGWINCH.
func watchTerminalSize(fd int, resize func(cols, rows int)) (stop func()) {
	send := func() {
		if cols, rows, err := term.GetSize(fd); err == nil {
			resize(cols, rows)
		}
	}
	send()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				send()
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}
