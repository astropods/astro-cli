import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { ConnectedRepoView, type ConnectedRepoViewProps } from "@/components/blueprint-detail/GitHubConnectionPanel";
import { SidebarSection } from "@/components/blueprint-detail/SidebarSection";
import { Button } from "@/components/ui/button";
import { Github, CheckCircle2 } from "lucide-react";
import { Spinner } from "@/components/ui/spinner";
import type { GitHubBuild } from "@/lib/api";

// ── Helpers ──────────────────────────────────────────────────────────────────

const GITHUB_LOGIN = "acme";

const GHTrailing = () => (
  <span className="flex items-center gap-1 font-mono text-[10px] text-foreground">
    <CheckCircle2 className="size-3 shrink-0 text-green-600 dark:text-green-400" />
    {GITHUB_LOGIN}
  </span>
);

function ConnectedSection({ children }: { children: React.ReactNode }) {
  return (
    <SidebarSection title="GitHub" trailing={<GHTrailing />}>
      {children}
    </SidebarSection>
  );
}

// ── Shared fixtures ──────────────────────────────────────────────────────────

const noop = { mutate: () => {}, isPending: false };
const pending = { mutate: () => {}, isPending: true };

const base: Omit<ConnectedRepoViewProps, "status" | "statusLoading"> = {
  account: "acme",
  name: "credit-card-agent",
  rebuild: noop,
  disconnect: noop,
};

const connectedStatus = {
  repo_full_name: "acme/credit-card-agent",
  branch: "main",
};

function makeBuild(overrides: Partial<GitHubBuild>): GitHubBuild {
  return {
    id: "build-1",
    build_id: "bld_abc123",
    commit_sha: "a1b2c3d4e5f6",
    branch: "main",
    commit_message: "feat: add transaction categorization",
    commit_author: "sohumdalal",
    enqueued_at: new Date(Date.now() - 90_000).toISOString(),
    status: "pending",
    ...overrides,
  };
}

// ── Decorator ────────────────────────────────────────────────────────────────

const meta = {
  title: "Features/GitHub/ConnectionPanel",
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div className="w-72 border-l border-border bg-background p-4">
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
  parameters: { layout: "centered" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

// ── 1. Not connected ─────────────────────────────────────────────────────────

export const NotConnected: Story = {
  render: () => (
    <SidebarSection title="GitHub">
      <div className="space-y-2">
        <p className="text-xs text-muted-foreground">
          Connect a GitHub repo to auto-build on every push to main.
        </p>
        <Button size="sm" variant="outline" className="w-full gap-2">
          <Github className="h-3.5 w-3.5" />
          Connect GitHub repo
        </Button>
      </div>
    </SidebarSection>
  ),
};

// ── 2. Connecting (OAuth pending) ────────────────────────────────────────────

export const Connecting: Story = {
  render: () => (
    <SidebarSection title="GitHub">
      <div className="space-y-2">
        <p className="text-xs text-muted-foreground">
          Connect a GitHub repo to auto-build on every push to main.
        </p>
        <Button size="sm" variant="outline" className="w-full gap-2" disabled>
          <Spinner size={14} />
          Connect GitHub repo
        </Button>
      </div>
    </SidebarSection>
  ),
};

// ── 3. Connected — waiting for astropods.yml ─────────────────────────────────

export const WaitingForSpec: Story = {
  render: () => (
    <ConnectedSection>
      <ConnectedRepoView
        {...base}
        statusLoading={false}
        status={{ ...connectedStatus, builds: [] }}
      />
    </ConnectedSection>
  ),
};

// ── 4. Connected — build pending ─────────────────────────────────────────────

export const BuildPending: Story = {
  render: () => (
    <ConnectedSection>
      <ConnectedRepoView
        {...base}
        statusLoading={false}
        status={{
          ...connectedStatus,
          builds: [makeBuild({ status: "pending" })],
        }}
      />
    </ConnectedSection>
  ),
};

// ── 5. Connected — building (step 1: fetching spec) ──────────────────────────

export const BuildingFetchingSpec: Story = {
  render: () => (
    <ConnectedSection>
      <ConnectedRepoView
        {...base}
        statusLoading={false}
        status={{
          ...connectedStatus,
          builds: [makeBuild({ status: "building", step: "fetching-spec" })],
        }}
      />
    </ConnectedSection>
  ),
};

// ── 6. Connected — building (step 2: building image) ─────────────────────────

export const BuildingImage: Story = {
  render: () => (
    <ConnectedSection>
      <ConnectedRepoView
        {...base}
        statusLoading={false}
        status={{
          ...connectedStatus,
          builds: [makeBuild({ status: "building", step: "building (1/2: agent)" })],
        }}
      />
    </ConnectedSection>
  ),
};

// ── 7. Connected — building (step 3: registering) ────────────────────────────

export const BuildingRegistering: Story = {
  render: () => (
    <ConnectedSection>
      <ConnectedRepoView
        {...base}
        statusLoading={false}
        status={{
          ...connectedStatus,
          builds: [makeBuild({ status: "building", step: "registering" })],
        }}
      />
    </ConnectedSection>
  ),
};

// ── 8. Connected — build succeeded ───────────────────────────────────────────

export const BuildSucceeded: Story = {
  render: () => (
    <ConnectedSection>
      <ConnectedRepoView
        {...base}
        statusLoading={false}
        status={{
          ...connectedStatus,
          builds: [
            makeBuild({
              status: "registered",
              completed_at: new Date(Date.now() - 30_000).toISOString(),
            }),
          ],
        }}
      />
    </ConnectedSection>
  ),
};

// ── 9. Connected — build failed ───────────────────────────────────────────────

export const BuildFailed: Story = {
  render: () => (
    <ConnectedSection>
      <ConnectedRepoView
        {...base}
        statusLoading={false}
        status={{
          ...connectedStatus,
          builds: [
            makeBuild({
              status: "failed",
              error: "failed to solve: failed to read dockerfile: open Dockerfile: no such file or directory",
              completed_at: new Date(Date.now() - 45_000).toISOString(),
            }),
          ],
        }}
      />
    </ConnectedSection>
  ),
};

// ── 10. Connected — previous success + new build in progress ─────────────────

export const BuildInProgressWithHistory: Story = {
  render: () => (
    <ConnectedSection>
      <ConnectedRepoView
        {...base}
        statusLoading={false}
        status={{
          ...connectedStatus,
          builds: [
            makeBuild({ status: "building", step: "building (1/2: agent)" }),
            makeBuild({
              id: "build-0",
              build_id: "bld_prev99",
              commit_sha: "f6e5d4c3b2a1",
              commit_message: "chore: update dependencies",
              status: "registered",
              completed_at: new Date(Date.now() - 3600_000).toISOString(),
            }),
          ],
        }}
      />
    </ConnectedSection>
  ),
};

// ── 11. Connected — disconnecting ─────────────────────────────────────────────

export const Disconnecting: Story = {
  render: () => (
    <ConnectedSection>
      <ConnectedRepoView
        {...base}
        disconnect={pending}
        statusLoading={false}
        status={{ ...connectedStatus, builds: [] }}
      />
    </ConnectedSection>
  ),
};
