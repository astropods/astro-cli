import { AgentMascotsIllustration } from "@/components/ui/svgs/agentMascotsIllustration";

/** Animated trio of agent mascots — float + wink, staggered delays */
export function AgentMascots({ size = 52 }: { size?: number }) {
  return (
    <div className="flex items-center justify-center py-1">
      <AgentMascotsIllustration size={size} />
    </div>
  );
}
