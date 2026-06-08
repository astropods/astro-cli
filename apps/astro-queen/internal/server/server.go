package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

// Server is an HTTP server that proxies admin gRPC calls
// and serves the embedded React SPA.
type Server struct {
	admin       adminv1.AdminServiceClient
	webFS       fs.FS
	port        int
	openapiJSON []byte
	env         string
	httpSrv     *http.Server
}

// New creates a new Server.
func New(admin adminv1.AdminServiceClient, webFS fs.FS, port int, openapiJSON []byte, env string) *Server {
	return &Server{
		admin:       admin,
		webFS:       webFS,
		port:        port,
		openapiJSON: openapiJSON,
		env:         env,
	}
}

// ListenAndServe starts the HTTP server and blocks until SIGINT/SIGTERM.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()

	// Admin API routes
	s.registerAdminRoutes(mux)
	s.registerQuotaRoutes(mux)
	s.registerClusterRoutes(mux)

	// OpenAPI spec
	mux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		writeRawJSON(w, http.StatusOK, s.openapiJSON)
	})

	// Environment info
	mux.HandleFunc("GET /api/env", func(w http.ResponseWriter, r *http.Request) {
		writeRawJSON(w, http.StatusOK, []byte(fmt.Sprintf(`{"env":%q}`, s.env)))
	})

	// OpenMeter reverse proxy
	s.registerOpenMeterRoutes(mux)

	// Astro server HTTP API proxy
	s.registerAstroProxyRoutes(mux)

	// Device auth flow (same as CLI — talks to WorkOS directly)
	mux.HandleFunc("POST /api/auth/device", s.handleDeviceAuthStart)
	mux.HandleFunc("POST /api/auth/device/poll", s.handleDeviceAuthPoll)

	// Job management routes (direct DB queries via admin gRPC)
	s.registerJobRoutes(mux)

	// SPA fallback
	s.registerSPAHandler(mux)

	s.httpSrv = &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("queen server listening on http://%s", s.httpSrv.Addr)
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("received %s, shutting down", sig)
	case err := <-errCh:
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) registerSPAHandler(mux *http.ServeMux) {
	// Pre-read index.html for SPA fallback
	indexHTML, _ := fs.ReadFile(s.webFS, "index.html")

	fileServer := http.FileServer(http.FS(s.webFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// API routes are handled by their own handlers
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve static files (JS, CSS, images)
		if r.URL.Path != "/" {
			f, err := s.webFS.Open(strings.TrimPrefix(r.URL.Path, "/"))
			if err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// SPA fallback: serve index.html directly (avoids FileServer redirect loop)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
}
