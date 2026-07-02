import type { ReactNode } from "react";
import { Loader2, SquareTerminal } from "lucide-react";
import { getIntegrationIcon } from "@/lib/integrationIcons";
import { formatRelativeTime, shortBuildId } from "@/lib/deployment-utils";
import { commitUrl } from "@/lib/github-utils";
import { useDeploymentStatus } from "@/api/queries/deployments";
import type { AgentDeployment, DeploymentStatusValue } from "@/lib/api";

type StatusColor = { bg: string; border: string; badgeBg: string; badgeText: string };

const SUCCESS_COLORS: StatusColor = {
  bg: "color-mix(in oklch, var(--success) 12%, transparent)",
  border: "color-mix(in oklch, var(--success) 25%, transparent)",
  badgeBg: "color-mix(in oklch, var(--success) 20%, transparent)",
  badgeText: "var(--success)",
};
export const WARNING_COLORS: StatusColor = {
  bg: "color-mix(in oklch, var(--warning) 12%, transparent)",
  border: "color-mix(in oklch, var(--warning) 25%, transparent)",
  badgeBg: "color-mix(in oklch, var(--warning) 22%, transparent)",
  badgeText: "var(--warning)",
};
const ERROR_COLORS: StatusColor = {
  bg: "color-mix(in oklch, var(--error) 12%, transparent)",
  border: "color-mix(in oklch, var(--error) 25%, transparent)",
  badgeBg: "color-mix(in oklch, var(--error) 20%, transparent)",
  badgeText: "var(--error)",
};
const NEUTRAL_COLORS: StatusColor = {
  bg: "var(--color-muted)",
  border: "var(--color-border)",
  badgeBg: "var(--color-muted)",
  badgeText: "var(--color-muted-foreground)",
};

const STATUS_COLORS: Record<DeploymentStatusValue, StatusColor> = {
  active:      SUCCESS_COLORS,
  deploying:   WARNING_COLORS,
  error:       ERROR_COLORS,
  undeploying: NEUTRAL_COLORS,
  inactive:    NEUTRAL_COLORS,
};

const STATUS_LABELS: Record<DeploymentStatusValue, string> = {
  active: "Active",
  deploying: "Deploying",
  error: "Error",
  undeploying: "Undeploying",
  inactive: "Inactive",
};

const SPINNING_STATUSES: ReadonlySet<DeploymentStatusValue> = new Set(["deploying", "undeploying"]);

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
  const colors = status ? (STATUS_COLORS[status] ?? NEUTRAL_COLORS) : NEUTRAL_COLORS;
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
          {status && (
            <span
              className="flex items-center gap-1.5 rounded-full px-2 py-0.5 text-mono-sm font-medium"
              style={{ backgroundColor: colors.badgeBg, color: colors.badgeText }}
            >
              {SPINNING_STATUSES.has(status) && <Loader2 className="size-3 animate-spin" />}
              {STATUS_LABELS[status]}
            </span>
          )}
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
