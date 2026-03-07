import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";

import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { Button } from "@/components/ui/button";

const meta = {
  title: "Design System/Composites/PageBreadcrumb",
  component: PageBreadcrumb,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
} satisfies Meta<typeof PageBreadcrumb>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    items: [
      { label: "Browse Agents", to: "/browse" },
      { label: "acme / support-bot", to: "/acme/support-bot" },
      { label: "Install" },
    ],
  },
};

export const TwoLevels: Story = {
  name: "Two Levels",
  args: {
    items: [
      { label: "Agents", to: "/agents" },
      { label: "support-bot" },
    ],
  },
};

export const SingleLevel: Story = {
  name: "Single Level",
  args: {
    items: [{ label: "Settings" }],
  },
};

export const WithActions: Story = {
  name: "With Actions",
  args: {
    items: [
      { label: "Agents", to: "/agents" },
      { label: "support-bot" },
    ],
    actions: (
      <Button variant="outline" size="sm">
        Edit
      </Button>
    ),
  },
};
