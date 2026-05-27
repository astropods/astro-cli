package k8s

import (
	networkingv1 "k8s.io/api/networking/v1"
)

const msgLaunchURLPending = "Launch is unavailable while we create your custom URL"

type EndpointReadinessOpts struct{}

func EvaluateEndpointReadiness(ingress *networkingv1.Ingress, opts *EndpointReadinessOpts) (ready bool, message string) {
	if opts == nil {
		return true, ""
	}
	if ingress == nil || len(ingress.Status.LoadBalancer.Ingress) == 0 {
		return false, msgLaunchURLPending
	}
	return true, ""
}
