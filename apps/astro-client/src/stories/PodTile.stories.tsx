import type { Meta, StoryObj } from "@storybook/react-vite";
import { Bot, Database, Activity, Brain } from "lucide-react";
import { ChatBubbleLeftRightIcon } from "@heroicons/react/24/outline";
import { PodTileContent, type PodTileContentProps } from "@/components/agent-detail/pods/PodTile";

const meta = {
  title: "Features/Agent Detail/PodTile",
  component: PodTileContent,
  decorators: [
    (Story) => (
      <div className="max-w-xs">
        <Story />
      </div>
    ),
  ],
  argTypes: {
    status: { control: "select", options: ["healthy", "warning", "unhealthy", "pending"] },
  },
} satisfies Meta<PodTileContentProps>;

export default meta;
type Story = StoryObj<PodTileContentProps>;

export const Healthy: Story = {
  args: {
    name: "agent",
    status: "healthy",
    age: "3d",
  },
};

export const Unhealthy: Story = {
  args: {
    name: "agent",
    status: "unhealthy",
    age: "12m",
  },
};

export const UnhealthyWithError: Story = {
  args: {
    name: "agent",
    status: "unhealthy",
    age: "2m",
    errorMessage: "RuntimeError: CUDA out of memory. Tried to allocate 256 MiB",
  },
};

export const UnhealthyWithLongError: Story = {
  args: {
    name: "agent",
    status: "unhealthy",
    age: "45s",
    errorMessage:
      "ConnectionRefusedError: [Errno 111] Connection refused — could not connect to database at postgres.internal:5432 after 30 retries",
  },
};

export const Flapping: Story = {
  args: {
    name: "agent",
    status: "warning",
    age: "45m",
    warningMessage: "Restarting frequently (8 restarts)",
  },
};

export const Pending: Story = {
  args: {
    name: "worker",
    status: "pending",
  },
};

export const LongName: Story = {
  args: {
    name: "knowledge-ingestion-worker",
    status: "healthy",
    age: "7d",
  },
};

export const AllStatuses: Story = {
  args: {
    name: "agent",
    status: "healthy",
    age: "3d",
  },
  decorators: [
    (Story) => (
      <div className="flex max-w-2xl flex-col gap-4">
        <Story />
      </div>
    ),
  ],
  render: () => (
    <>
      <PodTileContent name="agent" status="healthy" age="3d" icon={Bot} />
      <PodTileContent name="redis" status="healthy" age="3d" icon={Database} />
      <PodTileContent name="collector" status="healthy" age="3d" icon={Activity} />
      <PodTileContent name="llm" status="healthy" age="1d" icon={Brain} />
      <PodTileContent name="messaging" status="healthy" age="5d" icon={ChatBubbleLeftRightIcon} />
      <PodTileContent name="worker" status="pending" />
      <PodTileContent name="agent" status="warning" age="45m" icon={Bot} warningMessage="Restarting frequently (8 restarts)" />
      <PodTileContent name="postgres" status="unhealthy" age="12m" icon={Database} />
      <PodTileContent
        name="agent"
        status="unhealthy"
        age="2m"
        icon={Bot}
        errorMessage="RuntimeError: CUDA out of memory. Tried to allocate 256 MiB"
      />
    </>
  ),
};
