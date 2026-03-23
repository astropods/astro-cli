import { useMemo, useState } from "react";
import { Link } from "react-router";
import { EllipsisVertical, Share2, Trash2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { StatusIndicator } from "@/components/StatusIndicator";
import { AgentIdentity } from "@/components/AgentIdentity";
import { InlineBadge } from "@/components/InlineBadge";
import { deploymentStatusVariant, deploymentStatusLabel } from "@/lib/deployment-utils";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";
import { TradingCardModal } from "@/components/trading-card/TradingCardModal";
import { useAgent } from "@/api/queries/agents";
import { getAgentIntegrations } from "@/lib/agent-utils";
import type { CardData, CardAvatar } from "astro-trading-card";
import { stripSvgWrapper } from "astro-trading-card";
import { generateIdentity } from "identity-gen";

export type DeployedAgentStatus = "active" | "inactive" | "pending" | "undeploying" | "error";

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
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <span className="text-[13px] font-mono text-foreground tabular-nums">{value}</span>
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

  // Fetch agent data on demand for integrations (only when share modal is open)
  const { data: agent } = useAgent(account, name, { enabled: shareOpen });
  const integrations = agent ? getAgentIntegrations(agent) : [];

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
      { label: "Deployed", value: installedAt },
      { label: "From", value: `${account}/${name}` },
    ],
    barcodeId: deploymentId,
    qrUrl: `${window.location.origin}/${account}/${name}`,
  }), [name, displayName, account, cardAvatar, installedAt, deploymentId]);

  const cardClassName = cn(
    "group relative flex flex-col gap-3 rounded-md border border-stone-400 bg-background px-4 py-3 transition-all duration-150",
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
              className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent"
              aria-label="Agent options"
            >
              <EllipsisVertical className="h-4 w-4" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              onSelect={() => {
                setMenuOpen(false);
                setShareOpen(true);
              }}
            >
              <Share2 />
              Share agent badge
            </DropdownMenuItem>
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => {
                setMenuOpen(false);
                setDeleteOpen(true);
              }}
            >
              <Trash2 />
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
          <AgentIdentity
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
        <MetricCell label="Deployed" value={installedAt} />
        <MetricCell label="Updated" value={updatedAt} />
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
