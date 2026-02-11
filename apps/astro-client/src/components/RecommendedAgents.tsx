import { useOutletContext } from "react-router-dom";
import { AgentCard } from "@/components/AgentCard";
import type { LayoutContext } from "@/components/Layout";

export interface RecommendedAgent {
  slug: string;
  name: string;
  description: string;
  integrations: string[];
  categories: string[];
}

export interface RecommendedAgentsProps {
  agents: RecommendedAgent[];
}

export function RecommendedAgents({ agents }: RecommendedAgentsProps) {
  const { openAuthModal } = useOutletContext<LayoutContext>();

  if (agents.length === 0) return null;

  return (
    <section className="@container">
      <h2 className="text-lg font-semibold mb-4">Recommended Agents</h2>
      <div className="grid grid-cols-1 @[480px]:grid-cols-2 gap-4">
        {agents.map((agent) => (
          <AgentCard
            key={agent.slug}
            slug={agent.slug}
            name={agent.name}
            description={agent.description}
            integrations={agent.integrations}
            categories={agent.categories}
            onInstall={() => openAuthModal()}
          />
        ))}
      </div>
    </section>
  );
}
