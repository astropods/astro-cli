import { Outlet } from "react-router";
import { UserIcon, ChartBarIcon } from "@heroicons/react/24/outline";
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
    <div className="@container w-full flex-1 overflow-y-auto bg-surface px-6 pb-6 pt-8 md:px-8 md:pb-8 md:pt-10 max-w-[820px] mx-auto">
      <SidebarLayout>
        <SidebarNav label="Settings">
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
