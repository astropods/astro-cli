import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { DeployedAgentCard, type DeployedAgentCardProps } from "@/components/DeployedAgentCard";

const meta = {
  title: "Features/Agents/DeployedAgentCard",
  component: DeployedAgentCard,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div className="max-w-sm">
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
} satisfies Meta<DeployedAgentCardProps>;

export default meta;
type Story = StoryObj<DeployedAgentCardProps>;

export const Active: Story = {
  args: {
    name: "Incident Command",
    deploymentId: "dep-incident-command",
    account: "acme",
    href: "/agents/incident-command/review/v2",
    status: "active",
    requests: 156,
    lastActive: "2 min ago",
    installedAt: "Jan 12, 2026",
    updatedAt: "Feb 17, 2026",

  },
};

export const Inactive: Story = {
  args: {
    name: "Security Monitor",
    deploymentId: "dep-security-monitor",
    account: "acme",
    href: "/agents/security-monitor/review/v1",
    status: "inactive",
    requests: 1024,
    lastActive: "3 days ago",
    installedAt: "Dec 5, 2025",
    updatedAt: "Jan 20, 2026",

  },
};

export const Pending: Story = {
  args: {
    name: "Customer Insight Engine",
    deploymentId: "dep-customer-insight-engine",
    account: "acme",
    href: "/agents/customer-insight-engine/review/v1",
    status: "pending",
    requests: 0,
    lastActive: "Never",
    installedAt: "Feb 24, 2026",
    updatedAt: "Feb 24, 2026",

  },
};

export const Error: Story = {
  args: {
    name: "Data Pipeline Orchestrator",
    deploymentId: "dep-data-pipeline",
    account: "acme",
    href: "/agents/data-pipeline/review/v3",
    status: "error",
    requests: 8432,
    lastActive: "5 min ago",
    installedAt: "Oct 1, 2025",
    updatedAt: "Feb 20, 2026",

  },
};

export const WithCustomAvatar: Story = {
  args: {
    name: "Incident Command",
    deploymentId: "dep-incident-command",
    account: "acme",
    href: "/agents/incident-command/review/v2",
    status: "active",
    requests: 156,
    lastActive: "2 min ago",
    installedAt: "Jan 12, 2026",
    updatedAt: "Feb 17, 2026",

    avatarUrl: "https://picsum.photos/seed/agent/36/36",
  },
};

export const LongName: Story = {
  args: {
    name: "Automated Personalized Customer Support Response Generator",
    deploymentId: "dep-support-response",
    account: "enterprise-corp",
    href: "/agents/support-response/review/v1",
    status: "active",
    requests: 54321,
    lastActive: "Just now",
    installedAt: "Nov 15, 2025",
    updatedAt: "Feb 22, 2026",

  },
};

export const AllStatuses: Story = {
  args: {
    name: "Incident Command",
    deploymentId: "dep-incident-command",
    account: "acme",
    href: "/agents/incident-command/review/v2",
    status: "active",
    requests: 156,
    lastActive: "2 min ago",
    installedAt: "Jan 12, 2026",
    updatedAt: "Feb 17, 2026",

  },
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div className="grid max-w-3xl grid-cols-2 gap-4">
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
  render: () => (
    <>
      <DeployedAgentCard
        name="Incident Command"
        deploymentId="dep-incident-command"
        account="acme"
        href="/agents/incident-command/review/v2"
        status="active"
        requests={156}
        lastActive="2 min ago"
        installedAt="Jan 12, 2026"
        updatedAt="Feb 17, 2026"

      />
      <DeployedAgentCard
        name="Security Monitor"
        deploymentId="dep-security-monitor"
        account="acme"
        href="/agents/security-monitor/review/v1"
        status="inactive"
        requests={1024}
        lastActive="3 days ago"
        installedAt="Dec 5, 2025"
        updatedAt="Jan 20, 2026"

      />
      <DeployedAgentCard
        name="Customer Insight Engine"
        deploymentId="dep-customer-insight-engine"
        account="acme"
        href="/agents/customer-insight-engine/review/v1"
        status="pending"
        requests={0}
        lastActive="Never"
        installedAt="Feb 24, 2026"
        updatedAt="Feb 24, 2026"

      />
      <DeployedAgentCard
        name="Data Pipeline Orchestrator"
        deploymentId="dep-data-pipeline"
        account="acme"
        href="/agents/data-pipeline/review/v3"
        status="error"
        requests={8432}
        lastActive="5 min ago"
        installedAt="Oct 1, 2025"
        updatedAt="Feb 20, 2026"

      />
    </>
  ),
};
