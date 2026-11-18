package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

type shellResize struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// shellDialer is replaced in tests.
var shellDialer = websocket.DefaultDialer

func websocketURL(httpURL string) string {
	if rest, ok := strings.CutPrefix(httpURL, "https://"); ok {
		return "wss://" + rest
	}
	return "ws://" + strings.TrimPrefix(httpURL, "http://")
}

// runShell relays a terminal between the local terminal and the server's
// WebSocket until either side closes it.
func runShell(ctx context.Context, httpURL, token, origin string, in io.Reader, out io.Writer) error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("Origin", strings.TrimSuffix(origin, "/"))

	conn, resp, err := shellDialer.DialContext(ctx, websocketURL(httpURL), header)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close() //nolint:errcheck
	}
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return errSandboxShellNotFound()
		}
		return errSandboxShellConnect(err)
	}
	defer conn.Close() //nolint:errcheck

	var writeMu sync.Mutex
	send := func(kind int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(kind, data)
	}
	resize := func(cols, rows int) {
		body, err := json.Marshal(shellResize{Type: "resize", Cols: cols, Rows: rows})
		if err == nil {
			_ = send(websocket.TextMessage, body)
		}
	}

	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		state, err := term.MakeRaw(int(f.Fd()))
		if err == nil {
			defer term.Restore(int(f.Fd()), state) //nolint:errcheck
		}
		stop := watchTerminalSize(int(f.Fd()), resize)
		defer stop()
	} else {
		resize(80, 24)
	}

	done := make(chan error, 1)
	go func() {
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				var closeErr *websocket.CloseError
				if errors.As(err, &closeErr) {
					err = nil
				}
				done <- err
				return
			}
			if kind == websocket.BinaryMessage {
				if _, err := out.Write(data); err != nil {
					done <- err
					return
				}
			}
		}
	}()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := in.Read(buf)
			if n > 0 {
				if werr := send(websocket.BinaryMessage, append([]byte(nil), buf[:n]...)); werr != nil {
					return
				}
			}
			if err != nil {
				writeMu.Lock()
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
				writeMu.Unlock()
				return
			}
		}
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return nil
	}
}
