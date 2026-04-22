import type { Meta, StoryObj } from "@storybook/react-vite";
import { BlueprintCard } from "@/components/BlueprintCard";

const meta = {
  title: "Features/Agents/BlueprintCard",
  component: BlueprintCard,
} satisfies Meta<typeof BlueprintCard>;

export default meta;
type Story = StoryObj<typeof meta>;

const singleCardDecorator = (Story: React.ComponentType) => (
  <div className="max-w-sm">
    <Story />
  </div>
);

export const Default: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "acme/customer-insight-engine",
    account: "acme",
    name: "Customer Insight Engine",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
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
  },
};

export const ShortDescription: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "acme/security-monitor",
    account: "acme",
    name: "Security Monitor",
    description:
      "Continuously scans for vulnerabilities and alerts your security team.",
  },
};

export const DifferentAccount: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "postman/product-research-intel",
    account: "postman",
    name: "Product Research Intel",
    description:
      "Aggregates product research and competitive intelligence from multiple sources.",
  },
};

export const IsMember: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "acme/customer-insight-engine",
    account: "acme",
    name: "Customer Insight Engine",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
    visibility: "private",
    onArchive: () => {},
  },
};

export const OftenUsedTogether: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "steve_jobs/alert-router",
    account: "steve_jobs",
    name: "alert-router",
    description: "Routes alerts to the correct responders.",
    variant: "oftenUsedTogether",
    deployCount: 1203,
  },
};

const gridCards = [
  { slug: "acme/customer-insight-engine", account: "acme", name: "Customer Insight Engine", description: "Analyzes customer feedback to surface actionable insights and trends.", avatarUrl: "/assets/avatars/agents/chrisjpatty/chrisbot.jpg" },
  { slug: "acme/personalized-support-responses", account: "acme", name: "Personalized Support Responses", description: "This agent helps your support team respond faster by drafting personalized replies that consider the customer's history and context.", avatarUrl: "/assets/avatars/agents/chrisjpatty/slack-bot.jpg" },
  { slug: "acme/security-monitor", account: "acme", name: "Security Monitor", description: "Continuously scans for vulnerabilities and alerts your security team.", avatarUrl: "/assets/avatars/agents/chrisjpatty/deploy-test-agent.jpg" },
  { slug: "postman/api-test-agent", account: "postman", name: "API Test Agent", description: "Automatically generates and runs API tests based on your OpenAPI specs.", avatarUrl: "/assets/avatars/agents/chrisjpatty/directives-test.jpg" },
  { slug: "atlas/deploy-bot", account: "atlas", name: "Deploy Bot", description: "Manages zero-downtime deployments across multiple environments with rollback support.", avatarUrl: "/assets/avatars/agents/chrispattypm/directives-test.jpg" },
];

export const Grid: StoryObj = {
  render: () => (
    <div className="w-full bg-surface p-6">
      <div className="grid w-full grid-cols-3 gap-3">
        {gridCards.map((card) => (
          <BlueprintCard key={card.slug} {...card} />
        ))}
      </div>
    </div>
  ),
};
