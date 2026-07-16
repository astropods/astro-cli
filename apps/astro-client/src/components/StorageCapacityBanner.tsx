/** Warns when a deployment's file/persistent volume is approaching full, so
 *  users notice before uploads (and chat history persistence) start failing.
 *  Driven off the sidecar's statfs-based usage reading, so it works both in
 *  cluster deployments and local `ast dev`. Renders nothing until usage crosses
 *  the warning threshold, and auto-clears once space is freed. */
import { useDeploymentStorageUsage } from "@/api/queries/files";
import { ErrorPanel, WarningPanel } from "@/components/ui/status-panel";
import { formatBytes } from "@/lib/format-utils";

// Below WARN we stay silent; between WARN and CRITICAL a soft warning; at/above
// CRITICAL a stronger error-toned banner since writes are about to fail.
const WARN_THRESHOLD = 85;
const CRITICAL_THRESHOLD = 95;

export function StorageCapacityBanner({
  deploymentId,
  className,
}: {
  deploymentId: string;
  className?: string;
}) {
  const { data } = useDeploymentStorageUsage(deploymentId);

  if (!data?.available || data.percent_used < WARN_THRESHOLD) {
    return null;
  }

  const critical = data.percent_used >= CRITICAL_THRESHOLD;
  const pct = Math.round(data.percent_used);
  const usage = `${formatBytes(data.used_bytes)} of ${formatBytes(data.total_bytes)} used`;
  const Panel = critical ? ErrorPanel : WarningPanel;

  return (
    <div className={className}>
      <Panel
        title={critical ? `Storage almost full (${pct}%)` : `Storage ${pct}% full`}
        variant="inline"
      >
        {critical
          ? `${usage}. New uploads will start failing — delete files to free space.`
          : `${usage}. Uploads fail once the volume is full; consider deleting files you no longer need.`}
      </Panel>
    </div>
  );
}
