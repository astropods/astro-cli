package sandbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"time"
)

// The deployed control plane serves these five paths, and the agent SDK calls
// them. The broker serves the same paths with the same bodies, so one agent
// source runs locally and deployed.
//
// The deployed control plane owns a reservation, caps, a quota, an account gate
// and an archive. None of that exists here: local runs one developer, and a
// container is created on attach.

// HandleResponse is the body of an attach. It mirrors astro-server's
// SandboxHandleResponse.
type HandleResponse struct {
	Name      string            `json:"name"`
	Class     string            `json:"class"`
	State     string            `json:"state"`
	Endpoint  string            `json:"endpoint"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt string            `json:"expires_at"`
}

// RecordResponse mirrors astro-server's SandboxResponse.
type RecordResponse struct {
	Name         string `json:"name"`
	Class        string `json:"class"`
	State        string `json:"state"`
	CreatedAt    string `json:"created_at"`
	LastActiveAt string `json:"last_active_at"`
	CeilingAt    string `json:"ceiling_at,omitempty"`
}

// ListResponse mirrors astro-server's SandboxListResponse.
type ListResponse struct {
	Sandboxes []RecordResponse `json:"sandboxes"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// DefaultClass is the only class the broker serves, matching the deployed
// control plane's ResolvePolicy.
const DefaultClass = "default"

// sessionTokenHeader authorizes a request to the sandbox's data plane. The
// deployed control plane returns the same header, alongside the platform's own
// credential, which does not exist locally. The agent treats the map as opaque.
const sessionTokenHeader = "X-Access-Token" //nolint:gosec // a header name

// Sandboxes is the runtime the server drives.
type Sandboxes interface {
	Ensure(ctx context.Context, name, token string) (Instance, error)
	Remove(ctx context.Context, name string) error
	List(ctx context.Context) ([]Record, error)
}

// Record is a sandbox the runtime knows about.
type Record struct {
	Name      string
	State     string
	CreatedAt time.Time
}

// Server serves the control-plane routes for `ast dev`.
type Server struct {
	sandboxes Sandboxes
	secret    string
	log       *slog.Logger

	// tokens holds the session token installed in each sandbox, so a second
	// attach returns the token the running sandbox already accepts.
	tokens *tokenStore
}

func NewServer(sandboxes Sandboxes, secret string, log *slog.Logger) *Server {
	return &Server{
		sandboxes: sandboxes,
		secret:    secret,
		log:       log,
		tokens:    newTokenStore(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/sandboxes/{name}", s.authorize(http.HandlerFunc(s.attach)))
	mux.Handle("GET /api/v1/sandboxes", s.authorize(http.HandlerFunc(s.list)))
	mux.Handle("GET /api/v1/sandboxes/{name}", s.authorize(http.HandlerFunc(s.get)))
	mux.Handle("DELETE /api/v1/sandboxes/{name}", s.authorize(http.HandlerFunc(s.remove)))
	mux.Handle("POST /api/v1/sandboxes/{name}/stop", s.authorize(http.HandlerFunc(s.stop)))
	return mux
}

// authorize refuses a request the deployed control plane would refuse, so an
// author sees the same fault locally.
func (s *Server) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := DeploymentFromRequest(r, s.secret); err != nil {
			WriteAuthError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) attach(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "sandbox name is required")
		return
	}

	token := s.tokens.get(name)
	inst, err := s.sandboxes.Ensure(r.Context(), name, token)
	if err != nil {
		s.log.Error("sandbox broker: attach failed", "error", err, "name", name)
		s.writeError(w, http.StatusBadGateway, "could not bring the sandbox up")
		return
	}
	if inst.Started {
		// Ensure installed the token this attach generated, so record it for
		// the next attach on this name.
		s.tokens.set(name, token)
	}

	s.writeJSON(w, http.StatusOK, HandleResponse{
		Name:     name,
		Class:    DefaultClass,
		State:    "running",
		Endpoint: inst.Endpoint,
		Headers:  map[string]string{sessionTokenHeader: token},
		// A local session token does not expire. The field carries a far
		// expiry rather than nothing, because the SDK refreshes on it.
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour).UTC().Format(time.RFC3339),
	})
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	records, err := s.sandboxes.List(r.Context())
	if err != nil {
		s.log.Error("sandbox broker: get failed", "error", err, "name", name)
		s.writeError(w, http.StatusInternalServerError, "sandbox request failed")
		return
	}
	for _, rec := range records {
		if rec.Name == name {
			s.writeJSON(w, http.StatusOK, recordResponse(rec))
			return
		}
	}
	s.writeError(w, http.StatusNotFound, "sandbox not found")
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	records, err := s.sandboxes.List(r.Context())
	if err != nil {
		s.log.Error("sandbox broker: list failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "sandbox request failed")
		return
	}

	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.After(records[j].CreatedAt) })

	out := ListResponse{Sandboxes: make([]RecordResponse, 0, len(records))}
	for _, rec := range records {
		out.Sandboxes = append(out.Sandboxes, recordResponse(rec))
	}
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.sandboxes.Remove(r.Context(), name); err != nil {
		s.log.Error("sandbox broker: delete failed", "error", err, "name", name)
		s.writeError(w, http.StatusInternalServerError, "sandbox request failed")
		return
	}
	s.tokens.forget(name)
	w.WriteHeader(http.StatusNoContent)
}

// stop drops the sandbox. The deployed control plane suspends the instance and
// keeps the workspace; a container has no suspend that preserves a session, so
// stop and delete are the same operation here.
func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	s.remove(w, r)
}

func recordResponse(rec Record) RecordResponse {
	return RecordResponse{
		Name:         rec.Name,
		Class:        DefaultClass,
		State:        rec.State,
		CreatedAt:    rec.CreatedAt.UTC().Format(time.RFC3339),
		LastActiveAt: rec.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// writeJSON marshals before writing the status, so a body that cannot be
// encoded becomes a 500 rather than a 200 with a truncated body.
func (s *Server) writeJSON(w http.ResponseWriter, status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		s.log.Error("sandbox broker: encode the response failed", "error", err)
		http.Error(w, `{"error":"sandbox request failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, errorResponse{Error: message})
}

var _ Sandboxes = (*Runner)(nil)
