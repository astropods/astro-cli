package server

import (
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerClusterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/clusters", s.handleListClusters)
	mux.HandleFunc("POST /api/admin/clusters", s.handleRegisterCluster)
	mux.HandleFunc("PUT /api/admin/clusters/{id}", s.handleUpdateCluster)
	mux.HandleFunc("POST /api/admin/clusters/{id}/enable", s.handleEnableCluster)
	mux.HandleFunc("POST /api/admin/clusters/{id}/disable", s.handleDisableCluster)
	mux.HandleFunc("POST /api/admin/clusters/{id}/health-check", s.handleCheckClusterHealth)
	mux.HandleFunc("DELETE /api/admin/clusters/{id}", s.handleDeregisterCluster)
	mux.HandleFunc("GET /api/admin/clusters/{id}/blockers", s.handleGetClusterBlockers)
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

// clusterDeployBody is the subset of fields every register / update payload
// must carry. Empty values are rejected by astro-server (no env fallback for
// non-primary clusters), so the UI must collect all of them.
type clusterDeployBody struct {
	AgentIngressDomain     string `json:"agent_ingress_domain"`
	IngestionIngressDomain string `json:"ingestion_ingress_domain"`
	LangfuseBaseURLExt     string `json:"langfuse_base_url_ext"`
	LangfuseVPCEIPs        string `json:"langfuse_vpce_ips"`
	PodSubnetCIDRs         string `json:"pod_subnet_cidrs"`
}

func (s *Server) handleRegisterCluster(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID                 string `json:"id"`
		Region             string `json:"region"`
		EKSClusterName     string `json:"eks_cluster_name"`
		EKSClusterEndpoint string `json:"eks_cluster_endpoint"`
		EKSClusterCA       []byte `json:"eks_cluster_ca"` // base64-encoded PEM CA; required
		Enabled            *bool  `json:"enabled"`
		clusterDeployBody
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	req := &adminv1.RegisterClusterRequest{
		ID:                     body.ID,
		Region:                 body.Region,
		EKSClusterName:         body.EKSClusterName,
		EKSClusterEndpoint:     body.EKSClusterEndpoint,
		EKSClusterCA:           body.EKSClusterCA,
		AgentIngressDomain:     body.AgentIngressDomain,
		IngestionIngressDomain: body.IngestionIngressDomain,
		LangfuseBaseURLExt:     body.LangfuseBaseURLExt,
		LangfuseVPCEIPs:        body.LangfuseVPCEIPs,
		PodSubnetCIDRs:         body.PodSubnetCIDRs,
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

func (s *Server) handleUpdateCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Region             string `json:"region"`
		EKSClusterName     string `json:"eks_cluster_name"`
		EKSClusterEndpoint string `json:"eks_cluster_endpoint"`
		EKSClusterCA       []byte `json:"eks_cluster_ca"` // base64-encoded PEM CA; required
		clusterDeployBody
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := s.admin.UpdateCluster(r.Context(), &adminv1.UpdateClusterRequest{
		ID:                     id,
		Region:                 body.Region,
		EKSClusterName:         body.EKSClusterName,
		EKSClusterEndpoint:     body.EKSClusterEndpoint,
		EKSClusterCA:           body.EKSClusterCA,
		AgentIngressDomain:     body.AgentIngressDomain,
		IngestionIngressDomain: body.IngestionIngressDomain,
		LangfuseBaseURLExt:     body.LangfuseBaseURLExt,
		LangfuseVPCEIPs:        body.LangfuseVPCEIPs,
		PodSubnetCIDRs:         body.PodSubnetCIDRs,
	})
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

func (s *Server) handleGetClusterBlockers(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.GetClusterBlockers(r.Context(), &adminv1.GetClusterBlockersRequest{ID: id})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
