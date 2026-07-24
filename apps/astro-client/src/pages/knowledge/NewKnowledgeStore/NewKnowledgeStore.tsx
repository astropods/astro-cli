import { useState } from "react";
import type { Route } from "./+types/NewKnowledgeStore";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useAuth } from "@/lib/auth";
import { knowledgePath, accountProfilePath } from "@/lib/routes";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { UserAvatar } from "@/components/UserAvatar";
import { CreateInAccountPicker } from "@/components/CreateInAccountPicker";
import { PROVIDER_LABELS } from "@/components/knowledge/knowledge-utils";
import type { KnowledgeProvider } from "@/lib/api";
import { ProviderList } from "./ProviderList";
import { ConfigureForm } from "./ConfigureForm";

export const meta: Route.MetaFunction = () => [{ title: "Add Store | Knowledge Stores | Astro" }];

function NewKnowledgeStoreContent() {
  const { activeAccount } = useActiveAccount();
  const { accounts } = useAuth();
  const [selectedAccount, setSelectedAccount] = useState<string | null>(null);
  const account = selectedAccount ?? activeAccount;
  const avatarUrl = accounts.find((a) => a.name === account)?.avatar_url;
  const [provider, setProvider] = useState<KnowledgeProvider | null>(null);

  return (
    <div className="flex-1 bg-background">
      {provider !== null && (
        <PageBreadcrumb
          items={[
            { label: "Knowledge Stores", to: knowledgePath },
            { label: account, to: accountProfilePath(account) },
            { label: `Add store / ${PROVIDER_LABELS[provider]}` },
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
      )}

      <div className="px-6 py-8">
        {provider === null ? (
          <div className="mx-auto max-w-2xl">
            <h1 className="text-heading-1 text-foreground">Add new knowledge store</h1>
            <div className="mt-6 space-y-8">
              <CreateInAccountPicker value={account} onChange={setSelectedAccount} />
              <ProviderList onSelect={setProvider} />
            </div>
          </div>
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
