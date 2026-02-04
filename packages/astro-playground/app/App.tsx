import { useState, useRef, useEffect, useCallback, useMemo } from "react";
import {
  Send,
  Bot,
  User,
  Loader2,
  Sparkles,
  ChevronDown,
  ChevronRight,
  Wrench,
  Brain,
  Check,
  AlertCircle,
  Cpu,
  Copy,
  CheckCheck,
  MessageSquare,
  Settings2,
  FileText,
} from "lucide-react";
import Markdown from "react-markdown";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism";
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Handle,
  useReactFlow,
  Position,
  type Node as FlowNode,
  type Edge as FlowEdge,
  type NodeProps,
} from "@xyflow/react";
import dagre from "@dagrejs/dagre";
import "@xyflow/react/dist/style.css";

const API_URL = "http://localhost:3001";

type AgentInfo = {
  id: string;
  title: string;
  description: string;
};

type ToolConfig = {
  name: string;
  title: string;
  description: string;
  type: "graph" | "other";
  graph?: {
    nodes: { id: string; name: string; type: string }[];
    edges: { id: string; source: string; target: string }[];
  };
};

type AgentConfig = {
  systemPrompt: string;
  tools: ToolConfig[];
};

type ViewMode = "chat" | "config";

type Message = {
  id: string;
  role: "user" | "assistant";
  content: string;
  steps?: Step[];
  reasoning?: string;
  isStreaming?: boolean;
};

type Step = {
  id: string;
  name: string;
  type: "tool";
  status: "running" | "completed";
};

type ModelOption = {
  id: string;
  name: string;
  provider: string;
  supportsReasoning?: boolean;
};

const AVAILABLE_MODELS: ModelOption[] = [
  // OpenAI Frontier Models
  { id: "openai/gpt-5.2", name: "GPT-5.2", provider: "OpenAI", supportsReasoning: true },
  { id: "openai/gpt-5.2-pro", name: "GPT-5.2 Pro", provider: "OpenAI", supportsReasoning: true },
  { id: "openai/gpt-5.1", name: "GPT-5.1", provider: "OpenAI", supportsReasoning: true },
  { id: "openai/gpt-5", name: "GPT-5", provider: "OpenAI", supportsReasoning: true },
  { id: "openai/gpt-5-mini", name: "GPT-5 Mini", provider: "OpenAI", supportsReasoning: true },
  { id: "openai/gpt-5-nano", name: "GPT-5 Nano", provider: "OpenAI", supportsReasoning: true },
  { id: "openai/gpt-4.1", name: "GPT-4.1", provider: "OpenAI" },
  { id: "openai/gpt-4.1-mini", name: "GPT-4.1 Mini", provider: "OpenAI" },
  { id: "openai/gpt-4.1-nano", name: "GPT-4.1 Nano", provider: "OpenAI" },
  // OpenAI Reasoning Models
  { id: "openai/o3", name: "o3", provider: "OpenAI", supportsReasoning: true },
  { id: "openai/o4-mini", name: "o4 Mini", provider: "OpenAI", supportsReasoning: true },
  { id: "openai/o3-mini", name: "o3 Mini", provider: "OpenAI", supportsReasoning: true },
  // OpenAI Legacy Models
  { id: "openai/gpt-4o", name: "GPT-4o", provider: "OpenAI" },
  { id: "openai/gpt-4o-mini", name: "GPT-4o Mini", provider: "OpenAI" },
  // Anthropic Models
  { id: "anthropic/claude-sonnet-4-20250514", name: "Claude Sonnet 4", provider: "Anthropic" },
  { id: "anthropic/claude-3-5-haiku-20241022", name: "Claude 3.5 Haiku", provider: "Anthropic" },
  // Google Models
  { id: "google/gemini-2.0-flash", name: "Gemini 2.0 Flash", provider: "Google" },
  { id: "google/gemini-2.5-pro-preview-05-06", name: "Gemini 2.5 Pro", provider: "Google" },
];

function generateId() {
  return Math.random().toString(36).substring(2, 15);
}

// Custom code block theme based on oneDark but tweaked for our design
const codeTheme = {
  ...oneDark,
  'pre[class*="language-"]': {
    ...oneDark['pre[class*="language-"]'],
    background: "transparent",
    margin: 0,
    padding: 0,
    fontSize: "0.85em",
    lineHeight: 1.6,
  },
  'code[class*="language-"]': {
    ...oneDark['code[class*="language-"]'],
    background: "transparent",
    fontSize: "inherit",
  },
};

// Custom pre component to avoid double-wrapping code blocks
function Pre({ children }: { children?: React.ReactNode }) {
  // Check if the child is a code block (will be handled by CodeBlock component)
  // If so, just render children without the pre wrapper styling
  return <>{children}</>;
}

function CodeBlock({
  children,
  className,
  ...props
}: {
  children?: React.ReactNode;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);
  const match = /language-(\w+)/.exec(className || "");
  const language = match ? match[1] : "";
  const codeString = String(children).replace(/\n$/, "");

  const handleCopy = async () => {
    await navigator.clipboard.writeText(codeString);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  // Inline code (no language specified, single line)
  if (!match) {
    return (
      <code className={className} {...props}>
        {children}
      </code>
    );
  }

  // If the language is markdown/md and the content contains code fences,
  // render it as actual markdown instead of as a code block.
  // This handles cases where the AI wraps markdown content in ```md blocks
  // but the content itself contains nested code fences which break parsing.
  if ((language === "md" || language === "markdown") && /^```\w*$/m.test(codeString)) {
    return (
      <div className="nested-markdown-content">
        <Markdown
          components={{
            pre: Pre,
            code: CodeBlock,
          }}
        >
          {codeString}
        </Markdown>
      </div>
    );
  }

  // Code block with syntax highlighting
  return (
    <div className="code-block-wrapper group relative">
      <div className="code-block-header flex items-center justify-between px-4 py-2 bg-[#1e1e2e] border-b border-[var(--color-border)] rounded-t-lg">
        <span className="text-xs font-medium text-[var(--color-text-muted)] uppercase tracking-wider">
          {language}
        </span>
        <button
          onClick={handleCopy}
          className="flex items-center gap-1.5 px-2 py-1 text-xs text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)] hover:bg-[var(--color-bg-tertiary)] rounded transition-all"
          title="Copy code"
        >
          {copied ? (
            <>
              <CheckCheck className="w-3.5 h-3.5 text-[var(--color-success)]" />
              <span className="text-[var(--color-success)]">Copied!</span>
            </>
          ) : (
            <>
              <Copy className="w-3.5 h-3.5" />
              <span>Copy</span>
            </>
          )}
        </button>
      </div>
      <SyntaxHighlighter
        style={codeTheme}
        language={language}
        PreTag="div"
        className="code-block-content !bg-[#0d0d14] !rounded-t-none !rounded-b-lg !m-0 !p-4"
        showLineNumbers={codeString.split("\n").length > 3}
        lineNumberStyle={{
          minWidth: "2.5em",
          paddingRight: "1em",
          color: "var(--color-text-muted)",
          opacity: 0.5,
          userSelect: "none",
        }}
      >
        {codeString}
      </SyntaxHighlighter>
    </div>
  );
}

function AgentSelector({
  agents,
  selectedAgent,
  onSelect,
}: {
  agents: AgentInfo[];
  selectedAgent: string;
  onSelect: (id: string) => void;
}) {
  const [isOpen, setIsOpen] = useState(false);
  const selected = agents.find((a) => a.id === selectedAgent);

  if (!selected) return null;

  return (
    <div className="relative">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="flex items-center gap-3 px-4 py-3 rounded-xl bg-[var(--color-bg-tertiary)] border border-[var(--color-border)] hover:border-[var(--color-accent)] transition-all duration-200 min-w-[280px]"
      >
        <div className="w-10 h-10 rounded-lg bg-gradient-to-br from-[var(--color-accent)] to-purple-500 flex items-center justify-center">
          <Bot className="w-5 h-5 text-white" />
        </div>
        <div className="flex-1 text-left">
          <div className="text-sm font-medium text-[var(--color-text-primary)]">
            {selected.title}
          </div>
          <div className="text-xs text-[var(--color-text-muted)] truncate max-w-[180px]">
            {selected.description}
          </div>
        </div>
        <ChevronDown
          className={`w-4 h-4 text-[var(--color-text-muted)] transition-transform duration-200 ${isOpen ? "rotate-180" : ""}`}
        />
      </button>

      {isOpen && (
        <div className="absolute top-full left-0 right-0 mt-2 py-2 bg-[var(--color-bg-tertiary)] border border-[var(--color-border)] rounded-xl shadow-xl z-50 animate-fade-in">
          {agents.map((agent) => (
            <button
              key={agent.id}
              onClick={() => {
                onSelect(agent.id);
                setIsOpen(false);
              }}
              className={`w-full px-4 py-3 flex items-center gap-3 hover:bg-[var(--color-accent-soft)] transition-colors ${agent.id === selectedAgent ? "bg-[var(--color-accent-soft)]" : ""
                }`}
            >
              <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-[var(--color-accent)] to-purple-500 flex items-center justify-center">
                <Bot className="w-4 h-4 text-white" />
              </div>
              <div className="flex-1 text-left">
                <div className="text-sm font-medium">{agent.title}</div>
                <div className="text-xs text-[var(--color-text-muted)]">
                  {agent.description}
                </div>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function ModelSelector({
  selectedModel,
  onSelect,
}: {
  selectedModel: string;
  onSelect: (id: string) => void;
}) {
  const [isOpen, setIsOpen] = useState(false);
  const selected = AVAILABLE_MODELS.find((m) => m.id === selectedModel);

  if (!selected) return null;

  return (
    <div className="relative">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="flex items-center gap-2 px-3 py-2 rounded-lg bg-[var(--color-bg-secondary)] border border-[var(--color-border)] hover:border-[var(--color-accent)] transition-all duration-200 text-sm"
      >
        <Cpu className="w-4 h-4 text-[var(--color-accent)]" />
        <span className="text-[var(--color-text-primary)]">{selected.name}</span>
        {selected.supportsReasoning && (
          <Brain className="w-3 h-3 text-amber-400" />
        )}
        <ChevronDown
          className={`w-3 h-3 text-[var(--color-text-muted)] transition-transform duration-200 ${isOpen ? "rotate-180" : ""}`}
        />
      </button>

      {isOpen && (
        <div className="absolute bottom-full left-0 mb-2 py-2 bg-[var(--color-bg-tertiary)] border border-[var(--color-border)] rounded-xl shadow-xl z-50 animate-fade-in min-w-[220px] max-h-[400px] overflow-y-auto">
          {AVAILABLE_MODELS.map((model) => (
            <button
              key={model.id}
              onClick={() => {
                onSelect(model.id);
                setIsOpen(false);
              }}
              className={`w-full px-4 py-2.5 flex items-center gap-3 hover:bg-[var(--color-accent-soft)] transition-colors ${model.id === selectedModel ? "bg-[var(--color-accent-soft)]" : ""
                }`}
            >
              <Cpu className="w-4 h-4 text-[var(--color-accent)] shrink-0" />
              <div className="flex-1 text-left">
                <div className="text-sm font-medium flex items-center gap-2">
                  {model.name}
                  {model.supportsReasoning && (
                    <Brain className="w-3 h-3 text-amber-400" />
                  )}
                </div>
                <div className="text-xs text-[var(--color-text-muted)]">
                  {model.provider}
                </div>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function ViewToggle({
  viewMode,
  onToggle,
}: {
  viewMode: ViewMode;
  onToggle: (mode: ViewMode) => void;
}) {
  return (
    <div className="flex items-center bg-[var(--color-bg-tertiary)] rounded-lg p-1 border border-[var(--color-border)]">
      <button
        onClick={() => onToggle("chat")}
        className={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-all duration-200 ${viewMode === "chat"
          ? "bg-[var(--color-accent)] text-white shadow-sm"
          : "text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)]"
          }`}
      >
        <MessageSquare className="w-4 h-4" />
        Chat
      </button>
      <button
        onClick={() => onToggle("config")}
        className={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-all duration-200 ${viewMode === "config"
          ? "bg-[var(--color-accent)] text-white shadow-sm"
          : "text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)]"
          }`}
      >
        <Settings2 className="w-4 h-4" />
        Config
      </button>
    </div>
  );
}

function GraphNode({ data }: NodeProps) {
  const isStartOrEnd = data.isStart || data.isEnd;
  const bgColor = isStartOrEnd ? "#7c3aed" : "#1e1e2e";

  // Map string positions to Position enum
  const targetPositionMap: Record<string, Position> = {
    left: Position.Left,
    top: Position.Top,
    right: Position.Right,
    bottom: Position.Bottom,
  };
  const sourcePositionMap: Record<string, Position> = {
    left: Position.Left,
    top: Position.Top,
    right: Position.Right,
    bottom: Position.Bottom,
  };

  const targetPosition = targetPositionMap[data.targetPosition as string] || Position.Left;
  const sourcePosition = sourcePositionMap[data.sourcePosition as string] || Position.Right;

  return (
    <div
      style={{
        background: bgColor,
        color: "#fff",
        border: "1px solid #3f3f5a",
        borderRadius: "8px",
        padding: "8px 16px",
        fontSize: "12px",
        minWidth: "120px",
        textAlign: "center",
      }}
    >
      <Handle type="target" position={targetPosition} />
      {typeof data.label === "string" ? data.label : JSON.stringify(data.label)}
      <Handle type="source" position={sourcePosition} />
    </div>
  );
}

const nodeTypes = { graphNode: GraphNode };

// Dagre layout constants
const NODE_WIDTH = 172;
const NODE_HEIGHT = 36;

/**
 * Uses dagre to calculate positions for nodes in a directed graph layout
 */
function getLayoutedElements(
  nodes: FlowNode[],
  edges: FlowEdge[],
  direction: "TB" | "LR" = "LR"
): { nodes: FlowNode[]; edges: FlowEdge[] } {
  const dagreGraph = new dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}));
  const isHorizontal = direction === "LR";

  dagreGraph.setGraph({ rankdir: direction, nodesep: 50, ranksep: 80 });

  nodes.forEach((node) => {
    dagreGraph.setNode(node.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  });

  edges.forEach((edge) => {
    dagreGraph.setEdge(edge.source, edge.target);
  });

  dagre.layout(dagreGraph);

  const newNodes = nodes.map((node) => {
    const nodeWithPosition = dagreGraph.node(node.id);
    return {
      ...node,
      data: {
        ...node.data,
        targetPosition: isHorizontal ? "left" : "top",
        sourcePosition: isHorizontal ? "right" : "bottom",
      },
      position: {
        x: nodeWithPosition.x - NODE_WIDTH / 2,
        y: nodeWithPosition.y - NODE_HEIGHT / 2,
      },
    };
  });

  return { nodes: newNodes, edges };
}

function ToolGraphFlowInner({ tool }: { tool: ToolConfig }) {
  const { fitView } = useReactFlow();

  const { nodes, edges } = useMemo(() => {
    if (!tool.graph) return { nodes: [], edges: [] };

    // Find terminal nodes (no outgoing edges)
    const nodesWithOutgoing = new Set(tool.graph.edges.map((e) => e.source));
    const terminalNodeIds = tool.graph.nodes
      .filter((node) => !nodesWithOutgoing.has(node.id))
      .map((node) => node.id);

    // Create initial flow nodes (positions will be set by dagre)
    const flowNodes: FlowNode[] = tool.graph.nodes.map((node) => ({
      id: node.id,
      type: "graphNode",
      position: { x: 0, y: 0 },
      data: { label: node.name, isStart: node.type === "start" },
    }));

    // Add End node
    const endNodeId = "__end__";
    flowNodes.push({
      id: endNodeId,
      type: "graphNode",
      position: { x: 0, y: 0 },
      data: { label: "End", isEnd: true },
    });

    const flowEdges: FlowEdge[] = tool.graph.edges.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      animated: true,
    }));

    // Add edges from terminal nodes to End node
    terminalNodeIds.forEach((nodeId, index) => {
      flowEdges.push({
        id: `__end_edge_${index}`,
        source: nodeId,
        target: endNodeId,
        animated: true,
      });
    });

    // Apply dagre layout
    return getLayoutedElements(flowNodes, flowEdges, "LR");
  }, [tool.graph]);

  // Fit view when nodes change
  useEffect(() => {
    if (nodes.length > 0) {
      // Small delay to ensure nodes are rendered
      const timer = setTimeout(() => {
        fitView({ padding: 0.3, duration: 200 });
      }, 50);
      return () => clearTimeout(timer);
    }
  }, [nodes, fitView]);

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      fitView
      fitViewOptions={{ padding: 0.3 }}
      nodesDraggable={false}
      nodesConnectable={false}
      elementsSelectable={false}
      panOnDrag={true}
      zoomOnScroll={true}
      zoomOnPinch={true}
      zoomOnDoubleClick={false}
      minZoom={0.1}
      maxZoom={2}
      proOptions={{ hideAttribution: true }}
    >
      <Background color="#3f3f5a" gap={16} size={1} />
    </ReactFlow>
  );
}

function ToolGraphView({ tool }: { tool: ToolConfig }) {
  if (!tool.graph) return null;

  // Calculate height based on number of nodes (more nodes = taller graph for branches)
  const nodeCount = tool.graph.nodes.length;
  const height = Math.max(200, Math.min(400, nodeCount * 30));

  return (
    <div
      className="w-full bg-[#0d0d14] rounded-lg border border-[var(--color-border)] mt-3"
      style={{ height: `${height}px` }}
    >
      <ReactFlowProvider>
        <ToolGraphFlowInner tool={tool} />
      </ReactFlowProvider>
    </div>
  );
}

function ToolCard({ tool }: { tool: ToolConfig }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const hasGraph = tool.type === "graph" && tool.graph;

  return (
    <div className="bg-[var(--color-bg-secondary)] rounded-lg border border-[var(--color-border)] overflow-hidden">
      <button
        onClick={() => hasGraph && setIsExpanded(!isExpanded)}
        className={`w-full flex items-start gap-3 p-3 text-left ${hasGraph ? "cursor-pointer hover:bg-[var(--color-bg-tertiary)]" : "cursor-default"} transition-colors`}
        disabled={!hasGraph}
      >
        <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-[var(--color-accent)] to-purple-500 flex items-center justify-center shrink-0">
          <Wrench className="w-4 h-4 text-white" />
        </div>
        <div className="flex-1 min-w-0">
          <h4 className="text-sm font-medium text-[var(--color-text-primary)]">
            {tool.title}
          </h4>
          <p className="text-xs text-[var(--color-text-muted)] mt-1">
            {tool.description || "No description"}
          </p>
        </div>
        {hasGraph && (
          <div className="shrink-0 text-[var(--color-text-muted)]">
            {isExpanded ? (
              <ChevronDown className="w-4 h-4" />
            ) : (
              <ChevronRight className="w-4 h-4" />
            )}
          </div>
        )}
      </button>
      {isExpanded && hasGraph && (
        <div className="px-3 pb-3">
          <ToolGraphView tool={tool} />
        </div>
      )}
    </div>
  );
}

function AgentConfigView({
  config,
  isLoading,
}: {
  config: AgentConfig | null;
  isLoading: boolean;
}) {
  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <Loader2 className="w-8 h-8 text-[var(--color-accent)] animate-spin" />
      </div>
    );
  }

  if (!config) {
    return (
      <div className="flex-1 flex items-center justify-center text-[var(--color-text-muted)]">
        No configuration available
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto px-6 py-6">
      <div className="max-w-4xl mx-auto space-y-6">
        {/* System Prompt Section */}
        <div className="bg-[var(--color-bg-tertiary)] border border-[var(--color-border)] rounded-xl overflow-hidden">
          <div className="flex items-center gap-2 px-4 py-3 bg-[var(--color-bg-secondary)] border-b border-[var(--color-border)]">
            <FileText className="w-4 h-4 text-[var(--color-accent)]" />
            <h3 className="text-sm font-medium text-[var(--color-text-primary)]">
              System Prompt
            </h3>
          </div>
          <div className="p-4">
            <p className="text-sm text-[var(--color-text-secondary)] whitespace-pre-wrap leading-relaxed">
              {config.systemPrompt || "No system prompt configured"}
            </p>
          </div>
        </div>

        {/* Tools Section */}
        <div className="bg-[var(--color-bg-tertiary)] border border-[var(--color-border)] rounded-xl overflow-hidden">
          <div className="flex items-center gap-2 px-4 py-3 bg-[var(--color-bg-secondary)] border-b border-[var(--color-border)]">
            <Wrench className="w-4 h-4 text-[var(--color-accent)]" />
            <h3 className="text-sm font-medium text-[var(--color-text-primary)]">
              Available Tools
            </h3>
            <span className="ml-auto text-xs text-[var(--color-text-muted)] bg-[var(--color-bg-tertiary)] px-2 py-0.5 rounded-full">
              {config.tools.length} tool{config.tools.length !== 1 ? "s" : ""}
            </span>
          </div>
          <div className="p-4">
            {config.tools.length === 0 ? (
              <p className="text-sm text-[var(--color-text-muted)]">
                No tools configured
              </p>
            ) : (
              <div className="space-y-3">
                {config.tools.map((tool, index) => (
                  <ToolCard key={index} tool={tool} />
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function StepIndicator({ step }: { step: Step }) {
  return (
    <div className="flex items-center gap-2 px-3 py-2 bg-[var(--color-bg-tertiary)] rounded-lg border border-[var(--color-border)] text-sm">
      {step.status === "running" ? (
        <Loader2 className="w-4 h-4 text-[var(--color-accent)] animate-spin" />
      ) : (
        <Check className="w-4 h-4 text-[var(--color-success)]" />
      )}
      <Wrench className="w-3.5 h-3.5 text-[var(--color-text-muted)]" />
      <span className="text-[var(--color-text-secondary)]">{step.name}</span>
    </div>
  );
}

function LiveReasoning({ reasoning, isStreaming }: { reasoning: string; isStreaming: boolean }) {
  const [isVisible, setIsVisible] = useState(true);
  const [isFadingOut, setIsFadingOut] = useState(false);

  useEffect(() => {
    if (!isStreaming && reasoning) {
      // Start fade out when streaming stops
      setIsFadingOut(true);
      const timer = setTimeout(() => {
        setIsVisible(false);
      }, 500); // Match the CSS transition duration
      return () => clearTimeout(timer);
    }
  }, [isStreaming, reasoning]);

  if (!reasoning || !isVisible) return null;

  return (
    <div
      className={`mb-3 flex items-start gap-2 transition-opacity duration-500 ${isFadingOut ? "opacity-0" : "opacity-100"
        }`}
    >
      <Brain className="w-3.5 h-3.5 text-[var(--color-text-muted)] mt-0.5 shrink-0 animate-pulse" />
      <p className="text-xs text-[var(--color-text-muted)] italic leading-relaxed">
        {reasoning}
        {isStreaming && (
          <span className="inline-block w-1.5 h-3 bg-[var(--color-text-muted)] rounded-sm ml-1 animate-pulse opacity-50" />
        )}
      </p>
    </div>
  );
}

function ThinkingIndicator({ label = "Thinking" }: { label?: string }) {
  return (
    <div className="flex items-center gap-2 px-4 py-3 bg-[var(--color-bg-tertiary)] border border-[var(--color-border)] rounded-2xl">
      <div className="flex items-center gap-1.5">
        <span
          className="w-2 h-2 bg-[var(--color-accent)] rounded-full animate-bounce"
          style={{ animationDelay: "0ms", animationDuration: "600ms" }}
        />
        <span
          className="w-2 h-2 bg-[var(--color-accent)] rounded-full animate-bounce"
          style={{ animationDelay: "150ms", animationDuration: "600ms" }}
        />
        <span
          className="w-2 h-2 bg-[var(--color-accent)] rounded-full animate-bounce"
          style={{ animationDelay: "300ms", animationDuration: "600ms" }}
        />
      </div>
      <span className="text-sm text-[var(--color-text-muted)]">{label}</span>
    </div>
  );
}

function ChatMessage({ message }: { message: Message }) {
  const isUser = message.role === "user";
  const hasContent = message.content && message.content.trim().length > 0;
  const hasSteps = message.steps && message.steps.length > 0;
  const isThinking = message.isStreaming && !hasContent;
  const allStepsCompleted = hasSteps && message.steps!.every((s) => s.status === "completed");
  const isProcessingToolResults = isThinking && allStepsCompleted;

  return (
    <div
      className={`flex gap-3 animate-fade-in ${isUser ? "flex-row-reverse" : ""}`}
    >
      <div
        className={`w-9 h-9 rounded-xl flex items-center justify-center shrink-0 ${isUser
          ? "bg-gradient-to-br from-emerald-500 to-teal-600"
          : "bg-gradient-to-br from-[var(--color-accent)] to-purple-500"
          }`}
      >
        {isUser ? (
          <User className="w-4 h-4 text-white" />
        ) : (
          <Bot className="w-4 h-4 text-white" />
        )}
      </div>
      <div className={`flex-1 max-w-[80%] ${isUser ? "flex flex-col items-end" : ""}`}>
        {message.reasoning && (
          <LiveReasoning
            reasoning={message.reasoning}
            isStreaming={message.isStreaming ?? false}
          />
        )}

        {hasSteps && (
          <div className="flex flex-wrap gap-2 mb-3">
            {message.steps!.map((step) => (
              <StepIndicator key={step.id} step={step} />
            ))}
          </div>
        )}

        {/* Show thinking indicator when streaming with no content and no steps */}
        {isThinking && !hasSteps && <ThinkingIndicator />}

        {/* Show processing indicator when tool calls completed but text hasn't started */}
        {isProcessingToolResults && <ThinkingIndicator label="Processing results" />}

        {/* Show message bubble only when there's actual content */}
        {hasContent && (
          <div
            className={`px-4 py-3 rounded-2xl ${isUser
              ? "bg-gradient-to-br from-[var(--color-accent)] to-purple-600 text-white"
              : "bg-[var(--color-bg-tertiary)] border border-[var(--color-border)]"
              }`}
          >
            <div className={`markdown-content ${isUser ? "markdown-content-user" : ""}`}>
              <Markdown
                components={{
                  pre: Pre,
                  code: CodeBlock,
                }}
              >
                {message.content}
              </Markdown>
            </div>
            {message.isStreaming && (
              <span className="inline-block w-2 h-4 bg-[var(--color-accent)] rounded-sm ml-1 animate-pulse-soft" />
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function EmptyState() {
  return (
    <div className="flex-1 flex flex-col items-center justify-center text-center px-8">
      <div className="w-20 h-20 rounded-2xl bg-gradient-to-br from-[var(--color-accent)] to-purple-500 flex items-center justify-center mb-6 shadow-lg shadow-purple-500/20">
        <Sparkles className="w-10 h-10 text-white" />
      </div>
      <h2 className="text-2xl font-semibold text-[var(--color-text-primary)] mb-2">
        Astro Agents Playground
      </h2>
      <p className="text-[var(--color-text-muted)] max-w-md">
        Test and interact with your AI agents. Select an agent above and start a
        conversation to see how it responds.
      </p>
    </div>
  );
}

function ConnectionError({ onRetry }: { onRetry: () => void }) {
  return (
    <div className="flex-1 flex flex-col items-center justify-center text-center px-8">
      <div className="w-20 h-20 rounded-2xl bg-gradient-to-br from-red-500 to-orange-500 flex items-center justify-center mb-6 shadow-lg shadow-red-500/20">
        <AlertCircle className="w-10 h-10 text-white" />
      </div>
      <h2 className="text-2xl font-semibold text-[var(--color-text-primary)] mb-2">
        Server Not Running
      </h2>
      <p className="text-[var(--color-text-muted)] max-w-md mb-6">
        Make sure the playground server is running. Start it with:
      </p>
      <code className="px-4 py-2 bg-[var(--color-bg-tertiary)] border border-[var(--color-border)] rounded-lg text-sm font-mono text-[var(--color-text-secondary)] mb-6">
        bun run playground:server
      </code>
      <button
        onClick={onRetry}
        className="px-6 py-2 bg-[var(--color-accent)] hover:bg-[var(--color-accent-hover)] text-white rounded-lg transition-colors"
      >
        Retry Connection
      </button>
    </div>
  );
}

export default function App() {
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [selectedAgentId, setSelectedAgentId] = useState<string>("");
  const [selectedModel, setSelectedModel] = useState<string>(AVAILABLE_MODELS[0].id);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [connectionError, setConnectionError] = useState(false);
  const [threadId] = useState(() => generateId());
  const [userId] = useState(() => generateId());
  const [viewMode, setViewMode] = useState<ViewMode>("chat");
  const [agentConfig, setAgentConfig] = useState<AgentConfig | null>(null);
  const [isLoadingConfig, setIsLoadingConfig] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, []);

  const fetchAgents = useCallback(async () => {
    try {
      const res = await fetch(`${API_URL}/api/agents`);
      if (!res.ok) throw new Error("Failed to fetch agents");
      const data = await res.json();
      setAgents(data);
      if (data.length > 0 && !selectedAgentId) {
        setSelectedAgentId(data[0].id);
      }
      setConnectionError(false);
    } catch {
      setConnectionError(true);
    }
  }, [selectedAgentId]);

  const fetchAgentConfig = useCallback(async (agentId: string) => {
    if (!agentId) return;
    setIsLoadingConfig(true);
    try {
      const res = await fetch(`${API_URL}/api/agents/${agentId}/config`);
      if (!res.ok) throw new Error("Failed to fetch agent config");
      const data = await res.json();
      setAgentConfig(data);
    } catch {
      setAgentConfig(null);
    } finally {
      setIsLoadingConfig(false);
    }
  }, []);

  useEffect(() => {
    fetchAgents();
  }, [fetchAgents]);

  useEffect(() => {
    if (viewMode === "config" && selectedAgentId) {
      fetchAgentConfig(selectedAgentId);
    }
  }, [viewMode, selectedAgentId, fetchAgentConfig]);

  useEffect(() => {
    scrollToBottom();
  }, [messages, scrollToBottom]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || isLoading || !selectedAgentId) return;

    const userMessage: Message = {
      id: generateId(),
      role: "user",
      content: input.trim(),
    };

    const assistantMessageId = generateId();
    const assistantMessage: Message = {
      id: assistantMessageId,
      role: "assistant",
      content: "",
      steps: [],
      reasoning: "",
      isStreaming: true,
    };

    setMessages((prev) => [...prev, userMessage, assistantMessage]);
    setInput("");
    setIsLoading(true);

    try {
      const res = await fetch(`${API_URL}/api/chat`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          agentId: selectedAgentId,
          prompt: userMessage.content,
          threadId,
          userId,
          model: selectedModel,
        }),
      });

      if (!res.ok) throw new Error("Failed to connect to agent");

      const reader = res.body?.getReader();
      if (!reader) throw new Error("No response body");

      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n\n");
        buffer = lines.pop() || "";

        for (const line of lines) {
          if (line.startsWith("data: ")) {
            const jsonStr = line.slice(6);
            try {
              const event = JSON.parse(jsonStr);

              switch (event.type) {
                case "chunk":
                  setMessages((prev) =>
                    prev.map((msg) =>
                      msg.id === assistantMessageId
                        ? { ...msg, content: msg.content + event.data.text }
                        : msg
                    )
                  );
                  break;
                case "step-start":
                  setMessages((prev) =>
                    prev.map((msg) =>
                      msg.id === assistantMessageId
                        ? {
                          ...msg,
                          steps: [
                            ...(msg.steps || []),
                            { ...event.data, status: "running" as const },
                          ],
                        }
                        : msg
                    )
                  );
                  break;
                case "step-end":
                  setMessages((prev) =>
                    prev.map((msg) =>
                      msg.id === assistantMessageId
                        ? {
                          ...msg,
                          steps: msg.steps?.map((s) =>
                            s.id === event.data.id
                              ? { ...s, status: "completed" as const }
                              : s
                          ),
                        }
                        : msg
                    )
                  );
                  break;
                case "reasoning-chunk":
                  setMessages((prev) =>
                    prev.map((msg) =>
                      msg.id === assistantMessageId
                        ? {
                          ...msg,
                          reasoning: (msg.reasoning || "") + event.data.text,
                        }
                        : msg
                    )
                  );
                  break;
                case "finish":
                  setMessages((prev) =>
                    prev.map((msg) =>
                      msg.id === assistantMessageId
                        ? { ...msg, isStreaming: false }
                        : msg
                    )
                  );
                  break;
                case "error":
                  setMessages((prev) =>
                    prev.map((msg) =>
                      msg.id === assistantMessageId
                        ? {
                          ...msg,
                          content: `Error: ${event.data.message}`,
                          isStreaming: false,
                        }
                        : msg
                    )
                  );
                  break;
              }
            } catch {
              // Skip invalid JSON
            }
          }
        }
      }

      // Ensure streaming is marked as complete
      setMessages((prev) =>
        prev.map((msg) =>
          msg.id === assistantMessageId ? { ...msg, isStreaming: false } : msg
        )
      );
    } catch (error) {
      setMessages((prev) =>
        prev.map((msg) =>
          msg.id === assistantMessageId
            ? {
              ...msg,
              content: `Error: ${error instanceof Error ? error.message : "Unknown error"}`,
              isStreaming: false,
            }
            : msg
        )
      );
    } finally {
      setIsLoading(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  };

  const handleAgentChange = (agentId: string) => {
    setSelectedAgentId(agentId);
    setMessages([]);
  };

  if (connectionError) {
    return (
      <div className="h-full flex flex-col bg-[var(--color-bg-primary)]">
        <ConnectionError onRetry={fetchAgents} />
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-[var(--color-bg-primary)]">
      {/* Header */}
      <header className="shrink-0 px-6 py-4 border-b border-[var(--color-border)] bg-[var(--color-bg-secondary)]/50 backdrop-blur-sm relative z-10">
        <div className="max-w-4xl mx-auto flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-[var(--color-accent)] to-purple-500 flex items-center justify-center">
              <Sparkles className="w-5 h-5 text-white" />
            </div>
            <div>
              <h1 className="text-lg font-semibold tracking-tight">
                Agents Playground
              </h1>
              <p className="text-xs text-[var(--color-text-muted)]">
                Test your AI agents
              </p>
            </div>
          </div>
          <div className="flex items-center gap-4">
            <ViewToggle viewMode={viewMode} onToggle={setViewMode} />
            {agents.length > 0 && (
              <AgentSelector
                agents={agents}
                selectedAgent={selectedAgentId}
                onSelect={handleAgentChange}
              />
            )}
          </div>
        </div>
      </header>

      {viewMode === "config" ? (
        <AgentConfigView config={agentConfig} isLoading={isLoadingConfig} />
      ) : (
        <>
          {/* Messages */}
          <div className="flex-1 overflow-y-auto px-6 py-6">
            <div className="max-w-4xl mx-auto">
              {messages.length === 0 ? (
                <EmptyState />
              ) : (
                <div className="space-y-6">
                  {messages.map((message) => (
                    <ChatMessage key={message.id} message={message} />
                  ))}
                  <div ref={messagesEndRef} />
                </div>
              )}
            </div>
          </div>

          {/* Input */}
          <div className="shrink-0 px-6 py-4 border-t border-[var(--color-border)] bg-[var(--color-bg-secondary)]/50 backdrop-blur-sm">
            <form onSubmit={handleSubmit} className="max-w-4xl mx-auto">
              <div className="relative flex items-end gap-3 p-2 bg-[var(--color-bg-tertiary)] rounded-2xl border border-[var(--color-border)] focus-within:border-[var(--color-accent)] transition-colors">
                <ModelSelector
                  selectedModel={selectedModel}
                  onSelect={setSelectedModel}
                />
                <textarea
                  ref={inputRef}
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  onKeyDown={handleKeyDown}
                  placeholder="Send a message..."
                  rows={1}
                  className="flex-1 bg-transparent px-3 py-2 text-[var(--color-text-primary)] placeholder:text-[var(--color-text-muted)] resize-none outline-none text-sm min-h-[40px] max-h-[200px]"
                  style={{ height: "40px" }}
                  onInput={(e) => {
                    const target = e.target as HTMLTextAreaElement;
                    target.style.height = "40px";
                    target.style.height = `${Math.min(target.scrollHeight, 200)}px`;
                  }}
                  disabled={isLoading}
                />
                <button
                  type="submit"
                  disabled={!input.trim() || isLoading}
                  className="shrink-0 w-10 h-10 rounded-xl bg-gradient-to-br from-[var(--color-accent)] to-purple-600 flex items-center justify-center text-white disabled:opacity-50 disabled:cursor-not-allowed hover:shadow-lg hover:shadow-purple-500/25 transition-all duration-200"
                >
                  {isLoading ? (
                    <Loader2 className="w-4 h-4 animate-spin" />
                  ) : (
                    <Send className="w-4 h-4" />
                  )}
                </button>
              </div>
              <p className="text-center text-xs text-[var(--color-text-muted)] mt-3">
                Press Enter to send, Shift+Enter for new line
              </p>
            </form>
          </div>
        </>
      )}
    </div>
  );
}
