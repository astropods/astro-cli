import { Outlet } from "react-router";
import {
  SidebarLayout,
  SidebarNavItem,
  SidebarNavGroup,
  SidebarNavDivider,
  SidebarBody,
} from "@/components/ui/sidebar-layout";
import { SettingsSidebar } from "@/components/settings/SettingsSidebar";
import { Tag } from "@/components/Tag";
import { hasExperiments } from "@/lib/experiments";
import { useAuth } from "@/lib/auth";

function SettingsContent() {
  const { personalAccount } = useAuth();

  return (
    <div className="flex-1 overflow-y-auto bg-background flex flex-col">
      <div className="@container w-full px-4 pb-6 pt-8 md:px-6 md:pb-8 md:pt-10 max-w-[1120px] mx-auto flex-1">
        <SidebarLayout>
        <SettingsSidebar account={personalAccount?.name ?? ""}>
          <SidebarNavGroup label="Manage">
            <SidebarNavItem to="/settings/account">Account</SidebarNavItem>
            <SidebarNavItem to="/settings/billing">Billing</SidebarNavItem>
            <SidebarNavItem to="/settings/usage">Usage</SidebarNavItem>
            <SidebarNavItem to="/settings/notifications">Notifications</SidebarNavItem>
          </SidebarNavGroup>
          <SidebarNavGroup label="Access">
            <SidebarNavItem to="/settings/organizations">Organizations</SidebarNavItem>
            <SidebarNavItem to="/settings/audit-log">Audit Log</SidebarNavItem>
          </SidebarNavGroup>
          <SidebarNavGroup label="Integrations">
            <SidebarNavItem to="/settings/secrets">Variables &amp; Secrets</SidebarNavItem>
            <SidebarNavItem to="/settings/connectors">Connectors</SidebarNavItem>
            <SidebarNavItem to="/settings/api-keys">Data Sources</SidebarNavItem>
            <SidebarNavItem to="/settings/apps">OAuth Apps</SidebarNavItem>
          </SidebarNavGroup>
          {hasExperiments && (
            <>
              <SidebarNavDivider />
              <SidebarNavItem to="/settings/experiments" className="flex items-center gap-2">
                Experiments
                <Tag>Beta</Tag>
              </SidebarNavItem>
            </>
          )}
        </SettingsSidebar>
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
