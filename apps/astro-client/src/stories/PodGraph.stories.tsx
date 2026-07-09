import { useState, type ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Bot, Database, Activity, Brain, Box, Download } from "lucide-react";
import { PodGraph } from "@/components/agent-detail/pods/PodGraph";
import { PodTileContent, type PodStatus } from "@/components/agent-detail/pods/PodTile";
import { classify, brandIconId, type Role } from "@/components/agent-detail/pods/classify";
import { getIntegrationIcon } from "@/lib/integrationIcons";
import { StarField } from "@/components/agent-detail/starfield/StarField";

// A tile in these stories is described by its `component` (and `kind`), exactly
// as production data is: those drive the graph layout and relationship edges via
// `classify`. The rest is presentational chrome for the tile itself.
interface TileSpec {
  component: string;
  kind?: string;
  name?: string;
  status?: PodStatus;
  age?: string;
  warningMessage?: string;
  errorMessage?: string;
}

type IconComponent = React.ComponentType<{ className?: string }>;

// Mirrors PodTile's role→icon mapping so story tiles look like production.
const ROLE_ICONS: Record<Role, IconComponent> = {
  agent: Bot,
  knowledge: Database,
  model: Brain,
  integration: Box,
  ingestion: Download,
  collector: Activity,
  other: Box,
};

// Stand-in avatar for the agent tile — in the real app PodTile renders the
// deployment's actual avatar here; Storybook has no deployment, so we reuse the
// same placeholder asset the other agent stories (DeployedAgentCard, etc.) use.
const SAMPLE_AVATAR = "/assets/avatars/agents/chrisjpatty/chrisbot.jpg";

function leadingFor(component: string, role: Role): ReactNode {
  if (role === "agent") {
    return <img src={SAMPLE_AVATAR} alt="agent" className="size-5 shrink-0 rounded-sm object-cover" />;
  }
  const brandId = brandIconId(role, undefined, component);
  return brandId ? <span className="block size-5 shrink-0">{getIntegrationIcon(brandId)}</span> : undefined;
}

function renderTile(tiles: TileSpec[]) {
  return (i: number): ReactNode => {
    const t = tiles[i];
    const role = classify(t.component, t.kind);
    return (
      <PodTileContent
        name={t.name ?? t.component}
        status={t.status ?? "healthy"}
        age={t.age}
        warningMessage={t.warningMessage}
        errorMessage={t.errorMessage}
        icon={ROLE_ICONS[role]}
        leading={leadingFor(t.component, role)}
      />
    );
  };
}

function makeStory(tiles: TileSpec[]) {
  return {
    args: {
      count: tiles.length,
      keys: tiles.map((t, i) => `${t.component}-${i}`),
      components: tiles.map((t) => t.component),
      kinds: tiles.map((t) => t.kind ?? "Deployment"),
      renderTile: renderTile(tiles),
    },
  };
}

const meta = {
  title: "Features/Agent Detail/PodGraph",
  component: PodGraph,
  decorators: [
    (Story) => (
      <div className="relative h-[560px] w-full bg-background">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof PodGraph>;

export default meta;
type Story = StoryObj<typeof meta>;

// Just the agent — centered, no edges.
export const AgentOnly: Story = makeStory([
  { component: "agent", kind: "StatefulSet", age: "14d" },
]);

// Agent + one store — store column on the left, agent centered.
export const AgentAndDatabase: Story = makeStory([
  { component: "agent", kind: "StatefulSet", age: "14d" },
  { component: "knowledge-postgres", kind: "StatefulSet", name: "postgres", age: "14d" },
]);

// Two stores stacked in the knowledge column, feeding the agent.
export const AgentAndTwoStores: Story = makeStory([
  { component: "agent", kind: "StatefulSet", age: "14d" },
  { component: "knowledge-postgres", kind: "StatefulSet", name: "postgres", age: "14d" },
  { component: "knowledge-qdrant", kind: "StatefulSet", name: "qdrant", age: "14d" },
]);

// Ingestion column on the far left, fanning an edge to EVERY knowledge store.
export const KnowledgeAndIngestion: Story = makeStory([
  { component: "agent", kind: "StatefulSet", age: "14d" },
  { component: "knowledge-postgres", kind: "StatefulSet", name: "postgres", age: "14d" },
  { component: "knowledge-qdrant", kind: "StatefulSet", name: "qdrant", age: "14d" },
  { component: "ingestion-crawler", kind: "CronJob", name: "crawler", status: "healthy" },
]);

// The full flow: ingestion | knowledge | agent | others (model, tool, collector).
export const FullStack: Story = makeStory([
  { component: "agent", kind: "StatefulSet", age: "14d" },
  { component: "knowledge-postgres", kind: "StatefulSet", name: "postgres", age: "14d" },
  { component: "knowledge-qdrant", kind: "StatefulSet", name: "qdrant", age: "6d" },
  { component: "model-ollama", kind: "StatefulSet", name: "ollama", age: "6d" },
  { component: "tool-github", kind: "Deployment", name: "github", age: "6d" },
  { component: "ingestion-crawler", kind: "CronJob", name: "crawler" },
  { component: "collector", kind: "Deployment", age: "14d" },
]);

// One unhealthy store with a long error — layout must absorb the taller tile.
export const WithUnhealthyStore: Story = makeStory([
  { component: "agent", kind: "StatefulSet", age: "14d" },
  { component: "knowledge-postgres", kind: "StatefulSet", name: "postgres", age: "14d" },
  {
    component: "knowledge-qdrant",
    kind: "StatefulSet",
    name: "qdrant",
    status: "unhealthy",
    age: "45s",
    errorMessage:
      "ConnectionRefusedError: [Errno 111] Connection refused — could not connect to qdrant at qdrant.internal:6333 after 30 retries",
  },
  { component: "model-ollama", kind: "StatefulSet", name: "ollama", age: "6d" },
  { component: "collector", kind: "Deployment", age: "14d" },
]);

// Many stores — a tall knowledge column, agent still centered beside it.
export const ManyKnowledgeStores: Story = makeStory([
  { component: "agent", kind: "StatefulSet", age: "14d" },
  ...Array.from({ length: 9 }, (_, i): TileSpec => ({
    component: `knowledge-store-${i}`,
    kind: "StatefulSet",
    name: `store-${i}`,
    age: `${i + 1}d`,
  })),
]);

// A role-diverse deck the playground reveals one tile at a time, so you can
// watch the columns grow and edges reconnect as tiles are added/removed.
const PLAYGROUND_DECK: TileSpec[] = [
  { component: "agent", kind: "StatefulSet", age: "14d" },
  { component: "knowledge-postgres", kind: "StatefulSet", name: "postgres", age: "14d" },
  { component: "model-ollama", kind: "StatefulSet", name: "ollama", age: "6d" },
  { component: "ingestion-crawler", kind: "CronJob", name: "crawler" },
  { component: "knowledge-qdrant", kind: "StatefulSet", name: "qdrant", age: "6d" },
  { component: "collector", kind: "Deployment", age: "14d" },
  { component: "tool-github", kind: "Deployment", name: "github", age: "6d" },
  { component: "ingestion-webhook", kind: "Deployment", name: "webhook" },
  { component: "knowledge-neo4j", kind: "StatefulSet", name: "neo4j", age: "3d" },
  { component: "model-vllm", kind: "StatefulSet", name: "vllm", age: "2d" },
];

function ConfigurationPlayground() {
  const [count, setCount] = useState(4);
  const tiles = PLAYGROUND_DECK.slice(0, count);

  return (
    <div className="flex h-full flex-col">
      <div className="relative z-10 flex items-center gap-3 px-4 py-2">
        <button
          className="rounded bg-muted px-2 py-1 text-sm text-foreground"
          onClick={() => setCount((c) => Math.max(1, c - 1))}
        >
          −
        </button>
        <span className="text-sm text-foreground tabular-nums">
          {count} {count === 1 ? "tile" : "tiles"}
        </span>
        <button
          className="rounded bg-muted px-2 py-1 text-sm text-foreground"
          onClick={() => setCount((c) => Math.min(PLAYGROUND_DECK.length, c + 1))}
        >
          +
        </button>
      </div>
      <div className="relative flex-1 overflow-hidden">
        <StarField />
        <div className="relative z-10 h-full">
          <PodGraph
            count={tiles.length}
            keys={tiles.map((t, i) => `${t.component}-${i}`)}
            components={tiles.map((t) => t.component)}
            kinds={tiles.map((t) => t.kind ?? "Deployment")}
            renderTile={renderTile(tiles)}
          />
        </div>
      </div>
    </div>
  );
}

export const Playground: Story = {
  args: { count: 0, renderTile: () => null },
  decorators: [
    (Story) => (
      <div className="relative h-screen w-full">
        <Story />
      </div>
    ),
  ],
  render: () => <ConfigurationPlayground />,
};
