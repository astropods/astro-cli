import { Bot } from "lucide-react";
import type { BoundAgent, KnowledgeProvider } from "@/lib/api";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";

// Chip-matched dimensions: h-7 (28px) with compact horizontal padding
const CARD_H = 28;
const CARD_W = 120;
const GAP_X = 60;
const AGENT_GAP_Y = 8;
const PAD_X = 4;
const PAD_Y = 4;

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

  // Semantic colors via CSS vars — works in both light and dark mode
  const borderColor = "var(--border)";
  const mutedText = "var(--muted-foreground)";
  const fgText = "var(--foreground)";
  const cardBg = "var(--color-white, #fff)";

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="h-auto max-w-[400px] mx-auto" role="img" aria-label="Knowledge store bindings graph">
      <defs>
        <marker id="bind-arrow" markerWidth="4" markerHeight="3" refX="4" refY="1.5" orient="auto">
          <polygon points="0 0, 4 1.5, 0 3" fill={borderColor} />
        </marker>
      </defs>

      {/* Connection lines */}
      {agentNodes.map((agent) => {
        const x1 = storeX + CARD_W / 2;
        const x2 = agent.x - CARD_W / 2;
        const mx = (x1 + x2) / 2;
        return (
          <g key={agent.deployment_id}>
            <path
              d={`M ${x1} ${storeY} C ${mx} ${storeY}, ${mx} ${agent.y}, ${x2} ${agent.y}`}
              fill="none"
              stroke={borderColor}
              strokeWidth={1}
              strokeDasharray="3 2"
              markerEnd="url(#bind-arrow)"
            />
            <text
              x={mx} y={(storeY + agent.y) / 2 - 4}
              textAnchor="middle"
              fill={mutedText}
              fontSize={9} fontFamily="var(--font-mono, monospace)" letterSpacing="0.04em"
            >
              binding
            </text>
          </g>
        );
      })}

      {/* Store card (left) — matches Chip styling */}
      <g>
        <rect
          x={storeX - CARD_W / 2} y={storeY - CARD_H / 2}
          width={CARD_W} height={CARD_H} rx={2}
          fill={cardBg} stroke={borderColor} strokeWidth={1}
        />
        <foreignObject x={storeX - CARD_W / 2 + 8} y={storeY - 6} width={12} height={12}>
          <div className="flex items-center justify-center w-full h-full">
            <ProviderIcon provider={provider} className="size-3" />
          </div>
        </foreignObject>
        <text
          x={storeX - CARD_W / 2 + 24} y={storeY + 4}
          fill={fgText} fontSize={12} fontWeight={500}
        >
          {storeName}
        </text>
      </g>

      {/* Agent cards (right) — matches Chip styling */}
      {agentNodes.map((agent) => (
        <g key={agent.deployment_id}>
          <rect
            x={agent.x - CARD_W / 2} y={agent.y - CARD_H / 2}
            width={CARD_W} height={CARD_H} rx={2}
            fill={cardBg} stroke={borderColor} strokeWidth={1}
          />
          <foreignObject x={agent.x - CARD_W / 2 + 8} y={agent.y - 5.5} width={11} height={11}>
            <div className="flex items-center justify-center w-full h-full">
              <Bot className="size-[11px] text-muted-foreground" />
            </div>
          </foreignObject>
          <text
            x={agent.x - CARD_W / 2 + 24} y={agent.y + 4}
            fill={fgText} fontSize={12}
          >
            {agent.display_name || agent.agent_name}
          </text>
        </g>
      ))}

      {/* Empty state */}
      {agents.length === 0 && (
        <text x={width / 2} y={storeY + CARD_H / 2 + 14} textAnchor="middle" fill={mutedText} fontSize={11}>
          No agents bound
        </text>
      )}
    </svg>
  );
}
