package server

import (
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerQuotaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/quota-requests", s.handleListQuotaRequests)
	mux.HandleFunc("POST /api/admin/quota-requests/{id}/approve", s.handleApproveQuotaRequest)
	mux.HandleFunc("POST /api/admin/quota-requests/{id}/deny", s.handleDenyQuotaRequest)
}

func (s *Server) handleListQuotaRequests(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	resp, err := s.admin.ListQuotaIncreaseRequests(r.Context(), &adminv1.ListQuotaIncreaseRequestsRequest{
		Status: status,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleApproveQuotaRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		GrantAmount float64 `json:"grant_amount"`
		Note        string  `json:"note"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Step 1: Approve in the DB via admin gRPC
	resp, err := s.admin.ApproveQuotaIncrease(r.Context(), &adminv1.ApproveQuotaIncreaseRequest{
		RequestID:   id,
		GrantAmount: body.GrantAmount,
		Note:        body.Note,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}

	// Step 2: Create grant in OpenMeter directly (queen has OpenMeter access)
	// The grant is created by the queen-side OpenMeter proxy, which the frontend
	// will call separately after approval. This keeps the approve RPC simple.

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDenyQuotaRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Note string `json:"note"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := s.admin.DenyQuotaIncrease(r.Context(), &adminv1.DenyQuotaIncreaseRequest{
		RequestID: id,
		Note:      body.Note,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
