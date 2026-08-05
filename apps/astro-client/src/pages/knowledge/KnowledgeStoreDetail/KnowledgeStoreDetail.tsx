import { useState } from "react";
import { useParams, Link, useSearchParams } from "react-router";
import { BookOpen, ChevronRight } from "lucide-react";
import type { Route } from "./+types/KnowledgeStoreDetail";
import {
  Cog6ToothIcon,
} from "@heroicons/react/24/outline";
import { CopyButton } from "@/components/ui/copy-button";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/StatusBadge";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useKnowledgeStore } from "@/api/queries/knowledge";
import {
  statusToColor,
  isTransitionalStatus,
  statusLabel,
  PROVIDER_LABELS,
  displayProvider,
} from "@/components/knowledge/knowledge-utils";
import { knowledgePath, accountProfilePath } from "@/lib/routes";
import { cn } from "@/lib/utils";
import { resolvePageAccount } from "@/lib/user-resource-scope";
import { Chip } from "./Chip";
import { OverviewTab } from "./OverviewTab";
import { SettingsPanel } from "./SettingsPanel";

export const meta: Route.MetaFunction = () => [{ title: "Knowledge Store | Astro" }];

type Tab = "overview" | "settings";

function KnowledgeStoreDetailContent() {
  const { storeName } = useParams();
  const { accounts, isAuthenticated } = useAuth();
  const { activeAccount } = useActiveAccount();
  const [searchParams] = useSearchParams();
  const account = resolvePageAccount(
    searchParams.get("account"),
    accounts.map((membership) => membership.name),
    activeAccount,
  );

  const { data: store, isLoading } = useKnowledgeStore(account, storeName ?? "", isAuthenticated && !!storeName);
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { key: Tab; label: string; hidden?: boolean; icon: React.ReactNode }[] = [
    { key: "overview", label: "Overview", icon: <BookOpen className="size-3.5 shrink-0" /> },
    { key: "settings", label: "Settings", icon: <Cog6ToothIcon className="size-3.5 shrink-0" /> },
  ];

  if (isLoading && !store) {
    return null;
  }

  if (!store) {
    return (
      <div className="flex-1 bg-background">
        <div className="px-8 py-6">
          <p className="text-body-sm text-muted-foreground">Knowledge store not found.</p>
          <Button asChild variant="outline" className="mt-4">
            <Link to={knowledgePath}>Back to stores</Link>
          </Button>
        </div>
      </div>
    );
  }

  const fullValue = store.arn;
  const isArn = true;
  const display = isArn && fullValue ? `…${fullValue.split(":").pop() ?? fullValue}` : fullValue;

  return (
    <div className="flex flex-1 min-h-0 overflow-hidden relative bg-background">
      <div className="flex flex-1 flex-col min-w-0 min-h-0">

        <div className="bg-background border-b border-border shrink-0 pt-6">
          <div className="px-8 pb-7">
            <nav className="mb-4 flex items-center gap-1.5 font-mono text-mono-sm text-muted-foreground" aria-label="Breadcrumb">
              <Link to={knowledgePath} className="hover:text-foreground transition-colors">Knowledge</Link>
              <ChevronRight className="size-3.5 shrink-0 text-faint-foreground" />
              <Link to={accountProfilePath(account)} className="hover:text-foreground transition-colors">{account}</Link>
              <ChevronRight className="size-3.5 shrink-0 text-faint-foreground" />
              <span className="text-foreground truncate">{store.name}</span>
            </nav>

            <div className="flex items-center justify-between gap-x-3 gap-y-4 flex-wrap">
              <div className="flex items-center gap-x-2.5 gap-y-1.5 flex-wrap min-w-0">
                <h1 className="text-heading-1 text-foreground">{store.name}</h1>
                <StatusBadge color={statusToColor(store.status)} spinning={isTransitionalStatus(store.status)}>
                  {statusLabel(store.status)}
                </StatusBadge>
              </div>
              <div className="flex items-center gap-3 flex-wrap">
                <Chip>
                  <ProviderIcon provider={displayProvider(store)} className="size-3.5 shrink-0" />
                  {PROVIDER_LABELS[displayProvider(store)] ?? store.provider}
                  <span className="text-faint-foreground">·</span>
                  {store.mode === "managed" ? "Managed" : "External"}
                </Chip>
                {fullValue && (
                  <span className="inline-flex items-stretch h-7 max-w-full rounded-sm border border-border overflow-hidden">
                    <span className="inline-flex items-center px-2 font-sans text-label uppercase tracking-wide text-faint-foreground shrink-0">
                      {isArn ? "ARN" : "URL"}
                    </span>
                    <span className="w-px bg-border shrink-0" />
                    <span className="inline-flex items-center px-2 font-sans text-body-sm text-foreground min-w-0 truncate">
                      {display}
                    </span>
                    <CopyButton
                      copyText={fullValue}
                      className="h-7 w-7 rounded-none border-0 border-l border-border bg-transparent shadow-none hover:bg-muted shrink-0"
                      iconClassName="size-3.5"
                    />
                  </span>
                )}
              </div>
            </div>
          </div>

          <div className="flex px-8">
            {tabs.filter((t) => !t.hidden).map((t) => (
              <button
                key={t.key}
                type="button"
                onClick={() => setTab(t.key)}
                className={cn(
                  "flex items-center gap-1.5 bg-transparent border-0 font-sans text-heading-4 py-[11px] px-4 border-b-2 transition-colors duration-150",
                  t.key === tabs.filter((x) => !x.hidden)[0]?.key && "pl-0",
                  tab === t.key
                    ? "cursor-pointer font-medium text-foreground border-b-[var(--primary)]"
                    : "cursor-pointer font-normal text-faint-foreground border-b-transparent",
                )}
              >
                {t.icon}
                {t.label}
              </button>
            ))}
          </div>
        </div>

        <div className="dp-scroll flex-1 min-h-0 overflow-y-auto py-6 px-8">
          {tab === "overview" && <OverviewTab store={store} account={account} />}
          {tab === "settings" && <SettingsPanel store={store} account={account} />}
        </div>

      </div>
    </div>
  );
}

export default function KnowledgeStoreDetail() {
  return <KnowledgeStoreDetailContent />;
}
