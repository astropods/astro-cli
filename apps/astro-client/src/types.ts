import type { Node } from "@xyflow/react";

export type Port = {
  id: string;
  type: "input" | "output";
};

export type AgentNodeData = {
  label: string;
  nodeType: string;
  ports: Port[];
  nodeData?: Record<string, unknown>;
};

export type AgentNode = Node<AgentNodeData, "agent">;

// Types from astro-graph compile output
export type CompiledGraphNode = {
  id: string;
  name: string;
  type: string;
  data: Record<string, unknown>;
};

export type CompiledGraphEdge = {
  source: string;
  target: string;
  sourcePort: string;
  targetPort: string;
};

export type CompiledGraph = {
  meta: {
    title: string;
    description: string;
  };
  nodes: Record<string, CompiledGraphNode>;
  edges: Record<string, CompiledGraphEdge>;
};
