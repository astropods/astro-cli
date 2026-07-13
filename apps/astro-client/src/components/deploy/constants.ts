// Mount path for the persistent disk every agent gets by default. Must match
// the server-side default (astro-spec DefaultAgentVolumeMount). Shared by the
// deploy form so the placeholder, helper text, and the provisioning fallback
// can't drift apart.
export const DEFAULT_AGENT_VOLUME_MOUNT = "/data";

// Keep in sync with deploymentDisplayNameMaxLength in
// apps/astro-server/handlers/deploy.go.
export const DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH = 42;
