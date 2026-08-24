package server

import (
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerAuditRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/audit-findings", s.handleListAuditFindings)
	mux.HandleFunc("POST /api/admin/audit-findings/acknowledge", s.handleAcknowledgeAuditFinding)
}

func (s *Server) handleListAuditFindings(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListAuditFindings(r.Context(), &adminv1.ListAuditFindingsRequest{
		IncludeResolved: r.URL.Query().Get("include_resolved") == "true",
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAcknowledgeAuditFinding(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CheckName string `json:"check_name"`
		SubjectID string `json:"subject_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := s.admin.AcknowledgeAuditFinding(r.Context(), &adminv1.AcknowledgeAuditFindingRequest{
		CheckName: body.CheckName,
		SubjectID: body.SubjectID,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
