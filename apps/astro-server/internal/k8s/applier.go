package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// ApplierConfig holds configuration for the Applier
type ApplierConfig struct {
	Namespace         string
	RegistryURL       string
	ProxyRegistryHost string
	ImagePullPolicy   corev1.PullPolicy // Defaults to PullAlways; set PullNever for local dev
	// Ingress configuration
	IngressDomain     string
	ACMCertificateARN string
	ALBGroupName      string
}

// Applier applies Kubernetes manifests to a cluster
type Applier struct {
	clientset       *kubernetes.Clientset
	namespace       string
	registryURL     string
	imageResolver   *ImageResolver
	imagePullPolicy corev1.PullPolicy
	// Ingress configuration
	ingressDomain     string
	acmCertificateARN string
	albGroupName      string
}

// NewApplier creates a new applier
func NewApplier(client ClusterClient, cfg ApplierConfig) *Applier {
	pullPolicy := cfg.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}
	return &Applier{
		clientset:         client.Clientset(),
		namespace:         cfg.Namespace,
		registryURL:       cfg.RegistryURL,
		imageResolver:     NewImageResolver(cfg.ProxyRegistryHost, cfg.RegistryURL),
		imagePullPolicy:   pullPolicy,
		ingressDomain:     cfg.IngressDomain,
		acmCertificateARN: cfg.ACMCertificateARN,
		albGroupName:      cfg.ALBGroupName,
	}
}

// resolveContainerImage resolves a container image reference to its ECR path
func (a *Applier) resolveContainerImage(container spec.ContainerConfig) (spec.ContainerConfig, error) {
	if container.Image == "" {
		return container, nil
	}

	resolvedImage, err := a.imageResolver.ResolveImage(container.Image)
	if err != nil {
		return container, err
	}

	// Create a copy with resolved image
	resolved := container
	resolved.Image = resolvedImage
	return resolved, nil
}

// ApplyResult holds the result of applying manifests
type ApplyResult struct {
	Resources        []deployment.ResourceStatus
	ServiceEndpoints []deployment.ServiceEndpoint
	Errors           []deployment.DeploymentError
}

// Apply applies all manifests to the cluster
func (a *Applier) Apply(
	ctx context.Context,
	astroSpec *spec.AstroSpec,
	translationResult *deployment.TranslationResult,
	agentName string,
	version string,
	userCredentials map[string]string,
) (*ApplyResult, error) {
	result := &ApplyResult{
		Resources:        []deployment.ResourceStatus{},
		ServiceEndpoints: []deployment.ServiceEndpoint{},
		Errors:           []deployment.DeploymentError{},
	}

	// Ensure namespace exists
	if err := a.ensureNamespace(ctx); err != nil {
		return result, fmt.Errorf("failed to ensure namespace: %w", err)
	}

	// Phase 1: Create Secret
	if len(userCredentials) > 0 {
		secret := BuildSecret(a.namespace, agentName, version, userCredentials)
		status, err := a.applySecret(ctx, secret)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: secret.Name,
				Kind:     "Secret",
				Error:    err.Error(),
			})
		}
	}

	// Phase 2: Create ConfigMap
	connectionStrings := deployment.NewEnvBuilder(a.namespace).BuildConnectionStrings(astroSpec)
	if len(connectionStrings) > 0 {
		configMap := BuildConfigMap(a.namespace, agentName, version, connectionStrings)
		status, err := a.applyConfigMap(ctx, configMap)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: configMap.Name,
				Kind:     "ConfigMap",
				Error:    err.Error(),
			})
		}
	}

	secretName := deployment.GenerateCredentialSecretName(agentName, version)
	configMapName := deployment.GenerateConfigMapName(agentName, version)

	// Phase 3: Create Services (needed before Deployments/StatefulSets)
	for name, model := range astroSpec.Models {
		if model.Container.Image != "" || model.Container.Build != nil {
			resourceName := deployment.GenerateResourceName(agentName, "model", name)
			port := int32(model.Container.Port)
			if port == 0 {
				port = 8080
			}

			serviceCfg := ServiceConfig{
				Name:           resourceName,
				Namespace:      a.namespace,
				AgentName:      agentName,
				Version:        version,
				Component:      fmt.Sprintf("model-%s", name),
				Port:           port,
				ServiceType:    corev1.ServiceTypeClusterIP,
			}
			service := BuildService(serviceCfg)
			status, err := a.applyService(ctx, service)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: service.Name,
					Kind:     "Service",
					Error:    err.Error(),
				})
			}
		}
	}

	// Services for knowledge stores
	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.Container.Image != "" || knowledge.Container.Build != nil {
			resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)

			// Determine provider and default port
			provider := knowledge.Provider
			if provider == "" {
				provider = knowledge.Type
			}

			port := int32(knowledge.Container.Port)
			if port == 0 {
				switch strings.ToLower(provider) {
				case "qdrant":
					port = 6333
				case "redis":
					port = 6379
				case "postgres":
					port = 5432
				default:
					port = 6333
				}
			}

			labels := deployment.GenerateLabels(agentName, version, fmt.Sprintf("knowledge-%s", name))
			selector := deployment.GenerateSelector(agentName, fmt.Sprintf("knowledge-%s", name))

			// Build service ports based on provider
			var servicePorts []corev1.ServicePort
			if strings.ToLower(provider) == "qdrant" {
				// Qdrant needs both REST (6333) and gRPC (6334) ports
				servicePorts = []corev1.ServicePort{
					{
						Name:       "rest",
						Protocol:   corev1.ProtocolTCP,
						Port:       6333,
						TargetPort: intstr.FromInt(6333),
					},
					{
						Name:       "grpc",
						Protocol:   corev1.ProtocolTCP,
						Port:       6334,
						TargetPort: intstr.FromInt(6334),
					},
				}
			} else {
				// Other providers use single port
				servicePorts = []corev1.ServicePort{
					{
						Name:       "tcp",
						Protocol:   corev1.ProtocolTCP,
						Port:       port,
						TargetPort: intstr.FromInt(int(port)),
					},
				}
			}

			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: a.namespace,
					Labels:    labels,
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeClusterIP,
					Selector: selector,
					Ports:    servicePorts,
				},
			}

			status, err := a.applyService(ctx, service)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: service.Name,
					Kind:     "Service",
					Error:    err.Error(),
				})
			}
		}
	}

	// Services for tools
	for name, tool := range astroSpec.Tools {
		if tool.Container != nil && (tool.Container.Image != "" || tool.Container.Build != nil) {
			resourceName := deployment.GenerateResourceName(agentName, "tool", name)
			port := int32(tool.Container.Port)
			if port == 0 {
				port = 8080
			}

			serviceCfg := ServiceConfig{
				Name:           resourceName,
				Namespace:      a.namespace,
				AgentName:      agentName,
				Version:        version,
				Component:      fmt.Sprintf("tool-%s", name),
				Port:           port,
				ServiceType:    corev1.ServiceTypeClusterIP,
			}
			service := BuildService(serviceCfg)
			status, err := a.applyService(ctx, service)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: service.Name,
					Kind:     "Service",
					Error:    err.Error(),
				})
			}
		}
	}

	// Agent service
	agentResourceName := deployment.GenerateAgentResourceName(agentName, "agent")
	agentServiceCfg := ServiceConfig{
		Name:           agentResourceName,
		Namespace:      a.namespace,
		AgentName:      agentName,
		Version:        version,
		Component:      "agent",
		Port:           8080,
		ServiceType:    corev1.ServiceTypeClusterIP,
	}
	agentService := BuildService(agentServiceCfg)
	status, err := a.applyService(ctx, agentService)
	result.Resources = append(result.Resources, status)
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: agentService.Name,
			Kind:     "Service",
			Error:    err.Error(),
		})
	}

	// Phase 4: Create StatefulSets for persistent knowledge
	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.Container.Persistent && (knowledge.Container.Image != "" || knowledge.Container.Build != nil) {
			resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)
			port := int32(knowledge.Container.Port)
			if port == 0 {
				// Set default port based on provider
				provider := knowledge.Provider
				if provider == "" {
					provider = knowledge.Type
				}
				switch strings.ToLower(provider) {
				case "qdrant":
					port = 6333
				case "redis":
					port = 6379
				case "postgres":
					port = 5432
				default:
					port = 6333
				}
			}

			// Resolve image to ECR path
			resolvedContainer, err := a.resolveContainerImage(knowledge.Container)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: resourceName,
					Kind:     "StatefulSet",
					Error:    fmt.Sprintf("failed to resolve image: %v", err),
				})
				continue
			}

			statefulSetCfg := StatefulSetConfig{
				Name:            resourceName,
				Namespace:       a.namespace,
				AgentName:       agentName,
				Version:         version,
				Component:       fmt.Sprintf("knowledge-%s", name),
				Container:       resolvedContainer,
				Port:            port,
				SecretName:      secretName,
				ConfigMapName:   configMapName,
				StorageSize:     "10Gi",
				Healthcheck:     knowledge.Container.Healthcheck,
				Provider:        knowledge.Provider,
				ImagePullPolicy: a.imagePullPolicy,
			}
			statefulSet := BuildStatefulSet(statefulSetCfg)
			status, err := a.applyStatefulSet(ctx, statefulSet)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: statefulSet.Name,
					Kind:     "StatefulSet",
					Error:    err.Error(),
				})
			}
		}
	}

	// Phase 5: Create Deployments
	// Models
	for name, model := range astroSpec.Models {
		if model.Container.Image != "" || model.Container.Build != nil {
			resourceName := deployment.GenerateResourceName(agentName, "model", name)
			port := int32(model.Container.Port)
			if port == 0 {
				port = 8080
			}

			// Resolve image to ECR path
			resolvedContainer, err := a.resolveContainerImage(model.Container)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: resourceName,
					Kind:     "Deployment",
					Error:    fmt.Sprintf("failed to resolve image: %v", err),
				})
				continue
			}

			deploymentCfg := DeploymentConfig{
				Name:           resourceName,
				Namespace:      a.namespace,
				AgentName:      agentName,
				Version:        version,
				Component:      fmt.Sprintf("model-%s", name),
				Container:      resolvedContainer,
				Port:           port,
				SecretName:     secretName,
				ConfigMapName:  configMapName,
				Healthcheck:     model.Container.Healthcheck,
				Provider:        model.Provider,
				ImagePullPolicy: a.imagePullPolicy,
			}
			depl := BuildDeployment(deploymentCfg)
			status, err := a.applyDeployment(ctx, depl)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: depl.Name,
					Kind:     "Deployment",
					Error:    err.Error(),
				})
			}
		}
	}

	// Non-persistent knowledge
	for name, knowledge := range astroSpec.Knowledge {
		if !knowledge.Container.Persistent && (knowledge.Container.Image != "" || knowledge.Container.Build != nil) {
			resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)
			port := int32(knowledge.Container.Port)
			if port == 0 {
				// Set default port based on provider
				provider := knowledge.Provider
				if provider == "" {
					provider = knowledge.Type
				}
				switch strings.ToLower(provider) {
				case "qdrant":
					port = 6333
				case "redis":
					port = 6379
				case "postgres":
					port = 5432
				default:
					port = 6333
				}
			}

			// Resolve image to ECR path
			resolvedContainer, err := a.resolveContainerImage(knowledge.Container)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: resourceName,
					Kind:     "Deployment",
					Error:    fmt.Sprintf("failed to resolve image: %v", err),
				})
				continue
			}

			deploymentCfg := DeploymentConfig{
				Name:           resourceName,
				Namespace:      a.namespace,
				AgentName:      agentName,
				Version:        version,
				Component:      fmt.Sprintf("knowledge-%s", name),
				Container:      resolvedContainer,
				Port:           port,
				SecretName:     secretName,
				ConfigMapName:  configMapName,
				Healthcheck:     knowledge.Container.Healthcheck,
				Provider:        knowledge.Provider,
				ImagePullPolicy: a.imagePullPolicy,
			}
			depl := BuildDeployment(deploymentCfg)
			status, err := a.applyDeployment(ctx, depl)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: depl.Name,
					Kind:     "Deployment",
					Error:    err.Error(),
				})
			}
		}
	}

	// Tools
	for name, tool := range astroSpec.Tools {
		if tool.Container != nil && (tool.Container.Image != "" || tool.Container.Build != nil) {
			resourceName := deployment.GenerateResourceName(agentName, "tool", name)
			port := int32(tool.Container.Port)
			if port == 0 {
				port = 8080
			}

			// Resolve image to ECR path
			resolvedContainer, err := a.resolveContainerImage(*tool.Container)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: resourceName,
					Kind:     "Deployment",
					Error:    fmt.Sprintf("failed to resolve image: %v", err),
				})
				continue
			}

			deploymentCfg := DeploymentConfig{
				Name:           resourceName,
				Namespace:      a.namespace,
				AgentName:      agentName,
				Version:        version,
				Component:      fmt.Sprintf("tool-%s", name),
				Container:      resolvedContainer,
				Port:           port,
				SecretName:     secretName,
				ConfigMapName:  configMapName,
				Healthcheck:     tool.Container.Healthcheck,
				Provider:        tool.Type, // Use tool type as provider hint
				ImagePullPolicy: a.imagePullPolicy,
			}
			depl := BuildDeployment(deploymentCfg)
			status, err := a.applyDeployment(ctx, depl)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: depl.Name,
					Kind:     "Deployment",
					Error:    err.Error(),
				})
			}
		}
	}

	// Main agent deployment
	// Resolve agent container image to ECR path
	agentContainer := spec.ContainerConfig{Image: astroSpec.Container.Image}
	resolvedAgentContainer, err := a.resolveContainerImage(agentContainer)
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: agentResourceName,
			Kind:     "Deployment",
			Error:    fmt.Sprintf("failed to resolve image: %v", err),
		})
	} else {
		agentDeploymentCfg := DeploymentConfig{
			Name:           agentResourceName,
			Namespace:      a.namespace,
			AgentName:      agentName,
			Version:        version,
			Component:      "agent",
			Container:      resolvedAgentContainer,
			Port:           8080,
			SecretName:     secretName,
			ConfigMapName:  configMapName,
			Healthcheck:     astroSpec.Container.Healthcheck,
			Provider:        "", // Agent uses HTTP health check if defined
			ImagePullPolicy: a.imagePullPolicy,
		}
		agentDeployment := BuildDeployment(agentDeploymentCfg)
		status, err = a.applyDeployment(ctx, agentDeployment)
		result.Resources = append(result.Resources, status)
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: agentDeployment.Name,
				Kind:     "Deployment",
				Error:    err.Error(),
			})
		}
	}

	// Messaging interfaces (Slack, Web)
	for name, iface := range astroSpec.Interfaces {
		interfaceType := iface.Type
		if interfaceType == "slack" || interfaceType == "web" {
			resourceName := deployment.GenerateResourceName(agentName, "messaging", name)

			// Web adapter is enabled only for interfaces with type "web"
			webEnabled := interfaceType == "web"

			// Create service (gRPC on port 9090, and HTTP on 8080 if web enabled)
			messagingServiceCfg := ServiceConfig{
				Name:           resourceName,
				Namespace:      a.namespace,
				AgentName:      agentName,
				Version:        version,
				Component:      fmt.Sprintf("messaging-%s", name),
				Port:           9090,
				ServiceType:    corev1.ServiceTypeClusterIP,
			}
			messagingService := BuildService(messagingServiceCfg)
			// Rename the default port from "http" to "grpc" for messaging services
			messagingService.Spec.Ports[0].Name = "grpc"

			// Add HTTP port if web is enabled
			if webEnabled {
				messagingService.Spec.Ports = append(messagingService.Spec.Ports, corev1.ServicePort{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
				})
			}

			status, err := a.applyService(ctx, messagingService)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: messagingService.Name,
					Kind:     "Service",
					Error:    err.Error(),
				})
			}

			// Create deployment
			messagingDeploymentCfg := MessagingDeploymentConfig{
				Name:            resourceName,
				Namespace:       a.namespace,
				AgentName:       agentName,
				Version:         version,
				Component:       fmt.Sprintf("messaging-%s", name),
				Image:           fmt.Sprintf("%s/prod-astro-messaging:latest", a.registryURL),
				Port:            9090,
				SecretName:      secretName,
				InterfaceType:   interfaceType,
				WebEnabled:      webEnabled,
				ImagePullPolicy: a.imagePullPolicy,
			}
			messagingDepl := BuildMessagingDeployment(messagingDeploymentCfg)
			status, err = a.applyDeployment(ctx, messagingDepl)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: messagingDepl.Name,
					Kind:     "Deployment",
					Error:    err.Error(),
				})
			}

			// Create Ingress for external access (only if web is enabled and ingress domain is configured)
			if webEnabled && a.ingressDomain != "" {
				ingressName := deployment.GenerateResourceName(agentName, "ingress", name)
				host := GenerateIngressHost(agentName, a.namespace, a.ingressDomain)

				ingressCfg := IngressConfig{
					Name:              ingressName,
					Namespace:         a.namespace,
					AgentName:         agentName,
					Version:           version,
					Component:         fmt.Sprintf("messaging-%s", name),
					ServiceName:       resourceName,
					ServicePort:       8080, // Web adapter port
					Host:              host,
					ACMCertificateARN: a.acmCertificateARN,
					ALBGroupName:      a.albGroupName,
				}
				ingress := BuildIngress(ingressCfg)
				status, err = a.applyIngress(ctx, ingress)
				result.Resources = append(result.Resources, status)
				if err != nil {
					result.Errors = append(result.Errors, deployment.DeploymentError{
						Resource: ingress.Name,
						Kind:     "Ingress",
						Error:    err.Error(),
					})
				}

				// Add external URL to service endpoints
				externalURL := GenerateExternalURL(agentName, a.namespace, a.ingressDomain)
				result.ServiceEndpoints = append(result.ServiceEndpoints, deployment.ServiceEndpoint{
					Name: fmt.Sprintf("messaging-%s", name),
					Type: interfaceType,
					URL:  externalURL,
					Port: 443,
				})
			}
		} else if interfaceType == "custom" && iface.Service != nil {
			// Custom interface services
			resourceName := deployment.GenerateResourceName(agentName, "interface", name)

			// Determine port (default 8080)
			port := int32(8080)
			if len(iface.Service.Ports) > 0 {
				// Parse first port (format: "host:container" or "port")
				fmt.Sscanf(iface.Service.Ports[0], "%d", &port)
			}

			// Create service first
			interfaceServiceCfg := ServiceConfig{
				Name:        resourceName,
				Namespace:   a.namespace,
				AgentName:   agentName,
				Version:     version,
				Component:   fmt.Sprintf("interface-%s", name),
				Port:        port,
				ServiceType: corev1.ServiceTypeClusterIP,
			}
			interfaceService := BuildService(interfaceServiceCfg)
			status, err := a.applyService(ctx, interfaceService)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: interfaceService.Name,
					Kind:     "Service",
					Error:    err.Error(),
				})
			}

			// Create deployment with custom container
			// Resolve custom interface image to ECR path
			containerCfg := spec.ContainerConfig{
				Image: iface.Service.Image,
				Port:  int(port),
			}
			resolvedContainerCfg, err := a.resolveContainerImage(containerCfg)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: resourceName,
					Kind:     "Deployment",
					Error:    fmt.Sprintf("failed to resolve image: %v", err),
				})
				continue
			}

			interfaceDeploymentCfg := DeploymentConfig{
				Name:            resourceName,
				Namespace:       a.namespace,
				AgentName:       agentName,
				Version:         version,
				Component:       fmt.Sprintf("interface-%s", name),
				Container:       resolvedContainerCfg,
				Port:            port,
				SecretName:      secretName,
				ConfigMapName:   configMapName,
				Provider:        "", // Interface services use HTTP health check if defined
				ImagePullPolicy: a.imagePullPolicy,
			}
			interfaceDepl := BuildDeployment(interfaceDeploymentCfg)

			// Add custom environment variables if specified
			if len(iface.Service.Environment) > 0 {
				for key, val := range iface.Service.Environment {
					interfaceDepl.Spec.Template.Spec.Containers[0].Env = append(
						interfaceDepl.Spec.Template.Spec.Containers[0].Env,
						corev1.EnvVar{
							Name:  key,
							Value: val,
						},
					)
				}
			}

			status, err = a.applyDeployment(ctx, interfaceDepl)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: interfaceDepl.Name,
					Kind:     "Deployment",
					Error:    err.Error(),
				})
			}
		}
	}

	// Phase 6: Create CronJobs for injections
	for name, injection := range astroSpec.Injections {
		if injection.Trigger.Type == "schedule" && injection.Trigger.Cron != "" {
			resourceName := deployment.GenerateResourceName(agentName, "injection", name)

			// Extract collection name and vector size from injection target
			collectionName := "astro-docs" // fallback default
			vectorSize := 384               // fallback default

			// Find the upsert step and extract the target knowledge store
			for _, step := range injection.Pipeline {
				if step.Step == "upsert" && step.Target != "" {
					// Parse target reference like "knowledge.docs"
					parts := strings.Split(step.Target, ".")
					if len(parts) == 2 && parts[0] == "knowledge" {
						knowledgeName := parts[1]
						if knowledge, ok := astroSpec.Knowledge[knowledgeName]; ok {
							// Extract collection name from config
							if col, ok := knowledge.Config["collection"].(string); ok {
								collectionName = col
							}
							// Extract dimensions from config
							if dims, ok := knowledge.Config["dimensions"].(float64); ok {
								vectorSize = int(dims)
							} else if dims, ok := knowledge.Config["dimensions"].(int); ok {
								vectorSize = dims
							}
						}
					}
				}
			}

			cronJobCfg := CronJobConfig{
				Name:           resourceName,
				Namespace:      a.namespace,
				AgentName:      agentName,
				Version:        version,
				Component:      fmt.Sprintf("injection-%s", name),
				Schedule:       injection.Trigger.Cron,
				SecretName:     secretName,
				ConfigMapName:  configMapName,
				Injection:      injection,
				CollectionName: collectionName,
				VectorSize:     vectorSize,
				RegistryURL:    a.registryURL,
			}
			cronJob := BuildCronJob(cronJobCfg)
			status, err := a.applyCronJob(ctx, cronJob)
			result.Resources = append(result.Resources, status)
			if err != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: cronJob.Name,
					Kind:     "CronJob",
					Error:    err.Error(),
				})
			}
		}
	}

	// Collect service endpoints
	svc, err := a.clientset.CoreV1().Services(a.namespace).Get(ctx, agentResourceName, metav1.GetOptions{})
	if err == nil {
		endpoint := deployment.ServiceEndpoint{
			Name: "agent-http",
			Type: "http",
			Port: 8080,
		}

		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			// For LoadBalancer services, wait and get external endpoint
			time.Sleep(2 * time.Second)
			for _, ingress := range svc.Status.LoadBalancer.Ingress {
				if ingress.IP != "" {
					endpoint.URL = fmt.Sprintf("http://%s:8080", ingress.IP)
				} else if ingress.Hostname != "" {
					endpoint.URL = fmt.Sprintf("http://%s:8080", ingress.Hostname)
				}
			}
		} else {
			// For ClusterIP services, use DNS name
			endpoint.URL = fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", agentResourceName, a.namespace)
		}

		result.ServiceEndpoints = append(result.ServiceEndpoints, endpoint)
	}

	return result, nil
}

// ensureNamespace creates the namespace if it doesn't exist
func (a *Applier) ensureNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: a.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "astro-server",
			},
		},
	}

	_, err := a.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

// applySecret creates or updates a Secret
func (a *Applier) applySecret(ctx context.Context, secret *corev1.Secret) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "Secret",
		Name:      secret.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.CoreV1().Secrets(a.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = a.clientset.CoreV1().Secrets(a.namespace).Update(ctx, secret, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyConfigMap creates or updates a ConfigMap
func (a *Applier) applyConfigMap(ctx context.Context, cm *corev1.ConfigMap) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "ConfigMap",
		Name:      cm.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.CoreV1().ConfigMaps(a.namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = a.clientset.CoreV1().ConfigMaps(a.namespace).Update(ctx, cm, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyService creates or updates a Service
func (a *Applier) applyService(ctx context.Context, svc *corev1.Service) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "Service",
		Name:      svc.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.CoreV1().Services(a.namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// For services, we need to preserve the ClusterIP
			existing, err := a.clientset.CoreV1().Services(a.namespace).Get(ctx, svc.Name, metav1.GetOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			svc.Spec.ClusterIP = existing.Spec.ClusterIP
			svc.ResourceVersion = existing.ResourceVersion

			_, err = a.clientset.CoreV1().Services(a.namespace).Update(ctx, svc, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyDeployment creates or updates a Deployment
func (a *Applier) applyDeployment(ctx context.Context, depl *appsv1.Deployment) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "Deployment",
		Name:      depl.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.AppsV1().Deployments(a.namespace).Create(ctx, depl, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = a.clientset.AppsV1().Deployments(a.namespace).Update(ctx, depl, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyStatefulSet creates or updates a StatefulSet
func (a *Applier) applyStatefulSet(ctx context.Context, ss *appsv1.StatefulSet) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "StatefulSet",
		Name:      ss.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.AppsV1().StatefulSets(a.namespace).Create(ctx, ss, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = a.clientset.AppsV1().StatefulSets(a.namespace).Update(ctx, ss, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyCronJob creates or updates a CronJob
func (a *Applier) applyCronJob(ctx context.Context, cj *batchv1.CronJob) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "CronJob",
		Name:      cj.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.BatchV1().CronJobs(a.namespace).Create(ctx, cj, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = a.clientset.BatchV1().CronJobs(a.namespace).Update(ctx, cj, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}

// applyIngress creates or updates an Ingress
func (a *Applier) applyIngress(ctx context.Context, ing *networkingv1.Ingress) (deployment.ResourceStatus, error) {
	status := deployment.ResourceStatus{
		Kind:      "Ingress",
		Name:      ing.Name,
		Namespace: a.namespace,
	}

	_, err := a.clientset.NetworkingV1().Ingresses(a.namespace).Create(ctx, ing, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Get existing ingress to preserve resource version
			existing, getErr := a.clientset.NetworkingV1().Ingresses(a.namespace).Get(ctx, ing.Name, metav1.GetOptions{})
			if getErr != nil {
				status.Status = "failed"
				status.Message = getErr.Error()
				return status, getErr
			}
			ing.ResourceVersion = existing.ResourceVersion

			_, err = a.clientset.NetworkingV1().Ingresses(a.namespace).Update(ctx, ing, metav1.UpdateOptions{})
			if err != nil {
				status.Status = "failed"
				status.Message = err.Error()
				return status, err
			}
			status.Status = "updated"
			return status, nil
		}
		status.Status = "failed"
		status.Message = err.Error()
		return status, err
	}

	status.Status = "created"
	return status, nil
}
