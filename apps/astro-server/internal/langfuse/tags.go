package langfuse

import "slices"

// HasDeploymentTag reports whether a trace belongs to the deployment.
func HasDeploymentTag(tags []string, deploymentID string) bool {
	return slices.Contains(tags, "deployment:"+deploymentID)
}
