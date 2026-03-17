import type { ReactNode } from "react";
import { Star } from "lucide-react";
import { ArrowDownTrayIcon } from "@heroicons/react/24/outline";
import { SidebarSection } from "./SidebarSection";

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
}

export function SidebarStats({
  rating,
  installs,
  version,
  isSemver,
  updatedAt,
}: SidebarStatsProps) {
  if (rating == null && installs == null && !version && !updatedAt) {
    return null;
  }

  const rows: Array<{
    label: string;
    value: ReactNode;
  }> = [];

  if (rating != null) {
    rows.push({
      label: "Rating",
      value: (
        <span className="inline-flex items-center justify-end gap-1.5">
          <Star className="h-3 w-3 fill-current text-yellow-500" />
          {rating.toFixed(1)}
        </span>
      ),
    });
  }

  if (installs != null) {
    rows.push({
      label: "Installs",
      value: (
        <span className="inline-flex items-center justify-end gap-1.5">
          <ArrowDownTrayIcon className="h-3.5 w-3.5 text-foreground" />
          {new Intl.NumberFormat("en-US").format(installs)}
        </span>
      ),
    });
  }

  if (version) {
    rows.push({
      label: "Build Number",
      value: isSemver ? `v${version}` : version,
    });
  }

  if (updatedAt) {
    rows.push({
      label: "Updated",
      value: updatedAt,
    });
  }

  return (
    <SidebarSection title="Details" headerClassName="py-2" bodyClassName="px-0 py-1">
      <dl>
        {rows.map((row, index) => (
          <div key={row.label}>
            <div className="py-2">
              <div className="flex items-center justify-between gap-6">
                <dt className="text-[13px] text-muted-foreground">{row.label}</dt>
                <dd className="text-right font-mono text-mono-sm font-medium text-foreground">
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
