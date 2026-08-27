import { useMemo } from "react";
import { useLocation, useNavigate } from "react-router";
import { UserAvatar } from "@/components/UserAvatar";
import { SidebarNav } from "@/components/ui/sidebar-layout";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { comparePersonalFirst } from "@/lib/account-order";
import { useAuth } from "@/lib/auth";
import { settingsScopePath, settingsSectionFromPath } from "@/lib/settings-paths";

interface SettingsSidebarProps {
  account: string;
  children: React.ReactNode;
}

export function SettingsSidebar({ account, children }: SettingsSidebarProps) {
  const { accounts } = useAuth();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const sorted = useMemo(() => [...accounts].sort(comparePersonalFirst), [accounts]);
  const selected = sorted.find(a => a.name === account);

  function switchScope(name: string) {
    navigate(settingsScopePath(accounts, name, settingsSectionFromPath(pathname)));
  }

  return (
    <div className="flex w-full min-w-0 flex-col gap-3 md:w-48 md:shrink-0 md:pt-2">
      <div className="flex min-w-0 flex-col gap-1.5">
        <h1 className="px-3 text-heading-4 text-foreground">Settings</h1>
        <Select value={account} onValueChange={switchScope}>
          <SelectTrigger
            aria-label="Settings scope"
            className="h-8 max-w-full justify-start gap-2 px-2 text-body [&>svg]:ml-auto [&>svg]:shrink-0"
          >
            {selected && (
              <UserAvatar
                handle={selected.name}
                name={selected.display_name || selected.name}
                avatarUrl={selected.avatar_url}
                className="size-5 shrink-0"
              />
            )}
            <span className="truncate">
              {selected ? selected.display_name || selected.name : account}
            </span>
          </SelectTrigger>
          <SelectContent align="start" className="min-w-[var(--radix-select-trigger-width)]">
            {sorted.map(option => (
              <SelectItem key={option.id} value={option.name}>
                <span className="inline-flex min-w-0 items-center gap-2">
                  <UserAvatar
                    handle={option.name}
                    name={option.display_name || option.name}
                    avatarUrl={option.avatar_url}
                    className="size-[18px] shrink-0"
                  />
                  <span className="truncate">{option.display_name || option.name}</span>
                </span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <SidebarNav className="md:w-full md:pt-0">{children}</SidebarNav>
    </div>
  );
}
