import { Navigate } from "react-router";
import type { Route } from "./+types/Personal";
import { AccountAgentsList } from "@/components/browse/AccountAgentsList";
import { createServerApi } from "@/lib/api.server";
import { useAuth } from "@/lib/auth";
import { blueprintsPaths } from "@/lib/routes";

export async function loader({ request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const profile = await api.getProfile().catch(() => null);
  const personalAccount = profile?.accounts?.find((a) => a.type === "personal");
  if (!personalAccount) return { accountName: null, agentsData: null };
  const agentsData = await api.listAccountAgents(personalAccount.name).catch(() => null);
  return { accountName: personalAccount.name, agentsData };
}

export default function Personal({ loaderData }: Route.ComponentProps) {
  const { isAuthenticated } = useAuth();
  const accountName = loaderData?.accountName;

  if (!isAuthenticated || !accountName) {
    return <Navigate to={blueprintsPaths.discover} replace />;
  }

  return (
    <>
      <h1 className="text-heading-1 text-foreground">Personal</h1>
      <AccountAgentsList
        account={accountName}
        initialData={loaderData?.agentsData ?? undefined}
      />
    </>
  );
}
