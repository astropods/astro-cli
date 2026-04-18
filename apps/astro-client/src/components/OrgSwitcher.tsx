import { useId, useMemo, useState } from "react";
import { Link } from "react-router";
import { PlusIcon } from "@heroicons/react/24/outline";
import { Check, ChevronDown } from "lucide-react";
import { StarIcon } from "@heroicons/react/24/outline";
import { UserAvatar } from "@/components/UserAvatar";
import { Label } from "@/components/ui/label";
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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface OrgSwitcherProps {
  activeAccount: string;
  defaultAccount?: string;
  onChange: (account: string) => void;
  onSetDefault: (account: string) => void;
  compact?: boolean;
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

export function OrgSwitcher({
  activeAccount,
  defaultAccount,
  onChange,
  onSetDefault,
  compact = false,
}: OrgSwitcherProps) {
  const { accounts } = useAuth();
  const selectId = useId();

  const sorted = useMemo(
    () =>
      [...accounts].sort((a, b) =>
        a.type === "personal"
          ? -1
          : b.type === "personal"
            ? 1
            : a.name.localeCompare(b.name),
      ),
    [accounts],
  );

  const activeAccountObj = sorted.find((a) => a.name === activeAccount);
  const [open, setOpen] = useState(false);

  return (
    <div className="flex items-center gap-2">
      {!compact && (
        <Label
          htmlFor={selectId}
          className="mb-0 hidden shrink-0 cursor-default select-none font-sans text-xs font-medium normal-case text-foreground sm:inline"
        >
          View
        </Label>
      )}
      <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger asChild>
          {compact ? (
            <button
              id={selectId}
              type="button"
              className="flex h-8 cursor-pointer items-center gap-1.5 rounded-sm px-2.5 text-sm font-medium text-foreground transition-colors hover:bg-stone-200 dark:hover:bg-stone-700"
            >
              {activeAccountObj && (
                <span className="inline-flex items-center gap-1.5">
                  <AccountIcon account={activeAccountObj} />
                  <span className="max-w-28 truncate">
                    {activeAccountObj.name}
                  </span>
                </span>
              )}
              <ChevronDown className="size-3.5 shrink-0 opacity-50" />
            </button>
          ) : (
            <button
              id={selectId}
              type="button"
              className={cn(
                "flex h-8 w-56 items-center justify-between px-2.5 py-0 text-sm leading-none text-foreground",
                inputBase,
                inputFocusVisible,
              )}
            >
              {activeAccountObj && (
                <span className="inline-flex items-center gap-2">
                  <AccountIcon account={activeAccountObj} />
                  <span className="truncate">
                    {activeAccountObj.display_name || activeAccountObj.name}
                  </span>
                </span>
              )}
              <ChevronDown className="size-4 shrink-0 opacity-50" />
            </button>
          )}
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          {sorted.map((a) => {
            const isActive = a.name === activeAccount;
            const isDefault = a.name === defaultAccount;
            return (
              <DropdownMenuItem
                key={a.id}
                className="group relative cursor-pointer pl-8 pr-0"
                onSelect={(e) => e.preventDefault()}
                onClick={() => { onChange(a.name); setOpen(false); }}
              >
                <span className="pointer-events-none absolute left-2 flex size-3.5 items-center justify-center">
                  {isActive && <Check className="size-4" />}
                </span>
                <AccountIcon account={a} />
                <span className="truncate">{a.name}</span>
                {isDefault ? (
                  <button
                    type="button"
                    aria-label="Default view"
                    className="ml-auto mr-2 hidden shrink-0 cursor-pointer opacity-100 sm:block"
                    onClick={(e) => {
                      e.stopPropagation();
                      onSetDefault(a.name);
                    }}
                  >
                    <StarIcon className="size-3.5 fill-current text-primary" />
                  </button>
                ) : (
                  <TooltipProvider delayDuration={500}>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          aria-label="Set as default view"
                          className="ml-auto mr-2 hidden shrink-0 cursor-pointer opacity-0 transition-opacity group-data-[highlighted]:opacity-100 sm:block"
                          onClick={(e) => {
                            e.stopPropagation();
                            onSetDefault(a.name);
                          }}
                        >
                          <StarIcon className="size-3.5 text-muted-foreground" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side="top">
                        Set as default view
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                )}
              </DropdownMenuItem>
            );
          })}
          <DropdownMenuSeparator />
          <DropdownMenuItem asChild className="gap-2">
            <Link to="/organization/new">
              <PlusIcon className="size-4" />
              Create organization
            </Link>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
