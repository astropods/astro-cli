package deployment

// Project filters resolved rows to a single role and splits them into
// ConfigMap data (non-secret) and Secret data (secret) suitable for
// materialising as Kubernetes objects. Caller passes plaintext rows
// (already decrypted from deployment_build_env). The applier mounts
// the resulting ConfigMap + Secret via envFrom on the role's container.
//
// This is the only place the is_secret bit drives a routing decision —
// the schema makes it authoritative; the projector is mechanical.
func Project(rows []Resolution, role Role) (cmData, secData map[string]string) {
	cmData = map[string]string{}
	secData = map[string]string{}
	for _, r := range rows {
		if r.Role != role {
			continue
		}
		if r.IsSecret {
			secData[r.EnvName] = r.Value
		} else {
			cmData[r.EnvName] = r.Value
		}
	}
	return cmData, secData
}

// RolesIn returns the distinct set of roles present in rows, in the
// order they first appear. The applier iterates this to figure out
// which (ConfigMap, Secret) pairs to write per deployment.
func RolesIn(rows []Resolution) []Role {
	seen := map[Role]bool{}
	var out []Role
	for _, r := range rows {
		if seen[r.Role] {
			continue
		}
		seen[r.Role] = true
		out = append(out, r.Role)
	}
	return out
}
