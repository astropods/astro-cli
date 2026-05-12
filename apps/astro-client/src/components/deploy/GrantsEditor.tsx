import { useMemo, useState } from "react";
import { ShieldCheckIcon } from "@heroicons/react/24/outline";
import { useAuth } from "@/lib/auth";
import { useAccountMembers } from "@/api/queries";
import { TooltipProvider } from "@/components/ui/tooltip";
import { grantKey } from "./useDeployForm";
import { AddGrantMenu } from "./grants/AddGrantMenu";
import { GrantRow } from "./grants/GrantRow";
import { MemberPicker } from "./grants/MemberPicker";
import type { AuthGrant } from "@/lib/api";

export interface GrantsEditorProps {
  adapter: "web" | "slack";
  grants: AuthGrant[];
  onChange: (grants: AuthGrant[]) => void;
  /** Account whose members can be granted access via the user picker. */
  targetAccount?: string;
}

/** Composite editor for the auth-grant list of one adapter (web or slack).
 *  Owns add/remove orchestration and label resolution; each sub-piece (the
 *  add-grant menu, the per-row chip, the member picker) lives in its own file. */
export function GrantsEditor({ adapter, grants, onChange, targetAccount }: GrantsEditorProps) {
  const { accounts } = useAuth();
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

  const isAlreadyGranted = (g: AuthGrant) =>
    grants.some((x) => grantKey(x) === grantKey(g));
  const add = (g: AuthGrant) => {
    if (!isAlreadyGranted(g)) onChange([...grants, g]);
  };
  const removeAt = (idx: number) => onChange(grants.filter((_, i) => i !== idx));

  // An `anyone` grant subsumes every more-specific grant, so adding org/user
  // grants alongside it is just clutter. Hide the add menu until it's removed.
  const hasAnyone = grants.some((g) => g.anyone);

  const emptyHelp =
    adapter === "web"
      ? "No one can access this agent yet. Select who has access to this agent."
      : "Slack defaults to anyone in the workspace if no grants are set.";

  return (
    <TooltipProvider delayDuration={150}>
    <div className="space-y-2.5">
      <div className="flex items-center gap-2">
        <ShieldCheckIcon className="h-4 w-4 text-muted-foreground shrink-0" />
        <p className="text-[13px] font-medium text-foreground">Grant access</p>
      </div>

      {grants.length === 0 ? (
        <p className="text-[12px] text-muted-foreground">{emptyHelp}</p>
      ) : (
        <ul className="space-y-1.5">
          {grants.map((g, idx) => (
            <GrantRow
              key={`${grantKey(g)}-${idx}`}
              grant={g}
              adapter={adapter}
              accountById={accountById}
              memberByUserId={memberByUserId}
              onRemove={() => removeAt(idx)}
            />
          ))}
        </ul>
      )}

      {pickingUser && targetAccount ? (
        <MemberPicker
          account={targetAccount}
          adapter={adapter}
          isAlreadyGranted={(m) => isAlreadyGranted({ user_id: m.user_id })}
          onSelect={(m) => {
            add({ user_id: m.user_id });
            setPickingUser(false);
          }}
          onCancel={() => setPickingUser(false)}
        />
      ) : hasAnyone ? null : (
        <AddGrantMenu
          accounts={accounts}
          isAlreadyGranted={isAlreadyGranted}
          onPick={add}
          onPickUser={showUserPicker ? () => setPickingUser(true) : undefined}
        />
      )}
    </div>
    </TooltipProvider>
  );
}
