import { useMemo, useState } from "react";
import { Link } from "react-router";
import { EllipsisHorizontalIcon, ShareIcon, TrashIcon, BookOpenIcon, DocumentDuplicateIcon, CheckIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { StatusIndicator } from "@/components/StatusIndicator";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { InlineBadge } from "@/components/InlineBadge";
import { deploymentStatusVariant, deploymentStatusLabel } from "@/lib/deployment-utils";
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
import type { CardData, CardAvatar } from "astro-trading-card";
import { stripSvgWrapper } from "astro-trading-card";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { generateIdentity } from "identity-gen";

export type DeployedAgentStatus = "active" | "inactive" | "pending" | "undeploying" | "error";

function formatRelativeTime(isoString: string): string {
  const diffMs = new Date(isoString).getTime() - Date.now();
  const diffSecs = Math.round(diffMs / 1000);
  const diffMins = Math.round(diffSecs / 60);
  const diffHours = Math.round(diffMins / 60);
  const diffDays = Math.round(diffHours / 24);
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  if (Math.abs(diffSecs) < 60) return "less than a minute ago";
  if (Math.abs(diffMins) < 60) return rtf.format(diffMins, "minute");
  if (Math.abs(diffHours) < 24) return rtf.format(diffHours, "hour");
  return rtf.format(diffDays, "day");
}

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
  avatarUrl?: string;
  hasNewBuildAvailable?: boolean;
  className?: string;
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
  avatarUrl,
  hasNewBuildAvailable = false,
  className,
}: DeployedAgentCardProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const { copy: copyToClipboard, copied } = useCopyToClipboard(1600);

  const copyId = () => {
    void copyToClipboard(deploymentId);
  };

  // Fetch agent data on demand for integrations (only when share modal is open)
  const { data: agent } = useBlueprint(account, name, { enabled: shareOpen });
  const integrations = agent ? getBlueprintIntegrations(agent) : [];

  const cardAvatar = useMemo<CardAvatar | undefined>(() => {
    if (avatarUrl) return { url: avatarUrl };
    const svg = generateIdentity({ seed: `${account}/${name}`, size: 128 });
    return { svg: stripSvgWrapper(svg) };
  }, [avatarUrl, account, name]);

  const cardData = useMemo<CardData>(() => ({
    name,
    displayName,
    account,
    avatar: cardAvatar,
    stats: [
      { label: "Deployed", value: formatDateTime(installedAt) },
      { label: "From", value: `${account}/${name}` },
    ],
    barcodeId: deploymentId,
    qrUrl: `${window.location.origin}/${account}/${name}`,
  }), [name, displayName, account, cardAvatar, installedAt, deploymentId]);

  const cardClassName = cn(
    "group relative flex flex-col gap-3 rounded-md border border-stone-400 bg-white px-4 py-3 transition-all duration-150",
    href ? "hover:border-teal-500 hover:shadow-md dark:hover:border-teal-400" : "cursor-default opacity-70",
    className,
  );

  const cardContent = (
    <>
      <div className="absolute top-3 right-3" onClick={(e) => { e.preventDefault(); e.stopPropagation(); }}>
        <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="flex h-7 w-7 items-center justify-center rounded-md text-foreground transition-colors hover:bg-accent"
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
        {avatarUrl ? (
          <img
            src={avatarUrl}
            alt={name}
            className="h-9 w-9 shrink-0 rounded-sm object-cover"
          />
        ) : (
          <BlueprintIdentity
            account={account}
            name={name}
            size={36}
            className="h-9 w-9 shrink-0 rounded-sm overflow-hidden"
          />
        )}
        <div className="min-w-0 flex-1 pr-6">
          <p className="truncate text-sm font-semibold text-foreground transition-colors group-hover:text-primary dark:group-hover:text-primary-200">
            {displayName || name}
          </p>
          <div className="mt-1 flex items-center gap-2">
            <StatusIndicator
              variant={deploymentStatusVariant[status]}
              pulse={status === "pending" || status === "undeploying"}
              spinner={status === "pending" || status === "undeploying"}
            >
              {deploymentStatusLabel[status]}
            </StatusIndicator>
            {hasNewBuildAvailable && (
              <InlineBadge className="text-teal-700 bg-teal-50 border-teal-200 dark:text-teal-200 dark:bg-teal-900/40 dark:border-teal-300/30">
                update
              </InlineBadge>
            )}
          </div>
        </div>
      </div>

      <div className="mt-1 grid grid-cols-2 gap-x-4 gap-y-3">
        <MetricCell label="Requests" value={requests.toLocaleString()} />
        <MetricCell label="Last active" value={lastActive} />
        <MetricCell label="Deployed" value={formatDateTime(installedAt)} />
        <MetricCell label="Updated" value={formatRelativeTime(updatedAt)} />
      </div>
    </>
  );

  return (
    <>
      {href ? (
        <Link to={href} className={cardClassName}>{cardContent}</Link>
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
        integrations={integrations}
      />
    </>
  );
}
