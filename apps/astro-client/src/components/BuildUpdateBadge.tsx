import { InlineBadge } from "@/components/InlineBadge";
import { cn } from "@/lib/utils";

const LABEL_NEW_BUILD = "New build";
const LABEL_NEW_BUILD_AVAILABLE = "New build available";
const ARROW = "\u2192";

export interface BuildUpdateBadgeProps {
  currentBuildId?: string | null;
  latestBuildId?: string | null;
  className?: string;
  stacked?: boolean;
  availableLabel?: boolean;
}

export function BuildUpdateBadge({
  currentBuildId,
  latestBuildId,
  className,
  stacked = false,
  availableLabel = false,
}: BuildUpdateBadgeProps) {
  const hasBothBuildIds = !!currentBuildId && !!latestBuildId;
  const prefix = availableLabel ? LABEL_NEW_BUILD_AVAILABLE : LABEL_NEW_BUILD;
  const versionLabel = hasBothBuildIds ? `${currentBuildId} ${ARROW} ${latestBuildId}` : null;

  return (
    <InlineBadge className={cn(stacked && "flex-col items-start gap-0.5 normal-case", className)}>
      {versionLabel ? (
        stacked ? (
          <span className="flex flex-col items-start gap-0.5">
            <span>{prefix}</span>
            <span>{versionLabel}</span>
          </span>
        ) : (
          <span className="inline-flex items-center gap-1.5">
            <span>{prefix}</span>
            <span>{versionLabel}</span>
          </span>
        )
      ) : (
        <span>{prefix}</span>
      )}
    </InlineBadge>
  );
}
