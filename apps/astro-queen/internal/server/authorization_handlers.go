package server

import (
	"net/http"
	"strconv"

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

func (s *Server) handleListAuthorizationOperations(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32)
	response, err := s.admin.ListAuthorizationOperations(r.Context(), &adminv1.ListAuthorizationOperationsRequest{Limit: int32(limit)})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleStartAuthorizationResourceReset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DryRun         bool   `json:"dry_run"`
		ConfirmedCount *int32 `json:"confirmed_count"`
		AccountID      string `json:"account_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	response, err := s.admin.StartAuthorizationResourceReset(r.Context(), &adminv1.StartAuthorizationResourceResetRequest{
		DryRun:         body.DryRun,
		ConfirmedCount: body.ConfirmedCount,
		AccountID:      body.AccountID,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func (s *Server) handleReleaseAuthorizationMaintenance(w http.ResponseWriter, r *http.Request) {
	response, err := s.admin.ReleaseAuthorizationMaintenance(r.Context(), &adminv1.ReleaseAuthorizationMaintenanceRequest{
		OperationID: r.PathValue("id"),
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}
