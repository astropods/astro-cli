package server

import (
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) handleListAuthorizationResources(w http.ResponseWriter, r *http.Request) {
	response, err := s.admin.ListAuthorizationResources(r.Context(), &adminv1.ListAuthorizationResourcesRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}
