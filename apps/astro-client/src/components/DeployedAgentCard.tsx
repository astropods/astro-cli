import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import { EllipsisHorizontalIcon, ShareIcon, TrashIcon, BookOpenIcon, DocumentDuplicateIcon, CheckIcon, ArrowUpCircleIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { DeploymentStatusBadge } from "@/components/deployed-agent/DeploymentStatusBadge";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { Tag } from "@/components/Tag";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { formatRelativeTime, formatDaysActive } from "@/lib/deployment-utils";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";
import { TradingCardModal } from "@/components/trading-card/TradingCardModal";
import { useBlueprint } from "@/api/queries/blueprints";
import { getBlueprintIntegrations } from "@/lib/blueprint-utils";
import type { CardData } from "astro-trading-card";
import type { AvatarColors } from "@/lib/api";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";

export type DeployedAgentStatus = "active" | "inactive" | "deploying" | "undeploying" | "error" | "restarting" | "pausing" | "resuming";


function formatDateTime(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

export interface DeployedAgentCardProps {
  name: string;
  displayName?: string;
  deploymentId: string;
  account: string;
  href?: string;
  status: DeployedAgentStatus;
  requests: number;
  lastActive: string;
  installedAt: string;
  updatedAt: string;
  hasNewBuildAvailable?: boolean;
  /**
   * Latest build_id available for the agent's lineage. Combined with
   * hasNewBuildAvailable to render the click-to-upgrade affordance.
   */
  latestBuildId?: string;
  currentBuildId?: string;
  /**
   * Path to the deployment's detail page; used by the upgrade affordance to
   * navigate the user there in "configure with new build" mode after
   * confirming. When omitted the upgrade UI is hidden.
   */
  deploymentDetailHref?: string;
  /**
   * Server-supplied reason a deployment is in error/failed. Surfaced as a
   * tooltip on the status badge so dashboard viewers see why a deployment is
   * unhealthy without drilling in.
   */
  errorMessage?: string;
  avatarColors?: AvatarColors;
  className?: string;
  linkState?: Record<string, unknown>;
}

function MetricCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-body-sm text-muted-foreground">{label}</span>
      <span className="text-mono-sm text-foreground tabular-nums">{value}</span>
    </div>
  );
}

export function DeployedAgentCard({
  name,
  displayName,
  deploymentId,
  account,
  href,
  status,
  requests,
  lastActive,
  installedAt,
  updatedAt,
  hasNewBuildAvailable = false,
  latestBuildId,
  currentBuildId,
  deploymentDetailHref,
  errorMessage,
  avatarColors,
  className,
  linkState,
}: DeployedAgentCardProps) {
  const navigate = useNavigate();
  const [menuOpen, setMenuOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const [upgradeOpen, setUpgradeOpen] = useState(false);
  const { copy: copyToClipboard, copied } = useCopyToClipboard(1600);

  const copyId = () => {
    void copyToClipboard(deploymentId);
  };

  // Fetch agent data on demand for integrations (only when share modal is open)
  const { data: agent } = useBlueprint(account, name, { enabled: shareOpen });
  const integrations = agent ? getBlueprintIntegrations(agent) : [];

  const deploymentAvatarBust = useDeploymentAvatarBust(deploymentId);
  const deploymentAvatarUrl = deploymentAvatarBust ?? getDeploymentAvatarUrl(deploymentId);

  const cardData = useMemo<CardData>(() => ({
    name,
    displayName,
    account,
    avatar: { url: deploymentAvatarUrl },
    stats: [
      { label: "Deployed", value: formatDateTime(installedAt) },
      { label: "From", value: `${account}/${name}` },
    ],
    barcodeId: deploymentId,
    qrUrl: `${window.location.origin}/${account}/${name}`,
  }), [name, displayName, account, deploymentAvatarUrl, installedAt, deploymentId]);

  const cardClassName = cn(
    "group relative flex flex-col gap-3 rounded-md border border-stone-400 dark:border-teal-800 bg-white dark:bg-teal-900/30 px-4 py-3 transition-all duration-150",
    href ? "hover:border-teal-500 hover:shadow-md dark:hover:border-teal-700" : "cursor-default opacity-70",
    className,
  );

  const upgradeTarget = deploymentDetailHref ?? href ?? null;
  const upgradeEnabled = hasNewBuildAvailable && !!upgradeTarget && !!latestBuildId;
  const handleUpgradeConfirm = () => {
    if (!upgradeTarget) return;
    setUpgradeOpen(false);
    navigate(upgradeTarget, {
      state: { ...(linkState ?? {}), autoConfigureNewBuild: true },
    });
  };

  const cardContent = (
    <>
      <div className="absolute top-3 right-3" onClick={(e) => { e.preventDefault(); e.stopPropagation(); }}>
        <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="flex h-7 w-7 items-center justify-center rounded-sm text-foreground transition-colors hover:bg-accent"
              aria-label="Agent options"
            >
              <EllipsisHorizontalIcon className="h-4 w-4" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-[180px] rounded-[10px] p-0">
            <DropdownMenuItem asChild className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]">
              <Link to={`/${account}/${name}`} onClick={() => setMenuOpen(false)}>
                <BookOpenIcon className="h-4 w-4" />
                View blueprint
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => { setMenuOpen(false); setShareOpen(true); }} className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]">
              <ShareIcon className="h-4 w-4" />
              Share agent badge
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={copyId} className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]">
              {copied ? <CheckIcon className="h-4 w-4" /> : <DocumentDuplicateIcon className="h-4 w-4" />}
              {copied ? "Copied!" : "Copy deploy ID"}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => { setMenuOpen(false); setDeleteOpen(true); }}
              className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]"
            >
              <TrashIcon className="h-4 w-4" />
              Delete agent
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <div className="flex items-center gap-3">
        <BlueprintIdentity
          account={account}
          name={name}
          size={36}
          url={deploymentAvatarUrl}
          className="h-9 w-9 shrink-0 rounded-sm overflow-hidden"
        />
        <div className="min-w-0 flex-1 pr-6">
          <p className="truncate text-sm font-semibold text-foreground transition-colors group-hover:text-primary dark:group-hover:text-primary-200">
            {displayName || name}
          </p>
          <div className="mt-1 flex items-center gap-2">
            <DeploymentStatusBadge status={status} errorMessage={errorMessage} />
            {hasNewBuildAvailable && (
              upgradeEnabled ? (
                <button
                  type="button"
                  onClick={(e) => { e.preventDefault(); e.stopPropagation(); setUpgradeOpen(true); }}
                  className="rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  aria-label="Upgrade to newest build"
                >
                  <Tag color="yellow" className="gap-1 cursor-pointer hover:brightness-105">
                    <ArrowUpCircleIcon className="size-3 shrink-0" />
                    Update available
                  </Tag>
                </button>
              ) : (
                <Tag color="yellow" className="gap-1">
                  <ArrowUpCircleIcon className="size-3 shrink-0" />
                  Update available
                </Tag>
              )
            )}
          </div>
        </div>
      </div>

      <div className="mt-1 grid grid-cols-2 gap-x-4 gap-y-3">
        <MetricCell label="Requests" value={requests.toLocaleString()} />
        <MetricCell label="Last request" value={lastActive} />
        <MetricCell label="Days active" value={formatDaysActive(installedAt)} />
        <MetricCell label="Updated" value={formatRelativeTime(updatedAt)} />
      </div>
    </>
  );

  return (
    <>
      {href ? (
        <Link to={href} state={linkState} className={cardClassName}>{cardContent}</Link>
      ) : (
        <div className={cardClassName}>{cardContent}</div>
      )}


      <DeleteDeploymentDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        deploymentId={deploymentId}
        deploymentName={name}
        displayName={displayName}
        account={account}
      />

      <TradingCardModal
        open={shareOpen}
        onOpenChange={setShareOpen}
        data={cardData}
        avatarColors={avatarColors}
        integrations={integrations}
      />

      <Dialog open={upgradeOpen} onOpenChange={setUpgradeOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Upgrade to newest build?</DialogTitle>
            <DialogDescription>
              {currentBuildId && latestBuildId ? (
                <>
                  This deployment will be configured with build{' '}
                  <span className="font-mono">{latestBuildId.slice(0, 8)}</span>{' '}(currently on{' '}
                  <span className="font-mono">{currentBuildId.slice(0, 8)}</span>). You&apos;ll have a
                  chance to review variables before the deploy starts.
                </>
              ) : (
                <>You&apos;ll have a chance to review variables before the deploy starts.</>
              )}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setUpgradeOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleUpgradeConfirm}>Continue</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
