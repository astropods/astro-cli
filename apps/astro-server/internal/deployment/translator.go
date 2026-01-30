package deployment

import (
	"fmt"

	"github.com/postman/astro/packages/astro-spec"
)

// TranslationResult holds the result of spec translation
type TranslationResult struct {
	Manifests      []Manifest
	SecretName     string
	ConfigMapName  string
	ServiceDNSMap  map[string]string // Maps resource name to DNS name
	Errors         []error
}

// Manifest represents a Kubernetes manifest
type Manifest struct {
	Kind      string
	Name      string
	Namespace string
	Object    interface{} // The actual K8s object
}

// Translator translates AstroSpec to Kubernetes manifests
type Translator struct {
	agentName       string
	version         string
	k8sNamespace    string
	registryURL     string
	userCredentials map[string]string
	envBuilder      *EnvBuilder
}

// NewTranslator creates a new translator
func NewTranslator(agentName, version, k8sNamespace, registryURL string, userCredentials map[string]string) *Translator {
	return &Translator{
		agentName:       agentName,
		version:         version,
		k8sNamespace:    k8sNamespace,
		registryURL:     registryURL,
		userCredentials: userCredentials,
		envBuilder:      NewEnvBuilder(k8sNamespace),
	}
}

// Translate translates the AstroSpec into Kubernetes manifests
func (t *Translator) Translate(astroSpec *spec.AstroSpec) (*TranslationResult, error) {
	result := &TranslationResult{
		Manifests:     []Manifest{},
		ServiceDNSMap: make(map[string]string),
		Errors:        []error{},
	}

	// Generate names
	result.SecretName = GenerateCredentialSecretName(t.agentName, t.version)
	result.ConfigMapName = GenerateConfigMapName(t.agentName, t.version)

	// Build connection strings
	connectionStrings := t.envBuilder.BuildConnectionStrings(astroSpec)

	// Track deployment order for dependencies
	var deployments []Manifest
	var services []Manifest
	var statefulSets []Manifest
	var cronJobs []Manifest

	// 1. Create Secrets (credentials)
	if len(t.userCredentials) > 0 {
		secretManifest := Manifest{
			Kind:      "Secret",
			Name:      result.SecretName,
			Namespace: t.k8sNamespace,
			Object: map[string]interface{}{
				"data": t.userCredentials,
			},
		}
		result.Manifests = append(result.Manifests, secretManifest)
	}

	// 2. Create ConfigMaps (connection strings)
	if len(connectionStrings) > 0 {
		configMapManifest := Manifest{
			Kind:      "ConfigMap",
			Name:      result.ConfigMapName,
			Namespace: t.k8sNamespace,
			Object: map[string]interface{}{
				"data": connectionStrings,
			},
		}
		result.Manifests = append(result.Manifests, configMapManifest)
	}

	// 3. Process self-hosted models
	for name, model := range astroSpec.Models {
		if model.Container.Image != "" || model.Container.Build != nil {
			resourceName := GenerateResourceName(t.agentName, "model", name)

			// Create Deployment for model
			deployment := Manifest{
				Kind:      "Deployment",
				Name:      resourceName,
				Namespace: t.k8sNamespace,
				Object: map[string]interface{}{
					"container": model.Container,
					"component": fmt.Sprintf("model-%s", name),
				},
			}
			deployments = append(deployments, deployment)

			// Create Service for model
			port := model.Container.Port
			if port == 0 {
				port = 8080
			}
			service := Manifest{
				Kind:      "Service",
				Name:      resourceName,
				Namespace: t.k8sNamespace,
				Object: map[string]interface{}{
					"port":      port,
					"component": fmt.Sprintf("model-%s", name),
					"type":      "ClusterIP",
				},
			}
			services = append(services, service)

			// Track DNS
			result.ServiceDNSMap[resourceName] = GenerateServiceDNS(resourceName, t.k8sNamespace)
		}
	}

	// 4. Process knowledge stores
	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.Container.Image != "" || knowledge.Container.Build != nil {
			resourceName := GenerateResourceName(t.agentName, "knowledge", name)
			port := knowledge.Container.Port
			if port == 0 {
				port = 6333 // default for vector DB
			}

			if knowledge.Container.Persistent {
				// Create StatefulSet for persistent knowledge
				statefulSet := Manifest{
					Kind:      "StatefulSet",
					Name:      resourceName,
					Namespace: t.k8sNamespace,
					Object: map[string]interface{}{
						"container": knowledge.Container,
						"component": fmt.Sprintf("knowledge-%s", name),
						"port":      port,
					},
				}
				statefulSets = append(statefulSets, statefulSet)
			} else {
				// Create Deployment for non-persistent knowledge
				deployment := Manifest{
					Kind:      "Deployment",
					Name:      resourceName,
					Namespace: t.k8sNamespace,
					Object: map[string]interface{}{
						"container": knowledge.Container,
						"component": fmt.Sprintf("knowledge-%s", name),
					},
				}
				deployments = append(deployments, deployment)
			}

			// Create Service
			service := Manifest{
				Kind:      "Service",
				Name:      resourceName,
				Namespace: t.k8sNamespace,
				Object: map[string]interface{}{
					"port":      port,
					"component": fmt.Sprintf("knowledge-%s", name),
					"type":      "ClusterIP",
				},
			}
			services = append(services, service)

			// Track DNS
			result.ServiceDNSMap[resourceName] = GenerateServiceDNS(resourceName, t.k8sNamespace)
		}
	}

	// 5. Process tools with containers
	for name, tool := range astroSpec.Tools {
		if tool.Container != nil && (tool.Container.Image != "" || tool.Container.Build != nil) {
			resourceName := GenerateResourceName(t.agentName, "tool", name)
			port := tool.Container.Port
			if port == 0 {
				port = 8080
			}

			// Create Deployment for tool
			deployment := Manifest{
				Kind:      "Deployment",
				Name:      resourceName,
				Namespace: t.k8sNamespace,
				Object: map[string]interface{}{
					"container": *tool.Container,
					"component": fmt.Sprintf("tool-%s", name),
				},
			}
			deployments = append(deployments, deployment)

			// Create Service
			service := Manifest{
				Kind:      "Service",
				Name:      resourceName,
				Namespace: t.k8sNamespace,
				Object: map[string]interface{}{
					"port":      port,
					"component": fmt.Sprintf("tool-%s", name),
					"type":      "ClusterIP",
				},
			}
			services = append(services, service)

			// Track DNS
			result.ServiceDNSMap[resourceName] = GenerateServiceDNS(resourceName, t.k8sNamespace)
		}
	}

	// 6. Create main agent Deployment
	agentResourceName := GenerateAgentResourceName(t.agentName, "agent")
	agentDeployment := Manifest{
		Kind:      "Deployment",
		Name:      agentResourceName,
		Namespace: t.k8sNamespace,
		Object: map[string]interface{}{
			"container":   astroSpec.Container,
			"component":   "agent",
			"secretName":  result.SecretName,
			"configName":  result.ConfigMapName,
		},
	}
	deployments = append(deployments, agentDeployment)

	// 7. Create agent Service
	agentService := Manifest{
		Kind:      "Service",
		Name:      agentResourceName,
		Namespace: t.k8sNamespace,
		Object: map[string]interface{}{
			"port":      8080,
			"component": "agent",
			"type":      "LoadBalancer", // Agent exposed externally
		},
	}
	services = append(services, agentService)
	result.ServiceDNSMap[agentResourceName] = GenerateServiceDNS(agentResourceName, t.k8sNamespace)

	// 8. Process messaging interfaces (Slack, Discord, etc.)
	for name, iface := range astroSpec.Interfaces {
		interfaceType := iface.Type
		if interfaceType == "slack" || interfaceType == "discord" || interfaceType == "teams" {
			resourceName := GenerateResourceName(t.agentName, "messaging", name)

			// Create messaging sidecar Deployment
			messagingDeployment := Manifest{
				Kind:      "Deployment",
				Name:      resourceName,
				Namespace: t.k8sNamespace,
				Object: map[string]interface{}{
					"container": map[string]interface{}{
						"image": fmt.Sprintf("%s/astro-messaging:latest", t.registryURL),
					},
					"component":   fmt.Sprintf("messaging-%s", name),
					"secretName":  result.SecretName,
					"configName":  result.ConfigMapName,
					"interface":   iface,
				},
			}
			deployments = append(deployments, messagingDeployment)

			// Create messaging Service (gRPC on port 9090)
			messagingService := Manifest{
				Kind:      "Service",
				Name:      resourceName,
				Namespace: t.k8sNamespace,
				Object: map[string]interface{}{
					"port":      9090,
					"component": fmt.Sprintf("messaging-%s", name),
					"type":      "ClusterIP",
				},
			}
			services = append(services, messagingService)

			// Track DNS
			result.ServiceDNSMap[resourceName] = GenerateServiceDNS(resourceName, t.k8sNamespace)
		} else if interfaceType == "custom" && iface.Service != nil {
			// Custom interface services
			resourceName := GenerateResourceName(t.agentName, "interface", name)

			// Determine port (default 8080)
			port := 8080
			if len(iface.Service.Ports) > 0 {
				// Parse first port (format: "host:container" or "port")
				fmt.Sscanf(iface.Service.Ports[0], "%d", &port)
			}

			// Create custom interface Deployment
			customDeployment := Manifest{
				Kind:      "Deployment",
				Name:      resourceName,
				Namespace: t.k8sNamespace,
				Object: map[string]interface{}{
					"container": map[string]interface{}{
						"image":       iface.Service.Image,
						"environment": iface.Service.Environment,
					},
					"component":  fmt.Sprintf("interface-%s", name),
					"secretName": result.SecretName,
					"configName": result.ConfigMapName,
				},
			}
			deployments = append(deployments, customDeployment)

			// Create custom interface Service
			customService := Manifest{
				Kind:      "Service",
				Name:      resourceName,
				Namespace: t.k8sNamespace,
				Object: map[string]interface{}{
					"port":      port,
					"component": fmt.Sprintf("interface-%s", name),
					"type":      "ClusterIP",
				},
			}
			services = append(services, customService)

			// Track DNS
			result.ServiceDNSMap[resourceName] = GenerateServiceDNS(resourceName, t.k8sNamespace)
		}
	}

	// 9. Process injections with schedule triggers
	for name, injection := range astroSpec.Injections {
		if injection.Trigger.Type == "schedule" && injection.Trigger.Cron != "" {
			resourceName := GenerateResourceName(t.agentName, "injection", name)

			cronJob := Manifest{
				Kind:      "CronJob",
				Name:      resourceName,
				Namespace: t.k8sNamespace,
				Object: map[string]interface{}{
					"schedule":   injection.Trigger.Cron,
					"component":  fmt.Sprintf("injection-%s", name),
					"secretName": result.SecretName,
					"configName": result.ConfigMapName,
					"injection":  injection,
				},
			}
			cronJobs = append(cronJobs, cronJob)
		}
	}

	// Append in dependency order
	result.Manifests = append(result.Manifests, services...)
	result.Manifests = append(result.Manifests, statefulSets...)
	result.Manifests = append(result.Manifests, deployments...)
	result.Manifests = append(result.Manifests, cronJobs...)

	return result, nil
}
