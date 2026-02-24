import { AgentCard } from "@/components/AgentCard";

export interface RecommendedAgent {
  slug: string;
  account: string;
  name: string;
  description: string;
  categories: string[];
  ownerPictureUrl?: string;
}

export interface RecommendedAgentsProps {
  agents: RecommendedAgent[];
}

export function RecommendedAgents({ agents }: RecommendedAgentsProps) {
  if (agents.length === 0) return null;

  return (
    <section className="@container">
      <h2 className="text-lg font-semibold mb-4">Recommended Agents</h2>
      <div className="grid grid-cols-1 @[480px]:grid-cols-2 gap-4">
        {agents.map((agent) => (
          <AgentCard
            key={agent.slug}
            slug={agent.slug}
            account={agent.account}
            name={agent.name}
            description={agent.description}
            categories={agent.categories}
            ownerPictureUrl={agent.ownerPictureUrl}
          />
        ))}
      </div>
    </section>
  );
}
