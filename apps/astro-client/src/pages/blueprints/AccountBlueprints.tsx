import { Navigate, useParams } from "react-router";
import type { Route } from "./+types/AccountBlueprints";
import { AccountBlueprintsList } from "@/components/browse/AccountBlueprintsList";
import { createServerApi } from "@/lib/api.server";
import { blueprintsPaths } from "@/lib/routes";

export const meta: Route.MetaFunction = ({ params }) => [
  { title: `${params.account} Blueprints | Astro` },
];

export async function loader({ params, request }: Route.LoaderArgs) {
  const account = params.account ?? "";
  if (!account) return { blueprintsData: null };
  const api = createServerApi(request);
  const blueprintsData = await api.listAccountBlueprints(account).catch(() => null);
  return { blueprintsData };
}

export default function AccountBlueprints({ loaderData }: Route.ComponentProps) {
  const { account } = useParams<{ account: string }>();

  if (!account) {
    return <Navigate to={blueprintsPaths.discover} replace />;
  }

  return (
    <>
      <h1 className="text-heading-1 text-foreground">{account} blueprints</h1>
      <AccountBlueprintsList
        account={account}
        initialData={loaderData?.blueprintsData ?? undefined}
      />
    </>
  );
}
