// Package chatui serves astro-client's chat experience locally during
// `ast dev` / `ast project`.
//
// It embeds the prebuilt chat SPA (see embed.go) and serves it, while exposing
// the same deployment-scoped HTTP contract astro-server presents in production
// (`/api/v1/deployments/:id/{chat,messaging,files}/*`). Read endpoints the chat shell
// needs (deployments summary/list, status, runtime, the account's chats and their
// status) are synthesized for a single local deployment, and every dry-run
// permission check is allowed; chat/messaging traffic is proxied verbatim to the
// local messaging sidecar. Because the contract matches production, the embedded React
// runs unchanged whether the agent is local or deployed.
package chatui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	// LocalAccount and LocalDeploymentID name the single synthetic account and
	// deployment the chat shell sees locally. Keep in sync with the chat client's
	// chat-embed entry point.
	LocalAccount      = "local"
	LocalDeploymentID = "local"

	// HealthPath is the internal readiness endpoint the CLI polls after spawning
	// the detached worker: it confirms the worker actually bound, and (via the
	// returned pid) that the responder is that very worker rather than an
	// orphaned one still holding the port.
	HealthPath = "/__chatui/health"

	// localDeploymentStatus is the status the synthesized deployment reports to
	// the chat shell. The local agent is running whenever this server is.
	localDeploymentStatus = "active"
)

// healthResponse is the body served at HealthPath. PID lets the CLI distinguish
// "our worker is up" from "someone else is answering on this port".
type healthResponse struct {
	OK    bool   `json:"ok"`
	PID   int    `json:"pid"`
	Agent string `json:"agent"` // the spec name, so `project list` can match a worker to its project
}

// Config configures the local chat UI server.
type Config struct {
	// Addr is the host:port the chat UI listens on (e.g. "127.0.0.1:3100").
	Addr string
	// MessagingURL is the base URL of the local messaging sidecar's HTTP API
	// (e.g. "http://127.0.0.1:3110"); chat/messaging requests are proxied here.
	MessagingURL string
	// AgentName / AgentDisplay describe the local agent for the synthesized
	// deployment record the chat list renders.
	AgentName    string
	AgentDisplay string
}

// Server is the local chat UI HTTP server.
type Server struct {
	cfg       Config
	dist      fs.FS
	hasAssets bool
	proxy     *httputil.ReverseProxy
}

// New builds a Server. It returns an error only when MessagingURL is unparseable.
func New(cfg Config) (*Server, error) {
	target, err := url.Parse(cfg.MessagingURL)
	if err != nil {
		return nil, fmt.Errorf("invalid messaging URL %q: %w", cfg.MessagingURL, err)
	}

	dist, err := fs.Sub(distFS, "webdist")
	if err != nil {
		return nil, fmt.Errorf("open embedded chat assets: %w", err)
	}
	hasAssets := false
	if f, err := dist.Open("index.html"); err == nil {
		_ = f.Close()
		hasAssets = true
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			// req.URL.Path is rewritten to the messaging-native path by the
			// route handler before ServeHTTP is invoked.
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			log.Printf("chatui: messaging proxy error: %v", err)
			http.Error(w, "messaging sidecar unreachable", http.StatusBadGateway)
		},
	}

	return &Server{cfg: cfg, dist: dist, hasAssets: hasAssets, proxy: proxy}, nil
}

// Handler returns the configured router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Internal readiness probe (see HealthPath / cmd.startChatUI).
	mux.HandleFunc("GET "+HealthPath, s.handleHealth)

	// Synthesized read endpoints for the single local deployment.
	mux.HandleFunc("GET /api/v1/deployments/summary", s.handleSummary)
	mux.HandleFunc("GET /api/v1/deployments", s.handleListDeployments)
	mux.HandleFunc("GET /api/v1/deployments/{id}/status", s.handleStatus)
	mux.HandleFunc("GET /api/v1/deployments/{id}/runtime", s.handleRuntime)
	mux.HandleFunc("GET /api/v1/deployments/{id}/connections", s.handleConnections)
	mux.HandleFunc("GET /api/v1/accounts/{account}/chats", s.handleChats)
	mux.HandleFunc("GET /api/v1/accounts/{account}/chats/{id}/status", s.handleChatStatus)

	// Proxy the deployment-scoped chat + messaging contract to the sidecar,
	// translating to its native /api/* and /api/chat/* paths — exactly the
	// rewrite astro-server's messaging proxy performs in production.
	mux.HandleFunc("/api/v1/deployments/{id}/messaging/{rest...}", s.proxyMessaging)
	mux.HandleFunc("/api/v1/deployments/{id}/chat/{rest...}", s.proxyChat)

	// Files API — the chat composer's attachments (upload/list/download/delete)
	// hit /files directly (not under /messaging), mirroring astro-server's
	// dedicated files proxy. Both the bare list/create path and the /{key}[/content]
	// sub-paths rewrite to the sidecar's native /api/files* routes.
	mux.HandleFunc("/api/v1/deployments/{id}/files", s.proxyFiles)
	mux.HandleFunc("/api/v1/deployments/{id}/files/{rest...}", s.proxyFiles)

	// SPA (must be last so /api/* always wins).
	mux.Handle("/", s.spaHandler())

	return answerDryRuns(mux)
}

// answerDryRuns allows every dry-run permission check (?dryrun=true), as
// astro-server answers one it would allow: 204, nothing done. The local
// developer may do anything with their own agent, and a dry run must never
// reach the sidecar proxy, which would carry the request out.
func answerDryRuns(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dryRun, _ := strconv.ParseBool(r.URL.Query().Get("dryrun")); dryRun && strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ListenAndServe starts the server and blocks until ctx is canceled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// The shutdown deadline is intentionally derived from a fresh context: ctx is
	// already canceled by the time this goroutine wakes, so it can't carry the
	// grace period.
	go func() { //nolint:gosec // G118: shutdown ctx must outlive the already-canceled ctx
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) proxyMessaging(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	r.URL.Path = "/api/" + rest
	r.URL.RawPath = ""
	s.proxy.ServeHTTP(w, r)
}

func (s *Server) proxyChat(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	r.URL.Path = "/api/chat/" + rest
	r.URL.RawPath = ""
	s.proxy.ServeHTTP(w, r)
}

// proxyFiles serves both the bare files path (list/create) and the
// /{key}[/content] sub-paths, rewriting to the sidecar's /api/files* routes.
// The bare route yields an empty "rest".
func (s *Server) proxyFiles(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	if rest == "" {
		r.URL.Path = "/api/files"
	} else {
		r.URL.Path = "/api/files/" + rest
	}
	r.URL.RawPath = ""
	s.proxy.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{OK: true, PID: os.Getpid(), Agent: s.cfg.AgentName})
}

// handleSummary reports the same deployment handleListDeployments does. The
// chat shell reconciles the two and refetches the summary for as long as a
// chat-eligible deployment is missing from it, so an empty summary costs a
// summary read on every mount of the chat page.
func (s *Server) handleSummary(w http.ResponseWriter, _ *http.Request) {
	local := s.localDeployment()
	writeJSON(w, http.StatusOK, deploymentsSummaryResponse{
		Accounts: []accountDeploymentsSummary{{
			ID:          LocalAccount,
			Name:        LocalAccount,
			Type:        "personal",
			DisplayName: "Local",
			Deployments: []deploymentSummaryItem{{
				ID:                     local.ID,
				Name:                   local.Name,
				DisplayName:            local.DisplayName,
				Status:                 localDeploymentStatus,
				MessagingWebConfigured: local.MessagingWebConfigured,
			}},
		}},
	})
}

func (s *Server) handleListDeployments(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, deploymentsListResponse{
		Deployments: []agentDeploymentSummary{s.localDeployment()},
		Count:       1,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, localStatus())
}

func (s *Server) handleRuntime(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, deploymentRuntimeResponse{Runtime: localRuntime()})
}

// handleChats lists the agents the viewer may web chat with: the local one.
func (s *Server) handleChats(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("account") != LocalAccount {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
		return
	}
	local := s.localDeployment()
	chats := []chat{}
	if local.MessagingWebConfigured {
		chats = append(chats, chat{Deployment: chatDeployment{
			ID:          local.ID,
			Name:        local.Name,
			DisplayName: local.DisplayName,
			Status:      localDeploymentStatus,
		}})
	}
	writeJSON(w, http.StatusOK, listChatsResponse{Chats: chats})
}

// handleChatStatus gives the chat composer's readiness: the same answer
// handleStatus and handleRuntime give, in the shape astro-server's chat status
// route uses.
func (s *Server) handleChatStatus(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("account") != LocalAccount || r.PathValue("id") != LocalDeploymentID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "deployment not found"})
		return
	}
	status := localStatus()
	writeJSON(w, http.StatusOK, chatStatusResponse{
		Status:             chatStatusValue{Value: status.Value, Reason: status.Reason},
		MessagingReachable: localRuntime().MessagingReachable,
	})
}

// localStatus is the status the local deployment reports: running whenever this
// server is.
func localStatus() deploymentStatus {
	return deploymentStatus{Value: localDeploymentStatus, Reason: "ready", Details: "Local agent"}
}

func localRuntime() deploymentRuntime {
	return deploymentRuntime{Ready: 1, Replicas: 1, MessagingReachable: true}
}

func (s *Server) handleConnections(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, deploymentConnectionsResponse{Connections: []any{}})
}

func (s *Server) localDeployment() agentDeploymentSummary {
	display := s.cfg.AgentDisplay
	if display == "" {
		display = s.cfg.AgentName
	}
	return agentDeploymentSummary{
		ID:                     LocalDeploymentID,
		Name:                   s.cfg.AgentName,
		DisplayName:            display,
		BuildID:                "local",
		MessagingWebConfigured: true,
		CreatedAt:              time.Unix(0, 0).UTC().Format(time.RFC3339),
	}
}

// spaHandler serves embedded static files, falling back to index.html for
// client-side routes. When only the .gitkeep placeholder is embedded (no real
// build), it returns a clear 503 instead of a confusing 404.
func (s *Server) spaHandler() http.Handler {
	if !s.hasAssets {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w,
				"chat UI assets are not embedded in this build (the chat UI ships only in official release binaries)",
				http.StatusServiceUnavailable)
		})
	}
	fileServer := http.FileServer(http.FS(s.dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if name == "." {
			name = "index.html"
		}
		if _, err := s.dist.Open(name); err != nil {
			// Missing hashed assets and unmatched API paths are real 404s, not
			// client-side routes — don't mask them by serving index.html with a
			// 200. That would hand the browser HTML where it expected JS/CSS, or
			// JSON from a deployment endpoint whose shim isn't wired yet (an
			// Unexpected-token-'<' JSON parse error that's hard to trace back).
			if strings.HasPrefix(name, "assets/") || strings.HasPrefix(name, "api/") {
				http.NotFound(w, r)
				return
			}
			// Unknown non-asset path — let the SPA's client-side router handle it.
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
