import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { AgentDetailSidebar } from "@/components/agent-detail/AgentDetailSidebar";
import type { Agent, AccountPublic } from "@/lib/api";

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
    installs: 1203,
  },
  {
    slug: "steve_jobs/data-classifier",
    account: "steve_jobs",
    name: "data-classifier",
    description: "Classifies incoming events by urgency.",
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
    integrations: [
      { id: "slack", name: "Slack", known: true },
      { id: "github", name: "GitHub", known: true },
    ],
    capabilities: [
      "Read Slack messages",
      "Send Slack messages",
      "Access GitHub repositories",
    ],
    rating: 4.8,
    installs: 2841,
    recommendedAgents,
    initialAccountData: mockAccount,
  },
};

export const Full: Story = {
  args: {
    agent: mockAgent,
    integrations: [
      { id: "slack", name: "Slack", known: true },
      { id: "github", name: "GitHub", known: true },
      { id: "linear", name: "Linear", known: true },
      { id: "notion", name: "Notion", known: true },
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
    recommendedAgents,
    initialAccountData: mockAccount,
  },
};

export const DenseDetails: Story = {
  args: {
    agent: mockAgent,
    integrations: [
      { id: "slack", name: "Slack", known: true },
      { id: "github", name: "GitHub", known: true },
      { id: "linear", name: "Linear", known: true },
      { id: "notion", name: "Notion", known: true },
      { id: "google-drive", name: "Google Drive", known: true },
    ],
    capabilities: [
      "Read Slack messages",
      "Send Slack messages",
      "Access GitHub repositories",
      "Create Linear issues",
    ],
    rating: 4.8,
    installs: 2841,
    recommendedAgents,
    initialAccountData: mockAccount,
  },
};

export const Minimal: Story = {
  args: {
    agent: mockAgent,
    integrations: [],
    capabilities: [],
    rating: undefined,
    installs: undefined,
    initialAccountData: mockAccount,
  },
};

export const NoAvatar: Story = {
  args: {
    agent: mockAgent,
    integrations: [{ id: "slack", name: "Slack", known: true }],
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
    integrations: [{ id: "github", name: "GitHub", known: true }],
    capabilities: ["Access GitHub repositories"],
    initialAccountData: mockAccount,
  },
};
