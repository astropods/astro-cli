package server

import (
	"io"
	"net/http"
	"strings"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerOpenMeterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/openmeter/", s.omReverseProxy)
}

func (s *Server) omReverseProxy(w http.ResponseWriter, r *http.Request) {
	// Strip /api/openmeter prefix to get the relative path for the upstream.
	path := strings.TrimPrefix(r.URL.RequestURI(), "/api/openmeter")

	// Read incoming body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Collect single-value headers
	headers := make(map[string]string, len(r.Header))
	for k := range r.Header {
		headers[k] = r.Header.Get(k)
	}

	resp, err := s.admin.ProxyOpenMeter(r.Context(), &adminv1.OpenMeterProxyRequest{
		Method:  r.Method,
		Path:    path,
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	// Copy response headers and status
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(int(resp.StatusCode))
	_, _ = w.Write(resp.Body)
}
