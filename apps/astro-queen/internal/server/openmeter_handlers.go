package server

import (
	"io"
	"net/http"
	"strings"
)

func (s *Server) registerOpenMeterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/openmeter/", s.omReverseProxy)
}

func (s *Server) omReverseProxy(w http.ResponseWriter, r *http.Request) {
	// Strip /api/openmeter prefix, forward the rest to the OpenMeter server.
	target := strings.TrimRight(s.omServer, "/") + strings.TrimPrefix(r.URL.RequestURI(), "/api/openmeter")

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Copy headers from the original request
	for k, vv := range r.Header {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	// Set auth if configured
	if s.omAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.omAPIKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	// Copy response headers and status
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}
