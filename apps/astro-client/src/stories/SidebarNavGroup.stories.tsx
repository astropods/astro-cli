import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router-dom";
import {
  HomeIcon,
  UserGroupIcon,
  BriefcaseIcon,
  DocumentTextIcon,
  ChatBubbleLeftIcon,
  PaperAirplaneIcon,
} from "@heroicons/react/24/outline";

import { SidebarNavGroup } from "@/components/sidebar/SidebarNavGroup";
import { SidebarProvider, Sidebar, SidebarContent } from "@/components/ui/sidebar";

const meta = {
  title: "Components/Sidebar/NavGroup",
  component: SidebarNavGroup,
  decorators: [
    (Story) => (
      <MemoryRouter initialEntries={["/"]}>
        <SidebarProvider className="min-h-0">
          <Sidebar collapsible="none" className="h-auto">
            <SidebarContent>
              <Story />
            </SidebarContent>
          </Sidebar>
        </SidebarProvider>
      </MemoryRouter>
    ),
  ],
  argTypes: {
    label: { control: "text" },
    items: { control: false },
  },
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof SidebarNavGroup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const PrimaryNav: Story = {
  name: "Primary Navigation",
  args: {
    items: [
      { label: "Home", icon: HomeIcon, to: "/", isActive: true },
      { label: "Hire Agents", icon: UserGroupIcon, to: "/hire" },
      { label: "Your Agents", icon: BriefcaseIcon, to: "/agents" },
    ],
  },
};

export const WithLabel: Story = {
  name: "With Group Label",
  args: {
    label: "Resources",
    items: [
      { label: "Docs", icon: DocumentTextIcon, to: "https://docs.example.com", external: true },
      { label: "Community", icon: ChatBubbleLeftIcon, to: "https://community.example.com", external: true },
      { label: "Request Agent", icon: PaperAirplaneIcon, to: "/request-agent" },
    ],
  },
};

export const TextOnly: Story = {
  name: "Text Only (no icons)",
  args: {
    label: "Recent",
    items: [
      { label: "Research Assistant", to: "/agents/research-assistant" },
      { label: "Code Reviewer", to: "/agents/code-reviewer" },
      { label: "Data Analyst", to: "/agents/data-analyst" },
    ],
  },
};

export const WithActiveItem: Story = {
  name: "With Active Item",
  args: {
    label: "Navigation",
    items: [
      { label: "Home", icon: HomeIcon, to: "/" },
      { label: "Hire Agents", icon: UserGroupIcon, to: "/hire", isActive: true },
      { label: "Your Agents", icon: BriefcaseIcon, to: "/agents" },
    ],
  },
};
