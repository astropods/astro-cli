import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  useReactFlow,
  useNodesInitialized,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
  type NodeTypes,
  type NodeChange,
} from "@xyflow/react";
import dagre from "@dagrejs/dagre";
import "@xyflow/react/dist/style.css";
import { agents } from "astro-agents";
import { AgentNode } from "./components/AgentNode";
import type { AgentNodeData, Port, CompiledGraph } from "./types";

// Build a list of agents from the agents export
const agentList = Object.entries(agents).map(([key, agent]) => {
  const compiled = agent.compile() as CompiledGraph;

  return {
    id: key,
    title: compiled.meta.title || key,
    description: compiled.meta.description || "",
    compiled,
  };
});

// Custom node types for React Flow
const nodeTypes: NodeTypes = {
  agent: AgentNode,
};

// Get the output ports for a node type
function getOutputPorts(nodeType: string): string[] {
  if (nodeType === "if") {
    return ["then", "else"];
  }
  return ["output"];
}

// Transform compiled graph to React Flow format (initial positions don't matter, dagre will layout)
function transformToReactFlow(compiled: CompiledGraph) {
  const { nodes: graphNodes, edges: graphEdges } = compiled;

  // Transform nodes - initial positions are temporary, dagre will recalculate
  const nodes: Node<AgentNodeData>[] = Object.values(graphNodes).map(
    (node) => {
      const outputPorts = getOutputPorts(node.type);
      const ports: Port[] = [
        { id: "input", type: "input" },
        ...outputPorts.map((id) => ({ id, type: "output" as const })),
      ];

      return {
        id: node.id,
        type: "agent",
        position: { x: 0, y: 0 },
        data: {
          label: node.name,
          nodeType: node.type,
          ports,
          nodeData: node.data,
        },
      };
    }
  );

  // Transform edges
  const edges: Edge[] = Object.entries(graphEdges).map(([id, edge]) => ({
    id,
    source: edge.source,
    target: edge.target,
    sourceHandle: edge.sourcePort,
    targetHandle: edge.targetPort,
    animated: true,
  }));

  return { nodes, edges };
}

// Default node dimensions (fallback if not measured)
const DEFAULT_NODE_WIDTH = 180;
const DEFAULT_NODE_HEIGHT = 60;

// Apply dagre layout to nodes using measured dimensions
function getLayoutedElements(
  nodes: Node<AgentNodeData>[],
  edges: Edge[],
  nodeDimensions: Map<string, { width: number; height: number }>
) {
  const dagreGraph = new dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}));
  dagreGraph.setGraph({ rankdir: "TB", nodesep: 50, ranksep: 80 });

  // Add nodes with their measured dimensions
  nodes.forEach((node) => {
    const dims = nodeDimensions.get(node.id);
    const width = dims?.width || DEFAULT_NODE_WIDTH;
    const height = dims?.height || DEFAULT_NODE_HEIGHT;
    dagreGraph.setNode(node.id, { width, height });
  });

  // Add edges
  edges.forEach((edge) => {
    dagreGraph.setEdge(edge.source, edge.target);
  });

  // Run the layout
  dagre.layout(dagreGraph);

  // Apply calculated positions to nodes
  const layoutedNodes = nodes.map((node) => {
    const nodeWithPosition = dagreGraph.node(node.id);
    const dims = nodeDimensions.get(node.id);
    const width = dims?.width || DEFAULT_NODE_WIDTH;
    const height = dims?.height || DEFAULT_NODE_HEIGHT;

    return {
      ...node,
      position: {
        x: nodeWithPosition.x - width / 2,
        y: nodeWithPosition.y - height / 2,
      },
    };
  });

  return { nodes: layoutedNodes, edges };
}

interface FlowContentProps {
  selectedAgent: {
    id: string;
    title: string;
    description: string;
    compiled: CompiledGraph;
  };
}

function FlowContent({ selectedAgent }: FlowContentProps) {
  const { getNodes, fitView } = useReactFlow();
  const nodesInitialized = useNodesInitialized();
  const layoutAppliedRef = useRef(false);
  const relayoutTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastDimensionsRef = useRef<Map<string, { width: number; height: number }>>(new Map());

  // Transform the compiled graph to initial React Flow format
  const initialElements = useMemo(
    () => transformToReactFlow(selectedAgent.compiled),
    [selectedAgent]
  );

  const [nodes, setNodes, onNodesChange] = useNodesState(initialElements.nodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialElements.edges);

  // Apply layout using current measured dimensions
  const applyLayout = useCallback((shouldFitView = true) => {
    const currentNodes = getNodes();
    if (currentNodes.length === 0) return;

    // Get measured dimensions from React Flow's internal nodes
    const nodeDimensions = new Map<string, { width: number; height: number }>();
    currentNodes.forEach((node) => {
      if (node.measured?.width && node.measured?.height) {
        nodeDimensions.set(node.id, {
          width: node.measured.width,
          height: node.measured.height,
        });
      }
    });

    // Store dimensions for comparison
    lastDimensionsRef.current = nodeDimensions;

    // Apply dagre layout with measured dimensions
    const { nodes: layoutedNodes, edges: layoutedEdges } = getLayoutedElements(
      currentNodes as Node<AgentNodeData>[],
      edges,
      nodeDimensions
    );

    setNodes(layoutedNodes);
    setEdges(layoutedEdges);
    layoutAppliedRef.current = true;

    // Fit view after layout with a small delay for the DOM to update
    if (shouldFitView) {
      requestAnimationFrame(() => {
        fitView({ padding: 0.3, duration: 200 });
      });
    }
  }, [getNodes, edges, setNodes, setEdges, fitView]);

  // Handle node changes and detect dimension changes
  const handleNodesChange = useCallback(
    (changes: NodeChange<Node<AgentNodeData>>[]) => {
      // Apply the changes first
      onNodesChange(changes);

      // Check if any dimension changes occurred
      const hasDimensionChange = changes.some(
        (change) => change.type === "dimensions"
      );

      if (hasDimensionChange && layoutAppliedRef.current) {
        // Debounce the relayout to avoid excessive recalculations
        if (relayoutTimeoutRef.current) {
          clearTimeout(relayoutTimeoutRef.current);
        }
        relayoutTimeoutRef.current = setTimeout(() => {
          applyLayout(false);
        }, 50);
      }
    },
    [onNodesChange, applyLayout]
  );

  // Trigger layout when nodes are initialized
  useEffect(() => {
    if (nodesInitialized && !layoutAppliedRef.current && nodes.length > 0) {
      // Small delay to ensure measurements are complete
      const timer = setTimeout(() => applyLayout(true), 50);
      return () => clearTimeout(timer);
    }
  }, [nodesInitialized, nodes.length, applyLayout]);

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (relayoutTimeoutRef.current) {
        clearTimeout(relayoutTimeoutRef.current);
      }
    };
  }, []);

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={handleNodesChange}
      onEdgesChange={onEdgesChange}
      nodeTypes={nodeTypes}
      fitView
      fitViewOptions={{ padding: 0.3 }}
      proOptions={{ hideAttribution: true }}
    >
      <Background color="#3f3f46" gap={20} />
      <Controls className="bg-zinc-800! border-zinc-700! rounded-lg! [&>button]:bg-zinc-800! [&>button]:border-zinc-700! [&>button]:text-zinc-400! [&>button:hover]:bg-zinc-700!" />
    </ReactFlow>
  );
}

function App() {
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(
    agentList[0]?.id ?? null
  );

  const selectedAgent = useMemo(
    () => agentList.find((a) => a.id === selectedAgentId),
    [selectedAgentId]
  );

  return (
    <div className="flex h-screen w-screen bg-zinc-950 text-zinc-100">
      {/* Sidebar */}
      <aside className="w-72 shrink-0 border-r border-zinc-800 bg-zinc-900/50 flex flex-col">
        <div className="p-6 border-b border-zinc-800">
          <h1 className="text-2xl font-bold tracking-tight bg-linear-to-r from-violet-400 to-fuchsia-400 bg-clip-text text-transparent">
            Astro
          </h1>
        </div>
        <nav className="flex-1 overflow-y-auto p-3 space-y-1">
          {agentList.map((agent) => (
            <button
              key={agent.id}
              className={`w-full text-left p-3 rounded-lg transition-all duration-150 ${
                selectedAgentId === agent.id
                  ? "bg-violet-500/20 border border-violet-500/40 text-violet-200"
                  : "hover:bg-zinc-800/60 border border-transparent text-zinc-300"
              }`}
              onClick={() => setSelectedAgentId(agent.id)}
            >
              <span className="block font-medium text-sm">{agent.title}</span>
              <span className="block text-xs text-zinc-500 mt-0.5 line-clamp-2">
                {agent.description}
              </span>
            </button>
          ))}
        </nav>
      </aside>

      {/* Main content */}
      <main className="flex-1 flex flex-col min-w-0">
        {selectedAgent ? (
          <>
            <header className="p-6 border-b border-zinc-800 bg-zinc-900/30">
              <h2 className="text-xl font-semibold text-zinc-100">
                {selectedAgent.title}
              </h2>
              <p className="text-sm text-zinc-500 mt-1">
                {selectedAgent.description}
              </p>
            </header>
            <div className="flex-1">
              <ReactFlowProvider key={selectedAgent.id}>
                <FlowContent selectedAgent={selectedAgent} />
              </ReactFlowProvider>
            </div>
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-zinc-500">
            <p>Select an agent from the sidebar</p>
          </div>
        )}
      </main>
    </div>
  );
}

export default App;
