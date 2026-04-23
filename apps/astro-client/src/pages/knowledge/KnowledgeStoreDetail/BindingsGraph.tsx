import { Bot } from "lucide-react";
import type { BoundAgent, KnowledgeProvider } from "@/lib/api";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";

const WIDTH = 600;
const HEIGHT = 280;
const CENTER_X = WIDTH / 2;
const CENTER_Y = HEIGHT / 2;
const RADIUS = 110;

interface Props {
  storeName: string;
  provider: KnowledgeProvider;
  agents: BoundAgent[];
}

export function BindingsGraph({ storeName, provider, agents }: Props) {
  const count = agents.length;

  // Position agents in an arc above and around the center node.
  const agentNodes = agents.map((agent, i) => {
    // Spread evenly across the top arc (-π to 0 for 1 node at top, full arc for many)
    const spread = Math.min(Math.PI * 0.8, count > 1 ? Math.PI * 0.7 : 0);
    const startAngle = -Math.PI / 2 - spread / 2;
    const angle = count === 1 ? -Math.PI / 2 : startAngle + (spread / (count - 1)) * i;
    const x = CENTER_X + Math.cos(angle) * RADIUS;
    const y = CENTER_Y + Math.sin(angle) * RADIUS;
    return { ...agent, x, y, angle };
  });

  return (
    <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} className="w-full h-auto" role="img" aria-label="Knowledge store bindings graph">
      <defs>
        <marker id="arrowhead" markerWidth="6" markerHeight="4" refX="6" refY="2" orient="auto">
          <polygon points="0 0, 6 2, 0 4" className="fill-border" />
        </marker>
      </defs>

      {/* Connection lines */}
      {agentNodes.map((agent) => {
        // Shorten line so it doesn't overlap the nodes
        const dx = agent.x - CENTER_X;
        const dy = agent.y - CENTER_Y;
        const dist = Math.sqrt(dx * dx + dy * dy);
        const ux = dx / dist;
        const uy = dy / dist;
        // Start from edge of center node (28px radius), end before agent node (24px radius)
        const x1 = CENTER_X + ux * 30;
        const y1 = CENTER_Y + uy * 30;
        const x2 = agent.x - ux * 26;
        const y2 = agent.y - uy * 26;
        // Midpoint for label
        const mx = (x1 + x2) / 2;
        const my = (y1 + y2) / 2;
        return (
          <g key={agent.deployment_id}>
            <line
              x1={x1} y1={y1} x2={x2} y2={y2}
              className="stroke-border"
              strokeWidth={1.5}
              strokeDasharray="4 3"
              markerEnd="url(#arrowhead)"
            />
            <text x={mx} y={my - 6} textAnchor="middle" className="fill-muted-foreground text-[9px] font-mono">
              {agent.knowledge_name}
            </text>
          </g>
        );
      })}

      {/* Center node: knowledge store */}
      <g>
        <circle cx={CENTER_X} cy={CENTER_Y} r={28} className="fill-white stroke-border" strokeWidth={1.5} />
        <foreignObject x={CENTER_X - 10} y={CENTER_Y - 10} width={20} height={20}>
          <div className="flex items-center justify-center w-full h-full">
            <ProviderIcon provider={provider} className="size-4" />
          </div>
        </foreignObject>
        <text x={CENTER_X} y={CENTER_Y + 44} textAnchor="middle" className="fill-foreground text-[11px] font-medium">
          {storeName}
        </text>
        <text x={CENTER_X} y={CENTER_Y + 56} textAnchor="middle" className="fill-muted-foreground text-[9px] font-mono">
          {provider}
        </text>
      </g>

      {/* Agent nodes */}
      {agentNodes.map((agent) => (
        <g key={agent.deployment_id}>
          <circle cx={agent.x} cy={agent.y} r={22} className="fill-white stroke-border" strokeWidth={1.5} />
          <foreignObject x={agent.x - 8} y={agent.y - 8} width={16} height={16}>
            <div className="flex items-center justify-center w-full h-full">
              <Bot className="size-3.5 text-muted-foreground" />
            </div>
          </foreignObject>
          <text x={agent.x} y={agent.y + 34} textAnchor="middle" className="fill-foreground text-[10px] font-medium">
            {agent.display_name || agent.agent_name}
          </text>
        </g>
      ))}

      {/* Empty state */}
      {count === 0 && (
        <g>
          <circle cx={CENTER_X} cy={CENTER_Y} r={28} className="fill-white stroke-border" strokeWidth={1.5} />
          <foreignObject x={CENTER_X - 10} y={CENTER_Y - 10} width={20} height={20}>
            <div className="flex items-center justify-center w-full h-full">
              <ProviderIcon provider={provider} className="size-4" />
            </div>
          </foreignObject>
          <text x={CENTER_X} y={CENTER_Y + 44} textAnchor="middle" className="fill-foreground text-[11px] font-medium">
            {storeName}
          </text>
          <text x={CENTER_X} y={CENTER_Y + 60} textAnchor="middle" className="fill-muted-foreground text-[10px]">
            No agents bound
          </text>
        </g>
      )}
    </svg>
  );
}
