import { Fragment, useMemo, useState, type ReactNode } from "react";
import { useNavigate } from "react-router";
import { ArrowUp, ChevronUp, ChevronDown, EllipsisVertical, RotateCw, Rocket, Pause, Play, History, Copy, Check, Loader2 } from "lucide-react";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import {
  useDeploymentHistory,
  useDeploymentStatus,
  useRestartDeployment,
  useStopDeployment,
  useWakeUpDeployment,
} from "@/api/queries/deployments";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { useGitHubStatus } from "@/api/queries/github";
import { getIntegrationIcon } from "@/lib/integrationIcons";
import { isPausedState } from "@/lib/deployment-utils";
import { commitTitle, commitUrl, shortSha } from "@/lib/github-utils";
import type { AgentDeployment, DeploymentHistoryRecord, GitHubBuild } from "@/lib/api";
import { DeploymentTile, INFO_COLORS } from "./DeploymentTile";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

// ---------------------------------------------------------------------------
// Presentational shell — no data fetching, fully storybook-friendly
// ---------------------------------------------------------------------------

export interface DeploymentHistoryPanelContentProps {
  /** Whether the panel is expanded to full height. */
  expanded?: boolean;
  /** Callback to toggle expanded state. */
  onToggleExpanded?: () => void;
  children?: ReactNode;
}

export function DeploymentHistoryPanelContent({
  expanded,
  onToggleExpanded,
  children,
}: DeploymentHistoryPanelContentProps) {
  return (
    <div className="flex h-full w-full flex-col rounded-md border border-border bg-card dark:bg-surface">
      <div className="flex items-center justify-between px-5 py-4">
        <h2 className="text-lg font-normal text-foreground">Deployment History</h2>
        {onToggleExpanded && (
          <button
            onClick={onToggleExpanded}
            className="flex items-center gap-1 rounded px-2 py-1 text-mono-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            {expanded ? (
              <>Collapse <ChevronDown className="size-3.5" /></>
            ) : (
              <>View all <ChevronUp className="size-3.5" /></>
            )}
          </button>
        )}
      </div>
      <div className="flex flex-1 flex-col gap-2 overflow-y-auto px-3 pb-3">
        {children}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tile kebab menu — active deployment
// ---------------------------------------------------------------------------

function ActiveTileMenu({
  account,
  deployment,
}: {
  account: string;
  deployment: AgentDeployment;
}) {
  const navigate = useNavigate();
  const [restartOpen, setRestartOpen] = useState(false);
  const restartMutation = useRestartDeployment(account);
  const stopMutation = useStopDeployment(account);
  const wakeupMutation = useWakeUpDeployment(account);
  const { data: statusData } = useDeploymentStatus(deployment.id);
  const status = statusData?.value;

  const { copy, copied } = useCopyToClipboard();
  const paused = isPausedState(deployment);
  const deploying = status === "deploying" || status === "undeploying";
  const busy = restartMutation.isPending || stopMutation.isPending || wakeupMutation.isPending;

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            aria-label="Deployment actions"
            className="rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground"
          >
            <EllipsisVertical className="size-3.5" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={() => void copy(deployment.id)}>
            {copied ? <Check className="size-4 text-primary" /> : <Copy className="size-4" />}
            {copied ? "Copied!" : "Copy deploy ID"}
          </DropdownMenuItem>
          {paused ? (
            <DropdownMenuItem
              disabled={deploying || busy}
              onClick={() => wakeupMutation.mutate({ deploymentId: deployment.id })}
            >
              <Play className="size-4" />
              Resume
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem
              disabled={deploying || busy}
              onClick={() => stopMutation.mutate({ deploymentId: deployment.id })}
            >
              <Pause className="size-4" />
              Pause
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            disabled={deploying || busy}
            onClick={() => navigate("../configure", { relative: "path" })}
          >
            <Rocket className="size-4" />
            Redeploy
          </DropdownMenuItem>
          <DropdownMenuItem
            variant="destructive"
            disabled={paused || deploying || busy}
            onClick={() => setRestartOpen(true)}
          >
            <RotateCw className="size-4" />
            Restart
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={restartOpen} onOpenChange={setRestartOpen}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Restart deployment?</DialogTitle>
            <DialogDescription>
              All running containers will be restarted. There may be a brief interruption.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestartOpen(false)}>Cancel</Button>
            <Button
              variant="destructive"
              onClick={() => {
                setRestartOpen(false);
                restartMutation.mutate({ deploymentId: deployment.id });
              }}
            >
              Restart
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

// ---------------------------------------------------------------------------
// Tile kebab menu — historical deployment (rollback)
// ---------------------------------------------------------------------------

function HistoryTileMenu({ revision, buildId }: { revision: number; buildId: string }) {
  const navigate = useNavigate();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          aria-label="Revision actions"
          className="rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground"
        >
          <EllipsisVertical className="size-3.5" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          onClick={() =>
            navigate(`../configure?revision=${revision}&build=${encodeURIComponent(buildId)}`, {
              relative: "path",
            })
          }
        >
          <History className="size-4" />
          Rollback to this revision
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

// ---------------------------------------------------------------------------
// Upgrade nudge — shown below active tile when a newer build exists
// ---------------------------------------------------------------------------

export function UpgradeNudge({
  currentBuildId,
  latestBuildId,
  commitMessage,
  commitSha,
  repoFullName,
}: {
  currentBuildId: string;
  latestBuildId: string;
  commitMessage?: string;
  commitSha?: string;
  repoFullName?: string;
}) {
  const navigate = useNavigate();
  // Prefer the target build's commit message (first line) so it's clear what the
  // upgrade brings; fall back to the build-id transition for direct CLI pushes.
  const summary = commitTitle(commitMessage);
  const sha = shortSha(commitSha);
  const commitLink = commitUrl(repoFullName, commitSha);

  return (
    <div
      className="flex w-full items-center justify-between gap-3 rounded border border-indigo-600/30 bg-indigo-500/15 px-3.5 py-2.5 dark:border-indigo-500/20 dark:bg-indigo-500/18"
    >
      <div className="min-w-0">
        <p className="text-mono-sm font-medium text-indigo-950 dark:text-indigo-100">New build available</p>
        <p className="mt-0.5 truncate text-mono-sm text-indigo-950/70 dark:text-indigo-100/60">
          {summary || `${currentBuildId.slice(0, 8)} → ${latestBuildId.slice(0, 8)}`}
        </p>
        {sha && (
          <div className="mt-1 flex items-center gap-1.5 overflow-hidden text-mono-sm text-muted-foreground">
            <span className="size-3 shrink-0">{getIntegrationIcon("github")}</span>
            {commitLink ? (
              <a
                href={commitLink}
                target="_blank"
                rel="noopener noreferrer"
                className="truncate font-mono underline decoration-current/20 underline-offset-2 hover:text-foreground"
              >
                {sha}
              </a>
            ) : (
              <span className="truncate font-mono">{sha}</span>
            )}
          </div>
        )}
      </div>
      <Button
        size="xs"
        className="shrink-0"
        onClick={() =>
          navigate(`../configure?build=${encodeURIComponent(latestBuildId)}`, { relative: "path" })
        }
      >
        <ArrowUp className="size-3" />
        Update
      </Button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Build-in-progress nudge — shown above active tile while a build is in flight
// ---------------------------------------------------------------------------

export function BuildInProgressNudge({
  build,
  repoFullName,
}: {
  build: GitHubBuild;
  repoFullName?: string;
}) {
  const preparing = build.status === "pending";
  const label = preparing ? "Preparing" : "Building";
  const title =
    commitTitle(build.commit_message) ||
    (preparing ? "Preparing build" : "Build in progress");
  const sha = shortSha(build.commit_sha);
  const commitLink = commitUrl(repoFullName, build.commit_sha);

  return (
    <div
      className="flex flex-col gap-1.5 rounded border px-3.5 py-3"
      style={{ backgroundColor: INFO_COLORS.bg, borderColor: INFO_COLORS.border }}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate text-body font-medium text-foreground">{title}</span>
        <span
          className="flex shrink-0 items-center gap-1.5 rounded-full px-2 py-0.5 text-mono-sm font-medium"
          style={{ backgroundColor: INFO_COLORS.badgeBg, color: INFO_COLORS.badgeText }}
        >
          <Loader2 className="size-3 animate-spin" />
          {label}
        </span>
      </div>
      {sha && (
        <div className="flex items-center gap-3 overflow-hidden text-mono-sm text-muted-foreground">
          <span className="flex min-w-0 items-center gap-1.5">
            <span className="size-3 shrink-0">{getIntegrationIcon("github")}</span>
            {build.branch && <span className="truncate">{build.branch}</span>}
          </span>
          {commitLink ? (
            <a
              href={commitLink}
              target="_blank"
              rel="noopener noreferrer"
              className="shrink-0 font-mono underline decoration-current/20 underline-offset-2 hover:text-foreground"
            >
              {sha}
            </a>
          ) : (
            <span className="shrink-0 font-mono">{sha}</span>
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Connected component — fetches data and renders tiles
// ---------------------------------------------------------------------------

interface DeploymentHistoryPanelProps {
  account: string;
  agentName: string;
  deploymentId: string;
  deployment: AgentDeployment;
  expanded?: boolean;
  onToggleExpanded?: () => void;
}

export function DeploymentHistoryPanel({
  account,
  agentName,
  deploymentId,
  deployment,
  expanded,
  onToggleExpanded,
}: DeploymentHistoryPanelProps) {

  const { data } = useDeploymentHistory(account, agentName, deploymentId);
  const allRecords = data?.deployments ?? [];

  // Collapsed: only show the active deployment
  const records = expanded ? allRecords : allRecords.filter((r) => r.is_current);
  const currentRecord = allRecords.find((r) => r.is_current);

  // Upgrade detection — compare deployed build against latest published build
  const sourceAccount = deployment.source_account || account;
  const { data: blueprintsData } = useAccountBlueprints(sourceAccount);
  const sourceBlueprint = blueprintsData?.agents?.find((a) => a.name === agentName);

  // The source account's GitHub status (and published builds) are only readable
  // when it's our own account or the blueprint is public. Mirror the upgrade
  // guard below so a cross-account private blueprint doesn't fire a request that
  // predictably fails.
  const sourceReadable =
    sourceAccount === account ||
    (!!sourceBlueprint && sourceBlueprint.visibility !== "private");

  // Build-in-progress detection — surface an in-flight GitHub build above the
  // current deploy. The query self-polls while builds[0] is pending/building.
  const { data: githubStatus } = useGitHubStatus(sourceAccount, agentName, {
    enabled: currentRecord?.source === "github" && sourceReadable,
  });
  const activeBuild = useMemo(() => {
    const latest = githubStatus?.builds?.[0];
    if (!latest || (latest.status !== "pending" && latest.status !== "building")) return null;
    return latest;
  }, [githubStatus]);
  // Deploy runs after the build, so the build-in-progress card and the tile's
  // live "Deploying" status are sequential phases, not concurrent. While a
  // deploy/undeploy is live the tile already shows that phase, so suppress the
  // build card to avoid stacking two in-progress indicators in this small panel.
  const { data: statusData } = useDeploymentStatus(deployment.id);
  const deploying = statusData?.value === "deploying" || statusData?.value === "undeploying";
  const upgrade = useMemo(() => {
    if (!sourceBlueprint?.versions?.length) return null;
    const latest = sourceBlueprint.versions.reduce((best, cur) =>
      new Date(cur.published_at).getTime() > new Date(best.published_at).getTime() ? cur : best,
    );
    // Only show upgrade if the source account matches or the blueprint is public
    if (sourceAccount !== account && sourceBlueprint.visibility === "private") return null;
    if (latest.build_id === deployment.build_id) return null;
    return {
      buildId: latest.build_id,
      commitMessage: latest.commit_message,
      commitSha: latest.commit_sha,
      repoFullName: latest.repo_full_name,
    };
  }, [sourceBlueprint, sourceAccount, account, deployment.build_id]);

  return (
    <DeploymentHistoryPanelContent
      expanded={expanded}
      onToggleExpanded={allRecords.length > 1 ? onToggleExpanded : undefined}
    >
      {records.map((record) => (
        <Fragment key={`${record.id}-${record.revision}`}>
          {record.is_current && activeBuild && !deploying && (
            <BuildInProgressNudge build={activeBuild} repoFullName={githubStatus?.repo_full_name} />
          )}
          {record.is_current && upgrade && (
            <UpgradeNudge
              currentBuildId={deployment.build_id}
              latestBuildId={upgrade.buildId}
              commitMessage={upgrade.commitMessage}
              commitSha={upgrade.commitSha}
              repoFullName={upgrade.repoFullName}
            />
          )}
          <DeploymentTile
            name={tileDisplayName(record)}
            active={record.is_current}
            deployment={record.is_current ? deployment : undefined}
            source={record.source}
            branch={record.branch}
            buildId={record.build_id}
            deployedAt={record.deployed_at}
            commitSha={record.commit_sha}
            repoFullName={record.repo_full_name}
            menu={
              record.is_current
                ? <ActiveTileMenu account={account} deployment={deployment} />
                : <HistoryTileMenu revision={record.revision} buildId={record.build_id} />
            }
          />
        </Fragment>
      ))}
    </DeploymentHistoryPanelContent>
  );
}

function tileDisplayName(record: DeploymentHistoryRecord): string {
  if (record.source === "github") {
    const title = commitTitle(record.commit_message);
    if (title) return title;
  }
  return record.display_name || record.agent_name;
}
