import { useCallback, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
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
import { comparePersonalFirst } from "@/lib/account-order";
import { useAuth } from "@/lib/auth";
import {
  getOrgSwitchProgress,
  subscribeOrgSwitchProgress,
} from "@/lib/org-switch-progress";
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
      avatarUrl={account.avatar_url}
      className="size-[18px] shrink-0"
    />
  );
}

export function OrgSwitcher({ activeAccount, onChange }: OrgSwitcherProps) {
  const { accounts } = useAuth();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const orgSwitching = useSyncExternalStore(
    subscribeOrgSwitchProgress,
    getOrgSwitchProgress,
    () => false,
  );

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (next && orgSwitching) return;
      setOpen(next);
    },
    [orgSwitching],
  );

  const handleSelect = useCallback(
    (accountName: string) => {
      onChange(accountName);
      setOpen(false);
      triggerRef.current?.blur();
    },
    [onChange],
  );

  useLayoutEffect(() => {
    if (orgSwitching) setOpen(false);
  }, [orgSwitching]);

  const sorted = useMemo(
    () => [...accounts].sort(comparePersonalFirst),
    [accounts],
  );

  const activeAccountObj = sorted.find((a) => a.name === activeAccount);

  const menuOpen = orgSwitching ? false : open;

  return (
    <DropdownMenu open={menuOpen} onOpenChange={handleOpenChange}>
      <DropdownMenuTrigger asChild>
        <button
          ref={triggerRef}
          type="button"
          aria-label="Switch account"
          className={cn(
            "flex h-8 w-full cursor-pointer items-center justify-between px-2.5 text-sm leading-none text-foreground transition-colors !bg-white dark:!bg-transparent hover:!bg-slate-50 dark:hover:!bg-slate-800 sm:w-48",
            inputBase,
            inputFocusVisible,
          )}
        >
          {activeAccountObj && (
            <span className="flex min-w-0 items-center gap-2">
              <AccountIcon account={activeAccountObj} />
              <span className="truncate">
                {activeAccountObj.display_name || activeAccountObj.name}
              </span>
            </span>
          )}
          <ChevronDown className="size-4 shrink-0 opacity-50" />
        </button>
      </DropdownMenuTrigger>
      {!orgSwitching ? (
      <DropdownMenuContent
        align="end"
        className="w-48"
        onCloseAutoFocus={(e) => e.preventDefault()}
      >
        {sorted.map((a) => {
          const isActive = a.name === activeAccount;
          return (
            <DropdownMenuItem
              key={a.id}
              className="relative cursor-pointer pl-8"
              onSelect={() => handleSelect(a.name)}
            >
              <span className="pointer-events-none absolute left-2 flex size-3.5 items-center justify-center">
                {isActive && <Check className="size-4" />}
              </span>
              <AccountIcon account={a} />
              <span className="truncate">{a.display_name || a.name}</span>
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
      ) : null}
    </DropdownMenu>
  );
}
