import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";

import { RecommendedBlueprints } from "@/components/RecommendedBlueprints";

const meta = {
  title: "Features/Agents/RecommendedBlueprints",
  component: RecommendedBlueprints,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div className="max-w-3xl">
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
} satisfies Meta<typeof RecommendedBlueprints>;

export default meta;
type Story = StoryObj<typeof meta>;

const sampleBlueprints = [
  {
    slug: "ticket-scanner",
    account: "astro",
    name: "Ticket Scanner",
    description:
      "Surfaces actionable feedback from support tickets, reviews, and user conversations to guide product decisions.",
  },
  {
    slug: "sentiment-analyzer",
    account: "astro",
    name: "Sentiment Analyzer",
    description:
      "Monitors customer sentiment across channels and flags critical shifts in real time.",
  },
];

export const Default: Story = {
  args: {
    blueprints: sampleBlueprints,
  },
};

export const SingleBlueprint: Story = {
  args: {
    blueprints: [sampleBlueprints[0]],
  },
};

export const Empty: Story = {
  args: {
    blueprints: [],
  },
};
