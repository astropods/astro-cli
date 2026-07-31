package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// TestApplyDeploymentSpec_SecretValueRoundTrips confirms a secret value ending
// in "%" survives verbatim into the Secret bytes — i.e. nothing in the resolve
// or build path treats the value as a printf format string or otherwise mangles
// it. This is the "is the data itself corrupted?" half of the trailing-% bug.
func TestApplyDeploymentSpec_SecretValueRoundTrips(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Variables = map[string]deployment.Variable{
		"API_KEY": {Value: "sk-secret-%", Secret: true},
	}

	if _, err := a.ApplyDeploymentSpec(context.Background(), ds); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secretName := "my-agent-build-123-credentials"
	sec, err := a.clientset.CoreV1().Secrets("default").Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected secret %s: %v", secretName, err)
	}
	if got := string(sec.Data["API_KEY"]); got != "sk-secret-%" {
		t.Errorf("secret value corrupted: want %q, got %q", "sk-secret-%", got)
	}
}

// TestApplyDeploymentSpec_NoDanglingRefWhenSecretApplyFails reproduces the
// production wedge: a trailing "%" in a secret makes the cluster reject the
// Secret apply, but the applier records the error and *keeps going*, creating
// the agent StatefulSet whose envFrom still references the now-missing Secret.
// The pod then can't start ("secret/configmap not found"), never goes healthy,
// so the deployment sticks in "deploying" and blocks redeploys until the 30-min
// stale reaper fires.
//
// We simulate the cluster-side rejection with a reactor (the fake clientset does
// no value validation of its own). The invariant under test: if a workload's
// env source can't be created, the applier must not leave a workload pointing at
// a missing object — either it fails the whole apply, or it doesn't create the
// dangling workload.
func TestApplyDeploymentSpec_NoDanglingRefWhenSecretApplyFails(t *testing.T) {
	fakeClient := fake.NewClientset()
	// Reject any Secret whose value ends in "%", mimicking a real API-server
	// rejection of the resolved credential bundle.
	fakeClient.PrependReactor("create", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		sec, ok := action.(clienttesting.CreateAction).GetObject().(*corev1.Secret)
		if !ok {
			return false, nil, nil
		}
		for _, v := range sec.Data {
			if strings.HasSuffix(string(v), "%") {
				return true, nil, apierrors.NewInvalid(
					schema.GroupKind{Kind: "Secret"}, sec.Name, nil)
			}
		}
		return false, nil, nil
	})

	a := &Applier{clientset: fakeClient, namespace: "default", imagePullPolicy: corev1.PullNever}
	ds := minimalDeploymentSpec()
	ds.Variables = map[string]deployment.Variable{
		"API_KEY": {Value: "sk-secret-%", Secret: true},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	// The apply must fail loudly so the deployer marks the deployment failed
	// immediately, rather than silently leaving it to stall in "deploying".
	if err == nil {
		t.Fatal("expected apply to abort with an error when the agent Secret is rejected")
	}

	// The Secret apply must have surfaced an error.
	if len(result.Errors) == 0 {
		t.Fatal("expected the rejected Secret to surface an apply error")
	}

	// Invariant: no created StatefulSet may reference a Secret/ConfigMap that
	// does not exist in the cluster.
	sts, err := a.clientset.AppsV1().StatefulSets("default").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list statefulsets: %v", err)
	}
	for _, ss := range sts.Items {
		for _, ref := range referencedEnvSources(&ss) {
			if !envSourceExists(t, a.clientset, "default", ref) {
				t.Errorf("StatefulSet %q references %s %q which was never created — "+
					"dangling reference wedges the pod at startup and stalls the deploy",
					ss.Name, ref.kind, ref.name)
			}
		}
	}
}

type envSourceRef struct {
	kind string // "Secret" | "ConfigMap"
	name string
}

func referencedEnvSources(ss *appsv1.StatefulSet) []envSourceRef {
	var refs []envSourceRef
	for _, c := range ss.Spec.Template.Spec.Containers {
		for _, ef := range c.EnvFrom {
			if ef.SecretRef != nil && ef.SecretRef.Name != "" {
				refs = append(refs, envSourceRef{"Secret", ef.SecretRef.Name})
			}
			if ef.ConfigMapRef != nil && ef.ConfigMapRef.Name != "" {
				refs = append(refs, envSourceRef{"ConfigMap", ef.ConfigMapRef.Name})
			}
		}
	}
	return refs
}

func envSourceExists(t *testing.T, cs kubernetes.Interface, ns string, ref envSourceRef) bool {
	t.Helper()
	switch ref.kind {
	case "Secret":
		_, err := cs.CoreV1().Secrets(ns).Get(context.Background(), ref.name, metav1.GetOptions{})
		return err == nil
	case "ConfigMap":
		_, err := cs.CoreV1().ConfigMaps(ns).Get(context.Background(), ref.name, metav1.GetOptions{})
		return err == nil
	}
	return false
}
