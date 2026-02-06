import type React from "react";
import { NavLink } from "react-router-dom";

import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";

export interface NavItem {
  label: string;
  icon?: React.ComponentType<React.SVGProps<SVGSVGElement>>;
  to: string;
  external?: boolean;
  isActive?: boolean;
}

export interface SidebarNavGroupProps {
  label?: string;
  items: NavItem[];
}

export function SidebarNavGroup({ label, items }: SidebarNavGroupProps) {
  return (
    <SidebarGroup>
      {label && <SidebarGroupLabel>{label}</SidebarGroupLabel>}
      <SidebarGroupContent>
        <SidebarMenu>
          {items.map((item) => (
            <SidebarMenuItem key={item.label}>
              <SidebarMenuButton asChild isActive={item.isActive}>
                {item.external ? (
                  <a href={item.to} target="_blank" rel="noopener noreferrer">
                    {item.icon && <item.icon />}
                    <span>{item.label}</span>
                  </a>
                ) : (
                  <NavLink to={item.to}>
                    {item.icon && <item.icon />}
                    <span>{item.label}</span>
                  </NavLink>
                )}
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}
