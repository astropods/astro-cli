import { AppWindow } from "lucide-react";
import { CustomAccessControl } from "./CustomAccessControl";
import type { AuthGrant } from "@/lib/api";

export interface CustomInterfacePickerProps {
  /** Whether the custom interface is reachable without sign-in (open cohort). */
  isPublic: boolean;
  onPublicChange: (next: boolean) => void;
  /** Per-subject access grants for the protected (OIDC) ingress cohort. */
  grants: AuthGrant[];
  onGrantsChange: (grants: AuthGrant[]) => void;
  /** Account whose members can be granted access via the user picker. */
  targetAccount?: string;
}

/** Access controls for the agent's own custom web interface (the UI it serves
 *  itself), distinct from the platform's messaging web chat. The access mode —
 *  any signed-in account, no account at all, or specific people — is chosen in
 *  CustomAccessControl; this component is just the labeled card chrome.
 *
 *  Grants are captured here and persisted (deployment_authorization_grants,
 *  adapter='custom'), but platform-level enforcement of org/user grants at the
 *  custom ingress is a pending BE follow-up: today the ALB only gates signed-in
 *  vs not. Enforcing the grants requires an ext_authz hop in front of the agent
 *  ingress that calls GET /deployments/authorize (which already accepts
 *  adapter=custom). */
export function CustomInterfacePicker({
  isPublic,
  onPublicChange,
  grants,
  onGrantsChange,
  targetAccount,
}: CustomInterfacePickerProps) {
  return (
    <div className="rounded-[6px] border border-border bg-card">
      <div className="flex items-center gap-4 py-3 px-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-sm shrink-0 bg-muted text-muted-foreground">
          <AppWindow className="h-5 w-5" strokeWidth={1.5} />
        </div>
        <div className="flex flex-col gap-0.5 flex-1 min-w-0">
          <span className="text-[13px] font-medium text-foreground">Web Interface</span>
          <span className="text-[11px] text-muted-foreground">The agent serves its own UI or API.</span>
        </div>
      </div>
      <div className="border-t border-border bg-surface px-6 py-3 space-y-3 rounded-b-[6px]">
        <CustomAccessControl
          isPublic={isPublic}
          onPublicChange={onPublicChange}
          grants={grants}
          onGrantsChange={onGrantsChange}
          targetAccount={targetAccount}
        />
      </div>
    </div>
  );
}
