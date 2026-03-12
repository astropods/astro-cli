package server

import (
	"io"
	"net/http"
	"strings"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerAstroProxyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/astro/", s.astroReverseProxy)
}

func (s *Server) astroReverseProxy(w http.ResponseWriter, r *http.Request) {
	// Strip /api/astro prefix to get the path for the upstream gin router.
	path := strings.TrimPrefix(r.URL.RequestURI(), "/api/astro")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	headers := make(map[string]string, len(r.Header))
	for k := range r.Header {
		headers[k] = r.Header.Get(k)
	}

	resp, err := s.admin.ProxyHTTP(r.Context(), &adminv1.HTTPProxyRequest{
		Method:  r.Method,
		Path:    path,
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(int(resp.StatusCode))
	_, _ = w.Write(resp.Body)
}
