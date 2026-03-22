import { Loader2 } from "lucide-react";
import { AgentCard } from "@/components/AgentCard";
import { getAgentDescription } from "@/lib/agent-utils";
import type { Agent } from "@/lib/api";

export interface AgentListViewProps {
  agents: Agent[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
  emptyTitle?: string;
  emptyDescription?: string;
}

export function AgentListView({
  agents,
  isLoading,
  isError,
  error,
  refetch,
  emptyTitle = "No blueprints yet",
  emptyDescription = "There are no blueprints in the registry yet.",
}: AgentListViewProps) {
  if (isLoading) {
    return (
      <div role="status" aria-label="Loading agents" className="flex items-center justify-center py-12">
        <Loader2 size={32} className="animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (isError) {
    return (
      <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-red-700">
        <p className="font-medium">Failed to load blueprints</p>
        <p className="text-sm">
          {(error as { error_description?: string })?.error_description ??
            (error instanceof Error ? error.message : "An unexpected error occurred")}
        </p>
        <button
          type="button"
          onClick={() => refetch()}
          className="mt-2 cursor-pointer rounded-md border border-red-300 bg-white px-3 py-1 text-sm text-red-700 hover:bg-red-50"
        >
          Retry
        </button>
      </div>
    );
  }

  if (agents.length === 0) {
    return (
      <div className="rounded-lg border border-border p-8 text-center">
        <h3 className="mb-2 text-lg font-medium">{emptyTitle}</h3>
        <p className="text-sm text-muted-foreground">
          {emptyDescription}{" "}
          <a
            href="https://docs.astropods.ai"
            target="_blank"
            rel="noopener noreferrer"
            className="underline text-primary hover:text-primary/70"
          >
            Learn how to push a blueprint
          </a>
        </p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-3 @[540px]:grid-cols-2 @[900px]:grid-cols-3 content-start">
      {agents.map((agent) => (
        <AgentCard
          key={`${agent.account}/${agent.name}`}
          slug={`${agent.account}/${agent.name}`}
          account={agent.account}
          name={agent.name}
          description={getAgentDescription(agent)}
          visibility={agent.visibility}
          lifetimeMessages={agent.metrics?.lifetime_messages}
        />
      ))}
    </div>
  );
}
