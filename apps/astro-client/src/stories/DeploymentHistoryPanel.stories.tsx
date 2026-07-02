import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  DeploymentHistoryPanelContent,
  UpgradeNudge,
  BuildInProgressNudge,
  type DeploymentHistoryPanelContentProps,
} from "@/components/agent-detail/deployments/DeploymentHistoryPanel";
import { DeploymentTile } from "@/components/agent-detail/deployments/DeploymentTile";
import { StarField } from "@/components/agent-detail/starfield/StarField";
import type { AgentDeployment, GitHubBuild } from "@/lib/api";

function mockBuild(overrides: Partial<GitHubBuild> = {}): GitHubBuild {
  return {
    id: "build-1",
    build_id: "b4c5d6e7f8a9",
    commit_sha: "b4c5d6e7f8a9012345",
    branch: "main",
    status: "building",
    commit_message: "Add retries with exponential backoff",
    enqueued_at: new Date(Date.now() - 1000 * 30).toISOString(),
    ...overrides,
  };
}

function mockDeployment(overrides: Partial<AgentDeployment> = {}): AgentDeployment {
  return {
    id: "dep-1",
    name: "my-agent",
    build_id: "a1b2c3d4",
    namespace: "ns",
    status: "running",
    replicas: 1,
    created_at: new Date().toISOString(),
    components: ["agent"],
    ...overrides,
  };
}

const meta = {
  title: "Features/Agent Detail/DeploymentHistoryPanel",
  component: DeploymentHistoryPanelContent,
  decorators: [
    (Story) => (
      <div className="relative flex h-[600px] items-stretch justify-end bg-surface p-3">
        <StarField />
        <div className="relative z-10 w-[26rem]">
          <Story />
        </div>
      </div>
    ),
  ],
} satisfies Meta<DeploymentHistoryPanelContentProps>;

export default meta;
type Story = StoryObj<DeploymentHistoryPanelContentProps>;

export const Empty: Story = {
  args: {},
};

export const SingleActive: Story = {
  args: {
    children: (
      <DeploymentTile
        name="Add streaming support for tool calls"
        source="github"
        branch="main"
        buildId="f7e8d9c0b1a2"
        deployedAt={new Date(Date.now() - 1000 * 60 * 10).toISOString()}
        active
        deployment={mockDeployment()}
      />
    ),
  },
};

export const WithHistory: Story = {
  args: {
    children: (
      <>
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
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 24 * 3).toISOString()}
        />
        <DeploymentTile
          name="Initial deployment"
          source="direct"
          buildId="0000abcd1234"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 24 * 14).toISOString()}
        />
      </>
    ),
  },
};

export const UpgradeAvailable: Story = {
  args: {
    children: (
      <>
        <UpgradeNudge
          currentBuildId="f7e8d9c0b1a2"
          latestBuildId="b4c5d6e7f8a9"
          commitMessage="Add retries with exponential backoff"
          commitSha="b4c5d6e7f8a9012345"
          repoFullName="acme/my-agent"
        />
        <DeploymentTile
          name="Bump model to claude-sonnet-4-6"
          source="github"
          branch="main"
          buildId="f7e8d9c0b1a2"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 4).toISOString()}
          active
          deployment={mockDeployment()}
        />
      </>
    ),
  },
};

export const BuildInProgress: Story = {
  args: {
    children: (
      <>
        <BuildInProgressNudge
          build={mockBuild()}
          repoFullName="acme/my-agent"
        />
        <DeploymentTile
          name="Bump model to claude-sonnet-4-6"
          source="github"
          branch="main"
          buildId="f7e8d9c0b1a2"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 4).toISOString()}
          active
          deployment={mockDeployment()}
        />
      </>
    ),
  },
};

export const BuildInProgressLongTitle: Story = {
  args: {
    children: (
      <>
        <BuildInProgressNudge
          build={mockBuild({
            commit_message:
              "Add retries with exponential backoff and jitter to all outbound provider calls",
          })}
          repoFullName="acme/my-agent"
        />
        <DeploymentTile
          name="Bump model to claude-sonnet-4-6"
          source="github"
          branch="main"
          buildId="f7e8d9c0b1a2"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 4).toISOString()}
          active
          deployment={mockDeployment()}
        />
      </>
    ),
  },
};

export const BuildPreparing: Story = {
  args: {
    children: (
      <>
        <BuildInProgressNudge
          build={mockBuild({ status: "pending" })}
          repoFullName="acme/my-agent"
        />
        <DeploymentTile
          name="Bump model to claude-sonnet-4-6"
          source="github"
          branch="main"
          buildId="f7e8d9c0b1a2"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 4).toISOString()}
          active
          deployment={mockDeployment()}
        />
      </>
    ),
  },
};

export const UpgradeAvailableDirectPush: Story = {
  args: {
    children: (
      <>
        <UpgradeNudge currentBuildId="abcdef12" latestBuildId="98765432" />
        <DeploymentTile
          name="my-agent"
          source="direct"
          buildId="abcdef12"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60).toISOString()}
          active
          deployment={mockDeployment()}
        />
      </>
    ),
  },
};

export const DirectPushesOnly: Story = {
  args: {
    children: (
      <>
        <DeploymentTile
          name="my-agent"
          source="direct"
          buildId="abcdef12"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60).toISOString()}
          active
          deployment={mockDeployment()}
        />
        <DeploymentTile
          name="my-agent"
          source="direct"
          buildId="12345678"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 48).toISOString()}
        />
        <DeploymentTile
          name="my-agent"
          source="direct"
          buildId="98765432"
          deployedAt={new Date(Date.now() - 1000 * 60 * 60 * 24 * 7).toISOString()}
        />
      </>
    ),
  },
};
