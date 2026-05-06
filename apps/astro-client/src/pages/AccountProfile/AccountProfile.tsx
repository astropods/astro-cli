import type { MetaFunction } from "react-router";
import type { Route } from "./+types/AccountProfile";
import { createServerApi } from "@/lib/api.server";
import { IndividualProfile } from "./IndividualProfile";
import { OrgProfile } from "./OrgProfile";

export const meta: MetaFunction = ({ params }) => [
  { title: `${params.account} | Astro` },
];

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const accountName = params.account ?? "";
  if (!accountName) return { account: null, blueprints: null, orgs: null, deployments: null };

  const [account, blueprints, orgs] = await Promise.all([
    api.getAccount(accountName).catch(() => null),
    api.listAccountBlueprints(accountName).catch(() => null),
    api.getAccountOrgs(accountName).catch(() => null),
  ]);
  const deployments = await api.listDeployments(accountName).catch(() => null);

  return { account, blueprints, orgs, deployments };
}

export default function AccountProfile({ loaderData }: Route.ComponentProps) {
  if (!loaderData.account) {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <p className="text-muted-foreground">Account not found</p>
      </div>
    );
  }

  if (loaderData.account.type === "organization") {
    return <OrgProfile loaderData={loaderData} />;
  }

  return <IndividualProfile loaderData={loaderData} />;
}
