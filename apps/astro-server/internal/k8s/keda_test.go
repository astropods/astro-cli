package k8s

import (
	"context"
	"testing"

	"github.com/postman/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kfake "k8s.io/client-go/kubernetes/fake"
)

// newEKSApplier creates an Applier in EKS mode with a fake dynamic client and
// a fake k8s clientset. Returns both so tests can inspect created resources.
func newEKSApplier(dynClient *dynamicfake.FakeDynamicClient) (*Applier, *kfake.Clientset) {
	fakeClient := kfake.NewClientset()
	return &Applier{
		clientset:       fakeClient,
		namespace:       "default",
		registryURL:     "test-registry.example.com",
		imageResolver:   NewImageResolver("", "test-registry.example.com", "test"),
		imagePullPolicy: corev1.PullNever,
		isEKS:           true,
		dynamicClient:   dynClient,
		ingressDomain:   "agents.example.com",
		acmCertificateARN: "arn:aws:acm:test",
		albGroupName:    "test-alb",
	}, fakeClient
}

// newFakeDynClient creates a fake dynamic client with KEDA CRD list kinds registered.
func newFakeDynClient() *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			httpScaledObjectGVR: "HTTPScaledObjectList",
			scaledObjectGVR:     "ScaledObjectList",
		},
	)
}

func webAdapterSpec() *spec.AstroDeploymentSpec {
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http"},
		},
	}
	return ds
}

// TestApplyDeploymentSpec_EKSMode_WebAdapter verifies that on EKS with a web adapter:
// - Interceptor ingress is created in the keda namespace (not agent namespace)
// - HTTPScaledObject is created for the messaging deployment
// - WorkloadScaledObject is created to keep agent in sync with messaging
// - The messaging service endpoint is still returned
func TestApplyDeploymentSpec_EKSMode_WebAdapter(t *testing.T) {
	dynClient := newFakeDynClient()
	a, fakeClient := newEKSApplier(dynClient)
	ds := webAdapterSpec()
	ctx := context.Background()

	result, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	// Ingress must be in keda namespace, not agent namespace
	hasKedaIngress := false
	hasAgentNsIngress := false
	for _, r := range result.Resources {
		if r.Kind == "Ingress" {
			if r.Namespace == kedaNamespace {
				hasKedaIngress = true
			} else {
				hasAgentNsIngress = true
			}
		}
	}
	if !hasKedaIngress {
		t.Error("expected interceptor ingress in keda namespace")
	}
	if hasAgentNsIngress {
		t.Errorf("expected no ingress in agent namespace on EKS (interceptor should be in keda namespace)")
	}

	// Verify the ingress in keda namespace points to the interceptor service
	kedaIngresses, err := fakeClient.NetworkingV1().Ingresses(kedaNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list keda ingresses: %v", err)
	}
	if len(kedaIngresses.Items) == 0 {
		t.Fatal("interceptor ingress not found in keda namespace")
	}
	ing := kedaIngresses.Items[0]
	backend := ing.Spec.Rules[0].HTTP.Paths[0].Backend.Service
	if backend.Name != interceptorService {
		t.Errorf("backend service: expected %s, got %s", interceptorService, backend.Name)
	}
	if backend.Port.Number != int32(interceptorPort) {
		t.Errorf("backend port: expected %d, got %d", interceptorPort, backend.Port.Number)
	}

	// HTTPScaledObject for messaging deployment
	httpSOs, err := dynClient.Resource(httpScaledObjectGVR).Namespace(a.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list HTTPScaledObjects: %v", err)
	}
	if len(httpSOs.Items) == 0 {
		t.Error("expected HTTPScaledObject for messaging adapter")
	}

	// ScaledObject for agent deployment (watches messaging pods)
	scaledObjs, err := dynClient.Resource(scaledObjectGVR).Namespace(a.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list ScaledObjects: %v", err)
	}
	if len(scaledObjs.Items) == 0 {
		t.Error("expected ScaledObject for agent deployment")
	}

	// Service endpoint still emitted
	hasMsgEndpoint := false
	for _, ep := range result.ServiceEndpoints {
		if ep.Name == "messaging" {
			hasMsgEndpoint = true
		}
	}
	if !hasMsgEndpoint {
		t.Error("expected messaging service endpoint")
	}
}

// TestApplyDeploymentSpec_LocalMode_WebAdapter verifies that on non-EKS clusters
// the regular ingress is created in the agent namespace and no KEDA resources exist.
func TestApplyDeploymentSpec_LocalMode_WebAdapter(t *testing.T) {
	a := newTestApplier()
	a.ingressDomain = "agents.example.com"
	// isEKS defaults to false in newTestApplier

	result, err := a.ApplyDeploymentSpec(context.Background(), webAdapterSpec())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	// Ingress must be in agent namespace
	hasAgentNsIngress := false
	for _, r := range result.Resources {
		if r.Kind == "Ingress" {
			if r.Namespace == kedaNamespace {
				t.Error("expected no keda namespace ingress in local mode")
			}
			if r.Namespace == "test-ns" {
				hasAgentNsIngress = true
			}
		}
	}
	if !hasAgentNsIngress {
		t.Error("expected regular ingress in agent namespace in local mode")
	}

	// No KEDA resources — dynamicClient is nil so nothing can be created
	for _, r := range result.Resources {
		if r.Kind == "HTTPScaledObject" || r.Kind == "ScaledObject" {
			t.Errorf("expected no KEDA resources in local mode, got %s %s", r.Kind, r.Name)
		}
	}
}

// TestApplyDeploymentSpec_EKSMode_WebhookIngestion verifies that on EKS with a webhook
// ingestion entry, the interceptor ingress goes to the keda namespace and an
// HTTPScaledObject is created for the webhook deployment.
func TestApplyDeploymentSpec_EKSMode_WebhookIngestion(t *testing.T) {
	dynClient := newFakeDynClient()
	fakeClient := kfake.NewClientset()
	a := &Applier{
		clientset:              fakeClient,
		namespace:              "default",
		registryURL:            "test-registry.example.com",
		imageResolver:          NewImageResolver("", "test-registry.example.com", "test"),
		imagePullPolicy:        corev1.PullNever,
		isEKS:                  true,
		dynamicClient:          dynClient,
		ingestionIngressDomain: "ingestion.example.com",
		ingestionACMCertARN:    "arn:aws:acm:test",
		ingestionALBGroupName:  "ingestion-alb",
	}

	ds := minimalDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"hook": {
			Image:     "test-registry.example.com/ingest:latest",
			Endpoints: httpEp(9090),
			Trigger:   spec.DeploymentTrigger{Type: "webhook"},
		},
	}

	ctx := context.Background()
	result, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	// Interceptor ingress in keda namespace
	hasKedaIngress := false
	for _, r := range result.Resources {
		if r.Kind == "Ingress" && r.Namespace == kedaNamespace {
			hasKedaIngress = true
		}
		if r.Kind == "Ingress" && r.Namespace == "default" {
			t.Error("expected no ingress in agent namespace on EKS webhook ingestion")
		}
	}
	if !hasKedaIngress {
		t.Error("expected interceptor ingress in keda namespace for webhook")
	}

	// HTTPScaledObject for webhook
	httpSOs, err := dynClient.Resource(httpScaledObjectGVR).Namespace(a.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list HTTPScaledObjects: %v", err)
	}
	if len(httpSOs.Items) == 0 {
		t.Error("expected HTTPScaledObject for webhook ingestion")
	}

	// Webhook service endpoint still emitted
	hasWebhookEndpoint := false
	for _, ep := range result.ServiceEndpoints {
		if ep.Type == "webhook" {
			hasWebhookEndpoint = true
		}
	}
	if !hasWebhookEndpoint {
		t.Error("expected webhook service endpoint")
	}
}

// TestApplyDeploymentSpec_LocalMode_WebhookIngestion verifies that on non-EKS clusters
// webhook ingestion creates a regular ingress in the agent namespace.
func TestApplyDeploymentSpec_LocalMode_WebhookIngestion(t *testing.T) {
	a := newTestApplier()
	a.ingestionIngressDomain = "ingestion.example.com"

	ds := minimalDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"hook": {
			Image:     "test-registry.example.com/ingest:latest",
			Endpoints: httpEp(9090),
			Trigger:   spec.DeploymentTrigger{Type: "webhook"},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	hasIngress := false
	for _, r := range result.Resources {
		if r.Kind == "Ingress" {
			if r.Namespace == kedaNamespace {
				t.Error("expected no keda namespace ingress in local mode")
			}
			hasIngress = true
		}
	}
	if !hasIngress {
		t.Error("expected regular ingress for webhook ingestion in local mode")
	}
}

// TestApplyNetworkPolicies_AllowsKedaNamespace verifies that the
// allow-namespace-traffic NetworkPolicy includes an ingress rule permitting
// traffic from the keda namespace (for the interceptor proxy).
func TestApplyNetworkPolicies_AllowsKedaNamespace(t *testing.T) {
	fakeClient := kfake.NewClientset()
	a := &Applier{
		clientset:      fakeClient,
		namespace:      "test-ns",
		podSubnetCIDRs: []string{"10.0.0.0/8"},
	}
	ctx := context.Background()

	// Namespace must exist before applying policies
	_, _ = fakeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
	}, metav1.CreateOptions{})

	if err := a.applyNetworkPolicies(ctx); err != nil {
		t.Fatalf("applyNetworkPolicies: %v", err)
	}

	policies, err := fakeClient.NetworkingV1().NetworkPolicies("test-ns").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list policies: %v", err)
	}

	var allowPolicy *networkingv1.NetworkPolicy
	for i := range policies.Items {
		if policies.Items[i].Name == "allow-namespace-traffic" {
			allowPolicy = &policies.Items[i]
			break
		}
	}
	if allowPolicy == nil {
		t.Fatal("expected allow-namespace-traffic policy")
	}

	hasKedaRule := false
	for _, rule := range allowPolicy.Spec.Ingress {
		for _, peer := range rule.From {
			if peer.NamespaceSelector != nil {
				if peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == kedaNamespace {
					hasKedaRule = true
				}
			}
		}
	}
	if !hasKedaRule {
		t.Error("expected network policy to allow ingress from keda namespace")
	}
}
