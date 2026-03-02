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

	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"

	"github.com/postman/astro/apps/astro-queen/internal/openmeter"
)

// Server is an HTTP server that proxies admin gRPC and OpenMeter REST APIs
// and serves the embedded React SPA.
type Server struct {
	admin   adminv1.AdminServiceClient
	om      *openmeter.Client
	webFS   fs.FS
	port    int
	httpSrv *http.Server
}

// New creates a new Server.
func New(admin adminv1.AdminServiceClient, om *openmeter.Client, webFS fs.FS, port int) *Server {
	return &Server{
		admin: admin,
		om:    om,
		webFS: webFS,
		port:  port,
	}
}

// ListenAndServe starts the HTTP server and blocks until SIGINT/SIGTERM.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()

	// Admin API routes
	s.registerAdminRoutes(mux)

	// OpenMeter API routes
	s.registerOpenMeterRoutes(mux)

	// SPA fallback
	s.registerSPAHandler(mux)

	s.httpSrv = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: mux,
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
	fileServer := http.FileServer(http.FS(s.webFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// API routes are handled by their own handlers
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the file directly
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if file exists in the embedded FS
		f, err := s.webFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for client-side routing
		r.URL.Path = "/index.html"
		fileServer.ServeHTTP(w, r)
	})
}
