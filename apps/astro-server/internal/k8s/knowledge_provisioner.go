package k8s

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/arn"
	spec "github.com/astropods/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	StoreID    string
	AccountID  string
	ARN        string
	Provider   string
	Storage    string // e.g. "20Gi"
	SecretName string // credentials secret to mount via envFrom
	Public     bool
	LocalMode  bool
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
		Provider:        p.Provider,
		ProviderSection: "knowledge",
		LocalMode:       p.LocalMode,
		FsGroup:         prov.FsGroup,
	})
	if err != nil {
		return fmt.Errorf("build statefulset: %w", err)
	}

	// Override with store-specific labels so resources are identifiable without agent context.
	ss.Labels = labels
	ss.Spec.Template.Labels = labels
	ss.Spec.Selector.MatchLabels = selector

	_, err = client.Clientset().AppsV1().StatefulSets(ns).Create(ctx, ss, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create statefulset: %w", err)
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
		_, err = client.Clientset().CoreV1().Services(ns).Create(ctx, lbSvc, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create loadbalancer service: %w", err)
		}
	}

	return nil
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
	secretName := storeID + "-credentials"
	if err := client.Clientset().CoreV1().Secrets(ns).Delete(ctx, secretName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete secret: %w", err)
	}

	return nil
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
