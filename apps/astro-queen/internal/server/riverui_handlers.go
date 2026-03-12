package server

import (
	"io"
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerRiverUIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/riverui/", s.riverUIProxy)
}

// riverUIProxy forwards /riverui/* requests to astro-server's internal River UI
// via the admin gRPC ProxyHTTP call. River UI is never exposed on astro-server's
// public HTTP port — it only exists behind the admin gRPC boundary.
func (s *Server) riverUIProxy(w http.ResponseWriter, r *http.Request) {
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
		Path:    r.URL.RequestURI(),
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
