package clusterid

type Resolver struct {
	primary string
}

func New(primaryClusterID string) Resolver {
	return Resolver{primary: primaryClusterID}
}

func (r Resolver) Primary() string { return r.primary }

func (r Resolver) Canonical(clusterID string) string {
	if clusterID == "" {
		return r.primary
	}
	return clusterID
}

func (r Resolver) Same(clusterIDA, clusterIDB string) bool {
	return r.Canonical(clusterIDA) == r.Canonical(clusterIDB)
}

func (r Resolver) IsPrimary(clusterID string) bool {
	return r.Canonical(clusterID) == r.primary
}

func (r Resolver) Label(clusterID string) string {
	if id := r.Canonical(clusterID); id != "" {
		return id
	}
	return "unrecorded"
}
