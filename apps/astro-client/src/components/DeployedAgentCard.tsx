import { Link } from "react-router";
import { EllipsisVertical } from "lucide-react";
import { cn } from "@/lib/utils";
import { Badge, type BadgeVariant } from "@/components/Badge";
import { AgentIdentity } from "@/components/AgentIdentity";

export type DeployedAgentStatus = "active" | "inactive" | "pending" | "error";

const statusToBadgeVariant: Record<DeployedAgentStatus, BadgeVariant> = {
  active: "active",
  inactive: "inactive",
  pending: "pending",
  error: "error",
};

const statusLabels: Record<DeployedAgentStatus, string> = {
  active: "Active",
  inactive: "Inactive",
  pending: "Pending",
  error: "Error",
};

export interface DeployedAgentCardProps {
  name: string;
  displayName?: string;
  account: string;
  href: string;
  status: DeployedAgentStatus;
  requests: number;
  lastActive: string;
  installedAt: string;
  updatedAt: string;
  avatarUrl?: string;
  onOptionsClick?: () => void;
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
  account,
  href,
  status,
  requests,
  lastActive,
  installedAt,
  updatedAt,
  avatarUrl,
  onOptionsClick,
  className,
}: DeployedAgentCardProps) {
  return (
    <Link
      to={href}
      className={cn(
        "group relative flex flex-col gap-3 rounded-sm border border-border bg-background p-4 transition-colors hover:bg-accent",
        className,
      )}
    >
      <div className="absolute top-3 right-3">
        <button
          type="button"
          className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent"
          aria-label="Agent options"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onOptionsClick?.();
          }}
        >
          <EllipsisVertical className="h-4 w-4" />
        </button>
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
          <Badge variant={statusToBadgeVariant[status]} showDot>
            {statusLabels[status]}
          </Badge>
        </div>
      </div>

      <div className="mt-1 grid grid-cols-2 gap-x-4 gap-y-3">
        <MetricCell label="Requests" value={requests.toLocaleString()} />
        <MetricCell label="Last active" value={lastActive} />
        <MetricCell label="Installed" value={installedAt} />
        <MetricCell label="Updated" value={updatedAt} />
      </div>
    </Link>
  );
}
