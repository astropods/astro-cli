import { Check } from "lucide-react";
import { Label } from "@/components/ui/label";
import { useAccount } from "@/api/queries/accounts";
import { cn } from "@/lib/utils";
import type { AllowedCluster } from "@/lib/api";

export interface ClusterPickerProps {
  account: string;
  value: string;
  onChange: (clusterId: string) => void;
  currentClusterId?: string;
  deployed?: boolean;
}

function RegionRow({ cluster, selected }: { cluster: AllowedCluster; selected: boolean }) {
  return (
    <>
      <div
        className={cn(
          "flex h-9 w-9 items-center justify-center rounded-sm shrink-0 text-body transition-colors",
          selected ? "bg-primary/10 dark:bg-primary/25" : "bg-muted",
        )}
      >
        <span aria-hidden>{cluster.region_flag || "🌐"}</span>
      </div>
      <div className="flex flex-col gap-0.5 flex-1 min-w-0">
        <span className="text-body font-medium text-foreground">
          {cluster.region_label || cluster.region || cluster.cluster_id}
          {cluster.is_default && (
            <span className="ml-1.5 font-normal text-muted-foreground">Default</span>
          )}
        </span>
        <span className="text-body-sm text-muted-foreground">{cluster.region}</span>
      </div>
    </>
  );
}

export function ClusterPicker({ account, value, onChange, currentClusterId, deployed }: ClusterPickerProps) {
  const { data } = useAccount(account);
  const allowed = data?.allowed_clusters ?? [];
  if (allowed.length === 0) return null;

  const resolved =
    allowed.find((c) => c.cluster_id === currentClusterId) ??
    allowed.find((c) => c.is_default) ??
    allowed[0];
  const effective = value || resolved.cluster_id;
  const moving = !!deployed && effective !== resolved.cluster_id;

  return (
    <div>
      <Label size="md">Region</Label>
      {allowed.length === 1 ? (
        <div className="flex items-center gap-4 rounded-[6px] border border-border py-3 px-3">
          <RegionRow cluster={resolved} selected={false} />
        </div>
      ) : (
        <div className="space-y-2">
          {allowed.map((c) => {
            const isSelected = c.cluster_id === effective;
            return (
              <button
                key={c.cluster_id}
                type="button"
                aria-pressed={isSelected}
                onClick={() => onChange(c.cluster_id)}
                className={cn(
                  "w-full flex items-center gap-4 py-3 px-3 rounded-[6px] border text-left cursor-pointer transition-[border-color,background-color]",
                  isSelected
                    ? "border-primary/40 bg-primary/5"
                    : "border-border bg-transparent hover:bg-muted/50",
                )}
              >
                <RegionRow cluster={c} selected={isSelected} />
                <div
                  className={cn(
                    "w-5 h-5 rounded-full border-2 flex items-center justify-center shrink-0 transition-colors",
                    isSelected ? "border-primary bg-primary" : "border-input bg-background",
                  )}
                >
                  {isSelected && <Check size={12} strokeWidth={3} className="text-primary-foreground" />}
                </div>
              </button>
            );
          })}
        </div>
      )}
      <p className="mt-1.5 text-body-sm text-muted-foreground">
        {moving
          ? "Deploying moves this agent to the new region. It restarts, and its current region is torn down."
          : "Where this agent runs."}
      </p>
    </div>
  );
}
