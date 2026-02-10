import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router-dom";

import { BreadcrumbHeader } from "@/components/BreadcrumbHeader";

const meta = {
  title: "Layout/BreadcrumbHeader",
  component: BreadcrumbHeader,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
  argTypes: {
    onBack: { table: { disable: true } },
    onForward: { table: { disable: true } },
    onShare: { table: { disable: true } },
    onClose: { table: { disable: true } },
  },
  args: {
    onBack: () => {},
    onForward: () => {},
    onShare: () => {},
    onClose: () => {},
  },
} satisfies Meta<typeof BreadcrumbHeader>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    breadcrumbs: [
      { label: "Browse Agents", to: "/hire" },
      { label: "Customer Insights Engine" },
    ],
  },
};

export const DeepBreadcrumbs: Story = {
  name: "Deep Breadcrumbs",
  args: {
    breadcrumbs: [
      { label: "My Agents", to: "/agents" },
      { label: "Incident Command", to: "/agents/incident-command" },
      { label: "Settings" },
    ],
  },
};

export const SingleBreadcrumb: Story = {
  name: "Single Breadcrumb",
  args: {
    breadcrumbs: [{ label: "Hire Agents" }],
  },
};

export const NavigationOnly: Story = {
  name: "Navigation Only",
  args: {
    breadcrumbs: [
      { label: "Browse Agents", to: "/hire" },
      { label: "Customer Insights Engine" },
    ],
    onShare: undefined,
    onClose: undefined,
  },
};

export const BreadcrumbsOnly: Story = {
  name: "Breadcrumbs Only",
  args: {
    breadcrumbs: [
      { label: "Browse Agents", to: "/hire" },
      { label: "Customer Insights Engine" },
    ],
    onBack: undefined,
    onForward: undefined,
    onShare: undefined,
    onClose: undefined,
  },
};

export const BackDisabled: Story = {
  name: "Back Disabled",
  args: {
    breadcrumbs: [
      { label: "Browse Agents", to: "/hire" },
      { label: "Customer Insights Engine" },
    ],
    canGoBack: false,
  },
};
