package deploymentstore

import (
	"database/sql"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/lib/pq"
)

// Workload represents a single K8s workload row.
type Workload struct {
	ID                 int
	DeploymentID       string
	Name               string
	ComponentKind      string
	ComponentKey       string
	WorkloadType       string
	Image              string
	Replicas           int
	CPURequest         string
	MemoryRequest      string
	CPULimit           string
	MemoryLimit        string
	GPUVram            *string
	GPURuntime         *string
	GPUCount           *int
	UpdateStrategy     *string
	UpdateMaxUnavail   *string
	UpdateMaxSurge     *string
	HealthcheckPath    *string
	HealthcheckPort    *int
	HealthcheckIntv    *string
	HealthcheckTimeout *string
	HealthcheckRetries *int
	HealthcheckTest    *string
	TriggerType        *string
	TriggerSchedule    *string
	Persistent         bool
	Distributed        bool
}

// Service represents a K8s Service row.
type Service struct {
	ID         int
	WorkloadID int
	Name       string
	Port       int
	TargetPort int
	Protocol   string
}

// Ingress represents a K8s Ingress row.
type Ingress struct {
	ID         int
	ServiceID  int
	Hostname   string
	Path       string
	TLSEnabled bool
}

// Volume represents a PVC row.
type Volume struct {
	ID           int
	WorkloadID   int
	MountPath    string
	Size         string
	StorageClass *string
	AccessMode   string
}

// EnvVar represents an environment variable row.
type EnvVar struct {
	WorkloadID int
	Key        string
	Value      string
	Source     string
	Nonce      []byte
}

// Variable represents a deployment-level variable row.
type Variable struct {
	DeploymentID string
	Name         string
	Value        string
	Secret       bool
	Optional     bool
	Targets      []string
	Nonce        []byte
}

// SaveNormalizedSpec extracts workloads, services, ingresses, volumes, env vars,
// and variables from an AstroDeploymentSpec and inserts them into the normalized tables.
// This runs inside the caller's transaction.
func SaveNormalizedSpec(
	tx *sql.Tx,
	deploymentID string,
	ds *spec.AstroDeploymentSpec,
	resolved *deployment.ResolvedEnv,
	enc *envelope.Encryptor,
) error {
	agentName := ds.Source.Name

	// Helper: insert a workload row and return its ID
	insertWorkload := func(w *Workload) (int, error) {
		var id int
		err := tx.QueryRow(`
			INSERT INTO deployment_workloads (
				deployment_id, name, component_kind, component_key, workload_type,
				image, replicas, cpu_request, memory_request, cpu_limit, memory_limit,
				gpu_vram, gpu_runtime, gpu_count,
				update_strategy, update_max_unavailable, update_max_surge,
				healthcheck_path, healthcheck_port, healthcheck_interval,
				healthcheck_timeout, healthcheck_retries, healthcheck_test,
				trigger_type, trigger_schedule, persistent, distributed
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10, $11,
				$12, $13, $14,
				$15, $16, $17,
				$18, $19, $20,
				$21, $22, $23,
				$24, $25, $26, $27
			) RETURNING id
		`,
			deploymentID, w.Name, w.ComponentKind, w.ComponentKey, w.WorkloadType,
			w.Image, w.Replicas, w.CPURequest, w.MemoryRequest, w.CPULimit, w.MemoryLimit,
			w.GPUVram, w.GPURuntime, w.GPUCount,
			w.UpdateStrategy, w.UpdateMaxUnavail, w.UpdateMaxSurge,
			w.HealthcheckPath, w.HealthcheckPort, w.HealthcheckIntv,
			w.HealthcheckTimeout, w.HealthcheckRetries, w.HealthcheckTest,
			w.TriggerType, w.TriggerSchedule, w.Persistent, w.Distributed,
		).Scan(&id)
		return id, err
	}

	insertService := func(workloadID int, svc *Service) (int, error) {
		var id int
		err := tx.QueryRow(`
			INSERT INTO deployment_services (workload_id, name, port, target_port, protocol)
			VALUES ($1, $2, $3, $4, $5) RETURNING id
		`, workloadID, svc.Name, svc.Port, svc.TargetPort, svc.Protocol).Scan(&id)
		return id, err
	}

	insertIngress := func(serviceID int, ing *Ingress) error {
		_, err := tx.Exec(`
			INSERT INTO deployment_ingresses (service_id, hostname, path, tls_enabled)
			VALUES ($1, $2, $3, $4)
		`, serviceID, ing.Hostname, ing.Path, ing.TLSEnabled)
		return err
	}

	insertVolume := func(workloadID int, vol *Volume) error {
		_, err := tx.Exec(`
			INSERT INTO deployment_volumes (workload_id, mount_path, size, storage_class, access_mode)
			VALUES ($1, $2, $3, $4, $5)
		`, workloadID, vol.MountPath, vol.Size, vol.StorageClass, vol.AccessMode)
		return err
	}

	insertEnvVars := func(workloadID int, env map[string]string, source string) error {
		for k, v := range env {
			_, err := tx.Exec(`
				INSERT INTO deployment_env_vars (workload_id, key, value, source)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (workload_id, key) DO NOTHING
			`, workloadID, k, v, source)
			if err != nil {
				return err
			}
		}
		return nil
	}

	// Helper to build workload from a common container-like component
	type componentInput struct {
		kind       string
		key        string
		image      string
		replicas   int
		resources  spec.DeploymentResources
		update     spec.UpdateStrategy
		hc         *spec.Healthcheck
		endpoints  map[string]spec.Endpoint
		gpu        *spec.DeploymentGPU
		persistent bool
	}

	workloadTypeFor := func(ci componentInput) string {
		if ci.persistent {
			return "statefulset"
		}
		return "deployment"
	}

	buildWorkload := func(ci componentInput) *Workload {
		resourceName := deployment.GenerateResourceName(agentName, ci.kind, ci.key)
		if ci.key == "" {
			resourceName = deployment.GenerateAgentResourceName(agentName, ci.kind)
		}
		w := &Workload{
			Name:          resourceName,
			ComponentKind: ci.kind,
			ComponentKey:  ci.key,
			WorkloadType:  workloadTypeFor(ci),
			Image:         ci.image,
			Replicas:      ci.replicas,
			CPURequest:    ci.resources.CPU,
			MemoryRequest: ci.resources.Memory,
			CPULimit:      ci.resources.CPULimit,
			MemoryLimit:   ci.resources.MemoryLimit,
			Persistent:    ci.persistent,
		}
		if ci.update.Strategy != "" {
			w.UpdateStrategy = &ci.update.Strategy
			if ci.update.MaxUnavailable != "" {
				w.UpdateMaxUnavail = &ci.update.MaxUnavailable
			}
			if ci.update.MaxSurge != "" {
				w.UpdateMaxSurge = &ci.update.MaxSurge
			}
		}
		if ci.hc != nil {
			if ci.hc.Path != "" {
				w.HealthcheckPath = &ci.hc.Path
			}
			if ci.hc.Interval != "" {
				w.HealthcheckIntv = &ci.hc.Interval
			}
			if ci.hc.Timeout != "" {
				w.HealthcheckTimeout = &ci.hc.Timeout
			}
			if ci.hc.Retries != 0 {
				w.HealthcheckRetries = &ci.hc.Retries
			}
			if len(ci.hc.Test) > 0 {
				joined := ""
				for i, t := range ci.hc.Test {
					if i > 0 {
						joined += " "
					}
					joined += t
				}
				w.HealthcheckTest = &joined
			}
		}
		if ci.gpu != nil {
			if ci.gpu.VRAM != "" {
				w.GPUVram = &ci.gpu.VRAM
			}
			if ci.gpu.Runtime != "" {
				w.GPURuntime = &ci.gpu.Runtime
			}
			if ci.gpu.Count != 0 {
				w.GPUCount = &ci.gpu.Count
			}
		}
		return w
	}

	saveEndpoints := func(workloadID int, endpoints map[string]spec.Endpoint) error {
		for name, ep := range endpoints {
			proto := ep.Protocol
			if proto == "" {
				proto = "http"
			}
			svcID, err := insertService(workloadID, &Service{
				Name:       name,
				Port:       ep.Port,
				TargetPort: ep.Port,
				Protocol:   proto,
			})
			if err != nil {
				return fmt.Errorf("insert service %s: %w", name, err)
			}
			if ep.Expose != nil && ep.Expose.Enabled && ep.Expose.Domain != "" {
				if err := insertIngress(svcID, &Ingress{
					Hostname:   ep.Expose.Domain,
					Path:       "/",
					TLSEnabled: true,
				}); err != nil {
					return fmt.Errorf("insert ingress for %s: %w", name, err)
				}
			}
		}
		return nil
	}

	// --- Agent ---
	agentReplicas := ds.Agent.Replicas
	if agentReplicas == 0 {
		agentReplicas = 1
	}
	w := buildWorkload(componentInput{
		kind: "agent", image: ds.Agent.Image,
		replicas: agentReplicas, resources: ds.Agent.Resources,
		update: ds.Agent.Update, hc: ds.Agent.Healthcheck,
		endpoints: ds.Agent.Endpoints,
	})
	w.Distributed = ds.Agent.Distributed
	agentWID, err := insertWorkload(w)
	if err != nil {
		return fmt.Errorf("insert agent workload: %w", err)
	}
	if err := saveEndpoints(agentWID, ds.Agent.Endpoints); err != nil {
		return fmt.Errorf("agent endpoints: %w", err)
	}
	// Agent gets configmap + secret env vars
	if err := insertEnvVars(agentWID, resolved.ConfigMapData, "configmap"); err != nil {
		return fmt.Errorf("agent configmap env: %w", err)
	}
	secretEnv := make(map[string]string, len(resolved.SecretData))
	for k := range resolved.SecretData {
		secretEnv[k] = "" // Don't store secret values in env_vars — they go to deployment_variables
	}
	if err := insertEnvVars(agentWID, secretEnv, "secret"); err != nil {
		return fmt.Errorf("agent secret env: %w", err)
	}

	// --- Models ---
	for name, model := range ds.Models {
		replicas := model.Replicas
		if replicas == 0 {
			replicas = 1
		}
		w := buildWorkload(componentInput{
			kind: "model", key: name, image: model.Image,
			replicas: replicas, resources: model.Resources,
			update: model.Update, hc: model.Healthcheck,
			endpoints: model.Endpoints, gpu: model.GPU,
			persistent: model.Persistent,
		})
		wid, err := insertWorkload(w)
		if err != nil {
			return fmt.Errorf("insert model %s workload: %w", name, err)
		}
		if err := saveEndpoints(wid, model.Endpoints); err != nil {
			return fmt.Errorf("model %s endpoints: %w", name, err)
		}
		if model.Persistent {
			storageSize := "50Gi"
			mountPath := "/data"
			if model.Provider != "" {
				if prov, ok := spec.LookupBuiltin("models", model.Provider); ok && prov.MountPath != "" {
					mountPath = prov.MountPath
				}
			}
			if err := insertVolume(wid, &Volume{
				MountPath:  mountPath,
				Size:       storageSize,
				AccessMode: "ReadWriteOnce",
			}); err != nil {
				return fmt.Errorf("model %s volume: %w", name, err)
			}
		}
	}

	// --- Knowledge ---
	for name, knowledge := range ds.Knowledge {
		replicas := knowledge.Replicas
		if replicas == 0 {
			replicas = 1
		}
		w := buildWorkload(componentInput{
			kind: "knowledge", key: name, image: knowledge.Image,
			replicas: replicas, resources: knowledge.Resources,
			update: knowledge.Update, hc: knowledge.Healthcheck,
			endpoints: knowledge.Endpoints, persistent: knowledge.Persistent,
		})
		wid, err := insertWorkload(w)
		if err != nil {
			return fmt.Errorf("insert knowledge %s workload: %w", name, err)
		}
		if err := saveEndpoints(wid, knowledge.Endpoints); err != nil {
			return fmt.Errorf("knowledge %s endpoints: %w", name, err)
		}
		if knowledge.Persistent {
			storageSize := "10Gi"
			storageClass := ""
			accessMode := "ReadWriteOnce"
			if knowledge.Storage != nil {
				if knowledge.Storage.Size != "" {
					storageSize = knowledge.Storage.Size
				}
				storageClass = knowledge.Storage.Class
				if knowledge.Storage.AccessMode != "" {
					accessMode = knowledge.Storage.AccessMode
				}
			}
			mountPath := "/data"
			if prov := spec.GetProvider(knowledge.Provider); prov.MountPath != "" {
				mountPath = prov.MountPath
			}
			var sc *string
			if storageClass != "" {
				sc = &storageClass
			}
			if err := insertVolume(wid, &Volume{
				MountPath:    mountPath,
				Size:         storageSize,
				StorageClass: sc,
				AccessMode:   accessMode,
			}); err != nil {
				return fmt.Errorf("knowledge %s volume: %w", name, err)
			}
		}
	}

	// --- Tools ---
	for name, tool := range ds.Tools {
		replicas := tool.Replicas
		if replicas == 0 {
			replicas = 1
		}
		w := buildWorkload(componentInput{
			kind: "tool", key: name, image: tool.Image,
			replicas: replicas, resources: tool.Resources,
			update: tool.Update, hc: tool.Healthcheck,
			endpoints: tool.Endpoints,
		})
		wid, err := insertWorkload(w)
		if err != nil {
			return fmt.Errorf("insert tool %s workload: %w", name, err)
		}
		if err := saveEndpoints(wid, tool.Endpoints); err != nil {
			return fmt.Errorf("tool %s endpoints: %w", name, err)
		}
	}

	// --- Ingestion ---
	for name, ingestion := range ds.Ingestion {
		wtype := "deployment"
		switch ingestion.Trigger.Type {
		case "schedule":
			wtype = "cronjob"
		case "startup":
			wtype = "job"
		case "manual":
			continue // No K8s workload
		}
		resourceName := deployment.GenerateResourceName(agentName, "ingestion", name)
		triggerType := ingestion.Trigger.Type
		w := &Workload{
			Name:          resourceName,
			ComponentKind: "ingestion",
			ComponentKey:  name,
			WorkloadType:  wtype,
			Image:         ingestion.Image,
			Replicas:      0,
			CPURequest:    ingestion.Resources.CPU,
			MemoryRequest: ingestion.Resources.Memory,
			CPULimit:      ingestion.Resources.CPULimit,
			MemoryLimit:   ingestion.Resources.MemoryLimit,
			TriggerType:   &triggerType,
		}
		if ingestion.Trigger.Schedule != "" {
			w.TriggerSchedule = &ingestion.Trigger.Schedule
		}
		if ingestion.Trigger.Type == "webhook" {
			w.Replicas = 1
		}
		if ingestion.Healthcheck != nil {
			if ingestion.Healthcheck.Path != "" {
				w.HealthcheckPath = &ingestion.Healthcheck.Path
			}
		}
		wid, err := insertWorkload(w)
		if err != nil {
			return fmt.Errorf("insert ingestion %s workload: %w", name, err)
		}
		if err := saveEndpoints(wid, ingestion.Endpoints); err != nil {
			return fmt.Errorf("ingestion %s endpoints: %w", name, err)
		}
	}

	// --- Messaging (Interfaces) ---
	if ds.Interfaces != nil && len(ds.Interfaces.Adapters) > 0 {
		resourceName := deployment.GenerateAgentResourceName(agentName, "messaging")
		w := &Workload{
			Name:          resourceName,
			ComponentKind: "messaging",
			WorkloadType:  "deployment",
			Image:         ds.Interfaces.Image,
			Replicas:      1,
			CPURequest:    ds.Interfaces.Resources.CPU,
			MemoryRequest: ds.Interfaces.Resources.Memory,
			CPULimit:      ds.Interfaces.Resources.CPULimit,
			MemoryLimit:   ds.Interfaces.Resources.MemoryLimit,
		}
		wid, err := insertWorkload(w)
		if err != nil {
			return fmt.Errorf("insert messaging workload: %w", err)
		}
		if err := saveEndpoints(wid, ds.Interfaces.Endpoints); err != nil {
			return fmt.Errorf("messaging endpoints: %w", err)
		}
	}

	// --- Collector (Observability) ---
	if ds.Observability.Enabled {
		resourceName := deployment.GenerateAgentResourceName(agentName, "collector")
		w := &Workload{
			Name:          resourceName,
			ComponentKind: "collector",
			WorkloadType:  "deployment",
			Image:         ds.Observability.Image,
			Replicas:      1,
			CPURequest:    ds.Observability.Resources.CPU,
			MemoryRequest: ds.Observability.Resources.Memory,
			CPULimit:      ds.Observability.Resources.CPULimit,
			MemoryLimit:   ds.Observability.Resources.MemoryLimit,
		}
		wid, err := insertWorkload(w)
		if err != nil {
			return fmt.Errorf("insert collector workload: %w", err)
		}
		// Collector has OTLP ports from the spec
		otlpHTTPPort := ds.Observability.Port
		if otlpHTTPPort == 0 {
			otlpHTTPPort = 4318
		}
		otlpGRPCPort := otlpHTTPPort - 1
		if _, err := insertService(wid, &Service{
			Name: "otlp-grpc", Port: otlpGRPCPort, TargetPort: otlpGRPCPort, Protocol: "grpc",
		}); err != nil {
			return fmt.Errorf("collector grpc service: %w", err)
		}
		if _, err := insertService(wid, &Service{
			Name: "otlp-http", Port: otlpHTTPPort, TargetPort: otlpHTTPPort, Protocol: "http",
		}); err != nil {
			return fmt.Errorf("collector http service: %w", err)
		}
	}

	// --- Variables ---
	for name, v := range ds.Variables {
		val := v.Value
		var nonce []byte
		if v.Secret && val != "" && enc != nil {
			ciphertext, n, err := enc.Encrypt([]byte(val))
			if err != nil {
				return fmt.Errorf("encrypt variable %s: %w", name, err)
			}
			val = string(ciphertext)
			nonce = n
		} else if v.Secret {
			val = "" // Strip if no encryptor available
		}
		targets := v.Targets
		if targets == nil {
			targets = []string{}
		}
		_, err := tx.Exec(`
			INSERT INTO deployment_variables (deployment_id, name, value, secret, optional, targets, nonce)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, deploymentID, name, val, v.Secret, v.Optional, pq.Array(targets), nonce)
		if err != nil {
			return fmt.Errorf("insert variable %s: %w", name, err)
		}
	}

	return nil
}

// GetDeploymentVariables returns all variables for a deployment.
func (s *Store) GetDeploymentVariables(deploymentID string) ([]Variable, error) {
	rows, err := s.db.Query(`
		SELECT deployment_id, name, value, secret, optional, targets, nonce
		FROM deployment_variables
		WHERE deployment_id = $1
		ORDER BY name
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query deployment variables: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []Variable
	for rows.Next() {
		var v Variable
		if err := rows.Scan(&v.DeploymentID, &v.Name, &v.Value, &v.Secret, &v.Optional, pq.Array(&v.Targets), &v.Nonce); err != nil {
			return nil, fmt.Errorf("scan variable: %w", err)
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// GetWorkloads returns all workloads for a deployment.
func (s *Store) GetWorkloads(deploymentID string) ([]*Workload, error) {
	rows, err := s.db.Query(`
		SELECT id, deployment_id, name, component_kind, component_key, workload_type,
			image, replicas, cpu_request, memory_request, cpu_limit, memory_limit,
			gpu_vram, gpu_runtime, gpu_count,
			update_strategy, update_max_unavailable, update_max_surge,
			healthcheck_path, healthcheck_port, healthcheck_interval,
			healthcheck_timeout, healthcheck_retries, healthcheck_test,
			trigger_type, trigger_schedule, persistent, distributed
		FROM deployment_workloads
		WHERE deployment_id = $1
		ORDER BY id
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query workloads: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var workloads []*Workload
	for rows.Next() {
		var w Workload
		if err := rows.Scan(
			&w.ID, &w.DeploymentID, &w.Name, &w.ComponentKind, &w.ComponentKey, &w.WorkloadType,
			&w.Image, &w.Replicas, &w.CPURequest, &w.MemoryRequest, &w.CPULimit, &w.MemoryLimit,
			&w.GPUVram, &w.GPURuntime, &w.GPUCount,
			&w.UpdateStrategy, &w.UpdateMaxUnavail, &w.UpdateMaxSurge,
			&w.HealthcheckPath, &w.HealthcheckPort, &w.HealthcheckIntv,
			&w.HealthcheckTimeout, &w.HealthcheckRetries, &w.HealthcheckTest,
			&w.TriggerType, &w.TriggerSchedule, &w.Persistent, &w.Distributed,
		); err != nil {
			return nil, fmt.Errorf("scan workload: %w", err)
		}
		workloads = append(workloads, &w)
	}
	return workloads, rows.Err()
}

// WorkloadSummary is a lightweight view of a workload for API responses and metering.
type WorkloadSummary struct {
	ComponentKind string
	ComponentKey  string
	WorkloadType  string
	Image         string
	Replicas      int
	CPURequest    string
	MemoryRequest string
}

// GetWorkloadSummaries returns lightweight workload data for a deployment.
// Used by API responses and admin gRPC to avoid full spec JSON parsing.
func (s *Store) GetWorkloadSummaries(deploymentID string) ([]*WorkloadSummary, error) {
	rows, err := s.db.Query(`
		SELECT component_kind, component_key, workload_type, image, replicas, cpu_request, memory_request
		FROM deployment_workloads
		WHERE deployment_id = $1
		ORDER BY id
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query workload summaries: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []*WorkloadSummary
	for rows.Next() {
		var w WorkloadSummary
		if err := rows.Scan(&w.ComponentKind, &w.ComponentKey, &w.WorkloadType, &w.Image, &w.Replicas, &w.CPURequest, &w.MemoryRequest); err != nil {
			return nil, fmt.Errorf("scan workload summary: %w", err)
		}
		result = append(result, &w)
	}
	return result, rows.Err()
}

// ActiveDeploymentWorkload holds the fields the openmeter heartbeat needs per workload.
type ActiveDeploymentWorkload struct {
	AccountID     string
	AgentName     string
	Namespace     string
	DeploymentID  string
	ComponentKind string
	ComponentKey  string
	Replicas      int
	CPURequest    string
	MemoryRequest string
}

// GetActiveDeploymentWorkloads queries workloads for all active deployments directly,
// replacing the JSON parsing path in the openmeter heartbeat.
func (s *Store) GetActiveDeploymentWorkloads() ([]*ActiveDeploymentWorkload, error) {
	rows, err := s.db.Query(`
		SELECT d.account_id, d.agent_name, d.namespace, d.id,
			w.component_kind, w.component_key, w.replicas, w.cpu_request, w.memory_request
		FROM deployments d
		JOIN deployment_workloads w ON w.deployment_id = d.id
		WHERE d.status = 'active'
		ORDER BY d.id, w.id
	`)
	if err != nil {
		return nil, fmt.Errorf("query active deployment workloads: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []*ActiveDeploymentWorkload
	for rows.Next() {
		var w ActiveDeploymentWorkload
		if err := rows.Scan(&w.AccountID, &w.AgentName, &w.Namespace, &w.DeploymentID,
			&w.ComponentKind, &w.ComponentKey, &w.Replicas, &w.CPURequest, &w.MemoryRequest); err != nil {
			return nil, fmt.Errorf("scan active workload: %w", err)
		}
		result = append(result, &w)
	}
	return result, rows.Err()
}

// GetServices returns all services for a deployment (across all workloads).
func (s *Store) GetServices(deploymentID string) ([]*Service, error) {
	rows, err := s.db.Query(`
		SELECT ds.id, ds.workload_id, ds.name, ds.port, ds.target_port, ds.protocol
		FROM deployment_services ds
		JOIN deployment_workloads dw ON dw.id = ds.workload_id
		WHERE dw.deployment_id = $1
		ORDER BY ds.id
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query services: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []*Service
	for rows.Next() {
		var svc Service
		if err := rows.Scan(&svc.ID, &svc.WorkloadID, &svc.Name, &svc.Port, &svc.TargetPort, &svc.Protocol); err != nil {
			return nil, fmt.Errorf("scan service: %w", err)
		}
		result = append(result, &svc)
	}
	return result, rows.Err()
}

// GetIngresses returns all ingresses for a deployment.
func (s *Store) GetIngresses(deploymentID string) ([]*Ingress, error) {
	rows, err := s.db.Query(`
		SELECT di.id, di.service_id, di.hostname, di.path, di.tls_enabled
		FROM deployment_ingresses di
		JOIN deployment_services ds ON ds.id = di.service_id
		JOIN deployment_workloads dw ON dw.id = ds.workload_id
		WHERE dw.deployment_id = $1
		ORDER BY di.id
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query ingresses: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []*Ingress
	for rows.Next() {
		var ing Ingress
		if err := rows.Scan(&ing.ID, &ing.ServiceID, &ing.Hostname, &ing.Path, &ing.TLSEnabled); err != nil {
			return nil, fmt.Errorf("scan ingress: %w", err)
		}
		result = append(result, &ing)
	}
	return result, rows.Err()
}
