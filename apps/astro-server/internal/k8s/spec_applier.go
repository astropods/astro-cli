package k8s

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploytoken"
	spec "github.com/astropods/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
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

	// Guarantee every agent a persistent disk. The template generator defaults
	// the volume for fresh specs, but redeploys load the stored spec verbatim —
	// legacy specs (and any spec not produced by the generator) arrive with no
	// volume. Defaulting here, at the single choke point all deploys pass
	// through, makes the disk universal and keeps orphan cleanup (which keys off
	// Agent.Volume) consistent with what we actually apply.
	normalizeAgentStorageDefaults(ds)

	accountName := ds.Source.Account
	agentName := ds.Source.Name
	buildID := ds.Source.Build

	// Namespace always comes from ApplierConfig (server-owned)

	// Ensure namespace exists (must precede any k8s resource creation).
	if err := a.ensureNamespace(ctx); err != nil {
		return result, fmt.Errorf("failed to ensure namespace: %w", err)
	}

	// Phase 0: Ensure knowledge credential Secrets exist.
	// These are created once and reused across redeployments so passwords stay stable.
	// Must run before ResolveDeploymentSpecEnv so credential values are available
	// for ${knowledge.*.credentials.*} reference resolution.
	credResult := a.ensureKnowledgeCredentialSecrets(ctx, ds, accountName, agentName, buildID)
	knowledgeCredSecrets := credResult.SecretNames

	// Merge self-hosted credentials into bound credentials for unified resolution.
	allCredentials := a.boundCredentials
	if len(credResult.Credentials) > 0 {
		if allCredentials == nil {
			allCredentials = make(map[string]string, len(credResult.Credentials))
		}
		maps.Copy(allCredentials, credResult.Credentials)
	}

	// Surface the merged credential set on the result so the caller can
	// run deployment.Resolve over the spec and persist deployment_build_env
	// rows without re-deriving credentials.
	if len(allCredentials) > 0 {
		result.AllCredentials = make(map[string]string, len(allCredentials))
		maps.Copy(result.AllCredentials, allCredentials)
	}

	// Resolve the agent's externally reachable hostname (if any) before
	// building env so ASTRO_EXTERNAL_AGENT_URL can be injected when the
	// agent has a frontend ingress. Same logic as the ingress block below.
	externalAgentHost := a.resolveAgentIngressHost(ds, agentName)

	// Resolve all ${} references and build ConfigMap/Secret data
	rctx := deployment.ResolveContext{
		Namespace:         a.namespace,
		AgentName:         agentName,
		BuildID:           buildID,
		SecretName:        deployment.GenerateSecretName(agentName, buildID),
		BoundKnowledge:    a.boundKnowledge,
		BoundCredentials:  allCredentials,
		ExternalAgentHost: externalAgentHost,
		DeploymentID:      a.deploymentID,
	}
	resolved := deployment.ResolveDeploymentSpecEnv(ds, rctx)

	// AI Gateway: when the spec opts in (agent.astro_ai_gateway: true) the deployer
	// has minted a per-account virtual key and threaded it through
	// ApplierConfig. Inject the singular env-var pair into the agent's Secret.
	// The gateway routes to whichever model the agent picks at call time —
	// no per-model fanout, no whitelist, no provider entries in the spec.
	if ds.Agent.AIGateway && a.astroGatewayAPIKey != "" {
		resolved.SecretData["ASTRO_GATEWAY_URL"] = a.astroGatewayBaseURL
		resolved.SecretData["ASTRO_GATEWAY_API_KEY"] = a.astroGatewayAPIKey
	}

	// Filter out interface-only entries before building the agent's
	// shared CM+Secret. Variables targeting interface.* (e.g.
	// SLACK_BOT_TOKEN with Targets=["interface.slack"]) are consumed by
	// the messaging container's own scoped Secret further down; they
	// must not leak into the agent's bundle. The messaging carve-out
	// reads from the unfiltered `resolved.SecretData` directly, so this
	// filter only narrows the agent's view.
	agentSecData, agentCMData := scopeAgentEnv(ds, resolved)

	// Only generate resource names when there is data to back them;
	// referencing a non-existent Secret/ConfigMap causes K8s errors.
	secretName := ""
	if hasNonEmpty(agentSecData) {
		secretName = deployment.GenerateSecretName(agentName, buildID)
	}
	configMapName := ""
	if len(agentCMData) > 0 {
		configMapName = deployment.GenerateConfigMapName(agentName, buildID)
	}

	// Phase 1: Create Secret (credentials)
	if hasNonEmpty(agentSecData) {
		secret := BuildSecret(a.namespace, accountName, agentName, buildID, agentSecData)
		status, err := a.applySecret(ctx, secret)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: secret.Name, Kind: "Secret", Error: err.Error(),
			})
		}
	}

	// Phase 2: Create ConfigMap (connection strings + resolved env)
	if len(agentCMData) > 0 {
		configMap := BuildConfigMap(a.namespace, accountName, agentName, buildID, agentCMData)
		status, err := a.applyConfigMap(ctx, configMap)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: configMap.Name, Kind: "ConfigMap", Error: err.Error(),
			})
		}
	}

	// Compute a content hash of ConfigMap + Secret data. When injected as a pod
	// template annotation, it forces a rolling restart when only env vars change
	// (k8s does not restart pods for ConfigMap/Secret content changes alone).
	//
	// Hash the *filtered* maps the agent actually mounts, not the unfiltered
	// resolved set. This way rotating an interface-only secret (e.g.
	// SLACK_BOT_TOKEN) doesn't restart the agent pod — only the messaging
	// container's own scoped Secret changed.
	envHash := hashFilteredEnvData(agentCMData, agentSecData)

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
		if knowledge.IsBound() {
			continue
		}
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
		host := externalAgentHost
		if host != "" {
			ingress := BuildIngress(IngressConfig{
				Name: ingressName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
				BuildID: buildID, Component: "agent",
				ServiceName: agentResourceName, ServicePort: int32(ep.Port), //nolint:gosec
				Host: host,
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
		if knowledge.IsBound() || !knowledge.Persistent {
			continue
		}
		resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)
		port := primaryPort(knowledge.Endpoints)

		container := spec.ContainerConfig{Image: knowledge.Image, Port: int(port), Volume: knowledge.Volume, Environment: knowledge.Environment}
		resolvedContainer, err := a.resolveContainerImage(ctx, container)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: resourceName, Kind: "StatefulSet",
				Error: fmt.Sprintf("failed to resolve image: %v", err),
			})
			continue
		}

		storageSize, storageClass, accessMode := pvcConfigFromSpec(knowledge.Storage)

		// Use knowledge-specific credential secret if available.
		knowledgeSecretName := knowledgeCredSecretName(agentName, name)
		ssSecretName := secretName
		if slices.Contains(knowledgeCredSecrets, knowledgeSecretName) {
			ssSecretName = knowledgeSecretName
		}

		ssCfg := StatefulSetConfig{
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("knowledge-%s", name),
			Container: resolvedContainer, Port: port,
			SecretName:  ssSecretName,
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
		resolvedContainer, err := a.resolveContainerImage(ctx, container)
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
		resolvedContainer, err := a.resolveContainerImage(ctx, container)
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
		if knowledge.IsBound() || knowledge.Persistent {
			continue
		}
		resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)
		port := primaryPort(knowledge.Endpoints)

		container := spec.ContainerConfig{Image: knowledge.Image, Port: int(port), Environment: knowledge.Environment}
		resolvedContainer, err := a.resolveContainerImage(ctx, container)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: resourceName, Kind: "Deployment",
				Error: fmt.Sprintf("failed to resolve image: %v", err),
			})
			continue
		}

		// Use knowledge-specific credential secret if available.
		knowledgeSecretName := knowledgeCredSecretName(agentName, name)
		deplSecretName := secretName
		if slices.Contains(knowledgeCredSecrets, knowledgeSecretName) {
			deplSecretName = knowledgeSecretName
		}

		cfg := DeploymentConfig{
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
			BuildID: buildID, Component: fmt.Sprintf("knowledge-%s", name),
			Container: resolvedContainer, Port: port,
			SecretName: deplSecretName,
			Provider:   knowledge.Provider, ProviderSection: "knowledge",
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
		resolvedContainer, err := a.resolveContainerImage(ctx, container)
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

	// Sign the deploy token once for the deployment and inject it into both
	// the agent and the messaging sidecar as ASTRO_AUTHZ_TOKEN. Agent code
	// uses it to authenticate calls back to astro-server (e.g. the authorize
	// endpoint, future agent-side server APIs); messaging uses it for the
	// per-request authorization callback. The anyone_adapters claim is
	// derived from the spec's grants so the token agrees with what was
	// persisted in this same deploy.
	var anyoneAdapters []string
	hasGrants := false
	if ds.Interfaces != nil && ds.Interfaces.Auth != nil {
		if ds.Interfaces.Auth.Web != nil {
			hasGrants = hasGrants || len(ds.Interfaces.Auth.Web.Grants) > 0
			for _, g := range ds.Interfaces.Auth.Web.Grants {
				if g.Anyone {
					anyoneAdapters = append(anyoneAdapters, "web")
					break
				}
			}
		}
		if ds.Interfaces.Auth.Slack != nil {
			hasGrants = hasGrants || len(ds.Interfaces.Auth.Slack.Grants) > 0
			for _, g := range ds.Interfaces.Auth.Slack.Grants {
				if g.Anyone {
					anyoneAdapters = append(anyoneAdapters, "slack")
					break
				}
			}
		}
	}
	var deployToken string
	switch {
	case a.deployTokenSecret == "" && hasGrants:
		// The spec asks the messaging container to enforce specific grants,
		// but we have nothing to sign the token with. Without the token the
		// container falls back to AllowAll() and the grants are silently
		// ignored. Refuse rather than fail open.
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: agentName,
			Kind:     "Deployment",
			Error:    "DEPLOY_TOKEN_SECRET is unset but interfaces.auth grants are configured; refusing to apply because the messaging container would fall back to AllowAll() without a signed deploy token",
		})
		return result, nil
	case a.deployTokenSecret != "":
		signed, err := deploytoken.Sign(a.deploymentID, a.authzCallbackURL, anyoneAdapters, a.deployTokenSecret)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: agentName,
				Kind:     "Deployment",
				Error:    fmt.Sprintf("sign deploy token: %v", err),
			})
			return result, nil
		}
		deployToken = signed
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

		// Resolve web port from "http" endpoint, pushing it off any port
		// it would collide with on the messaging pod (agent app port —
		// same pod) or container (grpc port — same container). +10 each
		// shift, retrying until no collision.
		webPort := int32(0)
		if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "http"); ep != nil {
			webPort = int32(ep.Port) // nolint:gosec
		}
		if webPort == 0 {
			webPort = 8090
		}
		for webPort == agentPort || webPort == grpcPort {
			webPort = webPort + 10
		}

		// Resolve interface resources from deployment spec
		var msgResources *corev1.ResourceRequirements
		if r := BuildResourceRequirements(ds.Interfaces.Resources); r != nil {
			msgResources = r
		}

		// Resolve interface environment — collect only entries whose keys are
		// defined in interfaces.environment. Non-secret values become inline
		// env vars; secret values land in a messaging-only Secret below so we
		// don't leak the agent's full credentials bundle into the sidecar.
		// Skip empty/unresolved secret values so stripped specs (no real values
		// rehydrated yet) don't produce an empty Secret resource.
		resolvedIfaceEnv := make(map[string]string)
		messagingSecretData := make(map[string]string)
		for key := range ds.Interfaces.Environment {
			if val, ok := resolved.ConfigMapData[key]; ok {
				resolvedIfaceEnv[key] = val
				continue
			}
			if val, ok := resolved.SecretData[key]; ok && val != "" && !spec.IsReference(val) {
				messagingSecretData[key] = val
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

		// Apply the messaging-only Secret when the spec references any secret
		// values from interfaces.environment. The messaging sidecar mounts only
		// this narrower Secret — never the agent's full credentials bundle.
		messagingSecretName := ""
		if len(messagingSecretData) > 0 {
			messagingSecretName = deployment.GenerateMessagingSecretName(agentName, buildID)
			msgSecret := BuildNamedSecret(a.namespace, messagingSecretName, accountName, agentName, buildID, "messaging-variables", messagingSecretData)
			status, err := a.applySecret(ctx, msgSecret)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: msgSecret.Name, Kind: "Secret", Error: err.Error(),
				})
			}
		}

		msgSidecar = &MessagingDeploymentConfig{
			Name: resourceName, Namespace: a.namespace, AgentName: agentName,
			BuildID: buildID, DeploymentID: a.deploymentID, Component: "messaging",
			Image: msgImage, Port: grpcPort, SecretName: messagingSecretName,
			ConfigMapName: "",
			SlackEnabled:  slackEnabled, WebEnabled: webEnabled,
			WebPort:         webPort,
			ImagePullPolicy: a.imagePullPolicy,
			Resources:       msgResources,
			Environment:     resolvedIfaceEnv,
			DeployToken:     deployToken,
			AuthTestUserID:  a.authTestUserID,
		}

		// Share the agent's persistent disk with the messaging sidecar. Every
		// agent runs as a StatefulSet with the "data" volume, so the sidecar can
		// always mount it — under its own subPath so its files never collide
		// with the agent's.
		msgSidecar.VolumeName = agentDataVolumeName
		msgSidecar.VolumeMountPath = spec.DefaultAgentVolumeMount
		msgSidecar.VolumeSubPath = messagingVolumeSubPath

		// Service — selects the agent pod (messaging is a sidecar container).
		// In local mode we promote it to NodePort on the http port so the
		// Launch button can reach the messaging UI at http://localhost:<port>
		// without an ingress.
		svcType := corev1.ServiceTypeClusterIP
		if a.localMode && webEnabled {
			svcType = corev1.ServiceTypeNodePort
		}
		msgSvc := BuildService(ServiceConfig{
			Name: resourceName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
			BuildID: buildID, Component: "agent",
			Port: grpcPort, ServiceType: svcType,
		})
		msgSvc.Spec.Ports[0].Name = "grpc"
		// The messaging sidecar runs inside the agent pod. By default, Services
		// only route to Ready endpoints — but the app container won't become
		// ready until it can reach the messaging Service, creating a circular
		// readiness deadlock. Publishing not-ready addresses breaks the cycle.
		msgSvc.Spec.PublishNotReadyAddresses = true
		if webEnabled {
			httpPort := corev1.ServicePort{
				Name: "http", Protocol: corev1.ProtocolTCP,
				Port: webPort, TargetPort: intstr.FromInt(int(webPort)),
			}
			// In local mode we let Kubernetes auto-allocate the NodePort from
			// the default 30000–32767 range. Pinning it would cap concurrent
			// local deployments at one (second apply gets a NodePort collision).
			msgSvc.Spec.Ports = append(msgSvc.Spec.Ports, httpPort)
		}
		a.applyServiceAndRecord(ctx, msgSvc, result)

		// Ingress — expose web adapter if configured.
		// In local mode there's no ingress controller; instead the NodePort
		// above surfaces the messaging UI on the host. We re-read the Service
		// to learn which port kube-proxy assigned, then record it as the
		// deployment's external URL so the Launch button works per-deployment.
		if webEnabled && a.localMode {
			if host, port := a.resolveLocalMessagingHost(ctx, msgSvc.Name); host != "" {
				result.ServiceEndpoints = append(result.ServiceEndpoints, deployment.ServiceEndpoint{
					Name: "messaging", Type: "web",
					URL:  "http://" + host,
					Port: port,
				})
				if a.persistMessagingHost != nil {
					if perr := a.persistMessagingHost(a.deploymentID, host); perr != nil {
						result.Errors = append(result.Errors, deployment.DeploymentError{
							Resource: msgSvc.Name, Kind: "Service",
							Error: fmt.Sprintf("persist messaging host: %v", perr),
						})
					}
				}
			}
		}
		if webEnabled && !a.localMode {
			ingressName := deployment.GenerateAgentResourceName(agentName, "ingress-messaging")
			host := ""
			if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "http"); ep != nil && ep.Expose != nil {
				host = ep.Expose.Domain
			}
			if host == "" {
				if domain := a.webIngressDomain(ds.Interfaces.WebPublic()); domain != "" {
					host = GenerateMessagingIngressHost(agentName, a.namespace, domain)
				}
			}
			if host != "" {
				// OIDC is enforced at the front-door ALB listener rule
				// (host=*.agents.<domain>); the per-tenant messaging-oidc
				// Secret is no longer used. See astro-infra
				// docs/plans/tenant-router-migration.md.
				ingress := BuildIngress(IngressConfig{
					Name: ingressName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
					BuildID: buildID, Component: "messaging",
					ServiceName: resourceName, ServicePort: webPort, Host: host,
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

	// Main agent workload — messaging is colocated as a sidecar container.
	// When the agent declares a persistent volume we run it as a StatefulSet
	// with a PVC; otherwise a stateless Deployment.
	a.applyAgentWorkload(ctx, agentWorkloadInput{
		ds: ds, agentName: agentName, accountName: accountName, buildID: buildID,
		resourceName: agentResourceName, port: agentPort, deployToken: deployToken,
		secretName: secretName, configMapName: configMapName, envHash: envHash,
		knowledgeCredSecrets: knowledgeCredSecrets,
		msgSidecar:           msgSidecar,
	}, result)

	// Phase 7: CronJobs/Jobs for ingestion
	//
	// Ingestion containers need the same per-store knowledge credentials
	// the agent does. With template.go's ${knowledge.x.credentials.y}
	// auto-injection removed, those values no longer flow through the
	// agent's full Secret (which ingestion used to envFrom). Instead we
	// pass the same secretKeyRef entries knowledgeCredEnvVars produces
	// for the agent — the K8s objects are shared between roles.
	ingestionExtraEnv := knowledgeCredEnvVars(ds, agentName, knowledgeCredSecrets)

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
					ExtraEnv:        ingestionExtraEnv,
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
				ExtraEnv:        ingestionExtraEnv,
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
				ExtraEnv:        ingestionExtraEnv,
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

// pvcConfigFromSpec unpacks a spec.StorageConfig into the size/class/access-mode
// triple BuildStatefulSet expects. Defaults match DefaultStorageConfig().
// normalizeAgentStorageDefaults ensures the agent has a persistent volume.
// When no mount path is set it applies the platform defaults (mount path +
// modest storage size); an explicitly requested volume or storage config is
// left untouched. Idempotent — re-applying a defaulted spec is a no-op.
func normalizeAgentStorageDefaults(ds *spec.AstroDeploymentSpec) {
	if ds.Agent.Volume != "" {
		return
	}
	ds.Agent.Volume = spec.DefaultAgentVolumeMount
	if ds.Agent.Storage == nil {
		s := spec.DefaultStorageConfig()
		s.Size = spec.DefaultAgentStorageSize
		ds.Agent.Storage = &s
	}
}

func pvcConfigFromSpec(s *spec.StorageConfig) (size, class string, mode corev1.PersistentVolumeAccessMode) {
	size, mode = "10Gi", corev1.ReadWriteOnce
	if s == nil {
		return
	}
	if s.Size != "" {
		size = s.Size
	}
	class = s.Class
	if s.AccessMode == "ReadWriteMany" {
		mode = corev1.ReadWriteMany
	}
	return
}

// agentWorkloadInput collects the cluster-side inputs needed to apply the
// agent's Deployment or StatefulSet. Pulled out so the branch in
// ApplyDeploymentSpec stays readable.
type agentWorkloadInput struct {
	ds                                              *spec.AstroDeploymentSpec
	agentName, accountName, buildID, resourceName   string
	port                                            int32
	deployToken, secretName, configMapName, envHash string
	knowledgeCredSecrets                            []string
	msgSidecar                                      *MessagingDeploymentConfig
}

// applyAgentWorkload resolves the agent image, builds its extra env, and
// applies either a StatefulSet (when ds.Agent.Volume is set) or a Deployment.
// Errors land on result.Errors so callers don't have to thread them back.
func (a *Applier) applyAgentWorkload(ctx context.Context, in agentWorkloadInput, result *ApplyResult) {
	agentContainer := spec.ContainerConfig{Image: in.ds.Agent.Image, Volume: in.ds.Agent.Volume}
	resolvedContainer, err := a.resolveContainerImage(ctx, agentContainer)
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: in.resourceName, Kind: "StatefulSet",
			Error: fmt.Sprintf("failed to resolve image: %v", err),
		})
		return
	}

	var extraEnv []corev1.EnvVar
	if in.deployToken != "" {
		extraEnv = append(extraEnv, corev1.EnvVar{Name: "ASTRO_AUTHZ_TOKEN", Value: in.deployToken})
	}
	// Wire knowledge-store credentials (USER/PASSWORD) onto the agent using
	// per-key secretKeyRef rather than envFrom: envFrom mounts every key in a
	// Secret, so two same-provider stores sharing literal keys (e.g.
	// POSTGRES_USER in both postgres-creds and users-creds) collide silently.
	// Per-key refs let us rename per store (POSTGRES_USER + POSTGRES_USERS_USER)
	// per RFC §8.2 and avoid the collision entirely.
	extraEnv = append(extraEnv, knowledgeCredEnvVars(in.ds, in.agentName, in.knowledgeCredSecrets)...)

	// Every agent runs as a StatefulSet with a PVC. normalizeAgentStorageDefaults
	// (in ApplyDeploymentSpec) guarantees a volume, so there is no stateless
	// Deployment path for agents — the disk is always present.
	size, class, mode := pvcConfigFromSpec(in.ds.Agent.Storage)
	ss, ssErr := BuildStatefulSet(StatefulSetConfig{
		Name: in.resourceName, Namespace: a.namespace, AccountID: in.accountName, AgentName: in.agentName,
		BuildID: in.buildID, Component: "agent",
		Container: resolvedContainer, Port: in.port,
		SecretName: in.secretName, ConfigMapName: in.configMapName,
		StorageSize: size, StorageClass: class, AccessMode: mode,
		Healthcheck:     in.ds.Agent.Healthcheck,
		ImagePullPolicy: a.imagePullPolicy,
		Replicas:        int32(in.ds.Agent.Replicas), //nolint:gosec
		Resources:       BuildResourceRequirements(in.ds.Agent.Resources),
		Strategy:        BuildStatefulSetUpdateStrategy(in.ds.Agent.Update),
		LocalMode:       a.localMode,
		EnvHash:         in.envHash,
		ExtraEnv:        extraEnv,
		Messaging:       in.msgSidecar,
	})
	if ssErr != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: in.resourceName, Kind: "StatefulSet", Error: ssErr.Error(),
		})
		return
	}
	// Retain the PVC when the StatefulSet is deleted so persistent data (e.g.
	// messaging history) survives a redeploy that recreates the StatefulSet.
	// Undeploy still removes the disk: the deleter explicitly deletes PVCs and
	// then the namespace (which cascades), independent of this policy — see
	// deleter.go. Scaled-down replicas delete their PVCs to avoid orphan disks.
	ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
		WhenScaled:  appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
	}
	status, applyErr := a.applyStatefulSet(ctx, ss)
	result.Resources = append(result.Resources, status)
	if applyErr != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: ss.Name, Kind: "StatefulSet", Error: applyErr.Error(),
		})
	}
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
// Policy 3 (allow-apiserver-proxy, conditional): when cpSubnetCIDRs is set,
// a sibling NP scoped to messaging sidecar pods (component=agent) allows
// service-proxy ingress on TCP 8090 only from apiserver ENI CIDRs.
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

	ingressRules := []networkingv1.NetworkPolicyIngressRule{
		{
			From: []networkingv1.NetworkPolicyPeer{
				{PodSelector: &metav1.LabelSelector{}},
			},
		},
		{
			From: []networkingv1.NetworkPolicyPeer{
				{IPBlock: &externalIPBlock},
			},
		},
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
				{
					Protocol: protocolPtr(corev1.ProtocolTCP),
					Port:     portPtr(intstr.FromInt32(4317)),
				},
				{
					Protocol: protocolPtr(corev1.ProtocolTCP),
					Port:     portPtr(intstr.FromInt32(4318)),
				},
			},
		},
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
			Ingress:     ingressRules,
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

	// Policy 3 (conditional): allow-apiserver-proxy. Scoped to messaging sidecar
	// pods only. Carves back the kubectl-proxy / services/proxy path that
	// allow-namespace-traffic excepts via its podSubnetCIDRs except list.
	if proxyNP := apiserverProxyNetworkPolicy(a.namespace, a.cpSubnetCIDRs); proxyNP != nil {
		if err := a.applyNetworkPolicy(ctx, proxyNP); err != nil {
			return fmt.Errorf("allow-apiserver-proxy: %w", err)
		}
	}

	// Policy 4: allow-from-tenant-router. The Contour Envoy fleet lives in
	// the projectcontour namespace (its pod IPs fall inside podSubnetCIDRs,
	// which allow-namespace-traffic above explicitly excludes). This NP
	// carves back ingress from projectcontour so the front-door ALB can
	// route tenant traffic via Contour. Belt-and-braces with the Kyverno
	// `generate-allow-from-tenant-router-np` policy in astro-infra, which
	// covers namespaces created outside astro-server. See astro-infra
	// docs/plans/tenant-router-migration.md.
	allowFromTenantRouter := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-from-tenant-router",
			Namespace: a.namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "projectcontour",
								},
							},
						},
					},
				},
			},
		},
	}
	if err := a.applyNetworkPolicy(ctx, allowFromTenantRouter); err != nil {
		return fmt.Errorf("allow-from-tenant-router: %w", err)
	}

	return nil
}

// apiserverProxyNetworkPolicy builds the sibling NetworkPolicy that allows the
// EKS apiserver to reach messaging sidecars via services/proxy. Returns nil
// when cpSubnetCIDRs is empty (local dev, clusters without netpol isolation),
// so callers skip the apply.
//
// Shape is intentionally narrow: podSelector restricts the destination to
// messaging sidecar pods (component=agent), source ipBlocks restrict to
// apiserver ENI subnets, and ports restrict to 8090 (messaging HTTP — the
// service-proxy path in-client chat uses). The gRPC 9090 surface is not
// exposed: nothing reaches it via apiserver-proxy today.
func apiserverProxyNetworkPolicy(namespace string, cpSubnetCIDRs []string) *networkingv1.NetworkPolicy {
	if len(cpSubnetCIDRs) == 0 {
		return nil
	}
	from := make([]networkingv1.NetworkPolicyPeer, 0, len(cpSubnetCIDRs))
	for _, cidr := range cpSubnetCIDRs {
		from = append(from, networkingv1.NetworkPolicyPeer{
			IPBlock: &networkingv1.IPBlock{CIDR: cidr},
		})
	}
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-apiserver-proxy",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": "agent",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: from,
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: protocolPtr(corev1.ProtocolTCP),
							Port:     portPtr(intstr.FromInt32(8090)),
						},
					},
				},
			},
		},
	}
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

// resolveAgentIngressHost returns the public hostname the agent's frontend
// ingress will be created on, or "" if the agent has no exposed endpoint or
// no host can be determined. Mirrors the host-selection used when building
// the ingress so callers can pre-compute the URL for env injection.
func (a *Applier) resolveAgentIngressHost(ds *spec.AstroDeploymentSpec, agentName string) string {
	ep := spec.ExposedEndpoint(ds.Agent.Endpoints)
	if ep == nil {
		return ""
	}
	if ep.Expose != nil && ep.Expose.Domain != "" {
		return ep.Expose.Domain
	}
	if domain := a.webIngressDomain(ds.Interfaces.CustomPublic()); domain != "" {
		return GenerateIngressHost(agentName, a.namespace, domain)
	}
	return ""
}

// webIngressDomain selects the parent ingress domain for a browser-facing web
// surface: the open (no-OIDC) cohort when public, else the authenticated agent
// domain. Hosts in the public cohort fall through the front-door ALB's OIDC rule.
func (a *Applier) webIngressDomain(public bool) string {
	if public {
		return a.agentPublicIngressDomain
	}
	return a.ingressDomain
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

// resolveLocalMessagingHost re-reads the messaging Service after apply to
// learn which NodePort kube-proxy assigned (we let k8s auto-allocate so
// multiple local deployments don't collide on a hardcoded port). Returns
// the host:port the Launch URL should point at and the raw port. Returns
// ("", 0) if the lookup fails or no NodePort was assigned — caller treats
// that as "no Launch URL yet".
func (a *Applier) resolveLocalMessagingHost(ctx context.Context, svcName string) (string, int32) {
	svc, err := a.clientset.CoreV1().Services(a.namespace).Get(ctx, svcName, metav1.GetOptions{})
	if err != nil {
		return "", 0
	}
	for _, p := range svc.Spec.Ports {
		if p.Name == "http" && p.NodePort != 0 {
			return fmt.Sprintf("localhost:%d", p.NodePort), p.NodePort
		}
	}
	return "", 0
}

func protocolPtr(p corev1.Protocol) *corev1.Protocol   { return &p }
func portPtr(p intstr.IntOrString) *intstr.IntOrString { return &p }

// knowledgeCredSecretName returns the name of the K8s Secret that holds
// auto-generated credentials for a self-hosted knowledge store.
func knowledgeCredSecretName(agentName, knowledgeName string) string {
	return deployment.GenerateResourceName(agentName, "knowledge", knowledgeName) + "-creds"
}

// knowledgeCredKey is one literal-key entry on a knowledge cred Secret that
// the agent should pick up under a (potentially renamed) env var name.
type knowledgeCredKey struct {
	suffix    string // e.g. "USER", "PASSWORD" — also the literal key in the Secret
	secretKey string // the literal key in the K8s Secret (same as suffix prefixed by EnvPrefix today)
}

// providerCredKeys returns the cred suffixes that a provider's auto-generated
// Secret contains, paired with the literal key names used inside the Secret.
// It mirrors generateKnowledgeCredentials' contract.
func providerCredKeys(provider string) []knowledgeCredKey {
	switch provider {
	case "postgres":
		return []knowledgeCredKey{
			{suffix: "USER", secretKey: "POSTGRES_USER"},         //nolint:gosec // env var key, not a credential
			{suffix: "PASSWORD", secretKey: "POSTGRES_PASSWORD"}, //nolint:gosec // env var key, not a credential
			// DB rounds out the BindCredentials triple. With the
			// template.go auto-injection of credential refs removed,
			// knowledgeCredEnvVars is the only path that puts the
			// database name on the agent + ingestion containers.
			{suffix: "DB", secretKey: "POSTGRES_DB"}, //nolint:gosec // env var key, not a credential
		}
	case "redis":
		return []knowledgeCredKey{
			{suffix: "PASSWORD", secretKey: "REDIS_PASSWORD"}, //nolint:gosec // env var key, not a credential
		}
	}
	return nil
}

// knowledgeCredEnvVars builds the agent container's env-var list for
// knowledge-store credentials, using secretKeyRef so each (store, key) pair
// maps to a unique, RFC §8.2-named env var (POSTGRES_USER vs
// POSTGRES_USERS_USER) regardless of how many stores share a provider. The
// knowledge container itself still mounts its cred Secret directly under
// the literal keys it expects (POSTGRES_USER) — those keys are properties of
// the upstream image, not the agent.
//
// existingSecrets is the list of cred Secret names the applier successfully
// created/found; we skip stores whose Secret didn't materialise rather than
// referencing a missing secret which would block pod startup.
func knowledgeCredEnvVars(ds *spec.AstroDeploymentSpec, agentName string, existingSecrets []string) []corev1.EnvVar {
	if len(ds.Knowledge) == 0 {
		return nil
	}
	have := make(map[string]bool, len(existingSecrets))
	for _, s := range existingSecrets {
		have[s] = true
	}

	// Group entries by provider EnvPrefix and pick a primary per group.
	type entry struct {
		name     string
		provider string
		prefix   string
	}
	groups := map[string][]entry{}
	names := make([]string, 0, len(ds.Knowledge))
	for n := range ds.Knowledge {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		k := ds.Knowledge[n]
		if k.Provider == "" {
			continue
		}
		prov := spec.GetProvider(k.Provider)
		if prov.EnvPrefix == "" {
			continue
		}
		groups[prov.EnvPrefix] = append(groups[prov.EnvPrefix], entry{name: n, provider: k.Provider, prefix: prov.EnvPrefix})
	}

	var out []corev1.EnvVar
	for prefix, group := range groups {
		// Pick primary per RFC §8.2: name == provider, else first alphabetically.
		groupNames := make([]string, 0, len(group))
		for _, e := range group {
			groupNames = append(groupNames, e.name)
		}
		primary := deployment.PickPrimaryName(append([]string(nil), groupNames...), group[0].provider)

		isDup := len(group) > 1
		for _, e := range group {
			secretName := knowledgeCredSecretName(agentName, e.name)
			if !have[secretName] {
				continue
			}
			isPrimary := e.name == primary
			for _, ck := range providerCredKeys(e.provider) {
				for _, envName := range deployment.ProviderEnvKeys(prefix, e.name, e.provider, ck.suffix, isDup, isPrimary) {
					out = append(out, corev1.EnvVar{
						Name: envName,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
								Key:                  ck.secretKey,
							},
						},
					})
				}
			}
		}
	}
	// Stable order for deterministic test assertions and diff cleanliness.
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// generateKnowledgeCredentials returns auto-generated credentials for a provider.
// agentName is used to derive the database name for postgres.
func generateKnowledgeCredentials(provider, agentName string) map[string][]byte {
	switch provider {
	case "postgres":
		return map[string][]byte{
			"POSTGRES_USER":     []byte("astro"),
			"POSTGRES_PASSWORD": []byte(randomCredHex(16)),
			"POSTGRES_DB":       []byte(spec.SanitizeDBName(agentName)),
		}
	case "redis":
		return map[string][]byte{
			"REDIS_PASSWORD": []byte(randomCredHex(16)),
		}
	default:
		return nil
	}
}

func randomCredHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// knowledgeCredResult holds the outputs of ensureKnowledgeCredentialSecrets.
type knowledgeCredResult struct {
	SecretNames []string          // k8s Secret names (for envFrom on knowledge containers and agent)
	Credentials map[string]string // "entryName.attr" → value (for credential reference resolution)
}

// ensureKnowledgeCredentialSecrets creates credential Secrets for self-hosted
// knowledge stores that need them (postgres, redis). If the Secret already exists
// (from a previous deploy), it is left untouched so credentials remain stable.
// Returns the secret names and a credentials map keyed by "entryName.attr" for
// use in credential reference resolution (${knowledge.*.credentials.*}).
func (a *Applier) ensureKnowledgeCredentialSecrets(
	ctx context.Context,
	ds *spec.AstroDeploymentSpec,
	accountName, agentName, buildID string,
) knowledgeCredResult {
	result := knowledgeCredResult{
		Credentials: make(map[string]string),
	}

	for name, knowledge := range ds.Knowledge {
		if knowledge.IsBound() {
			// Bound/external stores have no auto-generated password, but the
			// agent still needs the store's credentials. Materialise the
			// externally-resolved boundCredentials into a cred Secret so
			// knowledgeCredEnvVars references them via secretKeyRef exactly
			// like a self-hosted store. Without this the agent gets HOST/PORT
			// (from the ConfigMap) but no USER/PASSWORD/DB.
			if secretName, ok := a.ensureBoundCredentialSecret(ctx, ds, name, accountName, agentName, buildID); ok {
				result.SecretNames = append(result.SecretNames, secretName)
			}
			continue
		}
		creds := generateKnowledgeCredentials(knowledge.Provider, agentName)
		if len(creds) == 0 {
			continue
		}

		secretName := knowledgeCredSecretName(agentName, name)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: a.namespace,
				Labels:    deployment.GenerateLabels(accountName, agentName, buildID, "knowledge-creds"),
			},
			Type: corev1.SecretTypeOpaque,
			Data: creds,
		}

		_, err := a.clientset.CoreV1().Secrets(a.namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil && errors.IsAlreadyExists(err) {
			// Secret from a previous deploy — reuse it and read back stable values.
			// Refresh labels to the current build so cleanupStaleBuildResources
			// doesn't delete this secret as stale (the data stays put; only
			// metadata is updated).
			err = nil
			if existing, getErr := a.clientset.CoreV1().Secrets(a.namespace).Get(ctx, secretName, metav1.GetOptions{}); getErr == nil {
				creds = existing.Data
				existing.Labels = secret.Labels
				if _, updateErr := a.clientset.CoreV1().Secrets(a.namespace).Update(ctx, existing, metav1.UpdateOptions{}); updateErr != nil {
					err = updateErr
				}
			}
		}
		if err == nil {
			result.SecretNames = append(result.SecretNames, secretName)
			// Map storage keys to reference attributes for credential ref resolution.
			storageKeyMap := spec.CredentialStorageKeyMap(knowledge.Provider)
			for storageKey, data := range creds {
				if attr, ok := storageKeyMap[storageKey]; ok {
					result.Credentials[name+"."+attr] = string(data)
				}
			}
		}
	}

	return result
}

// ensureBoundCredentialSecret materialises a bound/external store's resolved
// credentials into a k8s Secret, keyed by the provider's literal storage keys
// (e.g. POSTGRES_USER/_PASSWORD/_DB), so knowledgeCredEnvVars can reference them
// via secretKeyRef just like a self-hosted store's Secret. The values come from
// boundCredentials ("name.attr"), populated by the deployer from the external
// store's decrypted credentials.
//
// Unlike self-hosted secrets — whose generated password must stay stable across
// deploys — this Secret is refreshed each deploy so it tracks the current
// external credentials. Returns ("", false) when no bound credentials exist for
// the store (nothing to reference).
func (a *Applier) ensureBoundCredentialSecret(
	ctx context.Context, ds *spec.AstroDeploymentSpec, name, accountName, agentName, buildID string,
) (string, bool) {
	storageKeyMap := spec.CredentialStorageKeyMap(ds.Knowledge[name].Provider) // storageKey -> attr
	if len(storageKeyMap) == 0 {
		return "", false
	}
	data := make(map[string][]byte, len(storageKeyMap))
	for storageKey, attr := range storageKeyMap {
		if v, ok := a.boundCredentials[name+"."+attr]; ok && v != "" {
			data[storageKey] = []byte(v)
		}
	}
	if len(data) == 0 {
		return "", false
	}

	secretName := knowledgeCredSecretName(agentName, name)
	labels := deployment.GenerateLabels(accountName, agentName, buildID, "knowledge-creds")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: a.namespace, Labels: labels},
		Type:       corev1.SecretTypeOpaque,
		Data:       data,
	}

	_, err := a.clientset.CoreV1().Secrets(a.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		// Refresh in place — external credentials may have rotated since the
		// last deploy.
		existing, getErr := a.clientset.CoreV1().Secrets(a.namespace).Get(ctx, secretName, metav1.GetOptions{})
		if getErr != nil {
			return "", false
		}
		existing.Data = data
		existing.Labels = labels
		if _, updErr := a.clientset.CoreV1().Secrets(a.namespace).Update(ctx, existing, metav1.UpdateOptions{}); updErr != nil {
			return "", false
		}
		return secretName, true
	}
	if err != nil {
		return "", false
	}
	return secretName, true
}

// scopeAgentEnv returns the (ConfigMap, Secret) data the agent and ingestion
// containers should mount. It filters out variables whose Targets are
// exclusively interface-scoped (e.g. SLACK_BOT_TOKEN with
// Targets=["interface.slack"]), which are consumed by the messaging
// container's own scoped Secret and must not leak into the agent's bundle.
//
// A variable is interface-only when every entry in its Targets begins with
// "interface.". Variables with mixed targets (e.g. ["agent","ingestion"])
// stay in the agent's view, since the agent legitimately needs them.
//
// Auto-emitted entries (ASTRO_AGENT_*, OTEL_EXPORTER_OTLP_ENDPOINT, etc.)
// are not in ds.Variables; they pass through unfiltered.
func scopeAgentEnv(ds *spec.AstroDeploymentSpec, resolved *deployment.ResolvedEnv) (sec, cm map[string]string) {
	exclude := interfaceOnlyKeys(ds)

	sec = make(map[string]string, len(resolved.SecretData))
	for k, v := range resolved.SecretData {
		if exclude[k] {
			continue
		}
		sec[k] = v
	}
	cm = make(map[string]string, len(resolved.ConfigMapData))
	for k, v := range resolved.ConfigMapData {
		if exclude[k] {
			continue
		}
		cm[k] = v
	}
	return sec, cm
}

// interfaceOnlyKeys returns the set of variable names whose Targets are
// exclusively "interface.*" entries. These belong to the messaging
// container only and must not appear in the agent's mounted Secret.
func interfaceOnlyKeys(ds *spec.AstroDeploymentSpec) map[string]bool {
	out := map[string]bool{}
	for name, v := range ds.Variables {
		if len(v.Targets) == 0 {
			continue
		}
		allInterface := true
		for _, t := range v.Targets {
			if !strings.HasPrefix(t, "interface.") {
				allInterface = false
				break
			}
		}
		if allInterface {
			out[strings.ToUpper(name)] = true
		}
	}
	return out
}

// hasNonEmpty reports whether any value in the map is non-empty and not
// an unresolved ${} reference. Mirrors ResolvedEnv.HasSecretValues so the
// applier can skip Secret creation for stripped/unresolved specs.
func hasNonEmpty(data map[string]string) bool {
	for _, v := range data {
		if v != "" && !spec.IsReference(v) {
			return true
		}
	}
	return false
}

// hashFilteredEnvData hashes the (cmData, secData) the agent + ingestion
// containers will actually mount. Variant of hashEnvData that takes the
// already-filtered maps instead of the full ResolvedEnv, so changes to
// interface-only entries don't trigger an unnecessary agent restart.
func hashFilteredEnvData(cmData, secData map[string]string) string {
	h := sha256.New()
	keys := make([]string, 0, len(cmData)+len(secData))
	for k := range cmData {
		keys = append(keys, "cm:"+k)
	}
	for k := range secData {
		keys = append(keys, "s:"+k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		if strings.HasPrefix(k, "cm:") {
			h.Write([]byte(cmData[strings.TrimPrefix(k, "cm:")]))
		} else {
			h.Write([]byte(secData[strings.TrimPrefix(k, "s:")]))
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
