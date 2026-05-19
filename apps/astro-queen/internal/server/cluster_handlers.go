package server

import (
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerClusterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/clusters", s.handleListClusters)
	mux.HandleFunc("POST /api/admin/clusters", s.handleRegisterCluster)
	mux.HandleFunc("POST /api/admin/clusters/{id}/enable", s.handleEnableCluster)
	mux.HandleFunc("POST /api/admin/clusters/{id}/disable", s.handleDisableCluster)
	mux.HandleFunc("DELETE /api/admin/clusters/{id}", s.handleDeregisterCluster)
}

func (s *Server) handleListClusters(w http.ResponseWriter, r *http.Request) {
	enabledOnly := r.URL.Query().Get("enabled_only") == "true"
	resp, err := s.admin.ListClusters(r.Context(), &adminv1.ListClustersRequest{
		EnabledOnly: enabledOnly,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRegisterCluster(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID                 string `json:"id"`
		Region             string `json:"region"`
		EKSClusterName     string `json:"eks_cluster_name"`
		EKSClusterEndpoint string `json:"eks_cluster_endpoint"`
		Enabled            *bool  `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	req := &adminv1.RegisterClusterRequest{
		ID:                 body.ID,
		Region:             body.Region,
		EKSClusterName:     body.EKSClusterName,
		EKSClusterEndpoint: body.EKSClusterEndpoint,
	}
	if body.Enabled != nil {
		req.Enabled = body.Enabled
	}

	resp, err := s.admin.RegisterCluster(r.Context(), req)
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleEnableCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.EnableCluster(r.Context(), &adminv1.EnableClusterRequest{ID: id})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDisableCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.DisableCluster(r.Context(), &adminv1.DisableClusterRequest{ID: id})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeregisterCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, err := s.admin.DeregisterCluster(r.Context(), &adminv1.DeregisterClusterRequest{ID: id})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
