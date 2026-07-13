//go:build integration && chatembed

// This smoke test needs the chat SPA compiled into the binary (webdist is
// gitignored and only a .gitkeep is tracked). The generic `-tags integration`
// CLI job does NOT build the embed, so this file is gated behind an extra
// `chatembed` tag and run by a dedicated CI job that builds the embed first
// (moon run astro-cli:embed-chat-ui) and provisions Chrome. Locally:
//
//	moon run astro-cli:embed-chat-ui   # or: moon run astro-cli:link
//	go test -tags 'integration chatembed' -run TestDevChatEmbedSmoke ./e2e/...

package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-cli/internal/chatui"
	composeBuilder "github.com/astropods/astro/apps/astro-cli/internal/compose"
	spec "github.com/astropods/astro/packages/astro-spec"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDevChatEmbedSmoke is an end-to-end smoke test for the CLI-served chat
// experience (feat/playground-deprecation). The build pipeline guarantees the
// embedded SPA shares astro-client's chat *components*, but two seams are
// hand-maintained and can drift silently without a test:
//
//  1. The embedded SPA is actually compiled into the CLI — webdist has a real
//     index.html, not just the tracked .gitkeep placeholder (which serves a 503).
//  2. The CLI's synthesized deployment endpoints + messaging proxy match what
//     the chat shell calls, AND the messaging sidecar comes up healthy on a
//     fresh (root-owned) named volume — i.e. the astro-messaging-init chown
//     container works, so the proxy reaches the sidecar instead of 502-ing.
//
// It boots the real messaging sidecar (+ init container) via the compose SDK and
// runs the real chatui.Server against it, then asserts the HTTP contract the chat
// shell depends on. Requires a local Docker daemon:
//
//	go test -tags integration -run TestDevChatEmbedSmoke ./e2e/...
//
// It stops short of a live agent streamed reply (that needs a real agent image);
// a browser-driven variant would additionally catch chat-embed shell runtime
// errors.
const chatEmbedAgent = "chat-embed-smoke"

// messagingHostPort is a high, uncommon host port for the sidecar's HTTP API so
// the test never collides with a real `ast dev` session (which uses 3110).
const messagingHostPort = "13110"

func TestDevChatEmbedSmoke(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
	svc := newIntegrationCompose(t)

	s := &spec.AstroSpec{
		Name:  chatEmbedAgent,
		Agent: spec.Container{Image: longRunningImage}, // spec validity only
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{Adapters: []string{"web"}},
			},
		},
	}

	project, err := composeBuilder.BuildProject(s, t.TempDir(), nil)
	require.NoError(t, err, "BuildProject")

	// Publish the sidecar's HTTP port on a high host port to avoid colliding with
	// a real `ast dev` on 3110; drop the gRPC publish (no agent runs here).
	msg := project.Services["astro-messaging"]
	msg.Ports = []types.ServicePortConfig{{Target: 8080, Published: messagingHostPort}}
	project.Services["astro-messaging"] = msg

	// The volume-ownership bug only manifests on a *fresh* (root-owned) named
	// volume, so the test must start from one. Docker's Down keeps named volumes,
	// so remove the chat-data volume up front and in cleanup; otherwise a volume
	// chowned by a prior run would mask a regression.
	var chatVol string
	for name := range project.Volumes {
		if strings.HasSuffix(name, "-chat-data") {
			chatVol = name
		}
	}
	require.NotEmpty(t, chatVol, "expected a *-chat-data volume in the built project")
	removeVolume(chatVol)

	upCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	// No Services filter: brings up astro-messaging-init (chown) → astro-messaging
	// in dependency order. The nginx "agent" also starts but is unused.
	require.NoError(t, svc.Up(upCtx, project, api.UpOptions{
		Create: api.CreateOptions{RemoveOrphans: true},
		Start:  api.StartOptions{Project: project},
	}), "svc.Up")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_ = svc.Down(ctx, composeBuilder.ProjectName(s), api.DownOptions{RemoveOrphans: true, Volumes: true})
		removeVolume(chatVol)
	})

	cs, err := chatui.New(chatui.Config{
		MessagingURL: "http://127.0.0.1:" + messagingHostPort,
		AgentName:    chatEmbedAgent,
		AgentDisplay: chatEmbedAgent,
	})
	require.NoError(t, err, "chatui.New")
	ui := httptest.NewServer(cs.Handler())
	t.Cleanup(ui.Close)

	// Seam 1: the embedded SPA is compiled in. A CLI built without
	// `astro-client:build-chat-embed` serves a 503 placeholder with no #root.
	body, code := httpGet(t, ui.URL+"/")
	require.Equal(t, http.StatusOK, code, "GET / (embedded chat SPA missing? run moon run astro-cli:link)")
	assert.Contains(t, body, `<div id="root">`,
		"GET / body has no #root div — embedded SPA not built into the CLI (503 placeholder?)")

	// Seam 2a: synthesized deployment endpoints the chat shell reads on load.
	var summary struct {
		Accounts []struct {
			ID string `json:"id"`
		} `json:"accounts"`
	}
	getJSON(t, ui.URL+"/api/v1/deployments/summary", &summary)
	require.Len(t, summary.Accounts, 1, "summary accounts")
	assert.Equal(t, chatui.LocalAccount, summary.Accounts[0].ID, "summary account id")

	var list struct {
		Deployments []struct {
			MessagingWebConfigured bool `json:"messaging_web_configured"`
		} `json:"deployments"`
		Count int `json:"count"`
	}
	getJSON(t, ui.URL+"/api/v1/deployments?account="+chatui.LocalAccount, &list)
	require.Equal(t, 1, list.Count, "deployments count")
	require.Len(t, list.Deployments, 1, "deployments")
	assert.True(t, list.Deployments[0].MessagingWebConfigured,
		"deployment messaging_web_configured = false, want true")

	var status struct {
		Value string `json:"value"`
	}
	getJSON(t, ui.URL+"/api/v1/deployments/"+chatui.LocalDeploymentID+"/status", &status)
	assert.Equal(t, "active", status.Value, "status.value")

	// An unmatched deployment endpoint must 404 — not fall through to the SPA and
	// return index.html with 200. Otherwise a caller of a not-yet-wired shim gets
	// HTML where it expects JSON (an opaque "Unexpected token '<'" parse error).
	_, notFoundCode := httpGet(t, ui.URL+"/api/v1/deployments/"+chatui.LocalDeploymentID)
	assert.Equal(t, http.StatusNotFound, notFoundCode,
		"unmatched /api path should 404, not fall through to the SPA")

	// Seam 2b: the proxy reaches a healthy sidecar. /messaging/chat/conversations
	// rewrites to the sidecar's /api/chat/conversations, which reads the SQLite
	// chat store — so a 200 proves the sidecar started on a fresh volume (the
	// init-container chown fix) and the proxy path is wired. A 502 means the
	// sidecar is down (volume-ownership regression) or the proxy is misrouted.
	deadline := time.Now().Add(90 * time.Second)
	var lastCode int
	var lastBody string
	for time.Now().Before(deadline) {
		lastBody, lastCode = httpGet(t, ui.URL+"/api/v1/deployments/"+chatui.LocalDeploymentID+"/messaging/chat/conversations")
		if lastCode == http.StatusOK {
			break
		}
		time.Sleep(2 * time.Second)
	}
	require.Equalf(t, http.StatusOK, lastCode,
		"proxy GET /messaging/chat/conversations (body %q); sidecar unhealthy or proxy misrouted "+
			"(502 => astro-messaging-init volume chown fix regressed?)", lastBody)

	// Seam 3 (browser): the embedded chat shell (src/chat-embed/main.tsx) actually
	// mounts in a real browser. The HTTP checks above can't catch a broken shell —
	// a missing provider/context/route in the hand-maintained entry throws at
	// runtime and leaves an empty #root. This drives headless Chrome against the
	// running chatui server and fails on an uncaught exception or an empty root.
	t.Run("browser_shell_mounts", func(t *testing.T) {
		chromePath := findChrome()
		if chromePath == "" {
			t.Skip("no Chrome/Chromium found; skipping browser shell smoke")
		}

		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.ExecPath(chromePath),
			chromedp.Headless,
			chromedp.NoSandbox,
			chromedp.DisableGPU,
		)
		allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
		defer cancelAlloc()
		bctx, cancelBrowser := chromedp.NewContext(allocCtx)
		defer cancelBrowser()
		// Generous ceiling: the embedded chat bundle is large and a cold headless
		// Chrome on a constrained CI runner can take tens of seconds to boot and
		// render before the async agent fetch even starts.
		bctx, cancelTimeout := context.WithTimeout(bctx, 120*time.Second)
		defer cancelTimeout()

		// A broken chat-embed shell throws on mount rather than rendering.
		var mu sync.Mutex
		var jsErrors []string
		chromedp.ListenTarget(bctx, func(ev any) {
			if ex, ok := ev.(*runtime.EventExceptionThrown); ok && ex.ExceptionDetails != nil {
				msg := ex.ExceptionDetails.Text
				if ex.ExceptionDetails.Exception != nil && ex.ExceptionDetails.Exception.Description != "" {
					msg = ex.ExceptionDetails.Exception.Description
				}
				mu.Lock()
				jsErrors = append(jsErrors, msg)
				mu.Unlock()
			}
		})

		require.NoError(t, chromedp.Run(bctx, chromedp.Navigate(ui.URL)), "navigate to chat shell")

		// Poll until the chat composer itself renders — not merely until #root has
		// "some" nodes. The loading/empty shell (spinner + app chrome) already
		// exceeds a handful of nodes, so gating on a raw node count races the async
		// agent fetch: on a slow runner we'd read the DOM mid-load, before the
		// composer mounts, and wrongly conclude the page is broken. The composer's
		// textarea carries a stable aria-label="Message input" and renders even
		// when the agent is "not ready", so its presence is the real signal that
		// the chat page reached the thread view (not a blank/error/loading shell).
		hasComposerJS := `!!document.querySelector('[aria-label="Message input"]')`
		deadline := time.Now().Add(90 * time.Second)
		var (
			nodeCount   int
			hasComposer bool
		)
		for time.Now().Before(deadline) {
			_ = chromedp.Run(bctx,
				chromedp.Evaluate(`document.querySelectorAll('#root *').length`, &nodeCount),
				chromedp.Evaluate(hasComposerJS, &hasComposer),
			)
			if hasComposer {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		mu.Lock()
		errs := append([]string(nil), jsErrors...)
		mu.Unlock()
		assert.Emptyf(t, errs, "uncaught JS exceptions while mounting the chat shell: %v", errs)

		if !hasComposer {
			// Dump what actually rendered so a CI failure is diagnosable instead of
			// opaque ("did it error? still loading? wrong page?").
			var rootHTML string
			_ = chromedp.Run(bctx, chromedp.OuterHTML("#root", &rootHTML, chromedp.ByID))
			const maxDump = 2000
			if len(rootHTML) > maxDump {
				rootHTML = rootHTML[:maxDump] + "…(truncated)"
			}
			require.Failf(t, "chat composer never rendered",
				"no composer within 90s (#root has %d elements); the embedded SPA did not reach the thread view.\n#root HTML:\n%s", nodeCount, rootHTML)
		}
	})
}

// findChrome locates a Chrome/Chromium binary for the browser smoke test, or
// returns "" so the test can skip cleanly (e.g. CI images without a browser).
func findChrome() string {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chrome", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, c := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// removeVolume best-effort deletes a named Docker volume so the test starts from
// a fresh, root-owned mountpoint (which is what triggers the ownership bug).
func removeVolume(name string) {
	_ = exec.Command("docker", "volume", "rm", "-f", name).Run()
}

func httpGet(t *testing.T, url string) (string, int) {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // short-lived test request
	require.NoErrorf(t, err, "GET %s", url)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoErrorf(t, err, "read %s body", url)
	return string(b), resp.StatusCode
}

func getJSON(t *testing.T, url string, dst any) {
	t.Helper()
	body, code := httpGet(t, url)
	require.Equalf(t, http.StatusOK, code, "GET %s (body %q)", url, body)
	require.NoErrorf(t, json.Unmarshal([]byte(body), dst), "decode %s (body %q)", url, body)
}
