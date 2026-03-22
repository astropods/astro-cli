import { Navigate, useParams } from "react-router";
import type { Route } from "./+types/AccountBlueprints";
import { AccountAgentsList } from "@/components/browse/AccountAgentsList";
import { createServerApi } from "@/lib/api.server";
import { blueprintsPaths } from "@/lib/routes";

export async function loader({ params, request }: Route.LoaderArgs) {
  const account = params.account ?? "";
  if (!account) return { agentsData: null };
  const api = createServerApi(request);
  const agentsData = await api.listAccountAgents(account).catch(() => null);
  return { agentsData };
}

export default function AccountBlueprints({ loaderData }: Route.ComponentProps) {
  const { account } = useParams<{ account: string }>();

  if (!account) {
    return <Navigate to={blueprintsPaths.discover} replace />;
  }

  return (
    <>
      <h1 className="text-heading-1 text-foreground">{account}</h1>
      <AccountAgentsList
        account={account}
        initialData={loaderData?.agentsData ?? undefined}
      />
    </>
  );
}
