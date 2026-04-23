package k8s

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/arn"
	spec "github.com/astropods/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const (
	knowledgeNSPrefix = "knowledge-"
	knowledgeNSLabel  = "astro.io/namespace-type"
	knowledgeNSValue  = "knowledge"
)

// KnowledgeNamespace returns the K8s namespace name for an account's managed knowledge stores.
func KnowledgeNamespace(accountID string) string {
	return knowledgeNSPrefix + arn.AccountShortID(accountID)
}

// KnowledgeResourceName returns the K8s resource name for a store.
// Prefixed with "kn-" to guarantee the name starts with a letter,
// satisfying the DNS-1035 requirement for Service names.
func KnowledgeResourceName(storeID string) string {
	return "kn-" + storeID
}

// KnowledgeSecretName returns the K8s Secret name for a store's credentials.
func KnowledgeSecretName(storeID string) string {
	return storeID + "-credentials"
}

// knowledgeLabels returns the standard labels for a managed knowledge store resource.
func knowledgeLabels(accountID, storeID string) map[string]string {
	return map[string]string{
		"astro.io/account-id": accountID,
		"astro.io/store-id":   storeID,
		"astro.io/component":  "knowledge",
	}
}

// knowledgeSelector returns the pod selector labels for a managed knowledge store.
func knowledgeSelector(storeID string) map[string]string {
	return map[string]string{
		"astro.io/store-id":  storeID,
		"astro.io/component": "knowledge",
	}
}

// EnsureKnowledgeNamespace creates the per-account knowledge namespace if it does not exist.
func EnsureKnowledgeNamespace(ctx context.Context, client ClusterClient, accountID string) error {
	ns := KnowledgeNamespace(accountID)
	_, err := client.Clientset().CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get namespace %s: %w", ns, err)
	}

	_, err = client.Clientset().CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
			Labels: map[string]string{
				"astro.io/account-id": accountID,
				knowledgeNSLabel:      knowledgeNSValue,
			},
		},
	}, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// KnowledgeProvisionParams holds everything needed to create a managed store's K8s resources.
type KnowledgeProvisionParams struct {
	StoreID        string
	AccountID      string
	ARN            string
	Provider       string
	Storage        string // e.g. "20Gi"
	StorageClass   string // optional — falls back to cluster default if empty
	SecretName     string // credentials secret to mount via envFrom
	Public         bool
	PublicHost     string // friendly CNAME for external-dns (e.g. name.account.knowledge.domain)
	LocalMode      bool
	PodSubnetCIDRs []string // used for network policy egress external rule
}

// ProvisionKnowledgeStore creates the StatefulSet, ClusterIP Service, and (if Public)
// a LoadBalancer Service for a managed knowledge store.
func ProvisionKnowledgeStore(ctx context.Context, client ClusterClient, p KnowledgeProvisionParams) error {
	ns := KnowledgeNamespace(p.AccountID)
	labels := knowledgeLabels(p.AccountID, p.StoreID)
	selector := knowledgeSelector(p.StoreID)

	prov, ok := spec.LookupBuiltin("knowledge", p.Provider)
	if !ok {
		return fmt.Errorf("unknown provider: %s", p.Provider)
	}

	resourceName := KnowledgeResourceName(p.StoreID)

	// Per-provider resource requests/limits sized for production workloads.
	providerResources := knowledgeProviderResources(p.Provider)

	// Convert provider health check to spec.Healthcheck for liveness/readiness probes.
	var healthcheck *spec.Healthcheck
	if len(prov.HealthCheck) > 0 {
		healthcheck = &spec.Healthcheck{Test: prov.HealthCheck}
	} else if prov.HealthPath != "" {
		healthcheck = &spec.Healthcheck{Path: prov.HealthPath}
	}

	// Build the StatefulSet using the existing builder. We repurpose AgentName and BuildID
	// as the store ID so the underlying label logic has a stable identity.
	ss, err := BuildStatefulSet(StatefulSetConfig{
		Name:            resourceName,
		Namespace:       ns,
		AccountID:       p.AccountID,
		AgentName:       p.StoreID,
		BuildID:         p.StoreID,
		Component:       "knowledge",
		Container:       spec.ContainerConfig{Image: prov.Image, Environment: prov.DefaultEnv},
		Port:            int32(prov.DefaultPort), //nolint:gosec
		SecretName:      p.SecretName,
		StorageSize:     p.Storage,
		StorageClass:    p.StorageClass,
		Provider:        p.Provider,
		ProviderSection: "knowledge",
		LocalMode:       p.LocalMode,
		FsGroup:         prov.FsGroup,
		Healthcheck:     healthcheck,
		Resources:       &providerResources,
	})
	if err != nil {
		return fmt.Errorf("build statefulset: %w", err)
	}

	// Override with store-specific labels so resources are identifiable without agent context.
	ss.Labels = labels
	ss.Spec.Template.Labels = labels
	ss.Spec.Selector.MatchLabels = selector

	// Retain PVCs when the StatefulSet is deleted so data survives accidental deletion.
	// Delete PVCs from scaled-down replicas to avoid orphaned volumes.
	ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
		WhenScaled:  appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
	}

	// If the provider has init SQL, create a ConfigMap and mount it at the postgres
	// init directory so it runs on first boot.
	if prov.InitSQL != "" {
		cm := buildInitSQLConfigMap(resourceName, ns, labels, prov.InitSQL)
		_, cmErr := client.Clientset().CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
		if cmErr != nil && !apierrors.IsAlreadyExists(cmErr) {
			return fmt.Errorf("create init configmap: %w", cmErr)
		}
		ss.Spec.Template.Spec.Volumes = append(ss.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "initdb",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: resourceName + "-init"},
				},
			},
		})
		ss.Spec.Template.Spec.Containers[0].VolumeMounts = append(
			ss.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "initdb",
				MountPath: "/docker-entrypoint-initdb.d",
				ReadOnly:  true,
			},
		)
	}

	_, err = client.Clientset().AppsV1().StatefulSets(ns).Create(ctx, ss, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create statefulset: %w", err)
	}

	// PodDisruptionBudget — prevents node drain from evicting the pod without awareness.
	pdb := buildKnowledgePDB(resourceName, ns, labels, selector)
	_, err = client.Clientset().PolicyV1().PodDisruptionBudgets(ns).Create(ctx, pdb, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create pdb: %w", err)
	}

	// Network policies — deny-all default with explicit allow for agent namespaces.
	if err := applyKnowledgeNetworkPolicies(ctx, client, ns, selector, int32(prov.DefaultPort), p.PodSubnetCIDRs, p.Public); err != nil { //nolint:gosec
		return fmt.Errorf("apply network policies: %w", err)
	}

	// ClusterIP service — in-cluster access for agents.
	clusterSvc := buildKnowledgeService(resourceName, ns, labels, selector, int32(prov.DefaultPort), corev1.ServiceTypeClusterIP) //nolint:gosec
	_, err = client.Clientset().CoreV1().Services(ns).Create(ctx, clusterSvc, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create clusterip service: %w", err)
	}

	// LoadBalancer service — external access for public stores.
	if p.Public {
		lbSvc := buildKnowledgeService(resourceName+"-lb", ns, labels, selector, int32(prov.DefaultPort), corev1.ServiceTypeLoadBalancer) //nolint:gosec
		if lbSvc.Annotations == nil {
			lbSvc.Annotations = make(map[string]string)
		}
		// AWS Load Balancer Controller annotations for internet-facing NLB.
		lbSvc.Annotations["service.beta.kubernetes.io/aws-load-balancer-type"] = "external"
		lbSvc.Annotations["service.beta.kubernetes.io/aws-load-balancer-nlb-target-type"] = "ip"
		lbSvc.Annotations["service.beta.kubernetes.io/aws-load-balancer-scheme"] = "internet-facing"
		if p.PublicHost != "" {
			lbSvc.Annotations["external-dns.alpha.kubernetes.io/hostname"] = p.PublicHost
		}
		_, err = client.Clientset().CoreV1().Services(ns).Create(ctx, lbSvc, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create loadbalancer service: %w", err)
		}
	}

	return nil
}

// KnowledgeSecretReader reads plaintext credentials from a knowledge store's k8s Secret.
// Implements knowledgestore.SecretReader for the no-KMS fallback path.
type KnowledgeSecretReader struct {
	Clientset kubernetes.Interface
}

// ReadCredentials reads the credentials Secret for a store and returns plaintext key-value pairs.
func (r *KnowledgeSecretReader) ReadCredentials(ctx context.Context, storeID, namespace string) (map[string]string, error) {
	secretName := KnowledgeSecretName(storeID)
	secret, err := r.Clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("read secret %s/%s: %w", namespace, secretName, err)
	}
	result := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		result[k] = string(v)
	}
	return result, nil
}

// ApplyKnowledgeSecret creates or updates the credentials secret in the store's namespace.
// The StatefulSet mounts this via envFrom; the reconciler calls this to recreate it after
// a cluster migration or accidental deletion.
func ApplyKnowledgeSecret(ctx context.Context, client ClusterClient, accountID, storeID, secretName string, data map[string]string) error {
	ns := KnowledgeNamespace(accountID)

	secretData := make(map[string][]byte, len(data))
	for k, v := range data {
		secretData[k] = []byte(v)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ns,
			Labels: map[string]string{
				"astro.io/store-id":  storeID,
				"astro.io/component": "knowledge",
			},
		},
		Data: secretData,
	}

	_, err := client.Clientset().CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		_, err = client.Clientset().CoreV1().Secrets(ns).Update(ctx, secret, metav1.UpdateOptions{})
	}
	return err
}

// DeleteKnowledgeStore removes all K8s resources for a managed store.
// Errors for not-found resources are ignored.
func DeleteKnowledgeStore(ctx context.Context, client ClusterClient, accountID, storeID string, public bool) error {
	ns := KnowledgeNamespace(accountID)
	resourceName := KnowledgeResourceName(storeID)

	if err := client.Clientset().PolicyV1().PodDisruptionBudgets(ns).Delete(ctx, resourceName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete pdb: %w", err)
	}
	if err := client.Clientset().AppsV1().StatefulSets(ns).Delete(ctx, resourceName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete statefulset: %w", err)
	}
	if err := client.Clientset().CoreV1().Services(ns).Delete(ctx, resourceName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete clusterip service: %w", err)
	}
	if public {
		if err := client.Clientset().CoreV1().Services(ns).Delete(ctx, resourceName+"-lb", metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete lb service: %w", err)
		}
	}
	secretName := KnowledgeSecretName(storeID)
	if err := client.Clientset().CoreV1().Secrets(ns).Delete(ctx, secretName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete secret: %w", err)
	}
	initCM := resourceName + "-init"
	if err := client.Clientset().CoreV1().ConfigMaps(ns).Delete(ctx, initCM, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete init configmap: %w", err)
	}

	return nil
}

// knowledgeProviderResources returns production-sized resource requests/limits for each provider.
func knowledgeProviderResources(provider string) corev1.ResourceRequirements {
	type resourceDef struct{ cpuReq, memReq, cpuLim, memLim string }
	defaults := map[string]resourceDef{
		"postgres": {"250m", "256Mi", "1", "1Gi"},
		"qdrant":   {"250m", "512Mi", "2", "2Gi"},
		"redis":    {"50m", "64Mi", "500m", "256Mi"},
		"neo4j":    {"500m", "512Mi", "2", "2Gi"},
	}
	d, ok := defaults[provider]
	if !ok {
		d = resourceDef{"100m", "128Mi", "500m", "512Mi"}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(d.cpuReq),
			corev1.ResourceMemory: resource.MustParse(d.memReq),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(d.cpuLim),
			corev1.ResourceMemory: resource.MustParse(d.memLim),
		},
	}
}

// applyKnowledgeNetworkPolicies applies namespace isolation for a knowledge store namespace:
//   - default-deny-all: blocks all ingress and egress by default
//   - allow-knowledge-traffic: permits ingress from astro-managed namespaces on the DB port,
//     intra-namespace traffic, and egress to DNS + external (for image pulls).
func applyKnowledgeNetworkPolicies(ctx context.Context, client ClusterClient, ns string, selector map[string]string, dbPort int32, podSubnetCIDRs []string, public bool) error {
	policyTypes := []networkingv1.PolicyType{
		networkingv1.PolicyTypeIngress,
		networkingv1.PolicyTypeEgress,
	}

	apply := func(np *networkingv1.NetworkPolicy) error {
		_, err := client.Clientset().NetworkingV1().NetworkPolicies(ns).Create(ctx, np, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			_, err = client.Clientset().NetworkingV1().NetworkPolicies(ns).Update(ctx, np, metav1.UpdateOptions{})
		}
		return err
	}

	// Policy 1: deny all by default.
	if err := apply(&networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default-deny-all", Namespace: ns},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: policyTypes,
		},
	}); err != nil {
		return fmt.Errorf("default-deny-all: %w", err)
	}

	dbPortObj := intstr.FromInt32(dbPort)
	externalBlock := networkingv1.IPBlock{CIDR: "0.0.0.0/0", Except: podSubnetCIDRs}
	dnsPort53UDP := networkingv1.NetworkPolicyPort{Protocol: protocolPtr(corev1.ProtocolUDP), Port: portPtr(intstr.FromInt32(53))}
	dnsPort53TCP := networkingv1.NetworkPolicyPort{Protocol: protocolPtr(corev1.ProtocolTCP), Port: portPtr(intstr.FromInt32(53))}

	// Policy 2: allow ingress from astro-managed namespaces on the DB port,
	// intra-namespace traffic, and egress to DNS + external.
	ingressRules := []networkingv1.NetworkPolicyIngressRule{
		// Intra-namespace (health checks, etc.)
		{From: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{}}}},
		// From agent namespaces managed by astro-server on the DB port.
		{
			From: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app.kubernetes.io/managed-by": "astro-server"},
				},
			}},
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: protocolPtr(corev1.ProtocolTCP), Port: portPtr(dbPortObj)},
			},
		},
	}
	if public {
		// External clients and NLB health checks on the DB port.
		ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
			From: []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}}},
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: protocolPtr(corev1.ProtocolTCP), Port: portPtr(dbPortObj)},
			},
		})
	}
	if err := apply(&networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-knowledge-traffic", Namespace: ns},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: selector},
			PolicyTypes: policyTypes,
			Ingress:     ingressRules,
			Egress: []networkingv1.NetworkPolicyEgressRule{
				// Intra-namespace.
				{To: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{}}}},
				// DNS.
				{Ports: []networkingv1.NetworkPolicyPort{dnsPort53UDP, dnsPort53TCP}},
				// External (image pulls, etc.) — excludes pod subnets.
				{To: []networkingv1.NetworkPolicyPeer{{IPBlock: &externalBlock}}},
			},
		},
	}); err != nil {
		return fmt.Errorf("allow-knowledge-traffic: %w", err)
	}

	return nil
}

func buildKnowledgePDB(resourceName, ns string, labels, selector map[string]string) *policyv1.PodDisruptionBudget {
	minAvailable := intstr.FromInt(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector:     &metav1.LabelSelector{MatchLabels: selector},
		},
	}
}

func buildInitSQLConfigMap(resourceName, ns string, labels map[string]string, sql string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName + "-init",
			Namespace: ns,
			Labels:    labels,
		},
		Data: map[string]string{
			"init.sql": sql,
		},
	}
}

// IsStatefulSetReady returns true if the store's StatefulSet has at least one ready replica.
func IsStatefulSetReady(ctx context.Context, client ClusterClient, accountID, storeID string) (bool, error) {
	ns := KnowledgeNamespace(accountID)
	ss, err := client.Clientset().AppsV1().StatefulSets(ns).Get(ctx, KnowledgeResourceName(storeID), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ss.Status.ReadyReplicas >= 1, nil
}

// GetLoadBalancerHostname returns the hostname (or IP) assigned to the store's LB service,
// or an empty string if the cloud provider hasn't assigned one yet.
func GetLoadBalancerHostname(ctx context.Context, client ClusterClient, accountID, storeID string) (string, error) {
	ns := KnowledgeNamespace(accountID)
	svc, err := client.Clientset().CoreV1().Services(ns).Get(ctx, KnowledgeResourceName(storeID)+"-lb", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	for _, ing := range svc.Status.LoadBalancer.Ingress {
		if ing.Hostname != "" {
			return ing.Hostname, nil
		}
		if ing.IP != "" {
			return ing.IP, nil
		}
	}
	return "", nil
}

// SecretExists returns true if the credentials secret exists in the store's namespace.
func SecretExists(ctx context.Context, client ClusterClient, accountID, secretName string) (bool, error) {
	ns := KnowledgeNamespace(accountID)
	_, err := client.Clientset().CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func buildKnowledgeService(name, ns string, labels, selector map[string]string, port int32, svcType corev1.ServiceType) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: selector,
			Ports: []corev1.ServicePort{
				{
					Name:       "db",
					Protocol:   corev1.ProtocolTCP,
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
				},
			},
		},
	}
}
