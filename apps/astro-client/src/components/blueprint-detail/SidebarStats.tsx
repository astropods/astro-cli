import type { ReactNode } from "react";
import { RocketLaunchIcon } from "@heroicons/react/24/outline";
import { SidebarSection } from "./SidebarSection";
import { cn } from "@/lib/utils";

export interface SidebarStatDetail {
  label: string;
  value: ReactNode;
}

export interface SidebarStatsProps {
  rating?: number;
  installs?: number;
  version?: string;
  /** Whether the version is a semver string (will be prefixed with "v") */
  isSemver?: boolean;
  updatedAt?: string;
  visibility?: string;
  isDraft?: boolean;
}

const DASH = "–";

export function SidebarStats({
  rating,
  installs,
  version,
  isSemver,
  updatedAt,
  visibility,
  isDraft = false,
}: SidebarStatsProps) {
  if (!isDraft && rating == null && installs == null && !version && !updatedAt && !visibility) {
    return null;
  }

  const rows: Array<{ label: string; value: ReactNode; faint?: boolean }> = [];

  if (visibility) {
    rows.push({ label: "Visibility", value: <span className="capitalize">{visibility}</span> });
  }

  if (rating != null) {
    rows.push({ label: "Requests", value: rating.toFixed(1) });
  }

  rows.push({
    label: "Deployments",
    faint: isDraft || installs == null,
    value: isDraft || installs == null ? DASH : (
      <span className="inline-flex items-center justify-end gap-1.5">
        <RocketLaunchIcon className="h-3.5 w-3.5 text-foreground" />
        {new Intl.NumberFormat("en-US").format(installs)}
      </span>
    ),
  });

  rows.push({
    label: "Build Number",
    faint: isDraft || !version,
    value: isDraft || !version ? DASH : (isSemver ? `v${version}` : version),
  });

  rows.push({
    label: "Updated",
    faint: isDraft || !updatedAt,
    value: isDraft || !updatedAt ? DASH : updatedAt,
  });

  return (
    <SidebarSection title="Details" headerClassName="py-2" bodyClassName="py-1">
      <dl>
        {rows.map((row, index) => (
          <div key={row.label}>
            <div className="py-2">
              <div className="flex items-center justify-between gap-6">
                <dt className="text-[13px] text-muted-foreground">{row.label}</dt>
                <dd className={cn("text-right font-mono text-mono-sm font-medium", row.faint ? "text-muted-foreground" : "text-foreground")}>
                  {row.value}
                </dd>
              </div>
            </div>
            {index < rows.length - 1 && <div className="-mx-4 h-px bg-border" />}
          </div>
        ))}
      </dl>
    </SidebarSection>
  );
}
