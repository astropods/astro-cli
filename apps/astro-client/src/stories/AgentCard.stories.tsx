import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router-dom";
import { AgentCard } from "@/components/AgentCard";

const meta = {
  title: "Components/AgentCard",
  component: AgentCard,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div className="max-w-sm">
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
} satisfies Meta<typeof AgentCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    slug: "customer-insight-engine",
    name: "Customer Insight Engine",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
    integrations: ["Slack", "GitHub", "Linear", "Notion"],
    categories: ["Analytics"],
    onInstall: (slug) => console.log("Install", slug),
  },
};

export const LongDescription: Story = {
  args: {
    slug: "personalized-support-responses",
    name: "Personalized Support Responses",
    description:
      "This agent helps your support team respond faster by drafting personalized replies that consider the customer's history, previous interactions, and the specific context of their issue. Agents can review and send with a single click.",
    integrations: ["Slack", "Notion"],
    categories: ["Customer Support", "Analytics"],
    onInstall: (slug) => console.log("Install", slug),
  },
};

export const FewIntegrations: Story = {
  args: {
    slug: "security-monitor",
    name: "Security Monitor",
    description:
      "Continuously scans for vulnerabilities and alerts your security team.",
    integrations: ["GitHub"],
    categories: ["Security"],
    onInstall: (slug) => console.log("Install", slug),
  },
};

export const ManyIntegrations: Story = {
  args: {
    slug: "product-research-intel",
    name: "Product Research Intel",
    description:
      "Aggregates product research and competitive intelligence from multiple sources.",
    integrations: ["Slack", "Notion", "GitHub", "Linear", "Google Drive"],
    categories: ["Analytics", "Developer Tools"],
    onInstall: (slug) => console.log("Install", slug),
  },
};
