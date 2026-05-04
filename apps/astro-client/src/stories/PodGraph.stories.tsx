import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Bot, Database, Activity, Brain, Box } from "lucide-react";
import { ChatBubbleLeftRightIcon } from "@heroicons/react/24/outline";
import { PodGraph } from "@/components/agent-detail/pods/PodGraph";
import { PodTileContent, type PodTileContentProps } from "@/components/agent-detail/pods/PodTile";
import { StarField } from "@/components/agent-detail/starfield/StarField";

type TileData = PodTileContentProps;

function makeStory(tiles: TileData[]) {
  return {
    args: {
      count: tiles.length,
      renderTile: (i: number) => <PodTileContent {...tiles[i]} />,
    },
  };
}

const meta = {
  title: "Features/Agent Detail/PodGraph",
  component: PodGraph,
  decorators: [
    (Story) => (
      <div className="relative h-[500px] w-full bg-background">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof PodGraph>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SinglePod: Story = makeStory([
  { name: "agent", status: "healthy", age: "3d" },
]);

export const TwoPods: Story = makeStory([
  { name: "agent", status: "healthy", age: "3d" },
  { name: "worker", status: "healthy", age: "1d" },
]);

export const ThreePods: Story = makeStory([
  { name: "agent", status: "healthy", age: "3d" },
  { name: "worker", status: "healthy", age: "1d" },
  { name: "ingestion", status: "pending" },
]);

export const WithUnhealthyPod: Story = makeStory([
  { name: "agent", status: "healthy", age: "3d" },
  { name: "agent", status: "unhealthy", age: "12m" },
  { name: "worker", status: "healthy", age: "1d" },
]);

export const ManyPods: Story = makeStory(
  Array.from({ length: 8 }, (_, i) => ({
    name: i % 3 === 0 ? "agent" : i % 3 === 1 ? "worker" : "sidecar",
    status: "healthy" as const,
    age: `${i + 1}d`,
  })),
);

const ALL_MIXED_TILES: PodTileContentProps[] = [
  { name: "agent", status: "healthy", age: "14d", icon: Bot },
  {
    name: "agent",
    status: "unhealthy",
    age: "3m",
    icon: Bot,
    errorMessage: "RuntimeError: CUDA out of memory. Tried to allocate 256 MiB",
  },
  { name: "redis", status: "healthy", age: "6d", icon: Database },
  { name: "postgres", status: "pending", icon: Database },
  { name: "collector", status: "healthy", age: "14d", icon: Activity },
  {
    name: "ollama",
    status: "unhealthy",
    age: "45s",
    icon: Brain,
    errorMessage:
      "ConnectionRefusedError: [Errno 111] Connection refused — could not connect to inference server at localhost:11434 after 30 retries",
  },
  { name: "messaging", status: "healthy", age: "2d", icon: ChatBubbleLeftRightIcon },
  { name: "agent", status: "warning", age: "45m", icon: Bot, warningMessage: "Restarting frequently (8 restarts)" },
  { name: "gateway", status: "healthy", age: "5d", icon: Box },
  { name: "neo4j", status: "healthy", age: "10d", icon: Database },
];

function MixedStatesPlayground() {
  const [count, setCount] = useState(5);
  const tiles = ALL_MIXED_TILES.slice(0, count);

  return (
    <div className="flex h-full flex-col">
      <div className="relative z-10 flex items-center gap-3 px-4 py-2">
        <button
          className="rounded bg-muted px-2 py-1 text-sm"
          onClick={() => setCount((c) => Math.max(1, c - 1))}
        >
          −
        </button>
        <span className="text-sm text-foreground">{count} pods</span>
        <button
          className="rounded bg-muted px-2 py-1 text-sm"
          onClick={() => setCount((c) => Math.min(ALL_MIXED_TILES.length, c + 1))}
        >
          +
        </button>
      </div>
      <div className="relative flex-1 overflow-hidden">
        <StarField />
        <div className="relative z-10 h-full">
          <PodGraph
            count={tiles.length}
            renderTile={(i) => <PodTileContent {...tiles[i]} />}
          />
        </div>
      </div>
    </div>
  );
}

export const MixedStates: Story = {
  args: { count: 0, renderTile: () => null },
  decorators: [
    (Story) => (
      <div className="relative h-screen w-full">
        <Story />
      </div>
    ),
  ],
  render: () => <MixedStatesPlayground />,
};
