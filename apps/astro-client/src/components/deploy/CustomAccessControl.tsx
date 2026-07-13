import { useMemo, useState } from "react";
import { Plus, TriangleAlert } from "lucide-react";
import { ShieldCheckIcon } from "@heroicons/react/24/outline";
import { useAuth } from "@/lib/auth";
import { useAccountMembers } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { TooltipProvider } from "@/components/ui/tooltip";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { grantKey } from "./useDeployForm";
import { GrantRow } from "./grants/GrantRow";
import { MemberPicker } from "./grants/MemberPicker";
import type { AuthGrant } from "@/lib/api";

export interface CustomAccessControlProps {
  /** Whether the interface is reachable without sign-in (no-OIDC public cohort). */
  isPublic: boolean;
  onPublicChange: (next: boolean) => void;
  /** Per-subject grants for the OIDC-gated "specific people" mode. */
  grants: AuthGrant[];
  onGrantsChange: (grants: AuthGrant[]) => void;
  /** Account whose members can be granted access via the user picker. */
  targetAccount?: string;
}

type AccessMode = "anyone" | "public" | "specific";

/** Access control for the agent's own custom web interface. The blanket choice —
 *  any signed-in account, no account at all, or specific people — is a single
 *  dropdown; the per-person grant list only appears under "specific". */
export function CustomAccessControl({
  isPublic,
  onPublicChange,
  grants,
  onGrantsChange,
  targetAccount,
}: CustomAccessControlProps) {
  const { accounts, user } = useAuth();
  const showUserPicker = !!targetAccount;
  const { data: membersData } = useAccountMembers(showUserPicker ? targetAccount! : "");

  const accountById = useMemo(
    () => new Map(accounts.map((a) => [a.id, a])),
    [accounts],
  );
  const memberByUserId = useMemo(
    () => new Map((membersData?.members ?? []).map((m) => [m.user_id, m])),
    [membersData],
  );

  const [pickingUser, setPickingUser] = useState(false);

  const mode: AccessMode = isPublic
    ? "public"
    : grants.some((g) => g.anyone)
      ? "anyone"
      : "specific";

  const selectMode = (next: AccessMode) => {
    setPickingUser(false);
    if (next === "public") {
      onPublicChange(true);
      onGrantsChange([]);
    } else if (next === "anyone") {
      onPublicChange(false);
      onGrantsChange([{ anyone: true }]);
    } else {
      // Invited only: seed the deploying user so they always keep access.
      onPublicChange(false);
      onGrantsChange(user?.id ? [{ user_id: user.id }] : []);
    }
  };

  const isAlreadyGranted = (g: AuthGrant) =>
    grants.some((x) => grantKey(x) === grantKey(g));
  const add = (g: AuthGrant) => {
    if (!isAlreadyGranted(g)) onGrantsChange([...grants, g]);
  };
  const removeAt = (idx: number) => onGrantsChange(grants.filter((_, i) => i !== idx));

  return (
    <TooltipProvider delayDuration={150}>
      <div className="space-y-2.5">
        <div className="flex items-center gap-2">
          <ShieldCheckIcon className="h-4 w-4 text-muted-foreground shrink-0" />
          <p className="text-[13px] font-medium text-foreground">Who has access</p>
        </div>

        <Select value={mode} onValueChange={(v) => selectMode(v as AccessMode)}>
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="anyone">Anyone with an Astro account</SelectItem>
            <SelectItem value="public">Public</SelectItem>
            <SelectItem value="specific">Invited only</SelectItem>
          </SelectContent>
        </Select>

        {mode === "public" && (
          <div className="flex items-center gap-2 rounded-[6px] border border-warning/30 bg-warning/10 px-3 py-2">
            <TriangleAlert className="h-4 w-4 text-warning shrink-0" />
            <p className="text-[12px] text-foreground">
              This interface is open. Anyone with the link can reach it without signing in.
            </p>
          </div>
        )}

        {mode === "specific" && (
          <div className="space-y-2.5">
            {grants.length > 0 ? (
              <ul className="space-y-1.5">
                {grants.map((g, idx) => (
                  <GrantRow
                    key={`${grantKey(g)}-${idx}`}
                    grant={g}
                    adapter="custom"
                    accountById={accountById}
                    memberByUserId={memberByUserId}
                    onRemove={() => removeAt(idx)}
                    currentUserId={user?.id}
                  />
                ))}
              </ul>
            ) : !pickingUser ? (
              <p className="text-[12px] text-muted-foreground">
                No one is invited yet. Add the people who should have access.
              </p>
            ) : null}

            {pickingUser && targetAccount ? (
              <MemberPicker
                account={targetAccount}
                adapter="custom"
                isAlreadyGranted={(m) => isAlreadyGranted({ user_id: m.user_id })}
                onSelect={(m) => {
                  add({ user_id: m.user_id });
                  setPickingUser(false);
                }}
                onCancel={() => setPickingUser(false)}
              />
            ) : showUserPicker ? (
              <Button type="button" variant="outline" size="sm" onClick={() => setPickingUser(true)}>
                <Plus className="h-3.5 w-3.5" />
                Add member
              </Button>
            ) : null}
          </div>
        )}
      </div>
    </TooltipProvider>
  );
}
