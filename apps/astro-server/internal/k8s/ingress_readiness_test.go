package k8s

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateEndpointReadiness_NilOpts(t *testing.T) {
	ready, msg := EvaluateEndpointReadiness(nil, nil)
	assert.True(t, ready)
	assert.Empty(t, msg)
}

func TestEvaluateEndpointReadiness_NilIngress(t *testing.T) {
	opts := &EndpointReadinessOpts{}
	ready, msg := EvaluateEndpointReadiness(nil, opts)
	assert.False(t, ready)
	assert.Equal(t, msgLaunchURLPending, msg)
}

func TestEvaluateEndpointReadiness_EmptyLoadBalancer(t *testing.T) {
	opts := &EndpointReadinessOpts{}
	ing := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
	ready, msg := EvaluateEndpointReadiness(ing, opts)
	assert.False(t, ready)
	assert.Equal(t, msgLaunchURLPending, msg)
}

func TestEvaluateEndpointReadiness_LoadBalancerReady(t *testing.T) {
	opts := &EndpointReadinessOpts{}
	ing := &networkingv1.Ingress{
		Status: networkingv1.IngressStatus{
			LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{
					{Hostname: "lb.example.com"},
				},
			},
		},
	}
	ready, msg := EvaluateEndpointReadiness(ing, opts)
	assert.True(t, ready)
	assert.Empty(t, msg)
}
