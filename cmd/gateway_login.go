package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/tui"
)

// autoConnectGatewayAfterLogin never fails the login; a problem prints a hint.
func autoConnectGatewayAfterLogin(ctx context.Context, w io.Writer, verbose bool) {
	at, err := getCurrentAccountToken(ctx)
	if err != nil {
		return
	}
	mine, err := fetchMyGateways(ctx, at.Account, verbose)
	if err != nil {
		if !errors.Is(err, errGatewayServerUnavailable) {
			fmt.Fprintln(w, msgGatewayAutoConnectFailed(buildinfo.BinaryName, err)) //nolint:errcheck,gosec
		}
		return
	}
	state, err := loadGatewayState()
	if err != nil {
		fmt.Fprintln(w, msgGatewayAutoConnectFailed(buildinfo.BinaryName, err)) //nolint:errcheck,gosec
		return
	}
	if state.connected() {
		return
	}

	var candidates []string
	for _, a := range mine.Accounts {
		if a.AutoConnectOnLogin && !state.disconnectedFrom(a.Account) {
			candidates = append(candidates, a.Account)
		}
	}
	var account string
	switch {
	case len(candidates) == 0:
		return
	case len(candidates) == 1:
		account = candidates[0]
	case !interactiveTerminal():
		fmt.Fprintln(w)                                                                 //nolint:errcheck,gosec
		fmt.Fprintln(w, msgGatewayAutoConnectSkipped(buildinfo.BinaryName, candidates)) //nolint:errcheck,gosec
		return
	default:
		title, description := msgGatewayAutoConnectPick(buildinfo.BinaryName)
		picked, err := gatewayPick(title, description, candidates)
		if err != nil || strings.TrimSpace(picked) == "" {
			return
		}
		account = picked
	}

	fmt.Fprintln(w) //nolint:errcheck,gosec
	err = connectGateway(ctx, w, gatewayConnectOptions{
		account:     account,
		interactive: interactiveTerminal(),
		verbose:     verbose,
	})
	if err != nil && !errors.Is(err, tui.ErrCanceled) {
		fmt.Fprintln(w, msgGatewayAutoConnectFailed(buildinfo.BinaryName, err)) //nolint:errcheck,gosec
	}
}
