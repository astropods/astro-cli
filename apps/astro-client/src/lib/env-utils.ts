// roleFor maps a workload's (component, container_name) to the
// deployment_build_env role string used as the key on WorkloadSpec.env.
// Mirrors the server-side rolesForComponent logic, inverted to pick the
// right role for a specific container.
//
//   - "agent" component + container "messaging" → "messaging"
//   - "agent" component + any other container   → "agent"
//   - "collector" component                     → "collector"
//   - "knowledge-<name>"                        → "knowledge:<name>"
//   - "ingestion-<name>"                        → "ingestion:<name>"
//
// Returns "" for components not represented in deployment_build_env
// (e.g. ad-hoc integration sidecars) — callers should treat that as "no env".
export function roleFor(component: string, containerName: string): string {
  if (component === "agent") {
    return containerName === "messaging" ? "messaging" : "agent";
  }
  if (component === "collector") {
    return "collector";
  }
  if (component.startsWith("knowledge-")) {
    return "knowledge:" + component.slice("knowledge-".length);
  }
  if (component.startsWith("ingestion-")) {
    return "ingestion:" + component.slice("ingestion-".length);
  }
  return "";
}

export function isSensitiveEnvVar(key: string, value: string, source: string): boolean {
  if (source.startsWith("secret:")) return true;

  const upperKey = key.toUpperCase();
  const keyLooksSensitive =
    upperKey.includes("KEY") ||
    upperKey.includes("TOKEN") ||
    upperKey.includes("SECRET") ||
    upperKey.includes("PASSWORD") ||
    upperKey.includes("PASSWD") ||
    upperKey.includes("PRIVATE") ||
    upperKey.includes("CREDENTIAL") ||
    upperKey.includes("AUTH") ||
    upperKey.includes("DSN") ||
    upperKey.includes("WEBHOOK");

  const valueLooksSensitive =
    value.startsWith("sk-") ||
    value.startsWith("secret:") ||
    value.includes("••");

  return keyLooksSensitive || valueLooksSensitive;
}
