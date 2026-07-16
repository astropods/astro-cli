import { Loader2 } from "lucide-react";
import type { DeploymentStatusValue } from "@/lib/api";

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
// The build phase uses blue so it reads as distinct from the amber deployment
// phase it stacks above. The semantic token adapts across light and dark themes.
export const INFO_COLORS: StatusColor = {
  bg: "color-mix(in oklch, var(--info) 12%, transparent)",
  border: "color-mix(in oklch, var(--info) 28%, transparent)",
  badgeBg: "color-mix(in oklch, var(--info) 22%, transparent)",
  badgeText: "var(--info)",
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

export function getDeploymentStatusColors(status?: DeploymentStatusValue | null): StatusColor {
  return status ? (STATUS_COLORS[status] ?? NEUTRAL_COLORS) : NEUTRAL_COLORS;
}

export function DeploymentStatusBadge({
  status,
  label,
}: {
  status: DeploymentStatusValue;
  label?: string;
}) {
  const colors = getDeploymentStatusColors(status);

  return (
    <span
      className="flex items-center gap-1.5 rounded-full px-2 py-0.5 text-mono-sm font-medium"
      style={{ backgroundColor: colors.badgeBg, color: colors.badgeText }}
    >
      {SPINNING_STATUSES.has(status) && <Loader2 className="size-3 animate-spin" />}
      {label ?? STATUS_LABELS[status]}
    </span>
  );
}
