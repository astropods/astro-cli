import type { ReactNode } from "react";
import { SquareTerminal } from "lucide-react";
import { getIntegrationIcon } from "@/lib/integrationIcons";
import { formatRelativeTime, shortBuildId } from "@/lib/deployment-utils";
import { commitUrl } from "@/lib/github-utils";
import { useDeploymentStatus } from "@/api/queries/deployments";
import type { AgentDeployment, DeploymentStatusValue } from "@/lib/api";
import { DeploymentStatusBadge, getDeploymentStatusColors } from "./DeploymentStatusBadge";

export interface DeploymentTileProps {
  /** Primary label — commit message (first line) for GitHub deployments, display name otherwise. */
  name: string;
  /** Whether this is the currently active deployment. */
  active?: boolean;
  /** Live deployment object — used only to derive the deployment id for the
   *  status fetch, and to surface error_message when the badge is in 'error'. */
  deployment?: AgentDeployment;
  /** Deployment source: "github" or "direct". */
  source: "github" | "direct";
  /** Branch name (shown for GitHub deployments). */
  branch?: string;
  /** Short build ID hash. */
  buildId: string;
  /** ISO timestamp of when the deployment was created. */
  deployedAt: string;
  /** GitHub commit SHA (used to build commit link). */
  commitSha?: string;
  /** GitHub repo full name, e.g. "owner/repo" (used to build commit link). */
  repoFullName?: string;
  /** Optional menu (kebab dropdown) rendered in the top-right corner. */
  menu?: ReactNode;
}

export function DeploymentTile({
  name,
  active,
  deployment,
  source,
  branch,
  buildId,
  deployedAt,
  commitSha,
  repoFullName,
  menu,
}: DeploymentTileProps) {
  // Only the currently-active tile fetches status — historical tiles render
  // without a status badge.
  const { data: statusData } = useDeploymentStatus(deployment?.id ?? "", !!active && !!deployment);
  const status: DeploymentStatusValue | null = active && deployment ? statusData?.value ?? null : null;
  const colors = getDeploymentStatusColors(status);
  const shortBuild = shortBuildId(buildId);
  const commitLink = commitUrl(repoFullName, commitSha);
  const errorMessage = status === "error" ? deployment?.error_message : undefined;

  return (
    <div
      className="flex flex-col gap-1.5 rounded border px-3.5 py-3"
      style={{ backgroundColor: colors.bg, borderColor: colors.border }}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate text-body font-medium text-foreground">
          {name}
        </span>
        <div className="flex shrink-0 items-center gap-1.5">
          {status && <DeploymentStatusBadge status={status} />}
          {menu}
        </div>
      </div>
      <div className="flex items-center gap-3 overflow-hidden text-mono-sm text-muted-foreground">
        {commitLink ? (
          <a
            href={commitLink}
            target="_blank"
            rel="noopener noreferrer"
            className="flex min-w-0 shrink items-center gap-1.5 hover:text-foreground"
          >
            <span className="size-3 shrink-0">{getIntegrationIcon("github")}</span>
            <span className="truncate underline decoration-current/20 underline-offset-2">{branch || "GitHub"}</span>
          </a>
        ) : (
          <span className="flex min-w-0 shrink items-center gap-1.5">
            {source === "github" ? (
              <span className="size-3 shrink-0">{getIntegrationIcon("github")}</span>
            ) : (
              <SquareTerminal className="size-3 shrink-0" />
            )}
            <span className="truncate">{source === "github" ? branch || "GitHub" : "ast push"}</span>
          </span>
        )}
        <span className="shrink-0 font-mono">{shortBuild}</span>
        <span className="shrink-0">{formatRelativeTime(deployedAt)}</span>
      </div>
      {errorMessage && (
        <p
          className="mt-1 whitespace-pre-line break-words text-mono-sm"
          style={{ color: colors.badgeText }}
        >
          {errorMessage}
        </p>
      )}
    </div>
  );
}
