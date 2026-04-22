import { useState } from "react";
import { Link } from "react-router";
import type { Route } from "./+types/NewKnowledgeStore";
import { ArrowLeftIcon } from "@heroicons/react/24/outline";
import { useAuth } from "@/lib/auth";
import { useDefaultAccount } from "@/hooks/use-default-account";
import { knowledgePath } from "@/lib/routes";
import type { KnowledgeProvider } from "@/lib/api";
import { ProviderList } from "./ProviderList";
import { ConfigureForm } from "./ConfigureForm";

export const meta: Route.MetaFunction = () => [{ title: "Add Store | Knowledge Stores | Astro" }];

function NewKnowledgeStoreContent() {
  const { personalAccount } = useAuth();
  const { validStoredDefault } = useDefaultAccount();
  const [provider, setProvider] = useState<KnowledgeProvider | null>(null);

  const account = validStoredDefault || personalAccount?.name || "";

  return (
    <div className="flex-1 bg-surface">
      <div className="border-b border-border bg-surface px-6 py-3">
        <div className="flex items-center gap-2 text-body-sm">
          <Link
            to={knowledgePath}
            className="flex items-center gap-2 text-muted-foreground hover:text-foreground transition-colors"
          >
            <ArrowLeftIcon className="size-4" />
            Knowledge Stores
          </Link>
          <span className="text-muted-foreground">/</span>
          <span className="font-medium text-foreground">Add store</span>
        </div>
      </div>

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
