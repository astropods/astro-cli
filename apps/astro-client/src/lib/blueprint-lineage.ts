import type { AgentDeployment } from "@/lib/api";

type BlueprintLike = { versions?: { build_id: string }[] };

/*
 * The dashboard and deployment-detail views match a deployment to a blueprint
 * by name string only. A name match alone is not lineage proof: pruning,
 * republishing under a different account, or a name collision can all leave
 * the matched blueprint with a version list that does not contain the
 * deployment's pinned build. Comparing latest-vs-pinned in that case offers
 * an upgrade the server cannot honor (Redeploy returns "build not found").
 *
 * isDeploymentLineageMatch returns true only when the deployment's build_id
 * is actually present in the blueprint's versions, which is the minimum
 * evidence that the two share a build history.
 */
export const isDeploymentLineageMatch = (
  deployment: Pick<AgentDeployment, "build_id">,
  blueprint: BlueprintLike | undefined,
): boolean => {
  if (!blueprint?.versions?.length) return false;
  return blueprint.versions.some((v) => v.build_id === deployment.build_id);
};
