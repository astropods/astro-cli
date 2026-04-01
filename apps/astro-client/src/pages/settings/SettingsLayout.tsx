import { Outlet } from "react-router";
import { UserIcon, ChartBarIcon, BuildingOfficeIcon } from "@heroicons/react/24/outline";
import { KeyRound } from "lucide-react";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import {
  SidebarLayout,
  SidebarNav,
  SidebarNavItem,
  SidebarBody,
} from "@/components/ui/sidebar-layout";

function SettingsContent() {
  return (
    <div className="@container w-full flex-1 overflow-y-auto bg-surface px-4 pb-6 pt-8 md:px-6 md:pb-8 md:pt-10 max-w-[1120px] mx-auto">
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
              <KeyRound className="size-3.5" />
              Secrets & Variables
            </span>
          </SidebarNavItem>
          <SidebarNavItem to="/settings/organizations">
            <span className="flex items-center gap-2">
              <BuildingOfficeIcon className="size-3.5" />
              Organizations
            </span>
          </SidebarNavItem>
        </SidebarNav>
        <SidebarBody>
          <Outlet />
        </SidebarBody>
      </SidebarLayout>
    </div>
  );
}

export default function SettingsLayout() {
  return (
    <ProtectedRoute>
      <SettingsContent />
    </ProtectedRoute>
  );
}
