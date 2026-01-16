import type { CompiledGraph } from "./graph";

export interface AgentTool {
  type: "graph";
  graph: CompiledGraph;
}

export type AgentStep = {
  id: string;
  name: string;
  type: "tool" | "agent";
};
