package k8s

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// desiredIngress is one Ingress a deployment's current spec wants, alongside
// the ServiceEndpoint metadata SyncIngresses records when it applies clean.
type desiredIngress struct {
	ingress      *networkingv1.Ingress
	endpointName string
	endpointType string
	externalURL  string
}

// desiredIngresses computes every Ingress a deployment's current spec wants —
// the agent ingress, the messaging ingress, and any webhook-ingestion
// ingresses — purely from ds and the Applier's static config. No cluster
// reads, no side effects.
//
// Recomputes the same host/name/port values as ApplyDeploymentSpec's agent,
// messaging, and ingestion(webhook) ingress blocks. Kept in sync with those
// blocks by hand; the two paths intentionally don't share Service/Secret
// provisioning, so a shared builder would need to thread values (like the
// messaging webPort collision-avoidance result) that only exist mid-
// provisioning in the full apply path.
func (a *Applier) desiredIngresses(ds *deployment.AstroDeploymentSpec) []desiredIngress {
	var out []desiredIngress

	accountName := ds.Source.Account
	agentName := ds.Source.Name
	buildID := ds.Source.Build

	agentPort := primaryPort(ds.Agent.Endpoints)
	if agentPort == 0 {
		agentPort = 8080
	}

	if ep := deployment.ExposedEndpoint(ds.Agent.Endpoints); ep != nil {
		if host := a.resolveAgentIngressHost(ds, agentName); host != "" {
			agentResourceName := deployment.GenerateAgentResourceName(agentName, "agent")
			ingressName := deployment.GenerateAgentResourceName(agentName, "ingress-agent")
			ingress := BuildIngress(IngressConfig{
				Name: ingressName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
				BuildID: buildID, Component: "agent",
				ServiceName: agentResourceName, ServicePort: int32(ep.Port), //nolint:gosec
				Host: host, ResponseTimeout: ds.Agent.ResponseTimeout,
			})
			out = append(out, desiredIngress{
				ingress: ingress, endpointName: "agent", endpointType: "frontend",
				externalURL: fmt.Sprintf("https://%s", host),
			})
		}
	}

	if ds.Interfaces != nil && len(ds.Interfaces.Adapters) > 0 && !a.localMode {
		webEnabled := false
		for _, adapter := range ds.Interfaces.Adapters {
			if adapter == "web" {
				webEnabled = true
				break
			}
		}
		if webEnabled {
			grpcPort := int32(0)
			if ep := deployment.EndpointByName(ds.Interfaces.Endpoints, "grpc"); ep != nil {
				grpcPort = int32(ep.Port) //nolint:gosec
			}
			if grpcPort == 0 {
				grpcPort = int32(deployment.PrimaryPort(ds.Interfaces.Endpoints)) //nolint:gosec
			}
			if grpcPort == 0 {
				grpcPort = 9090
			}

			webPort := int32(0)
			if ep := deployment.EndpointByName(ds.Interfaces.Endpoints, "http"); ep != nil {
				webPort = int32(ep.Port) //nolint:gosec
			}
			if webPort == 0 {
				webPort = 8090
			}
			for webPort == agentPort || webPort == grpcPort {
				webPort += 10
			}

			host := ""
			if ep := deployment.EndpointByName(ds.Interfaces.Endpoints, "http"); ep != nil && ep.Expose != nil {
				host = ep.Expose.Domain
			}
			if host == "" {
				if domain := a.webIngressDomain(ds.Interfaces.WebPublic()); domain != "" {
					host = GenerateMessagingIngressHost(agentName, a.namespace, domain)
				}
			}
			if host != "" {
				resourceName := deployment.GenerateAgentResourceName(agentName, "messaging")
				ingressName := deployment.GenerateAgentResourceName(agentName, "ingress-messaging")
				ingress := BuildIngress(IngressConfig{
					Name: ingressName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
					BuildID: buildID, Component: "messaging",
					ServiceName: resourceName, ServicePort: webPort, Host: host,
					ExtraHosts:      []string{GenerateMessagingInternalHost(a.namespace)},
					ResponseTimeout: ds.Agent.ResponseTimeout,
				})
				out = append(out, desiredIngress{
					ingress: ingress, endpointName: "messaging", endpointType: "web",
					externalURL: fmt.Sprintf("https://%s", host),
				})
			}
		}
	}

	if a.ingestionIngressDomain != "" {
		for name, ingestion := range ds.Ingestion {
			if ingestion.Trigger.Type != "webhook" {
				continue
			}
			resourceName := deployment.GenerateResourceName(agentName, "ingestion", name)
			component := fmt.Sprintf("ingestion-%s", name)
			port := int32(deployment.PrimaryPort(ingestion.Endpoints)) //nolint:gosec
			ingressName := deployment.GenerateResourceName(agentName, "ingress", name)
			host := GenerateIngestionIngressHost(agentName, a.namespace, name, a.ingestionIngressDomain)
			ingress := BuildIngress(IngressConfig{
				Name: ingressName, Namespace: a.namespace, AccountID: accountName, AgentName: agentName,
				BuildID: buildID, Component: component,
				ServiceName: resourceName, ServicePort: port, Host: host,
				ResponseTimeout: ds.Agent.ResponseTimeout,
			})
			out = append(out, desiredIngress{
				ingress:      ingress,
				endpointName: fmt.Sprintf("ingestion-%s-webhook", name),
				endpointType: "webhook",
				externalURL:  GenerateIngestionExternalURL(agentName, a.namespace, name, a.ingestionIngressDomain),
			})
		}
	}

	return out
}

// SyncIngresses re-applies every Ingress a deployment's current spec wants,
// without touching Services, Secrets, or workloads. It targets already-
// running deployments whose Ingress objects predate a routing change (for
// example the tenant-router migration) and were never regenerated because
// nothing else about the deployment has changed since — a normal apply only
// touches an Ingress when something else in the same ApplyDeploymentSpec call
// causes it to run.
func (a *Applier) SyncIngresses(ctx context.Context, ds *deployment.AstroDeploymentSpec) (*ApplyResult, error) {
	result := &ApplyResult{
		Resources:        []deployment.ResourceStatus{},
		ServiceEndpoints: []deployment.ServiceEndpoint{},
		Errors:           []deployment.DeploymentError{},
	}
	for _, di := range a.desiredIngresses(ds) {
		a.applyAndRecordIngress(ctx, di.ingress, di.endpointName, di.endpointType, di.externalURL, result)
	}
	return result, nil
}

// applyAndRecordIngress applies ing and records the outcome on result: an
// error on failure, or a ServiceEndpoint on success.
func (a *Applier) applyAndRecordIngress(
	ctx context.Context,
	ing *networkingv1.Ingress,
	endpointName, endpointType, externalURL string,
	result *ApplyResult,
) {
	status, err := a.applyIngress(ctx, ing)
	result.Resources = append(result.Resources, status)
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: ing.Name, Kind: "Ingress", Error: err.Error(),
		})
		return
	}
	result.ServiceEndpoints = append(result.ServiceEndpoints, deployment.ServiceEndpoint{
		Name: endpointName, Type: endpointType, URL: externalURL, Port: 443,
	})
}

// CheckIngressDrift reports whether any Ingress a deployment's current spec
// wants is missing or diverges from the live object's class or host rules —
// read-only, no apply. Used by the tenant-router-ingress evaluator to detect
// deployments whose Ingress predates a routing change without touching them.
func (a *Applier) CheckIngressDrift(ctx context.Context, ds *deployment.AstroDeploymentSpec) (drifted bool, detail string, err error) {
	for _, di := range a.desiredIngresses(ds) {
		live, getErr := a.clientset.NetworkingV1().Ingresses(a.namespace).Get(ctx, di.ingress.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			return true, fmt.Sprintf("ingress %q is missing", di.ingress.Name), nil
		}
		if getErr != nil {
			return false, "", fmt.Errorf("get ingress %q: %w", di.ingress.Name, getErr)
		}
		if reason := ingressDiff(di.ingress, live); reason != "" {
			return true, fmt.Sprintf("ingress %q: %s", di.ingress.Name, reason), nil
		}
	}
	return false, "", nil
}

// ingressDiff compares a desired Ingress against the live object's class and
// host/backend rules, returning a human-readable mismatch reason or "" when
// every desired rule is present with a matching backend.
func ingressDiff(desired, live *networkingv1.Ingress) string {
	if desired.Spec.IngressClassName != nil {
		if live.Spec.IngressClassName == nil || *live.Spec.IngressClassName != *desired.Spec.IngressClassName {
			return fmt.Sprintf("ingress class is not %q", *desired.Spec.IngressClassName)
		}
	}

	liveRules := make(map[string]networkingv1.IngressRule, len(live.Spec.Rules))
	for _, r := range live.Spec.Rules {
		liveRules[r.Host] = r
	}

	for _, wantRule := range desired.Spec.Rules {
		gotRule, ok := liveRules[wantRule.Host]
		if !ok {
			return fmt.Sprintf("host %q is missing", wantRule.Host)
		}
		if wantRule.HTTP == nil || len(wantRule.HTTP.Paths) == 0 {
			continue
		}
		wantBackend := wantRule.HTTP.Paths[0].Backend.Service
		if wantBackend == nil {
			continue
		}
		if gotRule.HTTP == nil || len(gotRule.HTTP.Paths) == 0 || gotRule.HTTP.Paths[0].Backend.Service == nil {
			return fmt.Sprintf("host %q has no backend", wantRule.Host)
		}
		gotBackend := gotRule.HTTP.Paths[0].Backend.Service
		if gotBackend.Name != wantBackend.Name || gotBackend.Port.Number != wantBackend.Port.Number {
			return fmt.Sprintf("host %q backend is %s:%d, want %s:%d",
				wantRule.Host, gotBackend.Name, gotBackend.Port.Number, wantBackend.Name, wantBackend.Port.Number)
		}
	}
	return ""
}
