import { useMemo } from "react";
import { UserAvatar } from "@/components/UserAvatar";
import {
  MultiSelect,
  MultiSelectAllItem,
  MultiSelectContent,
  MultiSelectItem,
  MultiSelectList,
  MultiSelectTrigger,
} from "@/components/ui/multi-select";
import { comparePersonalFirst } from "@/lib/account-order";
import { cn } from "@/lib/utils";
import { useAuth } from "@/lib/auth";
import type { Account } from "@/lib/api";

interface AccountFilterProps {
  value: string[];
  onChange: (accounts: string[]) => void;
  className?: string;
}

function AccountIcon({ account }: { account: Account }) {
  return (
    <UserAvatar
      handle={account.name}
      name={account.display_name || account.name}
      avatarUrl={account.avatar_url}
      className="size-[18px] shrink-0"
    />
  );
}

export function AccountFilter({ value, onChange, className }: AccountFilterProps) {
  const { accounts } = useAuth();

  const sorted = useMemo(
    () => [...accounts].sort(comparePersonalFirst),
    [accounts],
  );

  const selected = useMemo(
    () => sorted.filter((a) => value.includes(a.name)),
    [sorted, value],
  );
  const allSelected = value.length === 0 || value.length === sorted.length;

  return (
    <MultiSelect value={value} onValueChange={onChange}>
      <MultiSelectTrigger
        aria-label="Filter by account"
        className={cn(
          "h-8 gap-1.5 px-2.5 text-sm bg-card dark:bg-background sm:w-auto sm:min-w-[13rem]",
          className,
        )}
      >
        <span className="flex min-w-0 items-center gap-1.5">
          {allSelected || selected.length === 0 ? (
            <span className="truncate">All accounts</span>
          ) : (
            <>
              <AccountIcon account={selected[0]} />
              <span className="truncate">
                {selected[0].display_name || selected[0].name}
              </span>
              {selected.length > 1 && (
                <span className="shrink-0 text-muted-foreground">
                  +{selected.length - 1}
                </span>
              )}
            </>
          )}
        </span>
      </MultiSelectTrigger>
      <MultiSelectContent>
        <MultiSelectAllItem>All accounts</MultiSelectAllItem>
        <MultiSelectList>
          {sorted.map((a) => (
            <MultiSelectItem key={a.id} value={a.name}>
              <AccountIcon account={a} />
              <span className="truncate">{a.display_name || a.name}</span>
            </MultiSelectItem>
          ))}
        </MultiSelectList>
      </MultiSelectContent>
    </MultiSelect>
  );
}
