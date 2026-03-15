package server

import (
	"net/http"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func (s *Server) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/accounts", s.handleListAccounts)
	mux.HandleFunc("PUT /api/admin/accounts/{id}/rename", s.handleRenameAccount)
	mux.HandleFunc("GET /api/admin/deployments", s.handleListDeployments)
	mux.HandleFunc("GET /api/admin/deployments/{namespace}", s.handleGetDeployment)
	mux.HandleFunc("DELETE /api/admin/deployments/{namespace}", s.handleDeleteDeployment)
	mux.HandleFunc("POST /api/admin/deployments/{namespace}/restart", s.handleRestartDeployment)
	mux.HandleFunc("GET /api/admin/cluster-status", s.handleGetClusterStatus)
	mux.HandleFunc("GET /api/admin/pods/{namespace}/{pod}/logs", s.handleGetPodLogs)
	mux.HandleFunc("GET /api/admin/pods/{namespace}/{pod}/env", s.handleGetPodEnv)
	mux.HandleFunc("GET /api/admin/agents", s.handleListAgents)
	mux.HandleFunc("GET /api/admin/agents/{account}/{name}/builds", s.handleGetAgentBuilds)
	mux.HandleFunc("GET /api/admin/devices", s.handleListConnectedDevices)
	mux.HandleFunc("POST /api/admin/devices/{deviceId}/command", s.handleSendCommand)
	mux.HandleFunc("GET /api/admin/deployments/{namespace}/events", s.handleGetDeploymentEvents)
	mux.HandleFunc("POST /api/admin/deployments/{namespace}/wakeup", s.handleWakeUpDeployment)
	mux.HandleFunc("POST /api/admin/deployments/{namespace}/rollback", s.handleRollbackDeployment)
	mux.HandleFunc("POST /api/admin/deployments/{namespace}/reapply", s.handleReapplyDeployment)
	mux.HandleFunc("GET /api/admin/deployments/{namespace}/jobs", s.handleGetDeploymentJobs)
	mux.HandleFunc("POST /api/admin/backfill-deployments", s.handleBackfillDeployments)
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

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.ListDeployments(r.Context(), &adminv1.ListDeploymentsRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	resp, err := s.admin.GetDeployment(r.Context(), &adminv1.GetDeploymentRequest{
		Namespace: ns,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeleteDeployment(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	resp, err := s.admin.DeleteDeployment(r.Context(), &adminv1.DeleteDeploymentRequest{
		Namespace: ns,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRestartDeployment(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	resp, err := s.admin.RestartDeployment(r.Context(), &adminv1.RestartDeploymentRequest{
		Namespace: ns,
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
	ns := r.PathValue("namespace")
	pod := r.PathValue("pod")
	container := r.URL.Query().Get("container")
	resp, err := s.admin.GetPodLogs(r.Context(), &adminv1.GetPodLogsRequest{
		Namespace: ns,
		Pod:       pod,
		Container: container,
		TailLines: 200,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetPodEnv(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	pod := r.PathValue("pod")
	resp, err := s.admin.GetPodEnv(r.Context(), &adminv1.GetPodEnvRequest{
		Namespace: ns,
		Pod:       pod,
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
	ns := r.PathValue("namespace")
	resp, err := s.admin.GetDeploymentEvents(r.Context(), &adminv1.GetDeploymentEventsRequest{
		Namespace: ns,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWakeUpDeployment(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	resp, err := s.admin.WakeUpDeployment(r.Context(), &adminv1.WakeUpDeploymentRequest{
		Namespace: ns,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRollbackDeployment(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	var body struct {
		Revision int32 `json:"revision"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := s.admin.RollbackDeployment(r.Context(), &adminv1.RollbackDeploymentRequest{
		Namespace: ns,
		Revision:  body.Revision,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleBackfillDeployments(w http.ResponseWriter, r *http.Request) {
	resp, err := s.admin.BackfillDeployments(r.Context(), &adminv1.BackfillDeploymentsRequest{})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleReapplyDeployment(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	resp, err := s.admin.ReapplyDeployment(r.Context(), &adminv1.ReapplyDeploymentRequest{
		Namespace: ns,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetDeploymentJobs(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	resp, err := s.admin.GetDeploymentJobs(r.Context(), &adminv1.GetDeploymentJobsRequest{
		Namespace: ns,
	})
	if err != nil {
		writeGRPCErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
