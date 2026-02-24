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
    categories: ["Customer Support", "Analytics"],
  },
};

export const FewCategories: Story = {
  args: {
    slug: "acme/security-monitor",
    account: "acme",
    name: "Security Monitor",
    description:
      "Continuously scans for vulnerabilities and alerts your security team.",
    categories: ["Security"],
  },
};

export const ManyCategories: Story = {
  args: {
    slug: "acme/product-research-intel",
    account: "acme",
    name: "Product Research Intel",
    description:
      "Aggregates product research and competitive intelligence from multiple sources.",
    categories: ["Analytics", "Developer Tools"],
  },
};
