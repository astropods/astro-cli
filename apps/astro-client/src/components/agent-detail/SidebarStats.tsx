import { Star } from "lucide-react";
import { SidebarSection } from "./SidebarSection";

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
  if (rating == null && installs == null && !version && !updatedAt) return null;

  return (
    <SidebarSection title="Details" bodyClassName="px-0 py-0">
      <dl>
        {rating != null && (
          <div className="flex items-center justify-between border-b border-border-strong px-4 py-3.5 last:border-b-0">
            <dt className="text-[13px] text-muted-foreground">Rating</dt>
            <dd className="inline-flex items-center gap-1 text-[14px] font-semibold text-foreground font-mono">
              <Star className="h-3 w-3 fill-current text-yellow-500" />
              {rating.toFixed(1)}
            </dd>
          </div>
        )}
        {installs != null && (
          <div className="flex items-center justify-between border-b border-border-strong px-4 py-3.5 last:border-b-0">
            <dt className="text-[13px] text-muted-foreground">Installs</dt>
            <dd className="text-[14px] font-semibold text-foreground font-mono">
              {new Intl.NumberFormat("en-US").format(installs)}
            </dd>
          </div>
        )}
        {version && (
          <div className="flex items-center justify-between border-b border-border-strong px-4 py-3.5 last:border-b-0">
            <dt className="text-[13px] text-muted-foreground">Version</dt>
            <dd className="text-[14px] font-semibold text-foreground font-mono">
              {isSemver ? `v${version}` : version}
            </dd>
          </div>
        )}
        {updatedAt && (
          <div className="flex items-center justify-between border-b border-border-strong px-4 py-3.5 last:border-b-0">
            <dt className="text-[13px] text-muted-foreground">Last updated</dt>
            <dd className="text-[14px] font-semibold text-foreground font-mono">
              {updatedAt}
            </dd>
          </div>
        )}
      </dl>
    </SidebarSection>
  );
}
