import { Fragment, useEffect, useMemo, useState, type ReactNode } from "react";
import { useNavigate } from "react-router";
import { ArrowUp, Ban, ChevronUp, ChevronDown, EllipsisVertical, RotateCw, Rocket, Pause, Play, History, Copy, Check, Loader2 } from "lucide-react";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import {
  useCancelDeployment,
  useDeploymentHistory,
  useDeploymentStatus,
  useRestartDeployment,
  useStopDeployment,
  useWakeUpDeployment,
} from "@/api/queries/deployments";
import { useAccountMembers } from "@/api/queries/accounts";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { useGitHubStatus } from "@/api/queries/github";
import { getIntegrationIcon } from "@/lib/integrationIcons";
import { hasNewerBuild, isPausedState } from "@/lib/deployment-utils";
import { commitTitle, commitUrl, shortSha } from "@/lib/github-utils";
import type { AgentDeployment, DeploymentHistoryRecord, GitHubBuild } from "@/lib/api";
import { DeploymentTile, DeploymentSourceLine } from "./DeploymentTile";
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
  /** Show a "Latest build" badge when there's nothing newer to deploy. */
  onLatestBuild?: boolean;
  children?: ReactNode;
}

export function DeploymentHistoryPanelContent({
  expanded,
  onToggleExpanded,
  onLatestBuild,
  children,
}: DeploymentHistoryPanelContentProps) {
  return (
    <div className="flex h-full w-full flex-col rounded-md border border-border bg-card">
      <div className="flex items-center justify-between px-5 py-4">
        <div className="flex items-center gap-2">
          <h2 className="text-lg font-normal text-foreground">Deployment History</h2>
          {onLatestBuild && (
            // Reassures the user they're current after an upgrade nudge clears,
            // rather than the card just disappearing (issue #1627 design).
            <span className="flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-mono-sm text-muted-foreground">
              <Check className="size-3 text-success" />
              Latest build
            </span>
          )}
        </div>
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
  const cancelMutation = useCancelDeployment(account);
  const { data: statusData } = useDeploymentStatus(deployment.id);
  const deploying = statusData?.value === "deploying";

  const { copy, copied } = useCopyToClipboard();
  const paused = isPausedState(deployment);
  // Recovery actions stay enabled even mid-deploy so a stuck deploy never leaves
  // the user without recourse (issue #1584). The stuck state is surfaced on the
  // page banner, not here.
  const busy = restartMutation.isPending || stopMutation.isPending || wakeupMutation.isPending || cancelMutation.isPending;

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
            {copied ? <Check className="size-4 text-foreground-accent" /> : <Copy className="size-4" />}
            {copied ? "Copied!" : "Copy deploy ID"}
          </DropdownMenuItem>
          {deploying ? (
            <DropdownMenuItem
              disabled={busy}
              onClick={() => cancelMutation.mutate({ deploymentId: deployment.id })}
            >
              <Ban className="size-4" />
              Cancel deployment
            </DropdownMenuItem>
          ) : paused ? (
            <DropdownMenuItem
              disabled={busy}
              onClick={() => wakeupMutation.mutate({ deploymentId: deployment.id })}
            >
              <Play className="size-4" />
              Resume
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem
              disabled={busy}
              onClick={() => stopMutation.mutate({ deploymentId: deployment.id })}
            >
              <Pause className="size-4" />
              Pause
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            disabled={busy}
            onClick={() => navigate("../configure", { relative: "path" })}
          >
            <Rocket className="size-4" />
            Redeploy
          </DropdownMenuItem>
          <DropdownMenuItem
            variant="destructive"
            disabled={paused || busy}
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
  branch,
}: {
  currentBuildId: string;
  latestBuildId: string;
  commitMessage?: string;
  commitSha?: string;
  repoFullName?: string;
  branch?: string;
}) {
  const navigate = useNavigate();
  // Prefer the target build's commit message (first line) so it's clear what the
  // upgrade brings; fall back to the build-id transition for direct CLI pushes.
  const summary = commitTitle(commitMessage);

  return (
    <div
      className="flex w-full items-center justify-between gap-3 rounded border border-indigo-600/30 bg-indigo-500/15 px-3.5 py-2.5 dark:border-indigo-500/20 dark:bg-indigo-500/18"
    >
      <div className="min-w-0">
        {/* Eyebrow keeps the "new build available" label above the commit so it
            isn't lost once the metadata line is shown (issue #1627 design). */}
        <p className="text-[11px] font-semibold text-indigo-700/80 dark:text-indigo-300/80">
          New build available
        </p>
        <p className="mt-0.5 truncate text-mono-sm font-medium text-indigo-950 dark:text-indigo-100">
          {summary || `${currentBuildId.slice(0, 8)} → ${latestBuildId.slice(0, 8)}`}
        </p>
        {/* Same source line as the active tile so branch + build id read identically (#1629). */}
        <div className="mt-1">
          <DeploymentSourceLine
            source="github"
            branch={branch}
            buildId={latestBuildId}
            commitSha={commitSha}
            repoFullName={repoFullName}
          />
        </div>
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

// Rotating status text while building, so the card reads as live and distinct
// from the static active tile below it (issue #1627 design feedback).
const BUILD_PHASES = ["Pushing new build", "Building image", "Almost there"];

function useBuildPhase(active: boolean): string {
  const [index, setIndex] = useState(0);
  useEffect(() => {
    if (!active) return;
    const id = setInterval(() => setIndex((n) => (n + 1) % BUILD_PHASES.length), 2500);
    return () => clearInterval(id);
  }, [active]);
  return BUILD_PHASES[index];
}

// Slate card with a slight indigo pulse (Taylor's build-state design): reads as
// running without competing with the amber deploying / green active states.
const BUILD_CARD = {
  bg: "color-mix(in oklch, var(--color-slate-500) 10%, transparent)",
  border: "color-mix(in oklch, var(--color-slate-500) 24%, transparent)",
  shimmer: "color-mix(in oklch, var(--color-indigo-500) 26%, transparent)",
};

export function BuildInProgressNudge({
  build,
  repoFullName,
}: {
  build: GitHubBuild;
  repoFullName?: string;
}) {
  const preparing = build.status === "pending";
  const title =
    commitTitle(build.commit_message) ||
    (preparing ? "Preparing build" : "Build in progress");
  const sha = shortSha(build.commit_sha);
  const commitLink = commitUrl(repoFullName, build.commit_sha);
  const phase = useBuildPhase(!preparing);
  const statusText = preparing ? "Preparing build" : phase;

  return (
    <div
      className="relative flex items-center justify-between gap-3 overflow-hidden rounded border px-3.5 py-3"
      style={{ backgroundColor: BUILD_CARD.bg, borderColor: BUILD_CARD.border }}
    >
      {/* An indigo band sweeps across (slides + fades) so the card reads as
          actively running, distinct from the static active tile below it. */}
      <span
        aria-hidden
        className="dp-build-shimmer pointer-events-none absolute inset-0"
        style={{ background: `linear-gradient(100deg, transparent 35%, ${BUILD_CARD.shimmer}, transparent 65%)` }}
      />
      <div className="relative flex min-w-0 flex-col gap-1.5">
        <span className="min-w-0 truncate text-body font-medium text-foreground">{title}</span>
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
      <span className="relative flex shrink-0 items-center gap-1.5 text-mono-sm font-medium text-indigo-600 dark:text-indigo-400">
        {statusText}
        <Loader2 className="size-3.5 animate-spin" />
      </span>
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
  const { data: membersData } = useAccountMembers(account, {
    enabled:
      !!deployment.deployed_by ||
      allRecords.some((record) => !!record.deployed_by),
  });
  const deployersByUserID = useMemo(() => {
    const deployers = new Map<string, { name: string; handle: string; avatarUrl?: string }>();
    for (const member of membersData?.members ?? []) {
      if (!member.username) continue;
      deployers.set(member.user_id, {
        name: member.display_name || member.username,
        handle: member.username,
        avatarUrl: member.avatar_url,
      });
    }
    return deployers;
  }, [membersData]);

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
    // Baseline poll so a newly pushed build appears without a manual refresh (#1627).
    refetchInterval: 15_000,
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
  // Prefer the newest finished build from the polling GitHub status so a build
  // that just completed becomes the upgrade target without a refresh (#1627).
  const githubUpgrade = useMemo(() => {
    const finished = githubStatus?.builds?.find((b) => b.status === "registered");
    if (!finished || finished.build_id === deployment.build_id) return null;
    return {
      buildId: finished.build_id,
      commitMessage: finished.commit_message,
      commitSha: finished.commit_sha,
      repoFullName: githubStatus?.repo_full_name,
      branch: finished.branch,
    };
  }, [githubStatus, deployment.build_id]);

  // Fallback to the server's latest_build_id (shared authority; omitted for
  // cross-account private blueprints), enriched with commit metadata when the
  // blueprint versions are readable.
  const blueprintUpgrade = useMemo(() => {
    if (!hasNewerBuild(deployment)) return null;
    const latest = sourceBlueprint?.versions?.find(
      (version) => version.build_id === deployment.latest_build_id,
    );
    return {
      buildId: deployment.latest_build_id!,
      commitMessage: latest?.commit_message,
      commitSha: latest?.commit_sha,
      repoFullName: latest?.repo_full_name,
      // Blueprint versions carry no branch; the active record's is the right
      // proxy since the upgrade is the same lineage (#1629).
      branch: currentRecord?.branch,
    };
  }, [sourceBlueprint, deployment, currentRecord?.branch]);

  const upgrade = githubUpgrade ?? blueprintUpgrade;

  // Once the build state is known and there's nothing newer to deploy, mark the
  // panel "on latest build" so the header doesn't just fall silent when the
  // build / upgrade nudges clear (issue #1627 design).
  const onLatestBuild =
    currentRecord?.source === "github" &&
    sourceReadable &&
    (!!githubStatus || (sourceBlueprint?.versions?.length ?? 0) > 0) &&
    !activeBuild &&
    !upgrade &&
    !deploying;

  return (
    <DeploymentHistoryPanelContent
      expanded={expanded}
      onToggleExpanded={allRecords.length > 1 ? onToggleExpanded : undefined}
      onLatestBuild={onLatestBuild}
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
              branch={upgrade.branch}
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
            deployedBy={deployersByUserID.get(
              record.deployed_by ||
                (record.is_current ? deployment.deployed_by || "" : ""),
            )}
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
