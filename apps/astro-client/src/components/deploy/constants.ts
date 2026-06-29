// Mount path for the persistent disk every agent gets by default. Must match
// the server-side default (astro-spec DefaultAgentVolumeMount). Shared by the
// deploy form so the placeholder, helper text, and the provisioning fallback
// can't drift apart.
export const DEFAULT_AGENT_VOLUME_MOUNT = "/data";
