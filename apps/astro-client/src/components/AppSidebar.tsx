import {
  UserGroupIcon,
  HomeIcon,
  BriefcaseIcon,
  SparklesIcon,
  WrenchIcon,
  CommandLineIcon,
  DocumentTextIcon,
  ChatBubbleLeftIcon,
  PaperAirplaneIcon,
  ChatBubbleOvalLeftIcon,
} from "@heroicons/react/24/outline";
import type { User } from "../lib/api";
import { useDeployments } from "@/api/queries";

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

import { SidebarNavGroup, type NavItem } from "./sidebar/SidebarNavGroup";
import { SidebarCollapsibleGroup, type CollapsibleItem } from "./sidebar/SidebarCollapsibleGroup";
import { SidebarUserMenu } from "./sidebar/SidebarUserMenu";
import { useLocation } from "react-router-dom";

const primaryNav = [
  { label: "Home", icon: HomeIcon, to: "/", end: true },
  { label: "Hire Agents", icon: UserGroupIcon, to: "/hire", end: false },
  { label: "My Agents", icon: BriefcaseIcon, to: "/agents", end: true },
  { label: "Operator", icon: WrenchIcon, to: "/operator", end: false },
];

const resourceItems: NavItem[] = [
  { label: "CLI", icon: CommandLineIcon, to: "/dev" },
  { label: "Docs", icon: DocumentTextIcon, to: "https://docs.example.com", external: true },
  { label: "Community", icon: ChatBubbleLeftIcon, to: "https://community.example.com", external: true },
  { label: "Request Agent", icon: PaperAirplaneIcon, to: "/request-agent" },
];

export interface AppSidebarProps {
  user?: User | null;
  isLoading?: boolean;
  isAuthenticated?: boolean;
  onSignIn?: () => void;
  onSignOut?: () => void;
  onTalkToAstro?: () => void;
}

export function AppSidebar({
  user,
  isLoading = false,
  isAuthenticated = false,
  onSignIn,
  onSignOut,
  onTalkToAstro,
}: AppSidebarProps) {
  const location = useLocation();
  const { data: deployments } = useDeployments(isAuthenticated);

  const primaryNavItems: NavItem[] = primaryNav.map((item) => ({
    ...item,
    isActive: item.end
      ? location.pathname === item.to
      : location.pathname.startsWith(item.to),
  }));

  const agentItems: CollapsibleItem[] = (deployments?.deployments ?? []).map((d) => ({
    label: d.name,
    to: `/agents/${encodeURIComponent(d.name)}`,
  }));

  return (
    <Sidebar variant="inset">
      <SidebarHeader>
        <div className="flex items-center justify-between px-2 py-1">
          <div className="flex items-center gap-2">
            <SparklesIcon className="size-5" />
            <span className="text-lg font-bold">Astro</span>
          </div>
          <SidebarTrigger />
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarNavGroup items={primaryNavItems} />
        {agentItems.length > 0 && (
          <SidebarCollapsibleGroup label="My Agents" items={agentItems} />
        )}
        <SidebarNavGroup label="Resources" items={resourceItems} />
      </SidebarContent>

      {/* Talk to Astro CTA */}
      <div className="px-3 py-2">
        <Button className="w-full" onClick={onTalkToAstro}>
          <ChatBubbleOvalLeftIcon />
          Talk to Astro
        </Button>
      </div>

      <div className="px-3">
        <Separator className="bg-sidebar-border" />
      </div>

      <SidebarFooter className="py-3">
        <SidebarUserMenu
          user={user}
          isLoading={isLoading}
          isAuthenticated={isAuthenticated}
          onSignIn={onSignIn}
          onSignOut={onSignOut}
        />
      </SidebarFooter>
    </Sidebar>
  );
}

export { SidebarInset };
