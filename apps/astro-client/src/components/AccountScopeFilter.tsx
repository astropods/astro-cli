import { useMemo } from "react";
import { UserAvatar } from "@/components/UserAvatar";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select";
import { comparePersonalFirst } from "@/lib/account-order";
import { useAuth } from "@/lib/auth";
import { cn } from "@/lib/utils";

interface AccountScopeFilterProps {
  value: string;
  onChange: (account: string) => void;
  className?: string;
}

export function AccountScopeFilter({
  value,
  onChange,
  className,
}: AccountScopeFilterProps) {
  const { accounts } = useAuth();
  const sorted = useMemo(
    () => [...accounts].sort(comparePersonalFirst),
    [accounts],
  );
  const selected = sorted.find((account) => account.name === value);

  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        aria-label="Scope by account"
        className={cn(
          "group h-auto w-auto max-w-full justify-start gap-2 rounded border-transparent bg-transparent px-2 py-1 text-heading-1 text-foreground hover:border-border hover:bg-surface data-[state=open]:border-border data-[state=open]:bg-surface [&>svg]:ml-0 [&>svg]:shrink-0 [&>svg]:transition-transform data-[state=open]:[&>svg]:rotate-180",
          className,
        )}
      >
        {selected && (
          <UserAvatar
            handle={selected.name}
            name={selected.display_name || selected.name}
            avatarUrl={selected.avatar_url}
            className="size-6 shrink-0"
          />
        )}
        <span className="truncate">
          {selected ? selected.display_name || selected.name : "Select account"}
        </span>
      </SelectTrigger>
      <SelectContent
        align="start"
        sideOffset={6}
        className="min-w-[var(--radix-select-trigger-width)]"
      >
        {sorted.map((account) => (
          <SelectItem key={account.id} value={account.name}>
            <span className="inline-flex min-w-0 items-center gap-2">
              <UserAvatar
                handle={account.name}
                name={account.display_name || account.name}
                avatarUrl={account.avatar_url}
                className="size-[18px] shrink-0"
              />
              <span className="truncate">
                {account.display_name || account.name}
              </span>
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
