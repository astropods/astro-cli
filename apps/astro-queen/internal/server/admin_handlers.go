package server

import (
	"encoding/json"
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/accounts", s.handleListAccounts)
	mux.HandleFunc("PUT /api/admin/accounts/{id}/rename", s.handleRenameAccount)
	mux.HandleFunc("PUT /api/admin/accounts/{id}/cluster", s.handleSetAccountCluster)
	mux.HandleFunc("POST /api/admin/accounts/{id}/invalidate-cache", s.handleInvalidateAccountCaches)
	mux.HandleFunc("POST /api/admin/invalidate-cache", s.handleInvalidateAllCaches)
	mux.HandleFunc("GET /api/admin/deployments", s.handleListDeployments)
	mux.HandleFunc("GET /api/admin/deployments/{id}", s.handleGetDeployment)
	mux.HandleFunc("DELETE /api/admin/deployments/{id}", s.handleDeleteDeployment)
	mux.HandleFunc("POST /api/admin/deployments/{id}/restart", s.handleRestartDeployment)
	mux.HandleFunc("GET /api/admin/cluster-status", s.handleGetClusterStatus)
	mux.HandleFunc("GET /api/admin/pods/{id}/{pod}/logs", s.handleGetPodLogs)
	mux.HandleFunc("GET /api/admin/pods/{id}/{pod}/env", s.handleGetPodEnv)
	mux.HandleFunc("GET /api/admin/agents", s.handleListAgents)
	mux.HandleFunc("GET /api/admin/agents/{account}/{name}/builds", s.handleGetAgentBuilds)
	mux.HandleFunc("GET /api/admin/devices", s.handleListConnectedDevices)
	mux.HandleFunc("POST /api/admin/devices/{deviceId}/command", s.handleSendCommand)
	mux.HandleFunc("GET /api/admin/deployments/{id}/events", s.handleGetDeploymentEvents)
	mux.HandleFunc("POST /api/admin/deployments/{id}/wakeup", s.handleWakeUpDeployment)
	mux.HandleFunc("POST /api/admin/deployments/{id}/stop", s.handleStopDeployment)
	mux.HandleFunc("POST /api/admin/deployments/{id}/rollback", s.handleRollbackDeployment)
	mux.HandleFunc("POST /api/admin/deployments/{id}/reapply", s.handleReapplyDeployment)
	mux.HandleFunc("GET /api/admin/deployments/{id}/jobs", s.handleGetDeploymentJobs)
	mux.HandleFunc("POST /api/admin/deployments/{id}/repair-normalized", s.handleRepairNormalizedSpec)
	mux.HandleFunc("POST /api/admin/deployments/{id}/refresh-drift", s.handleRefreshDriftReport)
	mux.HandleFunc("POST /api/admin/deployments/{id}/adapters", s.handleSetAdapters)
	mux.HandleFunc("POST /api/admin/backfill-resolved-keys", s.handleBackfillResolvedKeys)
	mux.HandleFunc("POST /api/admin/openmeter-backfill", s.handleTriggerOpenMeterBackfill)
	mux.HandleFunc("GET /api/admin/feedback", s.handleListFeedback)
	mux.HandleFunc("GET /api/admin/migrations", s.handleListClusterMigrations)
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListAccounts(r.Context(), &adminv1.ListAccountsRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRenameAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		NewName string `json:"new_name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := s.admin.RenameAccount(r.Context(), &adminv1.RenameAccountRequest{
		AccountID: id,
		NewName:   body.NewName,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSetAccountCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		ClusterID string `json:"cluster_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := s.admin.SetAccountCluster(r.Context(), &adminv1.SetAccountClusterRequest{
		AccountID: id,
		ClusterID: body.ClusterID,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleInvalidateAccountCaches(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.InvalidateAccountCaches(r.Context(), &adminv1.InvalidateAccountCachesRequest{
		AccountID: id,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleInvalidateAllCaches(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.InvalidateAllCaches(r.Context(), &adminv1.InvalidateAllCachesRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListClusterMigrations(w http.ResponseWriter, r *http.Request) {
	mismatchesOnly := r.URL.Query().Get("mismatches_only") == "1"
	resp, err := s.admin.ListClusterMigrations(r.Context(), &adminv1.ListClusterMigrationsRequest{
		MismatchesOnly: mismatchesOnly,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListDeployments(r.Context(), &adminv1.ListDeploymentsRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.GetDeployment(r.Context(), &adminv1.GetDeploymentRequest{
		DeploymentId: id,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeleteDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.DeleteDeployment(r.Context(), &adminv1.DeleteDeploymentRequest{
		DeploymentId: id,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRestartDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Pod string `json:"pod"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Pod == "" {
		http.Error(w, `{"error":"pod is required in request body"}`, http.StatusBadRequest)
		return
	}
	resp, err := s.admin.RestartDeployment(r.Context(), &adminv1.RestartDeploymentRequest{
		DeploymentId: id,
		Pod:          body.Pod,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetClusterStatus(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	resp, err := s.admin.GetClusterStatus(r.Context(), &adminv1.GetClusterStatusRequest{
		Namespace: ns,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetPodLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pod := r.PathValue("pod")
	container := r.URL.Query().Get("container")
	resp, err := s.admin.GetPodLogs(r.Context(), &adminv1.GetPodLogsRequest{
		DeploymentId: id,
		Pod:          pod,
		Container:    container,
		TailLines:    200,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetPodEnv(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pod := r.PathValue("pod")
	resp, err := s.admin.GetPodEnv(r.Context(), &adminv1.GetPodEnvRequest{
		DeploymentId: id,
		Pod:          pod,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListAgents(r.Context(), &adminv1.ListAgentsRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSendCommand(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	var body struct {
		Command        string            `json:"command"`
		Shell          string            `json:"shell"`
		WorkingDir     string            `json:"working_dir"`
		Env            map[string]string `json:"env"`
		TimeoutSeconds uint32            `json:"timeout_seconds"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := s.admin.SendCommand(r.Context(), &adminv1.SendCommandRequest{
		DeviceID:       deviceID,
		Command:        body.Command,
		Shell:          body.Shell,
		WorkingDir:     body.WorkingDir,
		Env:            body.Env,
		TimeoutSeconds: body.TimeoutSeconds,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListConnectedDevices(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListConnectedDevices(r.Context(), &adminv1.ListConnectedDevicesRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetAgentBuilds(w http.ResponseWriter, r *http.Request) {
	account := r.PathValue("account")
	name := r.PathValue("name")
	resp, err := s.admin.GetAgentBuilds(r.Context(), &adminv1.GetAgentBuildsRequest{
		AccountName: account,
		AgentName:   name,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetDeploymentEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.GetDeploymentEvents(r.Context(), &adminv1.GetDeploymentEventsRequest{
		DeploymentId: id,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWakeUpDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.WakeUpDeployment(r.Context(), &adminv1.WakeUpDeploymentRequest{
		DeploymentId: id,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleStopDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.StopDeployment(r.Context(), &adminv1.StopDeploymentRequest{
		DeploymentId: id,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRollbackDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Revision int32 `json:"revision"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := s.admin.RollbackDeployment(r.Context(), &adminv1.RollbackDeploymentRequest{
		DeploymentId: id,
		Revision:     body.Revision,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleReapplyDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.ReapplyDeployment(r.Context(), &adminv1.ReapplyDeploymentRequest{
		DeploymentId: id,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRepairNormalizedSpec(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.RepairNormalizedSpec(r.Context(), &adminv1.RepairNormalizedSpecRequest{
		DeploymentId: id,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetDeploymentJobs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.GetDeploymentJobs(r.Context(), &adminv1.GetDeploymentJobsRequest{
		DeploymentId: id,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRefreshDriftReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.admin.RefreshDriftReport(r.Context(), &adminv1.RefreshDriftReportRequest{
		DeploymentId: id,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSetAdapters(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Adapters []string `json:"adapters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	resp, err := s.admin.SetAdapters(r.Context(), &adminv1.SetAdaptersRequest{
		DeploymentId: id,
		Adapters:     body.Adapters,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleBackfillResolvedKeys(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.BackfillResolvedKeys(r.Context(), &adminv1.BackfillResolvedKeysRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListFeedback(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListFeedback(r.Context(), &adminv1.ListFeedbackRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleTriggerOpenMeterBackfill(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.TriggerOpenMeterBackfill(r.Context(), &adminv1.TriggerOpenMeterBackfillRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
