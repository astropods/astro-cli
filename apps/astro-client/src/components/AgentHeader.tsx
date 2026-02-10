import type { ReactNode } from "react";
import { EllipsisHorizontalIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { ContentHeader } from "./ContentHeader";
import { Badge, type BadgeVariant } from "./Badge";
import { IntegrationBadge } from "./IntegrationBadge";

export type StatusVariant = Exclude<BadgeVariant, "default">;

export interface AgentHeaderProps {
  name: string;
  avatarUrl?: string;
  status: StatusVariant;
  integrations?: { name: string; icon: ReactNode }[];
  primaryAction?: { label: string; icon?: ReactNode; onClick: () => void };
  onMenuClick?: () => void;
  className?: string;
}

export function AgentHeader({
  name,
  avatarUrl,
  status,
  integrations = [],
  primaryAction,
  onMenuClick,
  className,
}: AgentHeaderProps) {
  const initial = name.charAt(0).toUpperCase();

  return (
    <ContentHeader className={className}>
      {/* Avatar */}
      {avatarUrl ? (
        <img
          src={avatarUrl}
          alt={name}
          className="size-7 rounded-full object-cover"
        />
      ) : (
        <div className="flex size-7 items-center justify-center rounded-full bg-muted text-xs font-medium">
          {initial}
        </div>
      )}

      {/* Name */}
      <span className="text-lg font-semibold">{name}</span>

      {/* Menu */}
      {onMenuClick && (
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={onMenuClick}
          aria-label="More options"
        >
          <EllipsisHorizontalIcon className="size-4" />
        </Button>
      )}

      {/* Status badge */}
      <Badge variant={status} showDot>{status}</Badge>

      {/* Spacer */}
      <div className="flex-1" />

      {/* Integrations */}
      {integrations.length > 0 && (
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">Access to</span>
          <div className="flex items-center">
            {integrations.map((integration, i) => (
              <IntegrationBadge
                key={integration.name}
                name={integration.name}
                icon={integration.icon}
                className={i > 0 ? "-ml-1" : undefined}
                style={{ zIndex: i }}
              />
            ))}
          </div>
        </div>
      )}

      {/* Primary action */}
      {primaryAction && (
        <Button size="sm" onClick={primaryAction.onClick}>
          {primaryAction.icon}
          {primaryAction.label}
        </Button>
      )}
    </ContentHeader>
  );
}
