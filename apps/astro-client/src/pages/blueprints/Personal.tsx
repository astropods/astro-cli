import { Navigate } from "react-router";
import type { Route } from "./+types/Personal";
import { AccountBlueprintsList } from "@/components/browse/AccountBlueprintsList";
import { createServerApi } from "@/lib/api.server";
import { useAuth } from "@/lib/auth";
import { blueprintsPaths } from "@/lib/routes";

export async function loader({ request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const profile = await api.getProfile().catch(() => null);
  const personalAccount = profile?.accounts?.find((a) => a.type === "personal");
  if (!personalAccount) return { accountName: null, blueprintsData: null };
  const blueprintsData = await api.listAccountBlueprints(personalAccount.name).catch(() => null);
  return { accountName: personalAccount.name, blueprintsData };
}

export default function Personal({ loaderData }: Route.ComponentProps) {
  const { isAuthenticated } = useAuth();
  const accountName = loaderData?.accountName;

  if (!isAuthenticated || !accountName) {
    return <Navigate to={blueprintsPaths.discover} replace />;
  }

  return (
    <>
      <h1 className="text-heading-1 text-foreground">Personal blueprints</h1>

      <AccountBlueprintsList
        account={accountName}
        initialData={loaderData?.blueprintsData ?? undefined}
      />
    </>
  );
}
