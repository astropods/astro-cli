import { Globe } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { WarningPanel } from "@/components/ui/status-panel";

export interface CustomInterfacePickerProps {
  /** Whether the custom interface is reachable without sign-in (open cohort). */
  isPublic: boolean;
  onPublicChange: (next: boolean) => void;
}

/** Access controls for the agent's own custom web interface (the UI it serves
 *  itself), distinct from the platform's messaging web chat. Only the protected
 *  toggle is exposed: it selects the no-OIDC ingress cohort. Per-subject grants
 *  aren't surfaced yet — the platform doesn't enforce them for the agent's own
 *  server, so showing a grants editor would be misleading. */
export function CustomInterfacePicker({ isPublic, onPublicChange }: CustomInterfacePickerProps) {
  return (
    <div className="rounded-[6px] border border-border bg-card">
      <div className="flex items-center gap-4 py-3 px-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-sm shrink-0 bg-muted text-muted-foreground">
          <Globe className="h-5 w-5" strokeWidth={1.5} />
        </div>
        <div className="flex flex-col gap-0.5 flex-1 min-w-0">
          <span className="text-[13px] font-medium text-foreground">Web Interface</span>
          <span className="text-[11px] text-muted-foreground">The agent serves its own UI or API.</span>
        </div>
      </div>
      <div className="border-t border-border bg-surface px-6 py-3 space-y-3 rounded-b-[6px]">
        <div className="flex items-center justify-between gap-4">
          <div className="flex flex-col gap-0.5 min-w-0">
            <span className="text-[13px] font-medium text-foreground">Protected</span>
            <span className="text-[11px] text-muted-foreground">Require an Astro account to reach the interface.</span>
          </div>
          <Switch checked={!isPublic} onCheckedChange={(checked) => onPublicChange(!checked)} />
        </div>
        {isPublic && (
          <WarningPanel variant="inline">
            Protection is off — anyone with the link can reach this interface without an Astro account.
          </WarningPanel>
        )}
      </div>
    </div>
  );
}
