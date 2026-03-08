import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { AgentCard } from "@/components/AgentCard";

const meta = {
  title: "Features/Agents/AgentCard",
  component: AgentCard,
} satisfies Meta<typeof AgentCard>;

export default meta;
type Story = StoryObj<typeof meta>;

const singleCardDecorator = (Story: React.ComponentType) => (
  <MemoryRouter>
    <div className="max-w-sm">
      <Story />
    </div>
  </MemoryRouter>
);

export const Default: Story = {
  decorators: [singleCardDecorator],
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
  decorators: [singleCardDecorator],
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
  decorators: [singleCardDecorator],
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
  decorators: [singleCardDecorator],
  args: {
    slug: "acme/product-research-intel",
    account: "acme",
    name: "Product Research Intel",
    description:
      "Aggregates product research and competitive intelligence from multiple sources.",
    categories: ["Analytics", "Developer Tools"],
  },
};

const gridCards = [
  { slug: "acme/customer-insight-engine", account: "acme", name: "Customer Insight Engine", description: "Analyzes customer feedback to surface actionable insights and trends.", categories: ["Analytics"] },
  { slug: "acme/personalized-support-responses", account: "acme", name: "Personalized Support Responses", description: "This agent helps your support team respond faster by drafting personalized replies that consider the customer's history and context.", categories: ["Support"] },
  { slug: "acme/security-monitor", account: "acme", name: "Security Monitor", description: "Continuously scans for vulnerabilities and alerts your security team.", categories: ["Security"] },
  { slug: "postman/api-test-agent", account: "postman", name: "API Test Agent", description: "Automatically generates and runs API tests based on your OpenAPI specs.", categories: ["Developer Tools"] },
  { slug: "atlas/deploy-bot", account: "atlas", name: "Deploy Bot", description: "Manages zero-downtime deployments across multiple environments with rollback support.", categories: ["DevOps"] },
  { slug: "nova/data-pipeline", account: "nova", name: "Data Pipeline", description: "Orchestrates ETL workflows and monitors data quality across your warehouse.", categories: ["Data"] },
];

export const Grid: StoryObj = {
  render: () => (
    <MemoryRouter>
      <div className="w-full bg-surface p-6">
        <div className="grid w-full grid-cols-3 gap-3">
          {gridCards.map((card) => (
            <AgentCard key={card.slug} {...card} />
          ))}
        </div>
      </div>
    </MemoryRouter>
  ),
};
