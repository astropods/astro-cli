// Package clusterfields defines required per-cluster deploy configuration
// fields shared by clusterstore (write-time) and clustercfg (deploy-time).
package clusterfields

// DeployConfig is the ingress / Langfuse / netpol slice of a
// clusters row or k8s.ClusterEntry. Every field must be non-empty on
// additional clusters; the primary cluster has no row and reads env vars.
type DeployConfig struct {
	AgentIngressDomain     string
	IngestionIngressDomain string
	LangfuseBaseURLExt     string
	LangfuseVPCEIPs        string
	PodSubnetCIDRs         string
}

// Registration adds EKS identity fields required only at clusterstore
// Register/Update time (not checked again at deploy via clustercfg).
type Registration struct {
	Region             string
	EKSClusterName     string
	EKSClusterEndpoint string
	Deploy             DeployConfig
}
