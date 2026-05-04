import type { Meta, StoryObj } from "@storybook/react-vite";
import { DeploymentTile, type DeploymentTileProps } from "@/components/agent-detail/deployments/DeploymentTile";
import type { AgentDeployment } from "@/lib/api";

function mockDeployment(overrides: Partial<AgentDeployment> = {}): AgentDeployment {
  return {
    id: "dep-1",
    name: "my-agent",
    build_id: "a1b2c3d4",
    namespace: "ns",
    status: "running",
    replicas: 1,
    ready: 1,
    created_at: new Date().toISOString(),
    components: ["agent"],
    ...overrides,
  };
}

const meta = {
  title: "Features/Agent Detail/DeploymentTile",
  component: DeploymentTile,
  decorators: [
    (Story) => (
      <div className="w-[26rem] bg-stone-950 p-4">
        <Story />
      </div>
    ),
  ],
  argTypes: {
    source: { control: "select", options: ["github", "direct"] },
    active: { control: "boolean" },
  },
} satisfies Meta<DeploymentTileProps>;

export default meta;
type Story = StoryObj<DeploymentTileProps>;

export const DirectPush: Story = {
  args: {
    name: "my-agent",
    source: "direct",
    buildId: "a1b2c3d4e5f6",
    deployedAt: new Date(Date.now() - 1000 * 60 * 30).toISOString(),
  },
};

export const DirectPushActive: Story = {
  args: {
    ...DirectPush.args,
    active: true,
    deployment: mockDeployment(),
  },
};

export const GitHubDeploy: Story = {
  args: {
    name: "Fix memory leak in worker pool",
    source: "github",
    branch: "main",
    buildId: "f7e8d9c0b1a2",
    deployedAt: new Date(Date.now() - 1000 * 60 * 60 * 2).toISOString(),
  },
};

export const GitHubDeployActive: Story = {
  args: {
    ...GitHubDeploy.args,
    active: true,
    deployment: mockDeployment(),
  },
};

export const LongCommitMessage: Story = {
  args: {
    name: "Refactor authentication middleware to support OAuth2 PKCE flow for mobile clients",
    source: "github",
    branch: "feat/oauth-pkce",
    buildId: "deadbeef1234",
    deployedAt: new Date(Date.now() - 1000 * 60 * 60 * 24 * 3).toISOString(),
  },
};

export const LongBranchName: Story = {
  args: {
    name: "Update retry logic for failed webhooks",
    source: "github",
    branch: "feat/PROJ-1234-update-retry-logic-for-failed-webhook-deliveries",
    buildId: "b3c4d5e6f7a8",
    deployedAt: new Date(Date.now() - 1000 * 60 * 60 * 6).toISOString(),
  },
};

export const LongBranchNameActive: Story = {
  args: {
    ...LongBranchName.args,
    active: true,
    deployment: mockDeployment(),
  },
};

export const VeryLongBranchName: Story = {
  args: {
    name: "Migrate database connection pooling to pgbouncer",
    source: "github",
    branch: "chore/migrate-database-connection-pooling-from-built-in-pool-to-pgbouncer-with-transaction-mode",
    buildId: "1a2b3c4d5e6f",
    deployedAt: new Date(Date.now() - 1000 * 60 * 60 * 12).toISOString(),
  },
};

export const GitHubWithCommitLink: Story = {
  args: {
    name: "Fix memory leak in worker pool",
    source: "github",
    branch: "main",
    buildId: "f7e8d9c0b1a2",
    deployedAt: new Date(Date.now() - 1000 * 60 * 60 * 2).toISOString(),
    commitSha: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0",
    repoFullName: "anthropics/my-agent",
  },
};

export const GitHubWithCommitLinkLongBranch: Story = {
  args: {
    name: "Update retry logic for failed webhooks",
    source: "github",
    branch: "feat/PROJ-1234-update-retry-logic-for-failed-webhook-deliveries",
    buildId: "b3c4d5e6f7a8",
    deployedAt: new Date(Date.now() - 1000 * 60 * 60 * 6).toISOString(),
    commitSha: "deadbeef1234567890abcdef1234567890abcdef",
    repoFullName: "anthropics/my-agent",
  },
};

export const GitHubNoCommitLink: Story = {
  args: {
    name: "Fix memory leak in worker pool",
    source: "github",
    branch: "main",
    buildId: "f7e8d9c0b1a2",
    deployedAt: new Date(Date.now() - 1000 * 60 * 60 * 2).toISOString(),
  },
};

export const ActiveDeploying: Story = {
  args: {
    name: "my-agent",
    source: "direct",
    buildId: "abcdef12",
    deployedAt: new Date(Date.now() - 1000 * 60 * 5).toISOString(),
    active: true,
    deployment: mockDeployment({ status: "pending", ready: 0 }),
  },
};

export const ActiveError: Story = {
  args: {
    name: "my-agent",
    source: "direct",
    buildId: "abcdef12",
    deployedAt: new Date(Date.now() - 1000 * 60 * 5).toISOString(),
    active: true,
    deployment: mockDeployment({ status: "error", ready: 0 }),
  },
};

export const AllVariants: Story = {
  args: { name: "", source: "direct", buildId: "", deployedAt: "" },
  decorators: [
    () => (
      <div className="flex w-[26rem] flex-col gap-2 bg-stone-950 p-4">
        <DeploymentTile
          name="Add streaming support for tool calls"
          source="github"
          branch="main"
          buildId="f7e8d9c0b1a2"
          deployedAt={new Date(Date.now() - 1000 * 60 * 10).toISOString()}
          active
          deployment={mockDeployment()}
        />
        <DeploymentTile
          name="Currently deploying"
          source="github"
          branch="feat/new-feature"
          buildId="b1c2d3e4f5a6"
          deployedAt={new Date(Date.now() - 1000 * 60 * 1).toISOString()}
          active
          deployment={mockDeployment({ status: "deploying", ready: 0 })}
        />
        <DeploymentTile
          name="Bump model to claude-sonnet-4-6"
          source="github"
          branch="main"
          buildId="a1b2c3d4e5f6"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 4).toISOString()}
        />
        <DeploymentTile
          name="my-agent"
          source="direct"
          buildId="deadbeef1234"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 24).toISOString()}
        />
        <DeploymentTile
          name="Fix rate limiting on /v1/chat endpoint"
          source="github"
          branch="fix/rate-limit"
          buildId="cafe0123abcd"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 24 * 7).toISOString()}
        />
      </div>
    ),
  ],
};
