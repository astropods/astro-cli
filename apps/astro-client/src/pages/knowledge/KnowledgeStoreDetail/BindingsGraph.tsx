import { Bot } from "lucide-react";
import type { BoundAgent, KnowledgeProvider } from "@/lib/api";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";

const NODE_W = 48;
const NODE_H = 48;
const GAP_X = 120; // horizontal gap between store and agents column
const AGENT_GAP_Y = 64; // vertical spacing between agent nodes
const PAD_X = 80;
const PAD_Y = 40;

interface Props {
  storeName: string;
  provider: KnowledgeProvider;
  agents: BoundAgent[];
}

export function BindingsGraph({ storeName, provider, agents }: Props) {
  const count = agents.length || 1; // at least 1 row for layout
  const agentsHeight = count * NODE_H + (count - 1) * (AGENT_GAP_Y - NODE_H);
  const height = Math.max(agentsHeight, NODE_H) + PAD_Y * 2;
  const storeX = PAD_X;
  const storeY = height / 2;
  const agentsX = PAD_X + NODE_W / 2 + GAP_X + NODE_W / 2;
  const width = agentsX + PAD_X + NODE_W / 2;

  // Vertical positions for agent nodes, centered
  const agentNodes = agents.map((agent, i) => {
    const totalH = count * NODE_H + (count - 1) * (AGENT_GAP_Y - NODE_H);
    const startY = height / 2 - totalH / 2 + NODE_H / 2;
    const y = startY + i * AGENT_GAP_Y;
    return { ...agent, x: agentsX, y };
  });

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-auto max-h-[320px]" role="img" aria-label="Knowledge store bindings graph">
      <defs>
        <marker id="bind-arrow" markerWidth="6" markerHeight="4" refX="6" refY="2" orient="auto">
          <polygon points="0 0, 6 2, 0 4" className="fill-border" />
        </marker>
      </defs>

      {/* Connection lines: store → each agent */}
      {agentNodes.map((agent) => {
        const x1 = storeX + NODE_W / 2 + 2;
        const y1 = storeY;
        const x2 = agent.x - NODE_W / 2 - 2;
        const y2 = agent.y;
        const mx = (x1 + x2) / 2;
        return (
          <g key={agent.deployment_id}>
            <path
              d={`M ${x1} ${y1} C ${mx} ${y1}, ${mx} ${y2}, ${x2} ${y2}`}
              fill="none"
              className="stroke-border"
              strokeWidth={1.5}
              strokeDasharray="4 3"
              markerEnd="url(#bind-arrow)"
            />
            <text x={mx} y={Math.min(y1, y2) + (Math.abs(y2 - y1) / 2) - 6} textAnchor="middle" className="fill-muted-foreground text-[9px] font-mono">
              {agent.knowledge_name}
            </text>
          </g>
        );
      })}

      {/* Store node (left) */}
      <g>
        <rect
          x={storeX - NODE_W / 2} y={storeY - NODE_H / 2}
          width={NODE_W} height={NODE_H} rx={10}
          className="fill-white stroke-border" strokeWidth={1.5}
        />
        <foreignObject x={storeX - 10} y={storeY - 10} width={20} height={20}>
          <div className="flex items-center justify-center w-full h-full">
            <ProviderIcon provider={provider} className="size-4" />
          </div>
        </foreignObject>
        <text x={storeX} y={storeY + NODE_H / 2 + 16} textAnchor="middle" className="fill-foreground text-[11px] font-medium">
          {storeName}
        </text>
        <text x={storeX} y={storeY + NODE_H / 2 + 28} textAnchor="middle" className="fill-muted-foreground text-[9px] font-mono">
          {provider}
        </text>
      </g>

      {/* Agent nodes (right) */}
      {agentNodes.map((agent) => (
        <g key={agent.deployment_id}>
          <rect
            x={agent.x - NODE_W / 2} y={agent.y - NODE_H / 2}
            width={NODE_W} height={NODE_H} rx={10}
            className="fill-white stroke-border" strokeWidth={1.5}
          />
          <foreignObject x={agent.x - 8} y={agent.y - 8} width={16} height={16}>
            <div className="flex items-center justify-center w-full h-full">
              <Bot className="size-3.5 text-muted-foreground" />
            </div>
          </foreignObject>
          <text x={agent.x} y={agent.y + NODE_H / 2 + 16} textAnchor="middle" className="fill-foreground text-[10px] font-medium">
            {agent.display_name || agent.agent_name}
          </text>
        </g>
      ))}

      {/* Empty state: just the store node centered */}
      {agents.length === 0 && (
        <text x={storeX} y={storeY + NODE_H / 2 + 42} textAnchor="middle" className="fill-muted-foreground text-[10px]">
          No agents bound
        </text>
      )}
    </svg>
  );
}
