import { useMemo } from "react";
import { useAccountMembers } from "@/api/queries/accounts";
import { MultiSelectFilterBar, type FilterEntry } from "./MultiSelectFilterBar";
import {
  ALL_USERS_KEY,
  ALL_USERS_COLOR,
  UNATTRIBUTED_USER_KEY,
  UNATTRIBUTED_COLOR,
  UNIDENTIFIED_USER_KEY,
  UNIDENTIFIED_COLOR,
} from "./user-classification";

interface UserFilterBarProps {
  account: string;
  /** All user_ids seen in the period (from /users-summary). Members not in this
   *  list aren't offered; non-members in this list get a raw-id fallback row. */
  presentUserIds: string[];
  value: string[];
  onValueChange: (values: string[]) => void;
  colorMap: Record<string, string>;
}

export function UserFilterBar({
  account,
  presentUserIds,
  value,
  onValueChange,
  colorMap,
}: UserFilterBarProps) {
  const { data: membersData } = useAccountMembers(account);

  const entries = useMemo<FilterEntry[]>(() => {
    const memberById = new Map(membersData?.members.map((m) => [m.user_id, m]) ?? []);
    const seen = new Set<string>();
    const list: FilterEntry[] = [];
    let hasUnattributed = false;
    let hasUnidentified = false;

    for (const uid of presentUserIds) {
      if (!uid || uid === UNATTRIBUTED_USER_KEY) { hasUnattributed = true; continue; }
      if (uid === UNIDENTIFIED_USER_KEY) { hasUnidentified = true; continue; }
      if (seen.has(uid)) continue;
      seen.add(uid);
      const member = memberById.get(uid);
      list.push({
        key: uid,
        label: member ? (member.display_name || member.username) : uid,
        color: colorMap[uid] ?? "var(--color-muted-foreground)",
      });
    }

    list.sort((a, b) => a.label.localeCompare(b.label));
    if (hasUnidentified) list.push({ key: UNIDENTIFIED_USER_KEY, label: "Unidentified", color: UNIDENTIFIED_COLOR });
    if (hasUnattributed) list.push({ key: UNATTRIBUTED_USER_KEY, label: "Infrastructure", color: UNATTRIBUTED_COLOR });
    return list;
  }, [presentUserIds, membersData, colorMap]);

  return (
    <MultiSelectFilterBar
      value={value}
      onValueChange={onValueChange}
      entries={entries}
      allItem={{ key: ALL_USERS_KEY, label: "All users", color: ALL_USERS_COLOR }}
      placeholder="Search users..."
    />
  );
}
