import { Link } from "react-router";
import type { Route } from "./+types/AgentShare";
import { createServerApi } from "@/lib/api.server";
import type { AccountPublic } from "@/lib/api";

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const agentSlug = params.agentSlug ?? "";
  const origin = new URL(request.url).origin;

  const [agent, accountData] = await Promise.all([
    account && agentSlug ? api.getAgent(account, agentSlug).catch(() => null) : null,
    account ? api.getAccount(account).catch(() => null) : null,
  ]);

  const blueprintUrl = `${origin}/${account}/${agentSlug}`;
  const assetsBase = import.meta.env.VITE_ASSETS_URL?.replace(/\/$/, "");
  const avatarHandle = (accountData as AccountPublic | null)?.name || account;
  const avatarVersion = (accountData as AccountPublic | null)?.avatar_version;
  const ogImage = assetsBase && avatarHandle
    ? `${assetsBase}/avatars/${encodeURIComponent(avatarHandle)}.jpg${avatarVersion ? `?v=${avatarVersion}` : ""}`
    : `${origin}/assets/placeholders/accounts/avatar_01.svg`;

  return { account, agentSlug, agent, blueprintUrl, ogImage };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const account = data?.account ?? "";
  const agentSlug = data?.agentSlug ?? "";
  const agentName = data?.agent?.name ?? agentSlug;
  const title = `Just launched ${agentName} on Astro!`;
  const description = `Check out the blueprint: ${data?.blueprintUrl ?? `/${account}/${agentSlug}`}`;
  const shareURL = data?.blueprintUrl ?? "";

  return [
    { title },
    { name: "description", content: description },
    { property: "og:type", content: "website" },
    ...(shareURL ? [{ property: "og:url", content: shareURL } as const] : []),
    { property: "og:title", content: title },
    { property: "og:description", content: description },
    ...(data?.ogImage ? [{ property: "og:image", content: data.ogImage } as const] : []),
    { name: "twitter:card", content: "summary_large_image" },
    { name: "twitter:title", content: title },
    { name: "twitter:description", content: description },
    ...(data?.ogImage ? [{ name: "twitter:image", content: data.ogImage } as const] : []),
  ];
};

export default function AgentShare({ loaderData }: Route.ComponentProps) {
  return (
    <main className="flex min-h-screen items-center justify-center bg-surface px-6 py-16">
      <div className="mx-auto max-w-xl text-center">
        <h1 className="text-2xl font-semibold text-foreground">
          Just launched {loaderData.agent?.name ?? loaderData.agentSlug} on Astro
        </h1>
        <p className="mt-3 text-muted-foreground">
          Check out the blueprint and deploy your own version.
        </p>
        <Link
          className="mt-6 inline-flex rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:opacity-95"
          to={`/${loaderData.account}/${loaderData.agentSlug}`}
        >
          Open blueprint
        </Link>
      </div>
    </main>
  );
}
