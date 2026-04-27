import { useMemo, useState } from "react";
import { Link } from "react-router";
import { PlusIcon } from "@heroicons/react/24/outline";
import { Check, ChevronDown } from "lucide-react";
import { UserAvatar } from "@/components/UserAvatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { inputBase, inputFocusVisible } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { useAuth } from "@/lib/auth";
import type { Account } from "@/lib/api";

interface OrgSwitcherProps {
  activeAccount: string;
  onChange: (account: string) => void;
}

function AccountIcon({ account }: { account: Account }) {
  return (
    <UserAvatar
      handle={account.name}
      name={account.display_name || account.name}
      className="size-[18px] shrink-0"
    />
  );
}

export function OrgSwitcher({ activeAccount, onChange }: OrgSwitcherProps) {
  const { accounts } = useAuth();
  const [open, setOpen] = useState(false);

  const sorted = useMemo(
    () =>
      [...accounts].sort((a, b) =>
        a.type === "personal" ? -1 : b.type === "personal" ? 1 : a.name.localeCompare(b.name),
      ),
    [accounts],
  );

  const activeAccountObj = sorted.find((a) => a.name === activeAccount);

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="Switch account"
          className={cn(
            "flex h-8 w-full cursor-pointer items-center justify-between px-2.5 text-sm leading-none text-foreground transition-colors hover:bg-stone-200 dark:hover:bg-teal-800 sm:w-48",
            inputBase,
            inputFocusVisible,
          )}
        >
          {activeAccountObj && (
            <span className="inline-flex items-center gap-2">
              <AccountIcon account={activeAccountObj} />
              <span className="truncate">
                {activeAccountObj.name}
              </span>
            </span>
          )}
          <ChevronDown className="size-4 shrink-0 opacity-50" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        {sorted.map((a) => {
          const isActive = a.name === activeAccount;
          return (
            <DropdownMenuItem
              key={a.id}
              className="relative cursor-pointer pl-8"
              onSelect={(e) => e.preventDefault()}
              onClick={() => { onChange(a.name); setOpen(false); }}
            >
              <span className="pointer-events-none absolute left-2 flex size-3.5 items-center justify-center">
                {isActive && <Check className="size-4" />}
              </span>
              <AccountIcon account={a} />
              <span className="truncate">{a.name}</span>
            </DropdownMenuItem>
          );
        })}
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild className="cursor-pointer gap-2">
          <Link to="/organization/new">
            <PlusIcon className="size-4" />
            Create organization
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
