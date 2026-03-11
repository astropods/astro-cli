import { useState } from "react";
import { Link } from "react-router";
import { EllipsisVertical, Trash2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { StatusIndicator } from "@/components/StatusIndicator";
import { AgentIdentity } from "@/components/AgentIdentity";
import { deploymentStatusVariant, deploymentStatusLabel } from "@/lib/deployment-utils";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";

export type DeployedAgentStatus = "active" | "inactive" | "pending" | "error";

export interface DeployedAgentCardProps {
  name: string;
  displayName?: string;
  deploymentId: string;
  account: string;
  href: string;
  status: DeployedAgentStatus;
  requests: number;
  lastActive: string;
  installedAt: string;
  updatedAt: string;
  avatarUrl?: string;
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
  className,
}: DeployedAgentCardProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <>
      <Link
        to={href}
        className={cn(
          "group relative flex flex-col gap-3 rounded-md border border-stone-400 bg-background px-4 py-3 transition-all duration-150 hover:border-teal-500 hover:shadow-md dark:hover:border-teal-400",
          className,
        )}
      >
        <div className="absolute top-3 right-3">
          <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent"
                aria-label="Agent options"
                onClick={(e) => e.stopPropagation()}
              >
                <EllipsisVertical className="h-4 w-4" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => {
                  setMenuOpen(false);
                  setDeleteOpen(true);
                }}
              >
                <Trash2 />
                Delete <span className="max-w-[120px] truncate font-semibold">{displayName || name}</span>
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
            <StatusIndicator variant={deploymentStatusVariant[status]} pulse={status === "pending"}>
              {deploymentStatusLabel[status]}
            </StatusIndicator>
          </div>
        </div>

        <div className="mt-1 grid grid-cols-2 gap-x-4 gap-y-3">
          <MetricCell label="Requests" value={requests.toLocaleString()} />
          <MetricCell label="Last active" value={lastActive} />
          <MetricCell label="Installed" value={installedAt} />
          <MetricCell label="Updated" value={updatedAt} />
        </div>
      </Link>

      <DeleteDeploymentDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        deploymentId={deploymentId}
        deploymentName={name}
        displayName={displayName}
        account={account}
      />
    </>
  );
}
