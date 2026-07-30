import {
  useDeploymentRuntime,
  useDeploymentStatus,
} from "@/api/queries/deployments";
import {
  deriveChatComposerState,
  type ChatComposerState,
} from "@/lib/deployment-utils";

/**
 * Reachability gate for the deployment chat proxy routes (chat conversations,
 * agent/config). Those forward through astro-server to the messaging sidecar,
 * which only answers when the deployment is active AND messaging_reachable
 * (Service present + sidecar container ready, the runtime read-model's observed
 * signal). Firing them at a stopped, paused, deploying, or unreachable agent 5xxs
 * the per-route proxy and trips AstroServerHigh5xxRateByRoute, so dependent reads
 * gate on `ready` the same way the inspector's Settings tab does.
 *
 * `resolved` guards the initial load window: status and runtime are optimistically
 * absent at first, so callers must wait until both have *settled* before trusting
 * the derived state; otherwise a fresh mount fires against a possibly-unreachable
 * agent. A runtime *error* counts as settled: the runtime read is DB-backed and
 * cluster-independent, so it won't 503 on a briefly unreachable cluster.
 */
export function useDeploymentChatReadiness(deploymentId: string): {
  state: ChatComposerState;
  resolved: boolean;
  ready: boolean;
} {
  const { data: status } = useDeploymentStatus(deploymentId);
  const { data: runtimeData, isError: runtimeError } =
    useDeploymentRuntime(deploymentId);
  const state = deriveChatComposerState(status, runtimeData?.runtime);
  const resolved = !!status && (!!runtimeData || runtimeError);
  return { state, resolved, ready: resolved && state === "ready" };
}
