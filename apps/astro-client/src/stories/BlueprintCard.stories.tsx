import type { Meta, StoryObj } from "@storybook/react-vite";
import { BlueprintCard } from "@/components/BlueprintCard";
import type { AvatarColors } from "@/lib/api";

// Test color palettes for different avatar hues.
const warmColors: AvatarColors = {
  base: "#5a3d2a", vibrant: "#8b4513", vibrant_light: "#d2a679",
  accent: "#a0522d", accent_light: "#deb887", background: "#1a0e06",
  foreground: "#f5f0eb", glow: "#e8c9a0",
};
const coolColors: AvatarColors = {
  base: "#2a3d5a", vibrant: "#1e5a8b", vibrant_light: "#79a6d2",
  accent: "#2d6fa0", accent_light: "#87b8de", background: "#060e1a",
  foreground: "#ebf0f5", glow: "#a0c9e8",
};
const redColors: AvatarColors = {
  base: "#5a2a2a", vibrant: "#8b1313", vibrant_light: "#d27979",
  accent: "#a02d2d", accent_light: "#de8787", background: "#1a0606",
  foreground: "#f5ebeb", glow: "#e8a0a0",
};
const purpleColors: AvatarColors = {
  base: "#3d2a5a", vibrant: "#5a1e8b", vibrant_light: "#a679d2",
  accent: "#6f2da0", accent_light: "#b887de", background: "#0e061a",
  foreground: "#f0ebf5", glow: "#c9a0e8",
};
const tealColors: AvatarColors = {
  base: "#2a5a4d", vibrant: "#1e8b6f", vibrant_light: "#79d2b8",
  accent: "#2da07f", accent_light: "#87deb8", background: "#061a14",
  foreground: "#ebf5f0", glow: "#a0e8d2",
};

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
  { slug: "acme/customer-insight-engine", account: "acme", name: "Customer Insight Engine", description: "Analyzes customer feedback to surface actionable insights and trends.", avatarUrl: "/assets/avatars/agents/chrisjpatty/chrisbot.jpg", avatarColors: warmColors },
  { slug: "acme/personalized-support-responses", account: "acme", name: "Personalized Support Responses", description: "This agent helps your support team respond faster by drafting personalized replies that consider the customer's history and context.", avatarUrl: "/assets/avatars/agents/chrisjpatty/slack-bot.jpg", avatarColors: purpleColors },
  { slug: "acme/security-monitor", account: "acme", name: "Security Monitor", description: "Continuously scans for vulnerabilities and alerts your security team.", avatarUrl: "/assets/avatars/agents/chrisjpatty/deploy-test-agent.jpg", avatarColors: redColors },
  { slug: "postman/api-test-agent", account: "postman", name: "API Test Agent", description: "Automatically generates and runs API tests based on your OpenAPI specs.", avatarUrl: "/assets/avatars/agents/chrisjpatty/directives-test.jpg", avatarColors: coolColors },
  { slug: "atlas/deploy-bot", account: "atlas", name: "Deploy Bot", description: "Manages zero-downtime deployments across multiple environments with rollback support.", avatarUrl: "/assets/avatars/agents/chrispattypm/directives-test.jpg", avatarColors: tealColors },
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
