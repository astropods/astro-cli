import { useMemo, useState } from "react";
import { useAccountMembers } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { UserAvatar } from "@/components/UserAvatar";
import { SlackUnlinkedBadge } from "./SlackUnlinkedBadge";
import type { AccountMember } from "@/lib/api";

export interface MemberPickerProps {
  /** Account whose members are listed and searchable. */
  account: string;
  /** Adapter this grant is being added for. When "slack", members with no
   *  linked Slack workspaces get a warning badge. */
  adapter: "web" | "slack" | "custom";
  /** Called with the selected member when the user clicks one. */
  onSelect: (member: AccountMember) => void;
  /** Called when the user dismisses the picker without selecting. */
  onCancel: () => void;
  /** Predicate to disable rows that already have a grant. */
  isAlreadyGranted?: (member: AccountMember) => boolean;
}

/** Search-by-account-name picker over the members of a target account.
 *  Used by GrantsEditor to add a per-user grant. */
export function MemberPicker({ account, adapter, onSelect, onCancel, isAlreadyGranted }: MemberPickerProps) {
  const { data, isLoading } = useAccountMembers(account);
  const members: AccountMember[] = data?.members ?? [];

  const [query, setQuery] = useState("");
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return members;
    return members.filter(
      (m) =>
        m.username.toLowerCase().includes(q) ||
        m.display_name.toLowerCase().includes(q),
    );
  }, [members, query]);

  return (
    <div className="space-y-1.5">
      <Input
        autoFocus
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") onCancel();
        }}
        placeholder="Search by account name…"
        className="h-8"
      />
      <div className="max-h-48 overflow-y-auto rounded-[4px] border border-border bg-card">
        {isLoading ? (
          <p className="px-2.5 py-2 text-[12px] text-muted-foreground">Loading members…</p>
        ) : filtered.length === 0 ? (
          <p className="px-2.5 py-2 text-[12px] text-muted-foreground">
            {members.length === 0 ? "No members in this account." : "No matches."}
          </p>
        ) : (
          <ul>
            {filtered.map((m) => {
              const granted = isAlreadyGranted?.(m) ?? false;
              const slackUnlinked =
                adapter === "slack" && (m.slack_workspaces?.length ?? 0) === 0;
              return (
                <li key={m.user_id}>
                  <button
                    type="button"
                    disabled={granted}
                    onClick={() => onSelect(m)}
                    className="w-full flex items-center justify-between gap-2 px-2.5 py-1.5 text-left text-[13px] hover:bg-accent disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
                  >
                    <span className="flex items-center gap-2 min-w-0">
                      <UserAvatar
                        handle={m.username}
                        name={m.display_name || m.username}
                        avatarUrl={m.avatar_url}
                        className="size-6"
                      />
                      <span className="flex flex-col min-w-0">
                        <span className="flex items-center gap-1.5 min-w-0">
                          <span className="truncate text-foreground">
                            {m.display_name || m.username}
                          </span>
                          {slackUnlinked && <SlackUnlinkedBadge />}
                        </span>
                        <span className="truncate text-[11px] text-muted-foreground">
                          @{m.username}
                        </span>
                      </span>
                    </span>
                    {granted && (
                      <span className="text-[11px] text-muted-foreground shrink-0">added</span>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
      <div className="flex justify-end">
        <Button type="button" size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
