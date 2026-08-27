import { useId } from "react";
import { InlineBadge } from "@/components/InlineBadge";
import { FieldHeader } from "@/components/ui/field-header";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { useAccount } from "@/api/queries/accounts";
import { cn } from "@/lib/utils";
import type { AllowedCluster } from "@/lib/api";

const SINGLE_REGION_TOOLTIP =
  "Region availability is set for this account. Contact support to request another region.";

export interface ClusterPickerProps {
  account: string;
  value: string;
  onChange: (clusterId: string) => void;
  currentClusterId?: string;
  deployed?: boolean;
}

function getRegionLabel(cluster: AllowedCluster) {
  return cluster.region_label || cluster.region || cluster.cluster_id;
}

function getRegionAccessibleName(cluster: AllowedCluster) {
  const label = getRegionLabel(cluster);
  return [label, cluster.is_default ? "Default" : undefined, cluster.region !== label ? cluster.region : undefined]
    .filter(Boolean)
    .join(" ");
}

function RegionDetails({ cluster }: { cluster: AllowedCluster }) {
  const label = getRegionLabel(cluster);
  const labelParts = label.match(/^(.*?)\s*(\([^()]+\))$/);
  const primaryLabel = labelParts?.[1] || label;
  const locationLabel = labelParts?.[2];

  return (
    <div className="min-w-0 flex-1">
      <div className="flex min-w-0 items-center gap-2">
        <span className="min-w-0 truncate text-body font-medium text-foreground">
          {primaryLabel}
          {locationLabel && <span className="font-normal text-muted-foreground"> {locationLabel}</span>}
        </span>
        {cluster.is_default && (
          <InlineBadge
            variant="soft"
            className="shrink-0 bg-accent font-sans font-medium text-accent-foreground"
          >
            Default
          </InlineBadge>
        )}
      </div>
      {cluster.region && cluster.region !== label && (
        <span className="mt-0.5 block text-body-sm text-muted-foreground">{cluster.region}</span>
      )}
    </div>
  );
}

export function ClusterPicker({ account, value, onChange, currentClusterId, deployed }: ClusterPickerProps) {
  const { data } = useAccount(account);
  const labelId = useId();
  const descriptionId = useId();
  const movementId = useId();
  const allowed = data?.allowed_clusters ?? [];
  if (allowed.length === 0) return null;

  const resolved =
    allowed.find((cluster) => cluster.cluster_id === currentClusterId) ??
    allowed.find((cluster) => cluster.is_default) ??
    allowed[0];
  const effective = value || resolved.cluster_id;
  const moving = !!deployed && effective !== resolved.cluster_id;
  const readOnly = allowed.length === 1;

  return (
    <div className="w-full">
      <FieldHeader
        label="Deployment region"
        description={readOnly ? "This agent runs in the only available region." : "Select where this agent runs."}
        labelId={labelId}
        descriptionId={descriptionId}
        className={moving ? "mb-0" : undefined}
      />
      {moving && (
        <p id={movementId} role="status" className="mt-1.5 text-body-sm text-warning">
          Deploying moves this agent to the new region. It restarts, and its current region is torn down.
        </p>
      )}

      <TooltipProvider delayDuration={250}>
        <div
          role="radiogroup"
          aria-labelledby={labelId}
          aria-describedby={moving ? `${descriptionId} ${movementId}` : descriptionId}
          className={cn(
            "overflow-hidden rounded-[6px] border",
            moving && "mt-3",
            readOnly ? "border-border-strong" : "border-border",
          )}
        >
          {allowed.map((cluster, index) => {
            const isSelected = cluster.cluster_id === effective;
            const row = (
              <label
                key={cluster.cluster_id}
                aria-disabled={readOnly || undefined}
                tabIndex={readOnly ? 0 : undefined}
                className={cn(
                  "relative flex min-h-16 items-center gap-3 px-4 py-3 text-left transition-colors",
                  readOnly ? "cursor-help" : "cursor-pointer",
                  index > 0 && "border-t border-border",
                  isSelected && (readOnly ? "bg-muted/40" : "bg-muted"),
                  !isSelected && "bg-transparent hover:bg-muted/50",
                  readOnly &&
                    "rounded-[5px] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
                )}
              >
                <input
                  type="radio"
                  name={`cluster-${account}`}
                  value={cluster.cluster_id}
                  aria-label={getRegionAccessibleName(cluster)}
                  checked={isSelected}
                  disabled={readOnly}
                  onChange={() => onChange(cluster.cluster_id)}
                  className="peer sr-only"
                />
                <span
                  aria-hidden
                  className={cn(
                    "pointer-events-none absolute inset-0",
                    "peer-focus-visible:ring-2 peer-focus-visible:ring-inset peer-focus-visible:ring-ring",
                  )}
                />
                <RegionDetails cluster={cluster} />
                <span
                  aria-hidden
                  className={cn(
                    "flex size-5 shrink-0 items-center justify-center rounded-full border-2 bg-background transition-colors",
                    isSelected ? (readOnly ? "border-muted-foreground" : "border-primary") : "border-input",
                  )}
                >
                  {isSelected && (
                    <span
                      className={cn("size-2 rounded-full", readOnly ? "bg-muted-foreground" : "bg-primary")}
                    />
                  )}
                </span>
              </label>
            );

            return readOnly ? (
              <Tooltip key={cluster.cluster_id}>
                <TooltipTrigger asChild>{row}</TooltipTrigger>
                <TooltipContent side="top" sideOffset={6} className="max-w-72">
                  {SINGLE_REGION_TOOLTIP}
                </TooltipContent>
              </Tooltip>
            ) : row;
          })}
        </div>
      </TooltipProvider>
    </div>
  );
}
