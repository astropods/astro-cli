import { useMemo } from "react";
import { BuildingOffice2Icon } from "@heroicons/react/24/outline";
import { UserAvatar } from "@/components/UserAvatar";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuth } from "@/lib/auth";
import type { Account } from "@/lib/api";

interface OrgSwitcherProps {
  activeAccount: string;
  onChange: (account: string) => void;
}

function AccountIcon({ account }: { account: Account }) {
  if (account.type === "personal") {
    return (
      <UserAvatar
        handle={account.name}
        name={account.display_name || account.name}
        avatarVersion={account.avatar_version}
        className="size-[18px] shrink-0"
      />
    );
  }
  return (
    <span className="flex size-[18px] items-center justify-center rounded-md bg-accent shrink-0">
      <BuildingOffice2Icon className="size-2.5 text-muted-foreground" />
    </span>
  );
}

export function OrgSwitcher({ activeAccount, onChange }: OrgSwitcherProps) {
  const { accounts } = useAuth();

  const sorted = useMemo(
    () =>
      [...accounts].sort((a, b) =>
        a.type === "personal" ? -1 : b.type === "personal" ? 1 : a.name.localeCompare(b.name),
      ),
    [accounts],
  );

  return (
    <div className="flex items-center gap-2">
      <span className="text-xs font-normal text-foreground select-none">View</span>
      <Select value={activeAccount} onValueChange={onChange}>
        <SelectTrigger className="h-8 w-48 px-2.5 py-0 text-sm leading-none">
          <SelectValue />
        </SelectTrigger>
        <SelectContent align="end">
          {sorted.map((a) => (
            <SelectItem key={a.id} value={a.name}>
              <span className="inline-flex items-center gap-2">
                <AccountIcon account={a} />
                {a.display_name || a.name}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
