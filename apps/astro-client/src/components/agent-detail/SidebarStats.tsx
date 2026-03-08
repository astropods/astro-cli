export interface SidebarStatsProps {
  version?: string;
  /** Whether the version is a semver string (will be prefixed with "v") */
  isSemver?: boolean;
  updatedAt?: string;
}

export function SidebarStats({
  version,
  isSemver,
  updatedAt,
}: SidebarStatsProps) {
  if (!version && !updatedAt) return null;

  return (
    <>
      <div className="my-5 h-px bg-border-strong" />
      <div className="grid grid-cols-2 gap-3">
        {version && (
          <div className="flex flex-col gap-0.5">
            <span className="text-[11px] text-[var(--ink-faint)]">Version</span>
            <span className="text-[13px] font-normal text-foreground font-mono">
              {isSemver ? `v${version}` : version}
            </span>
          </div>
        )}
        {updatedAt && (
          <div className="flex flex-col gap-0.5">
            <span className="text-[11px] text-[var(--ink-faint)]">Updated</span>
            <span className="text-[13px] font-normal text-foreground font-mono">
              {updatedAt}
            </span>
          </div>
        )}
      </div>
    </>
  );
}
