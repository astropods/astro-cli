package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerJobRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/jobs/kinds", s.handleListJobKinds)
	mux.HandleFunc("POST /api/admin/jobs/trigger", s.handleTriggerJob)
	mux.HandleFunc("GET /api/admin/jobs/states", s.handleGetJobStates)
	mux.HandleFunc("GET /api/admin/jobs/queues", s.handleListAdminQueues)
	mux.HandleFunc("GET /api/admin/jobs", s.handleListJobs)
	mux.HandleFunc("GET /api/admin/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("POST /api/admin/jobs/cancel", s.handleCancelJobs)
	mux.HandleFunc("POST /api/admin/jobs/retry", s.handleRetryJobs)
	mux.HandleFunc("POST /api/admin/queues/{name}/pause", s.handlePauseQueue)
	mux.HandleFunc("POST /api/admin/queues/{name}/resume", s.handleResumeQueue)
}

func (s *Server) handleListJobKinds(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListJobKinds(r.Context(), &adminv1.ListJobKindsRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleTriggerJob(w http.ResponseWriter, r *http.Request) {
	var req adminv1.TriggerJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := s.admin.TriggerJob(r.Context(), &req)
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetJobStates(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.GetJobStates(r.Context(), &adminv1.GetJobStatesRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListAdminQueues(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListAdminQueues(r.Context(), &adminv1.ListAdminQueuesRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := &adminv1.ListJobsRequest{
		State: q.Get("state"),
		Queue: q.Get("queue"),
	}
	if kinds := q["kinds"]; len(kinds) > 0 {
		req.Kinds = kinds
	}
	if lim := q.Get("limit"); lim != "" {
		var n int
		if _, err := fmt.Sscan(lim, &n); err == nil {
			req.Limit = n
		}
	}
	if before := q.Get("before_id"); before != "" {
		var id int64
		if _, err := fmt.Sscan(before, &id); err == nil {
			req.BeforeID = id
		}
	}
	if anchor := q.Get("anchor_id"); anchor != "" {
		var id int64
		if _, err := fmt.Sscan(anchor, &id); err == nil {
			req.AnchorID = id
		}
	}
	resp, err := s.admin.ListJobs(r.Context(), req)
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid job id")
		return
	}
	resp, err := s.admin.GetJob(r.Context(), &adminv1.GetJobRequest{ID: id})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCancelJobs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := s.admin.CancelJobs(r.Context(), &adminv1.CancelJobsRequest{IDs: body.IDs})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRetryJobs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := s.admin.RetryJobs(r.Context(), &adminv1.RetryJobsRequest{IDs: body.IDs})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePauseQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.admin.PauseQueue(r.Context(), &adminv1.PauseQueueRequest{Name: name})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleResumeQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.admin.ResumeQueue(r.Context(), &adminv1.ResumeQueueRequest{Name: name})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
