import type { Meta, StoryObj } from "@storybook/react-vite";
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

function makeComp(name: string, status: BuildLogComponentData["status"], logs = SECTIONS): BuildLogComponentData {
  return { name, status, logs };
}

// ── Stories ───────────────────────────────────────────────────────────────────

export const Loading: Story = {
  args: { components: [], isLoading: true },
};

export const Error: Story = {
  args: { components: [], isError: true },
};

export const NoOutput: Story = {
  args: { components: [] },
};

export const SingleComponentSucceeded: Story = {
  args: {
    components: [makeComp("agent", "succeeded")],
  },
};

export const SingleComponentBuilding: Story = {
  args: {
    components: [makeComp("agent", "building", `=== events ===
Pulling image "ubuntu:22.04"
Successfully pulled image "ubuntu:22.04"
=== git-clone ===
Cloning into '/workspace'...
remote: Counting objects: 100% (142/142), done.
=== ecr-login ===
Login Succeeded
=== buildkit ===
[1/4] FROM docker.io/library/python:3.11-slim
[2/4] COPY requirements.txt .`)],
  },
};

export const ThreeComponentsPending: Story = {
  args: {
    components: [
      makeComp("agent", "pending", ""),
      makeComp("ingestion-startup", "pending", ""),
      makeComp("ingestion-schedule", "pending", ""),
    ],
  },
};

export const ThreeComponentsMixed: Story = {
  args: {
    components: [
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
    ],
  },
};

export const BuildFailed: Story = {
  args: {
    components: [
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
    ],
  },
};
