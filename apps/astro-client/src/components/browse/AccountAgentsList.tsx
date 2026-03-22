import { useAccountAgents } from "@/api/queries";
import { AgentListView } from "@/components/browse/AgentListView";
import type { AgentsListResponse } from "@/lib/api";

export function AccountAgentsList({ account, initialData }: { account: string; initialData?: AgentsListResponse }) {
  const { data, isLoading, isError, error, refetch } = useAccountAgents(account, { initialData });

  return (
    <AgentListView
      agents={data?.agents ?? []}
      isLoading={isLoading}
      isError={isError}
      error={error}
      refetch={refetch}
      emptyTitle="No blueprints yet"
      emptyDescription="This account has no blueprints in the registry."
    />
  );
}
