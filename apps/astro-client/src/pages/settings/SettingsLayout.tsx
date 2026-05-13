import { Outlet } from "react-router";
import { UserIcon, ChartBarIcon, BuildingOfficeIcon, KeyIcon } from "@heroicons/react/24/outline";
import { FlaskConical, ScrollText } from "lucide-react";
import {
  SidebarLayout,
  SidebarNav,
  SidebarNavItem,
  SidebarBody,
} from "@/components/ui/sidebar-layout";

function SettingsContent() {
  return (
    <div className="flex-1 overflow-y-auto bg-background flex flex-col">
      <div className="@container w-full px-4 pb-6 pt-8 md:px-6 md:pb-8 md:pt-10 max-w-[1120px] mx-auto flex-1">
        <SidebarLayout>
        <SidebarNav label="Settings" className="md:w-48">
          <SidebarNavItem to="/settings/account">
            <span className="flex items-center gap-2">
              <UserIcon className="size-3.5" />
              Account
            </span>
          </SidebarNavItem>
          <SidebarNavItem to="/settings/usage">
            <span className="flex items-center gap-2">
              <ChartBarIcon className="size-3.5" />
              Usage
            </span>
          </SidebarNavItem>
          <SidebarNavItem to="/settings/secrets">
            <span className="flex items-center gap-2">
              <KeyIcon className="size-3.5" />
              Variables & Secrets
            </span>
          </SidebarNavItem>
          <SidebarNavItem to="/settings/organizations">
            <span className="flex items-center gap-2">
              <BuildingOfficeIcon className="size-3.5" />
              Organizations
            </span>
          </SidebarNavItem>
          <SidebarNavItem to="/settings/audit-log">
            <span className="flex items-center gap-2">
              <ScrollText className="size-3.5" />
              Audit Log
            </span>
          </SidebarNavItem>
          <SidebarNavItem to="/settings/experiments">
            <span className="flex items-center gap-2">
              <FlaskConical className="size-3.5" />
              Experiments
            </span>
          </SidebarNavItem>
        </SidebarNav>
        <SidebarBody>
          <Outlet />
        </SidebarBody>
        </SidebarLayout>
      </div>
      <p className="text-body-sm text-faint-foreground text-center w-full py-6">Astro AI is currently in beta.</p>
    </div>
  );
}

export default function SettingsLayout() {
  return <SettingsContent />;
}
