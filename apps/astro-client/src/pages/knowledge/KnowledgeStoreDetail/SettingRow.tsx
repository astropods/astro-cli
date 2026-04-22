import type { ReactNode } from "react";

export function SettingRow({ label, description, children }: { label: string; description?: ReactNode; children: ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-2 px-5 py-4 sm:grid-cols-[220px_1fr] sm:gap-8 sm:items-center">
      <div>
        <p className="text-[13px] font-semibold text-foreground">{label}</p>
        {description && <p className="mt-0.5 text-body-sm text-muted-foreground">{description}</p>}
      </div>
      <div>{children}</div>
    </div>
  );
}
