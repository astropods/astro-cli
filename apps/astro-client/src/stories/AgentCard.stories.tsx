import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
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
    slug: "acme/customer-insight-engine",
    account: "acme",
    name: "Customer Insight Engine",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
    integrations: ["Slack", "GitHub", "Linear", "Notion"],
    categories: ["Analytics"],
  },
};

export const LongDescription: Story = {
  args: {
    slug: "acme/personalized-support-responses",
    account: "acme",
    name: "Personalized Support Responses",
    description:
      "This agent helps your support team respond faster by drafting personalized replies that consider the customer's history, previous interactions, and the specific context of their issue. Agents can review and send with a single click.",
    integrations: ["Slack", "Notion"],
    categories: ["Customer Support", "Analytics"],
  },
};

export const FewIntegrations: Story = {
  args: {
    slug: "acme/security-monitor",
    account: "acme",
    name: "Security Monitor",
    description:
      "Continuously scans for vulnerabilities and alerts your security team.",
    integrations: ["GitHub"],
    categories: ["Security"],
  },
};

export const ManyIntegrations: Story = {
  args: {
    slug: "acme/product-research-intel",
    account: "acme",
    name: "Product Research Intel",
    description:
      "Aggregates product research and competitive intelligence from multiple sources.",
    integrations: ["Slack", "Notion", "GitHub", "Linear", "Google Drive"],
    categories: ["Analytics", "Developer Tools"],
  },
};
