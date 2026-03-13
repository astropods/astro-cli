import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { AgentDetailSidebar } from "@/components/agent-detail/AgentDetailSidebar";
import type { Agent, AccountPublic, ResolvedIntegration } from "@/lib/api";

const mockAgent: Agent = {
  name: "customer-insight-engine",
  account: "acme",
  registry: "default",
  versions: [
    {
      build_id: "a1b2c3d4e5f6g7h8",
      version: "1.2.3",
      spec: {},
      readme: "",
      published_at: "2026-01-15T12:00:00Z",
    },
  ],
};

const mockAccount: AccountPublic = {
  id: "acc-001",
  name: "acme",
  type: "organization",
  owner: {
    first_name: "Jane",
    last_name: "Smith",
    profile_picture_url: "https://i.pravatar.cc/150?u=janesmith",
  },
  created_at: "2025-06-01T00:00:00Z",
  updated_at: "2026-01-15T00:00:00Z",
};

const recommendedAgents = [
  {
    slug: "steve_jobs/alert-router",
    account: "steve_jobs",
    name: "alert-router",
    description: "Routes alerts to the right team.",
    rating: 4.6,
    installs: 1203,
  },
  {
    slug: "steve_jobs/data-classifier",
    account: "steve_jobs",
    name: "data-classifier",
    description: "Classifies incoming events by urgency.",
    rating: 4.9,
    installs: 3571,
  },
];

const meta = {
  title: "Features/Agents/AgentDetailSidebar",
  component: AgentDetailSidebar,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div className="flex justify-end min-h-[600px]">
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
  // Override hidden lg:block so it always renders in Storybook
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof AgentDetailSidebar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    agent: mockAgent,
    description:
      "Analyzes customer feedback to surface actionable insights and trends across all channels.",
    integrations: [
      { id: "slack", name: "Slack" },
      { id: "github", name: "GitHub" },
    ],
    capabilities: [
      "Read Slack messages",
      "Send Slack messages",
      "Access GitHub repositories",
    ],
    rating: 4.8,
    installs: 2841,
    teammateInstallCount: 3,
    teammateInitials: ["AK", "MF", "BL"],
    recommendedAgents,
    initialAccountData: mockAccount,
  },
};

export const Full: Story = {
  args: {
    agent: mockAgent,
    description:
      "This agent monitors your production environment 24/7 and detects anomalies in real-time.",
    integrations: [
      { id: "slack", name: "Slack" },
      { id: "github", name: "GitHub" },
      { id: "linear", name: "Linear" },
      { id: "notion", name: "Notion" },
    ],
    capabilities: [
      "Read Slack messages",
      "Send Slack messages",
      "Access GitHub repositories",
      "Create Linear issues",
      "Read Notion pages",
      "Send email notifications",
    ],
    rating: 4.9,
    installs: 12481,
    teammateInstallCount: 7,
    teammateInitials: ["SO", "JD", "PK"],
    recommendedAgents,
    initialAccountData: mockAccount,
  },
};

export const Minimal: Story = {
  args: {
    agent: mockAgent,
    description: "",
    integrations: [],
    capabilities: [],
    rating: undefined,
    installs: undefined,
    teammateInstallCount: 0,
    initialAccountData: mockAccount,
  },
};

export const NoAvatar: Story = {
  args: {
    agent: mockAgent,
    description: "A lightweight monitoring agent.",
    integrations: [{ id: "slack", name: "Slack" }],
    capabilities: ["Read Slack messages"],
    initialAccountData: {
      ...mockAccount,
      owner: { first_name: "Acme", last_name: "Corp" },
    },
  },
};

export const BuildIdVersion: Story = {
  args: {
    agent: {
      ...mockAgent,
      versions: [
        {
          ...mockAgent.versions[0],
          version: undefined,
        },
      ],
    },
    description: "Uses a build ID instead of semver.",
    integrations: [{ id: "github", name: "GitHub" }],
    capabilities: ["Access GitHub repositories"],
    initialAccountData: mockAccount,
  },
};
