import { ChevronDownIcon, BuildingOffice2Icon, CheckIcon } from "@heroicons/react/24/outline";
import { UserAvatar } from "@/components/UserAvatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
  const { accounts, personalAccount } = useAuth();
  const current = accounts.find((a) => a.name === activeAccount) ?? personalAccount;

  if (!current) return <span className="font-semibold">{activeAccount}</span>;

  const label = current.display_name || current.name;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="inline-flex items-center gap-1.5 rounded-sm px-1.5 py-1 font-mono text-mono-sm font-semibold text-foreground transition-colors hover:bg-accent"
        >
          <AccountIcon account={current} />
          {label}
          <ChevronDownIcon className="size-3 text-muted-foreground" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56 p-2">
        {accounts.map((account) => (
          <DropdownMenuItem
            key={account.id}
            onSelect={() => onChange(account.name)}
            className="gap-2.5"
          >
            <AccountIcon account={account} />
            <span className="flex-1 truncate">{account.display_name || account.name}</span>
            {account.name === activeAccount && (
              <CheckIcon className="size-3.5 text-teal-600 shrink-0" />
            )}
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild className="gap-2">
          <a href="/organization/new">Create organization</a>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
