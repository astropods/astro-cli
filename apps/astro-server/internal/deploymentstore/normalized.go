package deploymentstore

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/lib/pq"
)

// encryptResolution prepares a Resolution's value for storage in
// deployment_build_env. Mirrors deployment_variables semantics:
//   - secret rows: encrypted ciphertext + nonce.
//   - non-secret rows: plaintext bytes, nil nonce.
//
// Storing non-secret values plaintext means the API + admingrpc can
// read them without KMS access. Secret values still require KMS to
// decrypt, just like before.
func encryptResolution(enc *envelope.Encryptor, r deployment.Resolution) ([]byte, []byte, error) {
	if !r.IsSecret || enc == nil {
		return []byte(r.Value), nil, nil
	}
	return enc.Encrypt([]byte(r.Value))
}

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
	ID           int
	WorkloadID   int
	Name         string
	Port         int
	TargetPort   int
	Protocol     string
	WorkloadName string // populated by GetServices join
}

// Sidecar represents a container that runs inside the agent pod (not a standalone K8s resource).
type Sidecar struct {
	ID            int
	DeploymentID  string
	Name          string
	ComponentKind string
	Image         string
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
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
	Ref          string
	Secret       bool
	Optional     bool
	Targets      []string
	Nonce        []byte
}

// NormalizedSpecConfig holds server-level configuration needed to generate
// ingress records. These values come from environment variables and are not
// part of the deployment spec itself.
type NormalizedSpecConfig struct {
	Namespace              string            // K8s namespace (for ingress host generation)
	IngressDomain          string            // e.g. "agents.astropods.ai"
	IngestionIngressDomain string            // e.g. "ingestion.astropods.ai"
	VarRefs                map[string]string // variable name → original account variable ref (before resolution)
	ManagedSecrets         map[string]string // platform-injected secrets (e.g. ANTHROPIC_API_KEY) that bypass user variables
	// SkipBuildEnvClear suppresses the DELETE FROM deployment_build_env that
	// normally runs at the start of the variables block. Set by RepairNormalizedSpec
	// so that existing build_env rows are preserved when variables cannot be
	// re-encrypted (KMS encryptor is unavailable at repair time).
	SkipBuildEnvClear bool
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
	nsCfg *NormalizedSpecConfig,
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
			INSERT INTO deployment_services (workload_id, sidecar_id, name, port, target_port, protocol)
			VALUES ($1, NULL, $2, $3, $4, $5) RETURNING id
		`, workloadID, svc.Name, svc.Port, svc.TargetPort, svc.Protocol).Scan(&id)
		return id, err
	}

	insertSidecar := func(sc *Sidecar) (int, error) {
		var id int
		err := tx.QueryRow(`
			INSERT INTO deployment_sidecars (deployment_id, name, component_kind, image,
				cpu_request, memory_request, cpu_limit, memory_limit)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id
		`, deploymentID, sc.Name, sc.ComponentKind, sc.Image,
			sc.CPURequest, sc.MemoryRequest, sc.CPULimit, sc.MemoryLimit).Scan(&id)
		return id, err
	}

	insertSidecarService := func(sidecarID int, svc *Service) (int, error) {
		var id int
		err := tx.QueryRow(`
			INSERT INTO deployment_services (workload_id, sidecar_id, name, port, target_port, protocol)
			VALUES (NULL, $1, $2, $3, $4, $5) RETURNING id
		`, sidecarID, svc.Name, svc.Port, svc.TargetPort, svc.Protocol).Scan(&id)
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

	// resolveIngressHost determines the hostname for an exposed endpoint.
	// Uses explicit domain from spec, falling back to generated host from ingressDomain.
	resolveIngressHost := func(ep *spec.Endpoint, ingressDomain string) string {
		if ep == nil {
			return ""
		}
		if ep.Expose != nil && ep.Expose.Domain != "" {
			return ep.Expose.Domain
		}
		if ingressDomain != "" && nsCfg != nil && nsCfg.Namespace != "" {
			return k8s.GenerateIngressHost(agentName, nsCfg.Namespace, ingressDomain)
		}
		return ""
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
	// Agent ingress — matches spec_applier logic: ExposedEndpoint + ingressDomain fallback
	if ep := spec.ExposedEndpoint(ds.Agent.Endpoints); ep != nil {
		ingressDomain := ""
		if nsCfg != nil {
			ingressDomain = nsCfg.IngressDomain
		}
		if host := resolveIngressHost(ep, ingressDomain); host != "" {
			// Find the service ID for the exposed endpoint's port
			var svcID int
			err := tx.QueryRow(`
				SELECT ds.id FROM deployment_services ds
				WHERE ds.workload_id = $1 AND ds.port = $2
				LIMIT 1
			`, agentWID, ep.Port).Scan(&svcID)
			if err == nil {
				if err := insertIngress(svcID, &Ingress{
					Hostname: host, Path: "/", TLSEnabled: true,
				}); err != nil {
					return fmt.Errorf("agent ingress: %w", err)
				}
			}
		}
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
			if knowledge.Volume != "" {
				mountPath = knowledge.Volume
			} else if prov := spec.GetProvider(knowledge.Provider); prov.MountPath != "" {
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

	// --- Integrations ---
	for name, tool := range ds.Integrations {
		replicas := tool.Replicas
		if replicas == 0 {
			replicas = 1
		}
		w := buildWorkload(componentInput{
			kind: "integration", key: name, image: tool.Image,
			replicas: replicas, resources: tool.Resources,
			update: tool.Update, hc: tool.Healthcheck,
			endpoints: tool.Endpoints,
		})
		wid, err := insertWorkload(w)
		if err != nil {
			return fmt.Errorf("insert integration %s workload: %w", name, err)
		}
		if err := saveEndpoints(wid, tool.Endpoints); err != nil {
			return fmt.Errorf("integration %s endpoints: %w", name, err)
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
		// Ingestion webhook ingress
		if ingestion.Trigger.Type == "webhook" && nsCfg != nil && nsCfg.IngestionIngressDomain != "" && nsCfg.Namespace != "" {
			host := k8s.GenerateIngestionIngressHost(agentName, nsCfg.Namespace, name, nsCfg.IngestionIngressDomain)
			// Find the http service for this ingestion workload
			var svcID int
			httpEp := ingestion.Endpoints["http"]
			if httpEp.Port > 0 {
				err := tx.QueryRow(`
					SELECT ds.id FROM deployment_services ds
					WHERE ds.workload_id = $1 AND ds.port = $2
					LIMIT 1
				`, wid, httpEp.Port).Scan(&svcID)
				if err == nil && svcID > 0 {
					if err := insertIngress(svcID, &Ingress{
						Hostname: host, Path: "/", TLSEnabled: true,
					}); err != nil {
						return fmt.Errorf("ingestion %s ingress: %w", name, err)
					}
				}
			}
		}
	}

	// --- Messaging (Interfaces) — sidecar in agent pod ---
	if ds.Interfaces != nil && len(ds.Interfaces.Adapters) > 0 {
		resourceName := deployment.GenerateAgentResourceName(agentName, "messaging")
		sc := &Sidecar{
			Name:          resourceName,
			ComponentKind: "messaging",
			Image:         ds.Interfaces.Image,
			CPURequest:    ds.Interfaces.Resources.CPU,
			MemoryRequest: ds.Interfaces.Resources.Memory,
			CPULimit:      ds.Interfaces.Resources.CPULimit,
			MemoryLimit:   ds.Interfaces.Resources.MemoryLimit,
		}
		scID, err := insertSidecar(sc)
		if err != nil {
			return fmt.Errorf("insert messaging sidecar: %w", err)
		}
		var webSvcID int
		webEnabled := false
		for _, adapter := range ds.Interfaces.Adapters {
			if adapter == "web" {
				webEnabled = true
				break
			}
		}
		for name, ep := range ds.Interfaces.Endpoints {
			proto := ep.Protocol
			if proto == "" {
				proto = "http"
			}
			svcID, err := insertSidecarService(scID, &Service{
				Name: name, Port: ep.Port, TargetPort: ep.Port, Protocol: proto,
			})
			if err != nil {
				return fmt.Errorf("messaging service %s: %w", name, err)
			}
			if name == "http" {
				webSvcID = svcID
			}
		}
		// Messaging ingress — when web adapter is enabled.
		// Uses GenerateMessagingIngressHost (not GenerateIngressHost) to match
		// the hostname that spec_applier creates in K8s.
		if webEnabled && webSvcID > 0 {
			httpEp := ds.Interfaces.Endpoints["http"]
			host := ""
			if httpEp.Expose != nil && httpEp.Expose.Domain != "" {
				host = httpEp.Expose.Domain
			} else if nsCfg != nil && nsCfg.IngressDomain != "" && nsCfg.Namespace != "" {
				host = k8s.GenerateMessagingIngressHost(agentName, nsCfg.Namespace, nsCfg.IngressDomain)
			}
			if host != "" {
				if err := insertIngress(webSvcID, &Ingress{
					Hostname: host, Path: "/", TLSEnabled: true,
				}); err != nil {
					return fmt.Errorf("messaging ingress: %w", err)
				}
			}
		}
	}

	// --- Collector (Observability) — standalone deployment ---
	if ds.Observability.Enabled {
		otlpHTTPPort := ds.Observability.Port
		if otlpHTTPPort == 0 {
			otlpHTTPPort = 4318
		}
		otlpGRPCPort := otlpHTTPPort - 1
		collectorW := &Workload{
			Name:          deployment.GenerateAgentResourceName(agentName, "collector"),
			ComponentKind: "collector",
			WorkloadType:  "deployment",
			Image:         ds.Observability.Image,
			Replicas:      1,
			CPURequest:    ds.Observability.Resources.CPU,
			MemoryRequest: ds.Observability.Resources.Memory,
			CPULimit:      ds.Observability.Resources.CPULimit,
			MemoryLimit:   ds.Observability.Resources.MemoryLimit,
		}
		collectorWID, err := insertWorkload(collectorW)
		if err != nil {
			return fmt.Errorf("insert collector workload: %w", err)
		}
		if _, err := insertService(collectorWID, &Service{
			Name: "otlp-grpc", Port: otlpGRPCPort, TargetPort: otlpGRPCPort, Protocol: "grpc",
		}); err != nil {
			return fmt.Errorf("collector grpc service: %w", err)
		}
		if _, err := insertService(collectorWID, &Service{
			Name: "otlp-http", Port: otlpHTTPPort, TargetPort: otlpHTTPPort, Protocol: "http",
		}); err != nil {
			return fmt.Errorf("collector http service: %w", err)
		}
	}

	// --- User-declared variables ---
	//
	// Two writes during the migration window:
	//   - deployment_build_env (rows with source='user_var') — primary;
	//     read by RehydrateSecrets and the deployment-detail API.
	//   - deployment_variables — legacy; still consumed by admingrpc
	//     and a few drift / template-prefill paths that haven't been
	//     migrated. Will be dropped together with those readers.
	//
	// The build_env write fans out per (variable, role) target;
	// deployment_variables stays a single row per (deployment, name).
	{
		varRefs := map[string]string{}
		if nsCfg != nil {
			varRefs = nsCfg.VarRefs
		}
		userRows := deployment.UserVarResolutions(ds, varRefs)
		if nsCfg == nil || !nsCfg.SkipBuildEnvClear {
			if _, err := tx.Exec(
				`DELETE FROM deployment_build_env WHERE deployment_id = $1`,
				deploymentID,
			); err != nil {
				return fmt.Errorf("clear deployment_build_env: %w", err)
			}
		}
		for _, r := range userRows {
			ct, nonce, err := encryptResolution(enc, r)
			if err != nil {
				return fmt.Errorf("encrypt %s/%s: %w", r.Role, r.EnvName, err)
			}
			var userVarName, accountVarRef sql.NullString
			var optional sql.NullBool
			if r.UserVarName != "" {
				userVarName = sql.NullString{String: r.UserVarName, Valid: true}
			}
			if r.AccountVarRef != "" {
				accountVarRef = sql.NullString{String: r.AccountVarRef, Valid: true}
			}
			optional = sql.NullBool{Bool: r.Optional, Valid: true}
			if _, err := tx.Exec(`
				INSERT INTO deployment_build_env
					(deployment_id, role, env_name, value_encrypted, nonce,
					 is_secret, source, user_var_name, account_var_ref, optional)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			`, deploymentID, string(r.Role), r.EnvName, ct, nonce,
				r.IsSecret, string(r.Source), userVarName, accountVarRef, optional); err != nil {
				return fmt.Errorf("insert build_env %s/%s: %w", r.Role, r.EnvName, err)
			}
		}
	}

	// deployment_resolved_keys is dropped along with deployment_variables.
	// Drift detection that consumed it is paused; a row-based replacement
	// will land alongside re-enabling the periodic job.

	return nil
}

// RepairNormalizedSpec re-parses the stored DeploymentSpecJSON and rebuilds
// the deployment_workloads, deployment_services, deployment_ingresses, and
// deployment_volumes tables. Variables are preserved (they require resolved
// env and encryptor which are not available at repair time).
//
// liveSecretData, if non-nil, supplies the actual K8s Secret data so that
// secret value hashes can be computed for drift detection. When nil, secret
// keys are still tracked but without value hashes.
func (s *Store) RepairNormalizedSpec(deploymentID string, nsCfg *NormalizedSpecConfig, liveSecretData map[string][]byte) (workloads, services, ingresses int, err error) {
	dep, err := s.GetDeploymentByID(deploymentID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		return 0, 0, 0, fmt.Errorf("deployment not found: %s", deploymentID)
	}
	if dep.DeploymentSpecJSON == "" {
		return 0, 0, 0, fmt.Errorf("deployment has no spec JSON")
	}

	var ds spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(dep.DeploymentSpecJSON), &ds); err != nil {
		return 0, 0, 0, fmt.Errorf("parse spec JSON: %w", err)
	}

	// Fill in namespace from the deployment record so ingress hosts can be generated
	if nsCfg != nil && nsCfg.Namespace == "" {
		nsCfg.Namespace = dep.Namespace
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Delete existing normalized data (workloads/sidecars cascade to services, ingresses, volumes).
	// Variables are intentionally NOT deleted — they contain KMS-encrypted
	// secret values that cannot be reconstructed from the spec JSON (which
	// has secrets stripped).
	if _, err := tx.Exec("DELETE FROM deployment_workloads WHERE deployment_id = $1", deploymentID); err != nil {
		return 0, 0, 0, fmt.Errorf("delete workloads: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM deployment_sidecars WHERE deployment_id = $1", deploymentID); err != nil {
		return 0, 0, 0, fmt.Errorf("delete sidecars: %w", err)
	}

	// Resolve env to regenerate the ConfigMap/Secret key sets for drift detection.
	// This must happen before clearing variables since resolving needs ${variables.*}.
	rctx := deployment.ResolveContext{
		Namespace:  dep.Namespace,
		AgentName:  dep.AgentName,
		BuildID:    dep.BuildID,
		SecretName: deployment.GenerateSecretName(dep.AgentName, dep.BuildID),
	}
	resolved := deployment.ResolveDeploymentSpecEnv(&ds, rctx)

	// The stored spec has secret values stripped, so ResolveDeploymentSpecEnv
	// won't include them in SecretData. When live K8s Secret data is available,
	// use it as the source of truth — it contains exactly the keys the spec
	// applier created (user secrets + managed credentials like ANTHROPIC_API_KEY,
	// minus optional secrets with no value).
	if liveSecretData != nil {
		// Trust the live Secret as the complete key set.
		for k, v := range liveSecretData {
			resolved.SecretData[k] = string(v)
		}
	} else {
		// No live data — fall back to variable definitions, but only include
		// secrets that had a value (matching spec applier behavior).
		for key, v := range ds.Variables {
			if v.Secret && v.Value != "" {
				resolved.SecretData[strings.ToUpper(key)] = ""
			}
		}
		// Also inject managed secrets if provided via config.
		if nsCfg != nil {
			for k, v := range nsCfg.ManagedSecrets {
				resolved.SecretData[k] = v
			}
		}
	}

	// Clear variables from the spec so SaveNormalizedSpec doesn't re-insert
	// them with empty/stripped values, duplicating existing rows.
	ds.Variables = nil

	// Tell SaveNormalizedSpec not to DELETE deployment_build_env — we are
	// preserving existing rows because the KMS encryptor is unavailable and
	// the secrets cannot be re-encrypted from the stripped spec JSON.
	if nsCfg == nil {
		nsCfg = &NormalizedSpecConfig{}
	}
	nsCfg.SkipBuildEnvClear = true

	// Re-run SaveNormalizedSpec with resolved env (for key tracking) but nil
	// encryptor and cleared variables so only workloads/services/ingresses
	// and resolved keys are regenerated.
	if err := SaveNormalizedSpec(tx, deploymentID, &ds, resolved, nil, nsCfg); err != nil {
		return 0, 0, 0, fmt.Errorf("save normalized spec: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, 0, fmt.Errorf("commit: %w", err)
	}

	// Count what was created
	sums, _ := s.GetWorkloadSummaries(deploymentID)
	svcs, _ := s.GetServices(deploymentID)
	ings, _ := s.GetIngresses(deploymentID)
	return len(sums), len(svcs), len(ings), nil
}

// GetDeploymentVariables returns the user-declared variables for a
// deployment, reconstructed from deployment_build_env user_var rows.
//
// One Variable per distinct user_var_name. Targets is reconstructed
// from the set of roles the variable fans out to:
//
//	role 'agent'              → "agent"
//	role 'messaging'          → "interface"   (adapter detail is lost)
//	role 'ingestion:<name>'   → "ingestion"
//	role 'knowledge:<name>'   → (skipped — knowledge containers aren't user targets)
//	role 'collector'          → (skipped — collector isn't a user target)
//
// Value is plaintext for non-secret rows and base64-encoded ciphertext
// for secret rows, matching the legacy deployment_variables shape so
// existing callers keep working.
func (s *Store) GetDeploymentVariables(deploymentID string) ([]Variable, error) {
	rows, err := s.db.Query(`
		SELECT role, env_name, value_encrypted, nonce, is_secret,
		       user_var_name, account_var_ref, optional
		FROM deployment_build_env
		WHERE deployment_id = $1 AND source = 'user_var'
		ORDER BY user_var_name, role
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query deployment_build_env user_vars: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	type pending struct {
		v       Variable
		targets map[string]bool
	}
	byName := map[string]*pending{}
	order := []string{}

	for rows.Next() {
		var (
			role, envName  string
			valueEncrypted []byte
			nonce          []byte
			isSecret       bool
			userVarName    sql.NullString
			accountVarRef  sql.NullString
			optional       sql.NullBool
		)
		if err := rows.Scan(&role, &envName, &valueEncrypted, &nonce, &isSecret,
			&userVarName, &accountVarRef, &optional); err != nil {
			return nil, fmt.Errorf("scan deployment_build_env: %w", err)
		}
		name := userVarName.String
		if name == "" {
			name = envName
		}

		p, ok := byName[name]
		if !ok {
			val := ""
			var n []byte
			if isSecret {
				if len(valueEncrypted) > 0 {
					val = base64.StdEncoding.EncodeToString(valueEncrypted)
					n = nonce
				}
			} else {
				val = string(valueEncrypted)
			}
			p = &pending{
				v: Variable{
					DeploymentID: deploymentID,
					Name:         name,
					Value:        val,
					Ref:          accountVarRef.String,
					Secret:       isSecret,
					Optional:     optional.Bool,
					Nonce:        n,
				},
				targets: map[string]bool{},
			}
			byName[name] = p
			order = append(order, name)
		}
		switch {
		case role == "agent":
			p.targets["agent"] = true
		case role == "messaging":
			p.targets["interface"] = true
		case strings.HasPrefix(role, "ingestion:"):
			p.targets["ingestion"] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]Variable, 0, len(order))
	for _, name := range order {
		p := byName[name]
		ts := make([]string, 0, len(p.targets))
		for t := range p.targets {
			ts = append(ts, t)
		}
		// Stable order for consumers that compare slices.
		strSort(ts)
		p.v.Targets = ts
		result = append(result, p.v)
	}
	return result, nil
}

// strSort is a tiny helper to keep the dependency footprint here narrow
// (avoids importing "sort" just for one call).
func strSort(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// ResolvedKeys holds the expected ConfigMap and Secret key sets for a deployment,
// along with SHA-256 hashes of each value for detecting value drift.
type ResolvedKeys struct {
	DeploymentID    string
	ConfigMapKeys   []string
	SecretKeys      []string
	ConfigMapHashes map[string]string // key → sha256(value)
	SecretHashes    map[string]string // key → sha256(value)
}

// GetResolvedKeys returns the stored resolved ConfigMap/Secret key sets for a deployment.
// Returns nil (without error) if no resolved keys have been stored yet.
func (s *Store) GetResolvedKeys(deploymentID string) (*ResolvedKeys, error) {
	rk := &ResolvedKeys{DeploymentID: deploymentID}
	var cmHashJSON, secHashJSON []byte
	err := s.db.QueryRow(`
		SELECT configmap_keys, secret_keys, configmap_hashes, secret_hashes
		FROM deployment_resolved_keys
		WHERE deployment_id = $1
	`, deploymentID).Scan(pq.Array(&rk.ConfigMapKeys), pq.Array(&rk.SecretKeys), &cmHashJSON, &secHashJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get resolved keys: %w", err)
	}
	if len(cmHashJSON) > 0 {
		_ = json.Unmarshal(cmHashJSON, &rk.ConfigMapHashes)
	}
	if len(secHashJSON) > 0 {
		_ = json.Unmarshal(secHashJSON, &rk.SecretHashes)
	}
	return rk, nil
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
	Name          string
	ComponentKind string
	ComponentKey  string
	WorkloadType  string
	Image         string
	Replicas      int
	CPURequest    string
	MemoryRequest string
	Persistent    bool
}

// GetWorkloadSummaries returns lightweight workload data for a deployment.
// Used by API responses and admin gRPC to avoid full spec JSON parsing.
func (s *Store) GetWorkloadSummaries(deploymentID string) ([]*WorkloadSummary, error) {
	rows, err := s.db.Query(`
		SELECT name, component_kind, component_key, workload_type, image, replicas, cpu_request, memory_request, persistent
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
		if err := rows.Scan(&w.Name, &w.ComponentKind, &w.ComponentKey, &w.WorkloadType, &w.Image, &w.Replicas, &w.CPURequest, &w.MemoryRequest, &w.Persistent); err != nil {
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

// GetServices returns all services for a deployment (across workloads and sidecars).
func (s *Store) GetServices(deploymentID string) ([]*Service, error) {
	rows, err := s.db.Query(`
		SELECT ds.id, COALESCE(ds.workload_id, 0), ds.name, ds.port, ds.target_port, ds.protocol, dw.name
		FROM deployment_services ds
		JOIN deployment_workloads dw ON dw.id = ds.workload_id
		WHERE dw.deployment_id = $1
		UNION ALL
		SELECT ds.id, 0, ds.name, ds.port, ds.target_port, ds.protocol, sc.name
		FROM deployment_services ds
		JOIN deployment_sidecars sc ON sc.id = ds.sidecar_id
		WHERE sc.deployment_id = $1
		ORDER BY id
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query services: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []*Service
	for rows.Next() {
		var svc Service
		if err := rows.Scan(&svc.ID, &svc.WorkloadID, &svc.Name, &svc.Port, &svc.TargetPort, &svc.Protocol, &svc.WorkloadName); err != nil {
			return nil, fmt.Errorf("scan service: %w", err)
		}
		result = append(result, &svc)
	}
	return result, rows.Err()
}

// GetSidecars returns all sidecars for a deployment.
func (s *Store) GetSidecars(deploymentID string) ([]*Sidecar, error) {
	rows, err := s.db.Query(`
		SELECT id, deployment_id, name, component_kind, image,
			cpu_request, memory_request, cpu_limit, memory_limit
		FROM deployment_sidecars
		WHERE deployment_id = $1
		ORDER BY id
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query sidecars: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []*Sidecar
	for rows.Next() {
		var sc Sidecar
		if err := rows.Scan(&sc.ID, &sc.DeploymentID, &sc.Name, &sc.ComponentKind, &sc.Image,
			&sc.CPURequest, &sc.MemoryRequest, &sc.CPULimit, &sc.MemoryLimit); err != nil {
			return nil, fmt.Errorf("scan sidecar: %w", err)
		}
		result = append(result, &sc)
	}
	return result, rows.Err()
}

// GetIngresses returns all ingresses for a deployment.
func (s *Store) GetIngresses(deploymentID string) ([]*Ingress, error) {
	rows, err := s.db.Query(`
		SELECT di.id, di.service_id, di.hostname, di.path, di.tls_enabled
		FROM deployment_ingresses di
		JOIN deployment_services ds ON ds.id = di.service_id
		LEFT JOIN deployment_workloads dw ON dw.id = ds.workload_id
		LEFT JOIN deployment_sidecars sc ON sc.id = ds.sidecar_id
		WHERE dw.deployment_id = $1 OR sc.deployment_id = $1
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
