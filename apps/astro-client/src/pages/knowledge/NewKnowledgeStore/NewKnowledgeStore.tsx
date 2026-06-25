import { useState } from "react";
import type { Route } from "./+types/NewKnowledgeStore";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useAuth } from "@/lib/auth";
import { knowledgePath, accountProfilePath } from "@/lib/routes";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { UserAvatar } from "@/components/UserAvatar";
import { PROVIDER_LABELS } from "@/components/knowledge/knowledge-utils";
import type { KnowledgeProvider } from "@/lib/api";
import { ProviderList } from "./ProviderList";
import { ConfigureForm } from "./ConfigureForm";

export const meta: Route.MetaFunction = () => [{ title: "Add Store | Knowledge Stores | Astro" }];

function NewKnowledgeStoreContent() {
  const { activeAccount: account } = useActiveAccount();
  const { accounts } = useAuth();
  const avatarUrl = accounts.find((a) => a.name === account)?.avatar_url;
  const [provider, setProvider] = useState<KnowledgeProvider | null>(null);

  return (
    <div className="flex-1 bg-background">
      <PageBreadcrumb
        items={[
          { label: "Knowledge Stores", to: knowledgePath },
          { label: account, to: accountProfilePath(account) },
          { label: provider ? `Add store / ${PROVIDER_LABELS[provider]}` : "Add store" },
        ]}
        mobileItems={[
          {
            label: (
              <span className="inline-flex items-center gap-2">
                <UserAvatar handle={account} name={account} avatarUrl={avatarUrl} className="size-5" />
                {account}
              </span>
            ),
            to: accountProfilePath(account),
          },
        ]}
      />

      <div className="px-6 py-8">
        {provider === null ? (
          <ProviderList onSelect={setProvider} />
        ) : (
          <ConfigureForm provider={provider} account={account} />
        )}
      </div>
    </div>
  );
}

export default function NewKnowledgeStore() {
  return <NewKnowledgeStoreContent />;
}
