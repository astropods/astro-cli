package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/postman/astro/apps/astro-server/internal/deployment"
	spec "github.com/postman/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ApplyDeploymentSpec applies a fully resolved AstroDeploymentSpec to the cluster.
func (a *Applier) ApplyDeploymentSpec(
	ctx context.Context,
	ds *spec.AstroDeploymentSpec,
) (*ApplyResult, error) {
	result := &ApplyResult{
		Resources:        []deployment.ResourceStatus{},
		ServiceEndpoints: []deployment.ServiceEndpoint{},
		Errors:           []deployment.DeploymentError{},
	}

	agentName := ds.Source.Name
	buildID := ds.Source.Build

	// Use the deployment spec's target namespace, falling back to the applier default
	if ds.Target.Namespace != "" {
		a.namespace = ds.Target.Namespace
	}

	// Resolve all ${} references and build ConfigMap/Secret data
	rctx := deployment.ResolveContext{
		Namespace:  a.namespace,
		AgentName:  agentName,
		BuildID:    buildID,
		SecretName: deployment.GenerateSecretName(agentName, buildID),
	}
	resolved := deployment.ResolveDeploymentSpecEnv(ds, rctx)

	// Only generate resource names when there is data to back them;
	// referencing a non-existent Secret/ConfigMap causes K8s errors.
	secretName := ""
	if len(resolved.SecretData) > 0 {
		secretName = deployment.GenerateSecretName(agentName, buildID)
	}
	configMapName := ""
	if len(resolved.ConfigMapData) > 0 {
		configMapName = deployment.GenerateConfigMapName(agentName, buildID)
	}

	// Ensure namespace exists
	if err := a.ensureNamespace(ctx); err != nil {
		return result, fmt.Errorf("failed to ensure namespace: %w", err)
	}

	// Clean up resources from previous builds whose names may have changed
	if cleanupErrs := a.cleanupStaleBuildResources(ctx, agentName, buildID); len(cleanupErrs) > 0 {
		for _, e := range cleanupErrs {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: "cleanup", Kind: "Cleanup", Error: e.Error(),
			})
		}
	}

	// Phase 1: Create Secret (credentials)
	if len(resolved.SecretData) > 0 {
		secret := BuildSecret(a.namespace, agentName, buildID, resolved.SecretData)
		status, err := a.applySecret(ctx, secret)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: secret.Name, Kind: "Secret", Error: err.Error(),
			})
		}
	}

	// Phase 2: Create ConfigMap (connection strings + resolved env)
	if len(resolved.ConfigMapData) > 0 {
		configMap := BuildConfigMap(a.namespace, agentName, buildID, resolved.ConfigMapData)
		status, err := a.applyConfigMap(ctx, configMap)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: configMap.Name, Kind: "ConfigMap", Error: err.Error(),
			})
		}
	}

	// Phase 3: Create Services
	// Model services
	for name, model := range ds.Models {
		resourceName := deployment.GenerateResourceName(agentName, "model", name)
		port := primaryPort(model.Endpoints)
		if port == 0 {
			port = 8080
		}
		svc := BuildService(ServiceConfig{
			Name: resourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("model-%s", name),
			Port: port, ServiceType: corev1.ServiceTypeClusterIP,
		})
		a.applyServiceAndRecord(ctx, svc, result)
	}

	// Knowledge services
	for name, knowledge := range ds.Knowledge {
		resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)
		port := primaryPort(knowledge.Endpoints)
		svc := a.buildKnowledgeService(resourceName, agentName, buildID, name, port)
		a.applyServiceAndRecord(ctx, svc, result)
	}

	// Tool services
	for name, tool := range ds.Tools {
		resourceName := deployment.GenerateResourceName(agentName, "tool", name)
		port := primaryPort(tool.Endpoints)
		if port == 0 {
			port = 8080
		}
		svc := BuildService(ServiceConfig{
			Name: resourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("tool-%s", name),
			Port: port, ServiceType: corev1.ServiceTypeClusterIP,
		})
		a.applyServiceAndRecord(ctx, svc, result)
	}

	// Agent service
	agentResourceName := deployment.GenerateAgentResourceName(agentName, "agent")
	agentPort := primaryPort(ds.Agent.Endpoints)
	if agentPort == 0 {
		agentPort = 8080
	}
	agentService := BuildService(ServiceConfig{
		Name: agentResourceName, Namespace: a.namespace, AgentName: agentName,
		BuildID: buildID, Component: "agent",
		Port: agentPort, ServiceType: corev1.ServiceTypeClusterIP,
	})
	a.applyServiceAndRecord(ctx, agentService, result)

	// Phase 4: Create StatefulSets for persistent knowledge
	for name, knowledge := range ds.Knowledge {
		if !knowledge.Persistent {
			continue
		}
		resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)
		port := primaryPort(knowledge.Endpoints)

		container := spec.ContainerConfig{Image: knowledge.Image, Port: int(port)}
		resolvedContainer, err := a.resolveContainerImage(container)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: resourceName, Kind: "StatefulSet",
				Error: fmt.Sprintf("failed to resolve image: %v", err),
			})
			continue
		}

		storageSize := "10Gi"
		storageClass := ""
		accessMode := corev1.ReadWriteOnce
		if knowledge.Storage != nil {
			if knowledge.Storage.Size != "" {
				storageSize = knowledge.Storage.Size
			}
			storageClass = knowledge.Storage.Class
			if knowledge.Storage.AccessMode == "ReadWriteMany" {
				accessMode = corev1.ReadWriteMany
			}
		}

		ssCfg := StatefulSetConfig{
			Name: resourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("knowledge-%s", name),
			Container: resolvedContainer, Port: port,
			StorageSize: storageSize, StorageClass: storageClass, AccessMode: accessMode,
			Healthcheck: knowledge.Healthcheck, ImagePullPolicy: a.imagePullPolicy,
			Replicas:        int32(knowledge.Replicas), //nolint:gosec
			Resources:       BuildResourceRequirements(knowledge.Resources),
			Strategy:        BuildStatefulSetUpdateStrategy(knowledge.Update),
			Provider:        knowledge.Provider,
			ProviderSection: "knowledge",
		}
		ss, err := BuildStatefulSet(ssCfg)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: resourceName, Kind: "StatefulSet", Error: err.Error(),
			})
			continue
		}
		status, err := a.applyStatefulSet(ctx, ss)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: ss.Name, Kind: "StatefulSet", Error: err.Error(),
			})
		}
	}

	// Phase 4b: Create StatefulSets for persistent models (e.g., ollama with model pull)
	for name, model := range ds.Models {
		if !model.Persistent {
			continue
		}
		resourceName := deployment.GenerateResourceName(agentName, "model", name)
		port := primaryPort(model.Endpoints)
		if port == 0 {
			port = 8080
		}

		container := spec.ContainerConfig{Image: model.Image, Port: int(port), Environment: model.Environment}
		resolvedContainer, err := a.resolveContainerImage(container)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: resourceName, Kind: "StatefulSet",
				Error: fmt.Sprintf("failed to resolve image: %v", err),
			})
			continue
		}

		resolvedContainer.Persistent = true

		// Override healthcheck with model-aware readiness when Model is set
		healthcheck := model.Healthcheck
		if model.Model != "" && healthcheck == nil {
			healthcheck = &spec.Healthcheck{
				Test: []string{"sh", "-c",
					fmt.Sprintf("ollama list | grep -q '%s'", model.Model),
				},
				Interval: "15s",
				Timeout:  "5s",
				Retries:  40, // ~10 min for large model pulls
			}
		}

		ssCfg := StatefulSetConfig{
			Name: resourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("model-%s", name),
			Container: resolvedContainer, Port: port,
			StorageSize: "50Gi", AccessMode: corev1.ReadWriteOnce,
			Healthcheck: healthcheck, ImagePullPolicy: a.imagePullPolicy,
			Replicas:        int32(model.Replicas), //nolint:gosec
			Resources:       BuildResourceRequirementsWithGPU(model.Resources, model.GPU),
			Strategy:        BuildStatefulSetUpdateStrategy(model.Update),
			NodeSelector:    BuildGPUNodeSelector(model.GPU),
			Tolerations:     BuildGPUTolerations(model.GPU),
			Provider:        model.Provider,
			ProviderSection: "models",
		}

		// Add model pull postStart hook
		if model.Model != "" {
			ssCfg.PostStartCommand = []string{
				"sh", "-c",
				fmt.Sprintf("until ollama list >/dev/null 2>&1; do sleep 1; done; ollama pull %s", model.Model),
			}
		}

		ss, err := BuildStatefulSet(ssCfg)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: resourceName, Kind: "StatefulSet", Error: err.Error(),
			})
			continue
		}
		status, err := a.applyStatefulSet(ctx, ss)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: ss.Name, Kind: "StatefulSet", Error: err.Error(),
			})
		}
	}

	// Phase 5: Create Deployments
	// Models (non-persistent)
	for name, model := range ds.Models {
		if model.Persistent {
			continue
		}
		resourceName := deployment.GenerateResourceName(agentName, "model", name)
		port := primaryPort(model.Endpoints)
		if port == 0 {
			port = 8080
		}

		container := spec.ContainerConfig{Image: model.Image, Port: int(port), Environment: model.Environment}
		resolvedContainer, err := a.resolveContainerImage(container)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: resourceName, Kind: "Deployment",
				Error: fmt.Sprintf("failed to resolve image: %v", err),
			})
			continue
		}

		cfg := DeploymentConfig{
			Name: resourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("model-%s", name),
			Container: resolvedContainer, Port: port,
			Provider: model.Provider, ProviderSection: "models",
			Healthcheck:     model.Healthcheck,
			ImagePullPolicy: a.imagePullPolicy,
			Replicas:        int32(model.Replicas), //nolint:gosec
			Resources:       BuildResourceRequirementsWithGPU(model.Resources, model.GPU),
			Strategy:        BuildDeploymentStrategy(model.Update),
			NodeSelector:    BuildGPUNodeSelector(model.GPU),
			Tolerations:     BuildGPUTolerations(model.GPU),
		}
		depl := BuildDeployment(cfg)
		status, err := a.applyDeployment(ctx, depl)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: depl.Name, Kind: "Deployment", Error: err.Error(),
			})
		}
	}

	// Non-persistent knowledge as Deployments
	for name, knowledge := range ds.Knowledge {
		if knowledge.Persistent {
			continue
		}
		resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)
		port := primaryPort(knowledge.Endpoints)

		container := spec.ContainerConfig{Image: knowledge.Image, Port: int(port), Environment: knowledge.Environment}
		resolvedContainer, err := a.resolveContainerImage(container)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: resourceName, Kind: "Deployment",
				Error: fmt.Sprintf("failed to resolve image: %v", err),
			})
			continue
		}

		cfg := DeploymentConfig{
			Name: resourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("knowledge-%s", name),
			Container: resolvedContainer, Port: port,
			Provider: knowledge.Provider, ProviderSection: "knowledge",
			Healthcheck:     knowledge.Healthcheck,
			ImagePullPolicy: a.imagePullPolicy,
			Replicas:        int32(knowledge.Replicas), //nolint:gosec
			Resources:       BuildResourceRequirements(knowledge.Resources),
			Strategy:        BuildDeploymentStrategy(knowledge.Update),
		}
		depl := BuildDeployment(cfg)
		status, err := a.applyDeployment(ctx, depl)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: depl.Name, Kind: "Deployment", Error: err.Error(),
			})
		}
	}

	// Tools
	for name, tool := range ds.Tools {
		resourceName := deployment.GenerateResourceName(agentName, "tool", name)
		port := primaryPort(tool.Endpoints)
		if port == 0 {
			port = 8080
		}

		container := spec.ContainerConfig{Image: tool.Image, Port: int(port), Environment: tool.Environment}
		resolvedContainer, err := a.resolveContainerImage(container)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: resourceName, Kind: "Deployment",
				Error: fmt.Sprintf("failed to resolve image: %v", err),
			})
			continue
		}

		cfg := DeploymentConfig{
			Name: resourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("tool-%s", name),
			Container: resolvedContainer, Port: port,
			ImagePullPolicy: a.imagePullPolicy,
			Replicas:        int32(tool.Replicas), //nolint:gosec
			Resources:       BuildResourceRequirements(tool.Resources),
			Strategy:        BuildDeploymentStrategy(tool.Update),
		}
		depl := BuildDeployment(cfg)
		status, err := a.applyDeployment(ctx, depl)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: depl.Name, Kind: "Deployment", Error: err.Error(),
			})
		}
	}

	// Main agent deployment
	agentContainer := spec.ContainerConfig{Image: ds.Agent.Image}
	resolvedAgentContainer, err := a.resolveContainerImage(agentContainer)
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: agentResourceName, Kind: "Deployment",
			Error: fmt.Sprintf("failed to resolve image: %v", err),
		})
	} else {
		cfg := DeploymentConfig{
			Name: agentResourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: "agent",
			Container: resolvedAgentContainer, Port: agentPort,
			SecretName: secretName, ConfigMapName: configMapName,
			Healthcheck: ds.Agent.Healthcheck, ImagePullPolicy: a.imagePullPolicy,
			Replicas:  int32(ds.Agent.Replicas), //nolint:gosec
			Resources: BuildResourceRequirements(ds.Agent.Resources),
			Strategy:  BuildDeploymentStrategy(ds.Agent.Update),
		}
		agentDepl := BuildDeployment(cfg)
		status, err := a.applyDeployment(ctx, agentDepl)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: agentDepl.Name, Kind: "Deployment", Error: err.Error(),
			})
		}
	}

	// Phase 5b: Messaging interfaces
	if ds.Interfaces != nil && len(ds.Interfaces.Adapters) > 0 {
		// Resolve interface grpc port: prefer "grpc" endpoint, fall back to primary, default 9090
		grpcPort := int32(0)
		if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "grpc"); ep != nil {
			grpcPort = int32(ep.Port) // nolint:gosec
		}
		if grpcPort == 0 {
			grpcPort = int32(spec.PrimaryPort(ds.Interfaces.Endpoints)) // nolint:gosec
		}
		if grpcPort == 0 {
			grpcPort = 9090
		}

		// Resolve web port from "http" endpoint
		webPort := int32(0)
		if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "http"); ep != nil {
			webPort = int32(ep.Port) // nolint:gosec
		}
		if webPort == 0 {
			webPort = 8080
		}

		// Resolve interface resources from deployment spec
		var msgResources *corev1.ResourceRequirements
		if r := BuildResourceRequirements(ds.Interfaces.Resources); r != nil {
			msgResources = r
		}

		// Resolve interface environment — collect only entries whose keys are
		// defined in interfaces.environment (resolved values are in the shared
		// ConfigMap already, but we also inject them directly so the messaging
		// container gets them even without the ConfigMap)
		resolvedIfaceEnv := make(map[string]string)
		for key := range ds.Interfaces.Environment {
			if val, ok := resolved.ConfigMapData[key]; ok {
				resolvedIfaceEnv[key] = val
			}
		}

		// Determine which adapters are enabled
		slackEnabled := false
		webEnabled := false
		for _, adapter := range ds.Interfaces.Adapters {
			switch adapter {
			case "slack":
				slackEnabled = true
			case "web":
				webEnabled = true
			}
		}

		resourceName := deployment.GenerateAgentResourceName(agentName, "messaging")

		// Service
		msgSvc := BuildService(ServiceConfig{
			Name: resourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: "messaging",
			Port: grpcPort, ServiceType: corev1.ServiceTypeClusterIP,
		})
		msgSvc.Spec.Ports[0].Name = "grpc"
		if webEnabled {
			msgSvc.Spec.Ports = append(msgSvc.Spec.Ports, corev1.ServicePort{
				Name: "http", Protocol: corev1.ProtocolTCP,
				Port: webPort, TargetPort: intstr.FromInt(int(webPort)),
			})
		}
		a.applyServiceAndRecord(ctx, msgSvc, result)

		// Deployment
		msgImage := ds.Interfaces.Image
		if msgImage == "" {
			msgImage = "astropods/messaging:latest"
		}
		msgCfg := MessagingDeploymentConfig{
			Name: resourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: "messaging",
			Image: msgImage, Port: grpcPort, SecretName: secretName,
			ConfigMapName: "",
			SlackEnabled:  slackEnabled, WebEnabled: webEnabled,
			WebPort:         webPort,
			ImagePullPolicy: a.imagePullPolicy,
			Resources:       msgResources,
			Environment:     resolvedIfaceEnv,
		}
		msgDepl := BuildMessagingDeployment(msgCfg)
		status, err := a.applyDeployment(ctx, msgDepl)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: msgDepl.Name, Kind: "Deployment", Error: err.Error(),
			})
		}

		// Ingress — expose web adapter if configured
		if webEnabled {
			ingressName := deployment.GenerateAgentResourceName(agentName, "ingress-messaging")
			host := ""
			if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "http"); ep != nil && ep.Expose != nil {
				host = ep.Expose.Domain
			}
			if host == "" && a.ingressDomain != "" {
				host = GenerateIngressHost(agentName, a.namespace, a.ingressDomain)
			}
			if host != "" {
				ingress := BuildIngress(IngressConfig{
					Name: ingressName, Namespace: a.namespace, AgentName: agentName,
					BuildID: buildID, Component: "messaging",
					ServiceName: resourceName, ServicePort: webPort, Host: host,
					ACMCertificateARN: a.acmCertificateARN, ALBGroupName: a.albGroupName,
				})
				status, err = a.applyIngress(ctx, ingress)
				result.Resources = append(result.Resources, status)
				if err != nil {
					result.Errors = append(result.Errors, deployment.DeploymentError{
						Resource: ingress.Name, Kind: "Ingress", Error: err.Error(),
					})
				}
				externalURL := fmt.Sprintf("https://%s", host)
				result.ServiceEndpoints = append(result.ServiceEndpoints, deployment.ServiceEndpoint{
					Name: "messaging", Type: "web",
					URL: externalURL, Port: 443,
				})
			}
		}
	}

	// Phase 6: Collector sidecar for observability
	if ds.Observability.Enabled {
		collectorResourceName := deployment.GenerateAgentResourceName(agentName, "collector")

		// Resolve ports from deployment spec
		otlpHTTPPort := int32(ds.Observability.Port) //nolint:gosec
		if otlpHTTPPort == 0 {
			otlpHTTPPort = 4318
		}
		otlpGRPCPort := otlpHTTPPort - 1

		// Resolve resources from deployment spec
		var collectorResources *corev1.ResourceRequirements
		if r := BuildResourceRequirements(ds.Observability.Resources); r != nil {
			collectorResources = r
		}

		// Resolve environment from deployment spec
		resolvedObsEnv := make(map[string]string)
		for key := range ds.Observability.Environment {
			if val, ok := resolved.ConfigMapData[key]; ok {
				resolvedObsEnv[key] = val
			}
		}

		// Collector service
		collectorSvc := BuildService(ServiceConfig{
			Name: collectorResourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, Component: "collector",
			Port: otlpGRPCPort, ServiceType: corev1.ServiceTypeClusterIP,
		})
		collectorSvc.Spec.Ports[0].Name = "otlp-grpc"
		collectorSvc.Spec.Ports = append(collectorSvc.Spec.Ports, corev1.ServicePort{
			Name: "otlp-http", Protocol: corev1.ProtocolTCP,
			Port: otlpHTTPPort, TargetPort: intstr.FromInt(int(otlpHTTPPort)),
		})
		a.applyServiceAndRecord(ctx, collectorSvc, result)

		// Collector image from deployment spec, fallback to registry default
		collectorImage := ds.Observability.Image
		if collectorImage == "" {
			collectorImage = fmt.Sprintf("%s/prod-astro-collector:latest", a.registryURL)
		}

		// Collector deployment
		collectorCfg := CollectorDeploymentConfig{
			Name: collectorResourceName, Namespace: a.namespace, AgentName: agentName,
			AgentVersion: ds.Source.Build,
			BuildID:      buildID, Component: "collector",
			DeploymentID:     buildID,
			Image:            collectorImage,
			Port:             otlpHTTPPort,
			ConfigMapName:    "",
			SecretName:       "",
			GalileoAPIKey:    a.galileoAPIKey,
			GalileoProject:   a.galileoProject,
			GalileoLogStream: fmt.Sprintf("%s-%s", agentName, buildID),
			ImagePullPolicy:  a.imagePullPolicy,
			Resources:        collectorResources,
			Environment:      resolvedObsEnv,
		}
		collectorDepl := BuildCollectorDeployment(collectorCfg)
		status, err := a.applyDeployment(ctx, collectorDepl)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: collectorDepl.Name, Kind: "Deployment", Error: err.Error(),
			})
		}
	}

	// Phase 7: CronJobs/Jobs for ingestion
	var manualIngestions []string
	for name, ingestion := range ds.Ingestion {
		resourceName := deployment.GenerateResourceName(agentName, "ingestion", name)
		component := fmt.Sprintf("ingestion-%s", name)

		ingestionSpec := spec.Ingestion{
			Container: spec.ContainerConfig{
				Image:       ingestion.Image,
				Port:        spec.PrimaryPort(ingestion.Endpoints),
				Environment: ingestion.Environment,
			},
			Trigger: spec.IngestionTrigger{
				Type: ingestion.Trigger.Type,
			},
		}

		switch ingestion.Trigger.Type {
		case "schedule":
			if ingestion.Trigger.Schedule != "" {
				cronJob := BuildCronJob(CronJobConfig{
					Name: resourceName, Namespace: a.namespace, AgentName: agentName,
					BuildID: buildID, Component: component,
					Schedule:   ingestion.Trigger.Schedule,
					SecretName: secretName, ConfigMapName: configMapName,
					Ingestion: ingestionSpec,
				})
				status, err := a.applyCronJob(ctx, cronJob)
				result.Resources = append(result.Resources, status)
				if err != nil {
					result.Errors = append(result.Errors, deployment.DeploymentError{
						Resource: cronJob.Name, Kind: "CronJob", Error: err.Error(),
					})
				}
			}

		case "startup":
			job := BuildJob(JobConfig{
				Name: resourceName, Namespace: a.namespace, AgentName: agentName,
				BuildID: buildID, Component: component,
				SecretName: secretName, ConfigMapName: configMapName,
				Ingestion: ingestionSpec,
			})
			status, err := a.applyJob(ctx, job)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: job.Name, Kind: "Job", Error: err.Error(),
				})
			}

		case "webhook":
			// port is required for webhook triggers (validated by deployment parser)
			port := int32(spec.PrimaryPort(ingestion.Endpoints)) // nolint:gosec
			// Service
			svc := BuildService(ServiceConfig{
				Name: resourceName, Namespace: a.namespace, AgentName: agentName,
				BuildID: buildID, Component: component,
				Port: port, ServiceType: corev1.ServiceTypeClusterIP,
			})
			a.applyServiceAndRecord(ctx, svc, result)

			// Deployment
			depl := BuildIngestionDeployment(JobConfig{
				Name: resourceName, Namespace: a.namespace, AgentName: agentName,
				BuildID: buildID, Component: component,
				SecretName: secretName, ConfigMapName: configMapName,
				Ingestion: ingestionSpec,
			}, port, a.imagePullPolicy)
			status, err := a.applyDeployment(ctx, depl)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: depl.Name, Kind: "Deployment", Error: err.Error(),
				})
			}

			// Ingress
			if a.ingestionIngressDomain != "" {
				ingressName := deployment.GenerateResourceName(agentName, "ingress", name)
				host := GenerateIngestionIngressHost(agentName, a.namespace, name, a.ingestionIngressDomain)
				ingress := BuildIngress(IngressConfig{
					Name: ingressName, Namespace: a.namespace, AgentName: agentName,
					BuildID: buildID, Component: component,
					ServiceName: resourceName, ServicePort: port, Host: host,
					ACMCertificateARN: a.ingestionACMCertARN, ALBGroupName: a.ingestionALBGroupName,
				})
				status, err = a.applyIngress(ctx, ingress)
				result.Resources = append(result.Resources, status)
				if err != nil {
					result.Errors = append(result.Errors, deployment.DeploymentError{
						Resource: ingress.Name, Kind: "Ingress", Error: err.Error(),
					})
				}
				externalURL := GenerateIngestionExternalURL(agentName, a.namespace, name, a.ingestionIngressDomain)
				result.ServiceEndpoints = append(result.ServiceEndpoints, deployment.ServiceEndpoint{
					Name: fmt.Sprintf("ingestion-%s-webhook", name), Type: "webhook",
					URL: externalURL, Port: 443,
				})
			}

		case "manual":
			manualIngestions = append(manualIngestions, name)
		}
	}

	// Annotate namespace with manual ingestion names so the listing API can surface them
	if len(manualIngestions) > 0 {
		ns, err := a.clientset.CoreV1().Namespaces().Get(ctx, a.namespace, metav1.GetOptions{})
		if err == nil {
			if ns.Annotations == nil {
				ns.Annotations = make(map[string]string)
			}
			ns.Annotations["astro.dev/manual-ingestions"] = strings.Join(manualIngestions, ",")
			_, _ = a.clientset.CoreV1().Namespaces().Update(ctx, ns, metav1.UpdateOptions{})
		}
	}

	// Collect agent service endpoint
	svc, err := a.clientset.CoreV1().Services(a.namespace).Get(ctx, agentResourceName, metav1.GetOptions{})
	if err == nil {
		endpoint := deployment.ServiceEndpoint{
			Name: "agent-http", Type: "http", Port: agentPort,
		}
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			time.Sleep(2 * time.Second)
			for _, ingress := range svc.Status.LoadBalancer.Ingress {
				if ingress.IP != "" {
					endpoint.URL = fmt.Sprintf("http://%s:%d", ingress.IP, agentPort)
				} else if ingress.Hostname != "" {
					endpoint.URL = fmt.Sprintf("http://%s:%d", ingress.Hostname, agentPort)
				}
			}
		} else {
			endpoint.URL = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", agentResourceName, a.namespace, agentPort)
		}
		result.ServiceEndpoints = append(result.ServiceEndpoints, endpoint)
	}

	return result, nil
}

// primaryPort returns the primary port from a component's Endpoints map as int32.
// Prefers the "http" endpoint; otherwise returns the first endpoint sorted alphabetically.
// Returns 0 if endpoints is nil or empty.
func primaryPort(endpoints map[string]spec.Endpoint) int32 {
	return int32(spec.PrimaryPort(endpoints)) // nolint:gosec
}

// ensureNamespace creates the namespace if it doesn't exist, or patches labels if it does.
func (a *Applier) ensureNamespace(ctx context.Context) error {
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "astro-server",
	}
	for k, v := range a.namespaceLabels {
		labels[k] = v
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   a.namespace,
			Labels: labels,
		},
	}
	_, err := a.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Patch labels on redeploy
			existing, getErr := a.clientset.CoreV1().Namespaces().Get(ctx, a.namespace, metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			if existing.Labels == nil {
				existing.Labels = make(map[string]string)
			}
			for k, v := range labels {
				existing.Labels[k] = v
			}
			_, updateErr := a.clientset.CoreV1().Namespaces().Update(ctx, existing, metav1.UpdateOptions{})
			if updateErr != nil {
				return updateErr
			}
		} else {
			return err
		}
	}

	if len(a.podSubnetCIDRs) > 0 {
		if err := a.applyNetworkPolicies(ctx); err != nil {
			return fmt.Errorf("failed to apply network policies: %w", err)
		}
	}
	return nil
}

// applyNetworkPolicies applies namespace isolation NetworkPolicies.
// Policy 1 (default-deny-all): deny all ingress and egress.
// Policy 2 (allow-namespace-traffic): allow intra-namespace pods, ALB/external
// traffic (matching ipBlock 0.0.0.0/0 except podSubnetCIDRs), and DNS.
func (a *Applier) applyNetworkPolicies(ctx context.Context) error {
	policyTypes := []networkingv1.PolicyType{
		networkingv1.PolicyTypeIngress,
		networkingv1.PolicyTypeEgress,
	}

	externalIPBlock := networkingv1.IPBlock{
		CIDR:   "0.0.0.0/0",
		Except: make([]string, 0, len(a.podSubnetCIDRs)),
	}
	externalIPBlock.Except = append(externalIPBlock.Except, a.podSubnetCIDRs...)

	// Policy 1: default-deny-all
	denyAll := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-all",
			Namespace: a.namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: policyTypes,
		},
	}
	if err := a.applyNetworkPolicy(ctx, denyAll); err != nil {
		return fmt.Errorf("default-deny-all: %w", err)
	}

	// Policy 2: allow-namespace-traffic
	allowNamespace := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-namespace-traffic",
			Namespace: a.namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: policyTypes,
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				// Allow from same-namespace pods
				{
					From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{}},
					},
				},
				// Allow from ALB/external (0.0.0.0/0 except pod subnets)
				{
					From: []networkingv1.NetworkPolicyPeer{
						{IPBlock: &externalIPBlock},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				// Allow to same-namespace pods
				{
					To: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{}},
					},
				},
				// Allow to external IPs and the cluster service CIDR (0.0.0.0/0 except
				// pod subnets). This covers both outbound internet traffic (e.g. OpenAI)
				// and DNS to the kube-dns service IP. A separate ports-only DNS rule is
				// intentionally omitted: the AWS VPC CNI PolicyEndpoint controller merges
				// a To-less egress rule onto the ipBlock rule, which would restrict
				// internet egress to port 53 only.
				{
					To: []networkingv1.NetworkPolicyPeer{
						{IPBlock: &externalIPBlock},
					},
				},
			},
		},
	}
	if err := a.applyNetworkPolicy(ctx, allowNamespace); err != nil {
		return fmt.Errorf("allow-namespace-traffic: %w", err)
	}

	return nil
}

// buildKnowledgeService builds a knowledge service with provider-aware extra ports.
func (a *Applier) buildKnowledgeService(
	resourceName, agentName, buildID, name string, port int32,
) *corev1.Service {
	labels := deployment.GenerateLabels(agentName, buildID, fmt.Sprintf("knowledge-%s", name))
	selector := deployment.GenerateSelector(agentName, fmt.Sprintf("knowledge-%s", name))

	servicePorts := []corev1.ServicePort{
		{
			Name: "tcp", Protocol: corev1.ProtocolTCP,
			Port: port, TargetPort: intstr.FromInt(int(port)),
		},
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceName, Namespace: a.namespace, Labels: labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP, Selector: selector, Ports: servicePorts,
		},
	}
}

// applyServiceAndRecord is a helper that applies a service and records the result.
func (a *Applier) applyServiceAndRecord(ctx context.Context, svc *corev1.Service, result *ApplyResult) {
	status, err := a.applyService(ctx, svc)
	result.Resources = append(result.Resources, status)
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: svc.Name, Kind: "Service", Error: err.Error(),
		})
	}
}
