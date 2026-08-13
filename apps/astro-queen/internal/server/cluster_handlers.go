package server

import (
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerClusterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/clusters", s.handleListClusters)
	mux.HandleFunc("POST /api/admin/clusters/{id}/health-check", s.handleCheckClusterHealth)
	mux.HandleFunc("DELETE /api/admin/clusters/{id}", s.handleDeregisterCluster)
	mux.HandleFunc("GET /api/admin/clusters/{id}/blockers", s.handleGetClusterBlockers)
	mux.HandleFunc("POST /api/admin/clusters/{id}/refresh-pull-secrets", s.handleRefreshClusterPullSecrets)
}

func (s *Server) handleListClusters(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListClusters(r.Context(), &adminv1.ListClustersRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCheckClusterHealth(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.CheckClusterHealth(r.Context(), &adminv1.CheckClusterHealthRequest{ID: id})
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

func (s *Server) handleRefreshClusterPullSecrets(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.RefreshClusterPullSecrets(r.Context(), &adminv1.RefreshClusterPullSecretsRequest{ClusterID: id})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetClusterBlockers(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.GetClusterBlockers(r.Context(), &adminv1.GetClusterBlockersRequest{ID: id})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
