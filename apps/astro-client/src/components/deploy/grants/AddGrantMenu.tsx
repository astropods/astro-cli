import { Plus, Globe, Building2, User as UserIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { UserAvatar } from "@/components/UserAvatar";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
} from "@/components/ui/dropdown-menu";
import type { Account, AuthGrant } from "@/lib/api";

export interface AddGrantMenuProps {
  /** Accounts available as account-scoped grant subjects. Personal accounts are filtered out — those go through the member picker. */
  accounts: Account[];
  /** Predicate for marking a candidate grant as already added (disables the row). */
  isAlreadyGranted: (grant: AuthGrant) => boolean;
  /** Add an "anyone" or "org" grant directly. */
  onPick: (grant: AuthGrant) => void;
  /** Open the user picker (web-only). When omitted, the user item is hidden. */
  onPickUser?: () => void;
}

export function AddGrantMenu({ accounts, isAlreadyGranted, onPick, onPickUser }: AddGrantMenuProps) {
  // Account-scope grants only make sense for organizations; a personal account's
  // member set is just one user, which is what the user picker covers.
  const orgAccounts = accounts.filter((a) => a.type === "organization");

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="outline" size="sm">
          <Plus className="h-3.5 w-3.5" />
          Add access
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-[14rem]">
        <DropdownMenuItem
          onSelect={() => onPick({ anyone: true })}
          disabled={isAlreadyGranted({ anyone: true })}
        >
          <Globe className="h-4 w-4" />
          Anyone
        </DropdownMenuItem>
        {orgAccounts.length > 0 && (
          <DropdownMenuSub>
            <DropdownMenuSubTrigger>
              <Building2 className="h-4 w-4" />
              Members of organization
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent className="min-w-[14rem]">
              {orgAccounts.map((a) => (
                <DropdownMenuItem
                  key={a.id}
                  onSelect={() => onPick({ org: a.id })}
                  disabled={isAlreadyGranted({ org: a.id })}
                >
                  <UserAvatar
                    handle={a.name}
                    name={a.display_name ?? a.name}
                    className="size-5 rounded-sm"
                  />
                  <span className="flex flex-col min-w-0">
                    <span className="truncate text-foreground">
                      {a.display_name ?? a.name}
                    </span>
                    <span className="truncate text-[11px] text-muted-foreground">
                      @{a.name}
                    </span>
                  </span>
                </DropdownMenuItem>
              ))}
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        )}
        {onPickUser && (
          <DropdownMenuItem
            onSelect={(e) => {
              e.preventDefault();
              onPickUser();
            }}
          >
            <UserIcon className="h-4 w-4" />
            Specific user…
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
