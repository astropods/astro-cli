package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro/packages/astro-spec"
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

	accountName := ds.Source.Account
	agentName := ds.Source.Name
	buildID := ds.Source.Build

	// Namespace always comes from ApplierConfig (server-owned)

	// Resolve all ${} references and build ConfigMap/Secret data
	rctx := deployment.ResolveContext{
		Namespace:  a.namespace,
		AgentName:  agentName,
		BuildID:    buildID,
		SecretName: deployment.GenerateSecretName(agentName, buildID),
	}
	resolved := deployment.ResolveDeploymentSpecEnv(ds, rctx)

	// Inject managed provider credentials (e.g. anthropic-managed).
	// These are platform-provided and bypass user variables entirely.
	if a.managedAnthropicAPIKey != "" {
		resolved.SecretData["ANTHROPIC_API_KEY"] = a.managedAnthropicAPIKey
	}

	// Only generate resource names when there is data to back them;
	// referencing a non-existent Secret/ConfigMap causes K8s errors.
	secretName := ""
	if resolved.HasSecretValues() {
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

	// Phase 1: Create Secret (credentials)
	if resolved.HasSecretValues() {
		secret := BuildSecret(a.namespace, accountName, agentName, buildID, resolved.SecretData)
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
		configMap := BuildConfigMap(a.namespace, accountName, agentName, buildID, resolved.ConfigMapData)
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
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("model-%s", name),
			Port: port, ServiceType: corev1.ServiceTypeClusterIP,
		})
		a.applyServiceAndRecord(ctx, svc, result)
	}

	// Knowledge services
	for name, knowledge := range ds.Knowledge {
		resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)
		svc := a.buildKnowledgeService(resourceName, accountName, agentName, buildID, name, knowledge.Endpoints)
		a.applyServiceAndRecord(ctx, svc, result)
	}

	// Tool services
	for name, tool := range ds.Integrations {
		resourceName := deployment.GenerateResourceName(agentName, "integration", name)
		port := primaryPort(tool.Endpoints)
		if port == 0 {
			port = 8080
		}
		svc := BuildService(ServiceConfig{
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
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
		Name: agentResourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
		BuildID: buildID, Component: "agent",
		Port: agentPort, ServiceType: corev1.ServiceTypeClusterIP,
	})
	a.applyServiceAndRecord(ctx, agentService, result)

	// Agent ingress — when frontend is exposed
	if ep := spec.ExposedEndpoint(ds.Agent.Endpoints); ep != nil {
		ingressName := deployment.GenerateAgentResourceName(agentName, "ingress-agent")
		host := ""
		if ep.Expose != nil {
			host = ep.Expose.Domain
		}
		if host == "" && a.ingressDomain != "" {
			host = GenerateIngressHost(agentName, a.namespace, a.ingressDomain)
		}
		if host != "" {
			ingress := BuildIngress(IngressConfig{
				Name: ingressName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
				BuildID: buildID, Component: "agent",
				ServiceName: agentResourceName, ServicePort: int32(ep.Port), //nolint:gosec
				Host:              host,
				ACMCertificateARN: a.acmCertificateARN, ALBGroupName: a.albGroupName,
			})
			status, err := a.applyIngress(ctx, ingress)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: ingress.Name, Kind: "Ingress", Error: err.Error(),
				})
			}
			externalURL := fmt.Sprintf("https://%s", host)
			result.ServiceEndpoints = append(result.ServiceEndpoints, deployment.ServiceEndpoint{
				Name: "agent", Type: "frontend", URL: externalURL, Port: 443,
			})
		}
	}

	// Phase 4: Create StatefulSets for persistent knowledge
	for name, knowledge := range ds.Knowledge {
		if !knowledge.Persistent {
			continue
		}
		resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)
		port := primaryPort(knowledge.Endpoints)

		container := spec.ContainerConfig{Image: knowledge.Image, Port: int(port), Volume: knowledge.Volume, Environment: knowledge.Environment}
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
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("knowledge-%s", name),
			Container: resolvedContainer, Port: port,
			StorageSize: storageSize, StorageClass: storageClass, AccessMode: accessMode,
			Healthcheck: knowledge.Healthcheck, ImagePullPolicy: a.imagePullPolicy,
			Replicas:        int32(knowledge.Replicas), //nolint:gosec
			Resources:       BuildResourceRequirements(knowledge.Resources),
			Strategy:        BuildStatefulSetUpdateStrategy(knowledge.Update),
			Provider:        knowledge.Provider,
			ProviderSection: "knowledge",
			LocalMode:       a.localMode,
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
			// model.Model is comma-separated; build compound grep check
			modelNames := strings.Split(model.Model, ",")
			var checks []string
			for _, m := range modelNames {
				checks = append(checks, fmt.Sprintf("ollama list | grep -q '%s'", m))
			}
			healthcheck = &spec.Healthcheck{
				Test: []string{"sh", "-c",
					strings.Join(checks, " && "),
				},
				Interval: "15s",
				Timeout:  "5s",
				Retries:  40, // ~10 min for large model pulls
			}
		}

		ssCfg := StatefulSetConfig{
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
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
			LocalMode:       a.localMode,
		}

		// Add model pull postStart hook
		if model.Model != "" {
			// model.Model is comma-separated; pull each model
			modelNames := strings.Split(model.Model, ",")
			var pullCmds []string
			for _, m := range modelNames {
				pullCmds = append(pullCmds, fmt.Sprintf("ollama pull %s", m))
			}
			ssCfg.PostStartCommand = []string{
				"sh", "-c",
				fmt.Sprintf("until ollama list >/dev/null 2>&1; do sleep 1; done; %s", strings.Join(pullCmds, " && ")),
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
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
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
			LocalMode:       a.localMode,
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
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("knowledge-%s", name),
			Container: resolvedContainer, Port: port,
			Provider: knowledge.Provider, ProviderSection: "knowledge",
			Healthcheck:     knowledge.Healthcheck,
			ImagePullPolicy: a.imagePullPolicy,
			Replicas:        int32(knowledge.Replicas), //nolint:gosec
			Resources:       BuildResourceRequirements(knowledge.Resources),
			Strategy:        BuildDeploymentStrategy(knowledge.Update),
			LocalMode:       a.localMode,
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
	for name, tool := range ds.Integrations {
		resourceName := deployment.GenerateResourceName(agentName, "integration", name)
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
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
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

	// Build optional sidecar configs for messaging and collector.
	// These are colocated in the agent pod instead of separate deployments.
	var msgSidecar *MessagingDeploymentConfig
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

		// Resolve web port from "http" endpoint, avoiding conflict with the agent port
		webPort := int32(0)
		if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "http"); ep != nil {
			webPort = int32(ep.Port) // nolint:gosec
		}
		if webPort == 0 {
			webPort = 8090
		}
		if webPort == agentPort {
			webPort = agentPort + 10
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

		// Messaging image
		msgImage := ds.Interfaces.Image
		if msgImage == "" {
			msgImage = "astropods/messaging:latest"
		}

		msgSidecar = &MessagingDeploymentConfig{
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

		// Service — selects the agent pod (messaging is a sidecar container)
		msgSvc := BuildService(ServiceConfig{
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
			BuildID: buildID, Component: "agent",
			Port: grpcPort, ServiceType: corev1.ServiceTypeClusterIP,
		})
		msgSvc.Spec.Ports[0].Name = "grpc"
		// The messaging sidecar runs inside the agent pod. By default, Services
		// only route to Ready endpoints — but the app container won't become
		// ready until it can reach the messaging Service, creating a circular
		// readiness deadlock. Publishing not-ready addresses breaks the cycle.
		msgSvc.Spec.PublishNotReadyAddresses = true
		if webEnabled {
			msgSvc.Spec.Ports = append(msgSvc.Spec.Ports, corev1.ServicePort{
				Name: "http", Protocol: corev1.ProtocolTCP,
				Port: webPort, TargetPort: intstr.FromInt(int(webPort)),
			})
		}
		a.applyServiceAndRecord(ctx, msgSvc, result)

		// Ingress — expose web adapter if configured
		if webEnabled {
			ingressName := deployment.GenerateAgentResourceName(agentName, "ingress-messaging")
			host := ""
			if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "http"); ep != nil && ep.Expose != nil {
				host = ep.Expose.Domain
			}
			if host == "" && a.ingressDomain != "" {
				host = GenerateMessagingIngressHost(agentName, a.namespace, a.ingressDomain)
			}
			if host != "" {
				// Resolve effective OIDC config: only apply when deployment opts in via auth.web.type: oidc
				var effectiveOIDCAuth *OIDCAuthConfig
				if ds.Interfaces.Auth != nil && ds.Interfaces.Auth.Web != nil && ds.Interfaces.Auth.Web.Type == "oidc" {
					effectiveOIDCAuth = a.messagingOIDCAuth
				}

				// Create OIDC credentials secret in agent namespace when auth is enabled
				if effectiveOIDCAuth != nil {
					oidcSecret := buildMessagingOIDCSecret(a.namespace, effectiveOIDCAuth)
					secretStatus, secretErr := a.applySecret(ctx, oidcSecret)
					result.Resources = append(result.Resources, secretStatus)
					if secretErr != nil {
						result.Errors = append(result.Errors, deployment.DeploymentError{
							Resource: oidcSecret.Name, Kind: "Secret", Error: secretErr.Error(),
						})
					}
				}

				ingress := BuildIngress(IngressConfig{
					Name: ingressName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
					BuildID: buildID, Component: "messaging",
					ServiceName: resourceName, ServicePort: webPort, Host: host,
					ACMCertificateARN: a.acmCertificateARN, ALBGroupName: a.albGroupName,
					OIDCAuth: effectiveOIDCAuth,
				})
				status, err := a.applyIngress(ctx, ingress)
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

		// Collector image from deployment spec, fallback to Docker Hub default
		collectorImage := ds.Observability.Image
		if collectorImage == "" {
			collectorImage = "astropods/collector:latest"
		}

		collectorCfg := CollectorDeploymentConfig{
			Name: collectorResourceName, Namespace: a.namespace, AgentName: agentName,
			AgentVersion: ds.Source.Build,
			BuildID:      buildID, Component: "collector",
			DeploymentID:      a.deploymentID,
			Image:             collectorImage,
			Port:              otlpHTTPPort,
			ConfigMapName:     "",
			SecretName:        "",
			LangfuseAuthToken: a.langfuseAuthToken,
			LangfuseBaseURL:   a.langfuseBaseURL,
			ImagePullPolicy:   a.imagePullPolicy,
			Resources:         collectorResources,
			Environment:       resolvedObsEnv,
			AccountID:         accountName,
		}

		// Collector as a standalone Deployment
		collectorDepl := BuildCollectorDeployment(collectorCfg)
		status, err := a.applyDeployment(ctx, collectorDepl)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: collectorDepl.Name, Kind: "Deployment", Error: err.Error(),
			})
		}

		// Collector service — selects the collector pod
		collectorSvc := BuildService(ServiceConfig{
			Name: collectorResourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
			BuildID: buildID, Component: "collector",
			Port: otlpGRPCPort, ServiceType: corev1.ServiceTypeClusterIP,
		})
		collectorSvc.Spec.Ports[0].Name = "otlp-grpc"
		collectorSvc.Spec.Ports = append(collectorSvc.Spec.Ports, corev1.ServicePort{
			Name: "otlp-http", Protocol: corev1.ProtocolTCP,
			Port: otlpHTTPPort, TargetPort: intstr.FromInt(int(otlpHTTPPort)),
		})
		a.applyServiceAndRecord(ctx, collectorSvc, result)
	}

	// Main agent deployment — messaging is colocated as a sidecar container
	agentContainer := spec.ContainerConfig{Image: ds.Agent.Image}
	resolvedAgentContainer, err := a.resolveContainerImage(agentContainer)
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: agentResourceName, Kind: "Deployment",
			Error: fmt.Sprintf("failed to resolve image: %v", err),
		})
	} else {
		cfg := DeploymentConfig{
			Name: agentResourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
			BuildID: buildID, Component: "agent",
			Container: resolvedAgentContainer, Port: agentPort,
			SecretName: secretName, ConfigMapName: configMapName,
			Healthcheck: ds.Agent.Healthcheck, ImagePullPolicy: a.imagePullPolicy,
			Replicas:  int32(ds.Agent.Replicas), //nolint:gosec
			Resources: BuildResourceRequirements(ds.Agent.Resources),
			Strategy:  BuildDeploymentStrategy(ds.Agent.Update),
			Messaging: msgSidecar,
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
					Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
					BuildID: buildID, Component: component,
					Schedule:   ingestion.Trigger.Schedule,
					SecretName: secretName, ConfigMapName: configMapName,
					Ingestion:       ingestionSpec,
					ImagePullPolicy: a.imagePullPolicy,
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
				Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
				BuildID: buildID, Component: component,
				SecretName: secretName, ConfigMapName: configMapName,
				Ingestion:       ingestionSpec,
				ImagePullPolicy: a.imagePullPolicy,
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
				Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
				BuildID: buildID, Component: component,
				Port: port, ServiceType: corev1.ServiceTypeClusterIP,
			})
			a.applyServiceAndRecord(ctx, svc, result)

			// Deployment
			depl := BuildIngestionDeployment(JobConfig{
				Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
				BuildID: buildID, Component: component,
				SecretName: secretName, ConfigMapName: configMapName,
				Ingestion:       ingestionSpec,
				ImagePullPolicy: a.imagePullPolicy,
			}, port)
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
					Name: ingressName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
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

	// Clean up resources from previous builds whose names may have changed (e.g. a renamed tool).
	// Runs after apply so that stable-named resources (Ingresses, Services, Deployments) are
	// updated in-place first — their buildID label is current by the time cleanup checks it,
	// so they are not deleted. Only genuinely stale names (orphaned by a rename) are removed.
	if cleanupErrs := a.cleanupStaleBuildResources(ctx, accountName, agentName, buildID); len(cleanupErrs) > 0 {
		for _, e := range cleanupErrs {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: "cleanup", Kind: "Cleanup", Error: e.Error(),
			})
		}
	}

	// Clean up orphaned resources from previous spec (e.g. removed tools/knowledge)
	expectedNames := computeExpectedResourceNames(ds, a.ingressDomain, a.ingestionIngressDomain)
	if orphanErrs := a.cleanupOrphanedResources(ctx, accountName, agentName, expectedNames); len(orphanErrs) > 0 {
		for _, e := range orphanErrs {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: "orphan-cleanup", Kind: "Cleanup", Error: e.Error(),
			})
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

	annotations := make(map[string]string)
	for k, v := range a.namespaceAnnotations {
		annotations[k] = v
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        a.namespace,
			Labels:      labels,
			Annotations: annotations,
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
			if existing.Annotations == nil {
				existing.Annotations = make(map[string]string)
			}
			for k, v := range annotations {
				existing.Annotations[k] = v
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
// traffic (matching ipBlock 0.0.0.0/0 except podSubnetCIDRs), DNS, and
// monitoring namespace ingress on port 9091 (for Alloy metrics scraping).
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
				// Allow from monitoring namespace (Alloy) to scrape messaging sidecar metrics
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"name": "monitoring",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: protocolPtr(corev1.ProtocolTCP),
							Port:     portPtr(intstr.FromInt32(9091)),
						},
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
	// Add egress rule for Langfuse PrivateLink VPCE IPs (inside pod subnets,
	// so not covered by the external ipBlock rule above).
	for _, ip := range a.langfuseVPCEIPs {
		allowNamespace.Spec.Egress = append(allowNamespace.Spec.Egress, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{IPBlock: &networkingv1.IPBlock{CIDR: ip + "/32"}},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: protocolPtr(corev1.ProtocolTCP),
					Port:     portPtr(intstr.FromInt32(3000)),
				},
			},
		})
	}

	if err := a.applyNetworkPolicy(ctx, allowNamespace); err != nil {
		return fmt.Errorf("allow-namespace-traffic: %w", err)
	}

	return nil
}

// buildKnowledgeService builds a knowledge service exposing all declared endpoints.
func (a *Applier) buildKnowledgeService(
	resourceName, accountName, agentName, buildID, name string, endpoints map[string]spec.Endpoint,
) *corev1.Service {
	labels := deployment.GenerateLabels(accountName, agentName, buildID, fmt.Sprintf("knowledge-%s", name))
	selector := deployment.GenerateSelector(accountName, agentName, fmt.Sprintf("knowledge-%s", name))

	servicePorts := make([]corev1.ServicePort, 0, len(endpoints))
	for epName, ep := range endpoints {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name: epName, Protocol: corev1.ProtocolTCP,
			Port: int32(ep.Port), TargetPort: intstr.FromInt(ep.Port), //nolint:gosec
		})
	}
	sort.Slice(servicePorts, func(i, j int) bool {
		return servicePorts[i].Name < servicePorts[j].Name
	})

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

func protocolPtr(p corev1.Protocol) *corev1.Protocol   { return &p }
func portPtr(p intstr.IntOrString) *intstr.IntOrString { return &p }

// buildMessagingOIDCSecret builds a Kubernetes Secret holding the OIDC client
// credentials for the ALB controller to use with the messaging ingress.
func buildMessagingOIDCSecret(namespace string, cfg *OIDCAuthConfig) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      messagingOIDCSecretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"clientId":     []byte(cfg.ClientID),
			"clientSecret": []byte(cfg.ClientSecret),
		},
	}
}
