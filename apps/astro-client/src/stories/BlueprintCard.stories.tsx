import type { Meta, StoryObj } from "@storybook/react-vite";
import { BlueprintCard } from "@/components/BlueprintCard";
import type { AvatarColors } from "@/lib/api";

// Extracted from actual avatar images via colorextract.ExtractFromJPEG.
const chrisbotColors: AvatarColors = {
  base: "#271414", vibrant: "#862d2f", vibrant_light: "#e08587",
  accent: "#300a0b", accent_light: "#dfc3a0",
  background: "#220b0c", foreground: "#f6f4f4", glow: "#edabac",
};
const slackBotColors: AvatarColors = {
  base: "#3f223e", vibrant: "#862d83", vibrant_light: "#df86dc",
  accent: "#4d144b", accent_light: "#dbb9a4",
  background: "#220b22", foreground: "#f6f4f6", glow: "#eaaee8",
};
const deployTestColors: AvatarColors = {
  base: "#af4f59", vibrant: "#862d36", vibrant_light: "#e0858f",
  accent: "#df1f34", accent_light: "#dea0b2",
  background: "#220b0e", foreground: "#f6f4f4", glow: "#f3a5ae",
};
const directivesTestColors: AvatarColors = {
  base: "#416eb3", vibrant: "#2d5086", vibrant_light: "#85a9e0",
  accent: "#0862ec", accent_light: "#e699c1",
  background: "#0b1522", foreground: "#f4f5f6", glow: "#9ec2fa",
};
const directivesTestPmColors: AvatarColors = {
  base: "#775a29", vibrant: "#86652d", vibrant_light: "#e0bf85",
  accent: "#9f6501", accent_light: "#e6a899",
  background: "#221a0b", foreground: "#f6f5f4", glow: "#fad89e",
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

// ---------------------------------------------------------------------------
// Default variant
// ---------------------------------------------------------------------------

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

export const WithAvatar: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "acme/customer-insight-engine",
    account: "acme",
    name: "Customer Insight Engine",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
    avatarUrl: "/assets/avatars/agents/chrisjpatty/chrisbot.jpg",
    avatarColors: chrisbotColors,
    deployCount: 842,
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
    avatarUrl: "/assets/avatars/agents/chrisjpatty/slack-bot.jpg",
    avatarColors: slackBotColors,
  },
};

export const Private: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "acme/security-monitor",
    account: "acme",
    name: "Security Monitor",
    description:
      "Continuously scans for vulnerabilities and alerts your security team.",
    visibility: "private",
    avatarUrl: "/assets/avatars/agents/chrisjpatty/deploy-test-agent.jpg",
    avatarColors: deployTestColors,
  },
};

export const PrivateWithArchive: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "acme/customer-insight-engine",
    account: "acme",
    name: "Customer Insight Engine",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
    visibility: "private",
    avatarUrl: "/assets/avatars/agents/chrisjpatty/chrisbot.jpg",
    avatarColors: chrisbotColors,
    onArchive: () => {},
  },
};

export const WithDeployCount: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "postman/api-test-agent",
    account: "postman",
    name: "API Test Agent",
    description:
      "Automatically generates and runs API tests based on your OpenAPI specs.",
    avatarUrl: "/assets/avatars/agents/chrisjpatty/directives-test.jpg",
    avatarColors: directivesTestColors,
    deployCount: 12450,
  },
};

export const SingleDeploy: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "atlas/deploy-bot",
    account: "atlas",
    name: "Deploy Bot",
    description: "Manages zero-downtime deployments across multiple environments.",
    avatarUrl: "/assets/avatars/agents/chrispattypm/directives-test.jpg",
    avatarColors: directivesTestPmColors,
    deployCount: 1,
  },
};

export const Draft: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "acme/new-agent",
    account: "acme",
    name: "New Agent",
    description: "An agent that is still being set up.",
    isDraft: true,
  },
};

export const DraftWithArchive: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "acme/abandoned-experiment",
    account: "acme",
    name: "Abandoned Experiment",
    description: "A draft that the owner might want to clean up.",
    isDraft: true,
    onArchive: () => {},
  },
};

// ---------------------------------------------------------------------------
// Often-used-together variant (compact)
// ---------------------------------------------------------------------------

export const OftenUsedTogether: Story = {
  decorators: [singleCardDecorator],
  args: {
    slug: "acme/customer-insight-engine",
    account: "acme",
    name: "Customer Insight Engine",
    description: "Analyzes customer feedback to surface actionable insights and trends.",
    avatarUrl: "/assets/avatars/agents/chrisjpatty/chrisbot.jpg",
    avatarColors: chrisbotColors,
    variant: "oftenUsedTogether",
    deployCount: 1203,
  },
};

// ---------------------------------------------------------------------------
// List variant
// ---------------------------------------------------------------------------

const listDecorator = (Story: React.ComponentType) => (
  <div className="max-w-2xl">
    <Story />
  </div>
);

export const List: Story = {
  decorators: [listDecorator],
  args: {
    slug: "acme/customer-insight-engine",
    account: "acme",
    name: "Customer Insight Engine",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
    avatarUrl: "/assets/avatars/agents/chrisjpatty/chrisbot.jpg",
    avatarColors: chrisbotColors,
    variant: "list",
    deployCount: 842,
    heartCount: 56,
  },
};

export const ListPrivate: Story = {
  decorators: [listDecorator],
  args: {
    slug: "acme/security-monitor",
    account: "acme",
    name: "Security Monitor",
    description:
      "Continuously scans for vulnerabilities and alerts your security team.",
    avatarUrl: "/assets/avatars/agents/chrisjpatty/deploy-test-agent.jpg",
    avatarColors: deployTestColors,
    variant: "list",
    visibility: "private",
    deployCount: 310,
    heartCount: 12,
  },
};

export const ListWithArchive: Story = {
  decorators: [listDecorator],
  args: {
    slug: "acme/customer-insight-engine",
    account: "acme",
    name: "Customer Insight Engine",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
    avatarUrl: "/assets/avatars/agents/chrisjpatty/chrisbot.jpg",
    avatarColors: chrisbotColors,
    variant: "list",
    deployCount: 842,
    heartCount: 56,
    onArchive: () => {},
  },
};

export const ListDraft: Story = {
  decorators: [listDecorator],
  args: {
    slug: "acme/new-agent",
    account: "acme",
    name: "New Agent",
    description: "An agent that is still being configured.",
    variant: "list",
    isDraft: true,
  },
};

export const ListDraftWithArchive: Story = {
  decorators: [listDecorator],
  args: {
    slug: "acme/abandoned-experiment",
    account: "acme",
    name: "Abandoned Experiment",
    description: "A draft that the owner might want to clean up.",
    variant: "list",
    isDraft: true,
    onArchive: () => {},
  },
};

// ---------------------------------------------------------------------------
// Grid composition
// ---------------------------------------------------------------------------

const gridCards = [
  { slug: "acme/customer-insight-engine", account: "acme", name: "Customer Insight Engine", description: "Analyzes customer feedback to surface actionable insights and trends.", avatarUrl: "/assets/avatars/agents/chrisjpatty/chrisbot.jpg", avatarColors: chrisbotColors },
  { slug: "acme/personalized-support-responses", account: "acme", name: "Personalized Support Responses", description: "This agent helps your support team respond faster by drafting personalized replies that consider the customer's history and context.", avatarUrl: "/assets/avatars/agents/chrisjpatty/slack-bot.jpg", avatarColors: slackBotColors },
  { slug: "acme/security-monitor", account: "acme", name: "Security Monitor", description: "Continuously scans for vulnerabilities and alerts your security team.", avatarUrl: "/assets/avatars/agents/chrisjpatty/deploy-test-agent.jpg", avatarColors: deployTestColors },
  { slug: "postman/api-test-agent", account: "postman", name: "API Test Agent", description: "Automatically generates and runs API tests based on your OpenAPI specs.", avatarUrl: "/assets/avatars/agents/chrisjpatty/directives-test.jpg", avatarColors: directivesTestColors },
  { slug: "atlas/deploy-bot", account: "atlas", name: "Deploy Bot", description: "Manages zero-downtime deployments across multiple environments with rollback support.", avatarUrl: "/assets/avatars/agents/chrispattypm/directives-test.jpg", avatarColors: directivesTestPmColors },
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

export const GridMixed: StoryObj = {
  render: () => (
    <div className="w-full bg-surface p-6">
      <div className="grid w-full grid-cols-3 gap-3">
        <BlueprintCard slug="acme/customer-insight-engine" account="acme" name="Customer Insight Engine" description="Analyzes customer feedback to surface actionable insights and trends." avatarUrl="/assets/avatars/agents/chrisjpatty/chrisbot.jpg" avatarColors={chrisbotColors} deployCount={842} />
        <BlueprintCard slug="acme/new-agent" account="acme" name="New Agent" description="An agent that is still being set up." isDraft />
        <BlueprintCard slug="acme/security-monitor" account="acme" name="Security Monitor" description="Continuously scans for vulnerabilities and alerts your security team." avatarUrl="/assets/avatars/agents/chrisjpatty/deploy-test-agent.jpg" avatarColors={deployTestColors} visibility="private" onArchive={() => {}} />
        <BlueprintCard slug="postman/api-test-agent" account="postman" name="API Test Agent" description="Automatically generates and runs API tests based on your OpenAPI specs." avatarUrl="/assets/avatars/agents/chrisjpatty/directives-test.jpg" avatarColors={directivesTestColors} deployCount={12450} />
        <BlueprintCard slug="acme/abandoned-experiment" account="acme" name="Abandoned Experiment" description="A draft the owner might want to clean up." isDraft onArchive={() => {}} />
        <BlueprintCard slug="atlas/deploy-bot" account="atlas" name="Deploy Bot" description="Manages zero-downtime deployments." avatarUrl="/assets/avatars/agents/chrispattypm/directives-test.jpg" avatarColors={directivesTestPmColors} deployCount={1} />
      </div>
    </div>
  ),
};
