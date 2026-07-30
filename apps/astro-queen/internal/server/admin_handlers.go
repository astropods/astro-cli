package server

import (
	"encoding/json"
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/accounts", s.handleListAccounts)
	mux.HandleFunc("GET /api/admin/accounts/{id}", s.handleGetAccount)
	mux.HandleFunc("GET /api/admin/accounts/{id}/metronome-aliases", s.handleGetAccountMetronomeAliases)
	mux.HandleFunc("POST /api/admin/accounts/{id}/metronome-aliases/recover", s.handleRecoverAccountMetronomeAliases)
	mux.HandleFunc("POST /api/admin/accounts/{id}/metronome/register", s.handleRegisterAccountMetronome)
	mux.HandleFunc("POST /api/admin/accounts/{id}/langfuse/recover", s.handleRecoverAccountLangfuse)
	mux.HandleFunc("POST /api/admin/accounts/{id}/bifrost/recover", s.handleRecoverAccountBifrost)
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
	mux.HandleFunc("GET /api/admin/feedback", s.handleListFeedback)
	mux.HandleFunc("GET /api/admin/migrations", s.handleListClusterMigrations)
	mux.HandleFunc("POST /api/admin/refresh-messaging-cache", s.handleRefreshMessagingCache)
	mux.HandleFunc("GET /api/admin/alerts", s.handleListAlerts)
	mux.HandleFunc("POST /api/admin/alerts/clear", s.handleClearAlert)
	mux.HandleFunc("POST /api/admin/alerts/mute", s.handleMuteAlert)
	mux.HandleFunc("POST /api/admin/alerts/unmute", s.handleUnmuteAlert)
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListAccounts(r.Context(), &adminv1.ListAccountsRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.GetAccount(r.Context(), &adminv1.GetAccountRequest{
		AccountID: r.PathValue("id"),
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetAccountMetronomeAliases(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.GetAccountMetronomeAliases(r.Context(), &adminv1.GetAccountMetronomeAliasesRequest{
		AccountID: r.PathValue("id"),
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRecoverAccountMetronomeAliases(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.RecoverAccountMetronomeAliases(r.Context(), &adminv1.RecoverAccountMetronomeAliasesRequest{
		AccountID: r.PathValue("id"),
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRegisterAccountMetronome(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.RegisterAccountMetronome(r.Context(), &adminv1.RegisterAccountMetronomeRequest{
		AccountID: r.PathValue("id"),
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRecoverAccountLangfuse(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.RecoverAccountLangfuse(r.Context(), &adminv1.RecoverAccountLangfuseRequest{
		AccountID: r.PathValue("id"),
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRecoverAccountBifrost(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.RecoverAccountBifrost(r.Context(), &adminv1.RecoverAccountBifrostRequest{
		AccountID: r.PathValue("id"),
	})
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

func (s *Server) handleListFeedback(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListFeedback(r.Context(), &adminv1.ListFeedbackRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRefreshMessagingCache(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.RefreshMessagingCache(r.Context(), &adminv1.RefreshMessagingCacheRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListAlerts(r.Context(), &adminv1.ListAlertsRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleClearAlert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeploymentID string `json:"deployment_id"`
		Workload     string `json:"workload"`
		Condition    string `json:"condition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DeploymentID == "" || body.Condition == "" {
		http.Error(w, `{"error":"deployment_id and condition are required in request body"}`, http.StatusBadRequest)
		return
	}
	resp, err := s.admin.ClearAlert(r.Context(), &adminv1.ClearAlertRequest{
		DeploymentID: body.DeploymentID,
		Workload:     body.Workload,
		Condition:    body.Condition,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMuteAlert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeploymentID    string `json:"deployment_id"`
		Condition       string `json:"condition"`
		DurationSeconds int64  `json:"duration_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DeploymentID == "" || body.Condition == "" || body.DurationSeconds <= 0 {
		http.Error(w, `{"error":"deployment_id, condition and a positive duration_seconds are required in request body"}`, http.StatusBadRequest)
		return
	}
	resp, err := s.admin.MuteAlert(r.Context(), &adminv1.MuteAlertRequest{
		DeploymentID:    body.DeploymentID,
		Condition:       body.Condition,
		DurationSeconds: body.DurationSeconds,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUnmuteAlert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeploymentID string `json:"deployment_id"`
		Condition    string `json:"condition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DeploymentID == "" || body.Condition == "" {
		http.Error(w, `{"error":"deployment_id and condition are required in request body"}`, http.StatusBadRequest)
		return
	}
	resp, err := s.admin.UnmuteAlert(r.Context(), &adminv1.UnmuteAlertRequest{
		DeploymentID: body.DeploymentID,
		Condition:    body.Condition,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
