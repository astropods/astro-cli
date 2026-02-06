import { NavLink } from "react-router-dom";
import { ChevronDownIcon } from "@heroicons/react/24/outline";

import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";

export interface CollapsibleItem {
  label: string;
  to: string;
  isActive?: boolean;
}

export interface SidebarCollapsibleGroupProps {
  label: string;
  items: CollapsibleItem[];
  defaultOpen?: boolean;
}

export function SidebarCollapsibleGroup({
  label,
  items,
  defaultOpen = true,
}: SidebarCollapsibleGroupProps) {
  return (
    <Collapsible defaultOpen={defaultOpen} className="group/collapsible">
      <SidebarGroup>
        <SidebarGroupLabel asChild>
          <CollapsibleTrigger className="flex w-full items-center justify-between rounded-md hover:bg-sidebar-accent hover:text-sidebar-accent-foreground cursor-pointer">
            {label}
            <ChevronDownIcon className="size-4 transition-transform group-data-[state=closed]/collapsible:rotate-[-90deg]" />
          </CollapsibleTrigger>
        </SidebarGroupLabel>
        <CollapsibleContent>
          <SidebarGroupContent>
            <SidebarMenu>
              {items.map((item) => (
                <SidebarMenuItem key={item.label}>
                  <SidebarMenuButton asChild isActive={item.isActive}>
                    <NavLink to={item.to}>
                      <span>{item.label}</span>
                    </NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </CollapsibleContent>
      </SidebarGroup>
    </Collapsible>
  );
}
