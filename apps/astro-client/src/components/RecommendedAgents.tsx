import { AgentCard, type AgentCardProps } from "@/components/AgentCard";

export type RecommendedAgent = AgentCardProps;

export interface RecommendedAgentsProps {
  agents: RecommendedAgent[];
}

export function RecommendedAgents({ agents }: RecommendedAgentsProps) {
  if (agents.length === 0) return null;

  return (
    <section className="@container mt-8 pt-8 border-t border-border">
      <h2 className="text-lg font-semibold mb-4">Recommended Agents</h2>
      <div className="grid grid-cols-1 @[480px]:grid-cols-2 gap-4">
        {agents.map((agent) => (
          <AgentCard key={agent.slug} {...agent} />
        ))}
      </div>
    </section>
  );
}
