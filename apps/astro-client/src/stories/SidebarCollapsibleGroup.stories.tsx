import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router-dom";

import { SidebarCollapsibleGroup } from "@/components/sidebar/SidebarCollapsibleGroup";
import { SidebarProvider, Sidebar, SidebarContent } from "@/components/ui/sidebar";

const meta = {
  title: "Components/Sidebar/CollapsibleGroup",
  component: SidebarCollapsibleGroup,
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
    defaultOpen: { control: "boolean" },
    items: { control: false },
  },
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof SidebarCollapsibleGroup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    label: "My Agents",
    defaultOpen: true,
    items: [
      { label: "Research Assistant", to: "/agents/research-assistant" },
      { label: "Code Reviewer", to: "/agents/code-reviewer" },
      { label: "Data Analyst", to: "/agents/data-analyst" },
      { label: "Content Writer", to: "/agents/content-writer" },
      { label: "QA Tester", to: "/agents/qa-tester" },
    ],
  },
};

export const Collapsed: Story = {
  args: {
    label: "My Agents",
    defaultOpen: false,
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
    label: "My Agents",
    defaultOpen: true,
    items: [
      { label: "Research Assistant", to: "/agents/research-assistant" },
      { label: "Code Reviewer", to: "/agents/code-reviewer", isActive: true },
      { label: "Data Analyst", to: "/agents/data-analyst" },
    ],
  },
};

export const SingleItem: Story = {
  args: {
    label: "Favorites",
    defaultOpen: true,
    items: [
      { label: "Research Assistant", to: "/agents/research-assistant" },
    ],
  },
};

export const ManyItems: Story = {
  name: "Long List",
  args: {
    label: "All Agents",
    defaultOpen: true,
    items: Array.from({ length: 12 }, (_, i) => ({
      label: `Agent ${i + 1}`,
      to: `/agents/agent-${i + 1}`,
    })),
  },
};
