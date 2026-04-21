import type { Meta, StoryObj } from "@storybook/react-vite";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { BuildLogViewer, type BuildLogComponentData } from "@/components/blueprint-detail/BuildLogViewer";

const meta = {
  title: "Features/GitHub/BuildLogViewer",
  component: BuildLogViewer,
  parameters: { layout: "centered" },
  decorators: [
    (Story) => (
      <div className="w-[720px] border border-border rounded-lg overflow-hidden bg-background">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof BuildLogViewer>;

export default meta;
type Story = StoryObj<typeof meta>;

// ── Fixtures ──────────────────────────────────────────────────────────────────

const SECTIONS = `=== events ===
Pulling image "ubuntu:22.04"
Successfully pulled image "ubuntu:22.04"
=== git-clone ===
Cloning into '/workspace'...
remote: Enumerating objects: 142, done.
remote: Counting objects: 100% (142/142), done.
Resolving deltas: 100% (89/89), done.
=== ecr-login ===
Login Succeeded
=== buildkit ===
[1/4] FROM docker.io/library/python:3.11-slim
[2/4] COPY requirements.txt .
[3/4] RUN pip install -r requirements.txt
[4/4] COPY . .
Successfully built a1b2c3d4e5f6
Successfully tagged 123456789.dkr.ecr.us-east-1.amazonaws.com/astro/credit-card-agent:bld_abc123`;

const DURATIONS: Record<string, string> = {
  "agent": "1m 12s",
  "ingestion-startup": "48s",
  "ingestion-schedule": "2m 3s",
};

function makeComp(name: string, status: BuildLogComponentData["status"], logs = SECTIONS): BuildLogComponentData {
  return { name, status, logs, duration: DURATIONS[name] };
}

// ── Edge case stories ─────────────────────────────────────────────────────────

export const Loading: Story = {
  args: { components: [], isLoading: true },
};

export const Error: Story = {
  args: { components: [], isError: true },
};

export const SingleComponentSucceeded: Story = {
  args: {
    commitSha: "a1b2c3d",
    buildId: "bld_abc123",
    totalDuration: "1m 12s",
    components: [makeComp("agent", "succeeded")],
  },
};

export const SingleComponentPendingBuild: Story = {
  args: {
    commitSha: "a1b2c3d",
    // no buildId — build number not yet assigned
    components: [makeComp("agent", "building")],
  },
};

// ── In-dialog stories (real context) ─────────────────────────────────────────

function DialogStory({ components, isLoading, isError }: {
  components: BuildLogComponentData[];
  isLoading?: boolean;
  isError?: boolean;
}) {
  return (
    <Dialog open>
      <DialogContent className="sm:max-w-3xl gap-0 p-0">
        <div className="overflow-y-auto max-h-[75vh] rounded-lg">
          <BuildLogViewer
            commitSha="a1b2c3d"
            buildId="bld_abc123"
            totalDuration="3m 23s"
            components={components}
            isLoading={isLoading}
            isError={isError}
          />
        </div>
      </DialogContent>
    </Dialog>
  );
}

export const DialogThreeComponentsMixed: Story = {
  parameters: { layout: "fullscreen" },
  render: () => (
    <DialogStory
      components={[
        makeComp("agent", "succeeded"),
        makeComp("ingestion-startup", "succeeded"),
        makeComp("ingestion-schedule", "building", `=== events ===
Pulling image "ubuntu:22.04"
Successfully pulled image "ubuntu:22.04"
=== git-clone ===
Cloning into '/workspace'...
remote: Counting objects: 100% (142/142), done.
=== ecr-login ===
Login Succeeded
=== buildkit ===
[1/4] FROM docker.io/library/python:3.11-slim
[2/4] COPY requirements.txt .`),
      ]}
    />
  ),
};

export const DialogBuildFailed: Story = {
  parameters: { layout: "fullscreen" },
  render: () => (
    <DialogStory
      components={[
        makeComp("agent", "succeeded"),
        makeComp("ingestion-startup", "failed", `=== events ===
Pulling image "ubuntu:22.04"
=== git-clone ===
Cloning into '/workspace'...
=== ecr-login ===
Login Succeeded
=== buildkit ===
[1/4] FROM docker.io/library/python:3.11-slim
[2/4] COPY requirements.txt .
ERROR: failed to solve: failed to read dockerfile: open Dockerfile: no such file or directory`),
      ]}
    />
  ),
};

export const DialogLoading: Story = {
  parameters: { layout: "fullscreen" },
  render: () => <DialogStory components={[]} isLoading />,
};
