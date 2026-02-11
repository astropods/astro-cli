import { useParams, useOutletContext, Link } from "react-router-dom";
import { ArrowRight, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/Badge";
import { AgentPreviewPanel } from "@/components/AgentPreviewPanel";
import { RecommendedAgents } from "@/components/RecommendedAgents";
import { integrationIconMap } from "@/lib/integrationIcons";
import { useAgent, useAgents } from "@/api/queries";
import type { Agent } from "@/lib/api";
import type { LayoutContext } from "@/components/Layout";

// ---------------------------------------------------------------------------
// Helpers (mirrors Hire page utilities)
// ---------------------------------------------------------------------------

function getLatestSpec(agent: Agent) {
  return agent.versions[0]?.spec;
}

function getAgentDescription(agent: Agent): string {
  return getLatestSpec(agent)?.meta?.description ?? agent.name;
}

function getAgentIntegrations(agent: Agent): string[] {
  const tools = getLatestSpec(agent)?.integrations?.tools;
  if (!tools) return [];
  return [...new Set(tools.map((t) => t.provider))];
}

function getAgentCategories(agent: Agent): string[] {
  return getLatestSpec(agent)?.meta?.tags ?? [];
}

// ---------------------------------------------------------------------------
// Loading skeleton
// ---------------------------------------------------------------------------

function AgentDetailSkeleton() {
  return (
    <div className="flex flex-1 min-h-0 lg:-m-8 lg:p-0">
      <div className="flex-1 min-w-0 md:overflow-y-auto">
        <div className="lg:p-8 max-w-4xl">
          {/* Title */}
          <Skeleton className="h-7 w-64 mb-3" />
          {/* Description */}
          <div className="space-y-2 mb-5">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
          </div>
          {/* CTA button */}
          <div className="mb-6">
            <Skeleton className="h-9 w-44 rounded-md" />
          </div>
          {/* Apps Used */}
          <div className="mb-6">
            <Skeleton className="h-3 w-16 mb-3" />
            <div className="flex gap-2">
              <Skeleton className="h-9 w-32 rounded-lg" />
              <Skeleton className="h-9 w-24 rounded-lg" />
              <Skeleton className="h-9 w-28 rounded-lg" />
            </div>
          </div>
          {/* Safety & Permissions */}
          <div className="mb-8">
            <Skeleton className="h-3 w-32 mb-3" />
            <div className="space-y-2">
              <Skeleton className="h-4 w-56" />
              <Skeleton className="h-4 w-48" />
              <Skeleton className="h-4 w-52" />
            </div>
          </div>
          {/* Recommended Agents */}
          <div>
            <Skeleton className="h-6 w-48 mb-4" />
            <div className="grid grid-cols-1 @[480px]:grid-cols-2 gap-4">
              <Skeleton className="h-44 rounded-lg" />
              <Skeleton className="h-44 rounded-lg" />
            </div>
          </div>
        </div>
      </div>
      {/* Right panel */}
      <div className="hidden lg:flex w-[400px] shrink-0 flex-col border-l border-border bg-muted/50 ml-auto">
        {/* Header */}
        <div className="flex items-center gap-2 border-b border-border px-4 py-3">
          <Skeleton className="size-4 rounded" />
          <div className="space-y-1">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-3 w-40" />
          </div>
        </div>
        {/* Empty state */}
        <div className="flex flex-1 flex-col items-center justify-center px-4">
          <Skeleton className="size-14 rounded-xl mb-3" />
          <Skeleton className="h-4 w-32 mb-6" />
          <Skeleton className="h-24 w-full rounded-lg" />
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page component
// ---------------------------------------------------------------------------

export function AgentDetail() {
  const { agentSlug } = useParams<{ agentSlug: string }>();
  const { openAuthModal } = useOutletContext<LayoutContext>();

  const { data: agent, isLoading, isError, error } = useAgent(agentSlug ?? "");
  const { data: agentsData } = useAgents();

  const recommendedAgents = (() => {
    if (!agent || !agentsData) return [];
    const currentIntegrations = new Set(getAgentIntegrations(agent));
    const currentCategories = new Set(getAgentCategories(agent));
    return agentsData.agents
      .filter((a) => a.name !== agentSlug)
      .map((a) => {
        const ints = getAgentIntegrations(a);
        const cats = getAgentCategories(a);
        const score =
          ints.filter((i) => currentIntegrations.has(i)).length +
          cats.filter((c) => currentCategories.has(c)).length;
        return { agent: a, integrations: ints, categories: cats, score };
      })
      .sort((a, b) => b.score - a.score)
      .slice(0, 2)
      .map(({ agent: a, integrations, categories }) => ({
        slug: a.name,
        name: a.name,
        description: getAgentDescription(a),
        integrations,
        categories,
      }));
  })();

  if (isLoading) {
    return <AgentDetailSkeleton />;
  }

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Something went wrong</h1>
        <p className="text-muted-foreground text-sm mb-4">
          {error instanceof Error ? error.message : "Failed to load agent details."}
        </p>
        <Button asChild>
          <Link to="/hire">Browse Agents</Link>
        </Button>
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Agent not found</h1>
        <p className="text-muted-foreground text-sm mb-4">
          The agent you're looking for doesn't exist or has been removed.
        </p>
        <Button asChild>
          <Link to="/hire">Browse Agents</Link>
        </Button>
      </div>
    );
  }

  const description = getAgentDescription(agent);
  const integrations = getAgentIntegrations(agent);
  const spec = getLatestSpec(agent);
  const credentials = spec?.integrations?.tools ?? [];
  const safetyPermissions = credentials.map(
    (t) => `Access to ${t.provider}`,
  );

  return (
    <div className="flex flex-1 min-h-0 lg:-m-8 lg:p-0">
      {/* Left scroll area — fills remaining space so scrollbar is flush with sidebar */}
      <div className="flex-1 min-w-0 md:overflow-y-auto">
        <div className="lg:p-8 max-w-4xl">
          <h1 className="text-xl font-semibold mb-3">{agent.name}</h1>

          <p className="text-sm text-muted-foreground leading-relaxed mb-5">
            {description}
          </p>

          <div className="flex items-center gap-3 mb-6">
            <Button onClick={openAuthModal} className="gap-2">
              Hire this agent
              <ArrowRight className="size-4" />
            </Button>
          </div>

          {/* Apps Used */}
          {integrations.length > 0 && (
            <section className="mb-6">
              <h2 className="text-xs font-medium text-muted-foreground mb-3">
                Apps Used
              </h2>
              <div className="flex flex-wrap gap-2">
                {integrations.map((name) => (
                  <Badge key={name} size="lg" icon={integrationIconMap[name]}>
                    {name}
                  </Badge>
                ))}
              </div>
            </section>
          )}

          {/* Safety & Permissions */}
          {safetyPermissions.length > 0 && (
            <section className="mb-8">
              <h2 className="text-xs font-medium text-muted-foreground mb-3">
                Safety & Permissions
              </h2>
              <ul className="space-y-2">
                {safetyPermissions.map((permission, i) => (
                  <li key={i} className="flex items-start gap-2 text-sm">
                    <ShieldCheck className="size-4 shrink-0 text-muted-foreground mt-0.5" />
                    <span>{permission}</span>
                  </li>
                ))}
              </ul>
            </section>
          )}

          <RecommendedAgents agents={recommendedAgents} />
        </div>
      </div>

      <AgentPreviewPanel suggestedPrompts={[]} />
    </div>
  );
}
