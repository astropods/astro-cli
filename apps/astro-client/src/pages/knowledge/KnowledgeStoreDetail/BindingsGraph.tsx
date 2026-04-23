import { Bot } from "lucide-react";
import type { BoundAgent, KnowledgeProvider } from "@/lib/api";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";

const CARD_W = 110;
const CARD_H = 24;
const GAP_X = 70; // horizontal gap between columns
const AGENT_GAP_Y = 10; // vertical spacing between agent cards
const PAD_X = 12;
const PAD_Y = 16;

interface Props {
  storeName: string;
  provider: KnowledgeProvider;
  agents: BoundAgent[];
}

export function BindingsGraph({ storeName, provider, agents }: Props) {
  const count = Math.max(agents.length, 1);
  const agentsHeight = count * CARD_H + (count - 1) * AGENT_GAP_Y;
  const height = agentsHeight + PAD_Y * 2;
  const storeX = PAD_X + CARD_W / 2;
  const storeY = height / 2;
  const agentsX = PAD_X + CARD_W + GAP_X + CARD_W / 2;
  const width = agentsX + CARD_W / 2 + PAD_X;

  const agentNodes = agents.map((agent, i) => {
    const startY = height / 2 - agentsHeight / 2 + CARD_H / 2;
    const y = startY + i * (CARD_H + AGENT_GAP_Y);
    return { ...agent, x: agentsX, y };
  });

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-auto max-h-[360px]" role="img" aria-label="Knowledge store bindings graph">
      <defs>
        <marker id="bind-arrow" markerWidth="5" markerHeight="3" refX="5" refY="1.5" orient="auto">
          <polygon points="0 0, 5 1.5, 0 3" className="fill-border" />
        </marker>
      </defs>

      {/* Connection lines */}
      {agentNodes.map((agent) => {
        const x1 = storeX + CARD_W / 2;
        const y1 = storeY;
        const x2 = agent.x - CARD_W / 2;
        const y2 = agent.y;
        const mx = (x1 + x2) / 2;
        const labelY = (y1 + y2) / 2 - 5;
        return (
          <g key={agent.deployment_id}>
            <path
              d={`M ${x1} ${y1} C ${mx} ${y1}, ${mx} ${y2}, ${x2} ${y2}`}
              fill="none"
              className="stroke-border"
              strokeWidth={1}
              strokeDasharray="3 2"
              markerEnd="url(#bind-arrow)"
            />
            <text x={mx} y={labelY} textAnchor="middle" className="fill-muted-foreground text-[7px]">
              binding
            </text>
          </g>
        );
      })}

      {/* Store card (left) */}
      <g>
        <rect
          x={storeX - CARD_W / 2} y={storeY - CARD_H / 2}
          width={CARD_W} height={CARD_H} rx={5}
          className="fill-white stroke-border" strokeWidth={1}
        />
        <foreignObject x={storeX - CARD_W / 2 + 7} y={storeY - 6} width={12} height={12}>
          <div className="flex items-center justify-center w-full h-full">
            <ProviderIcon provider={provider} className="size-2.5" />
          </div>
        </foreignObject>
        <text x={storeX - CARD_W / 2 + 24} y={storeY + 3} className="fill-foreground text-[9px] font-medium">
          {storeName}
        </text>
      </g>

      {/* Agent cards (right) */}
      {agentNodes.map((agent) => (
        <g key={agent.deployment_id}>
          <rect
            x={agent.x - CARD_W / 2} y={agent.y - CARD_H / 2}
            width={CARD_W} height={CARD_H} rx={5}
            className="fill-white stroke-border" strokeWidth={1}
          />
          <foreignObject x={agent.x - CARD_W / 2 + 7} y={agent.y - 5} width={10} height={10}>
            <div className="flex items-center justify-center w-full h-full">
              <Bot className="size-2.5 text-muted-foreground" />
            </div>
          </foreignObject>
          <text x={agent.x - CARD_W / 2 + 22} y={agent.y + 3} className="fill-foreground text-[8px] font-medium">
            {agent.display_name || agent.agent_name}
          </text>
        </g>
      ))}

      {/* Empty state */}
      {agents.length === 0 && (
        <text x={width / 2} y={storeY + CARD_H / 2 + 14} textAnchor="middle" className="fill-muted-foreground text-[8px]">
          No agents bound
        </text>
      )}
    </svg>
  );
}
