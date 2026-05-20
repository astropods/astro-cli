import { useState } from "react";
import { useParams, Link } from "react-router";
import { Bot, Calendar } from "lucide-react";
import type { Route } from "./+types/KnowledgeStoreDetail";
import {
  Squares2X2Icon,
  QueueListIcon,
  Cog6ToothIcon,
} from "@heroicons/react/24/outline";
import { CopyButton } from "@/components/ui/copy-button";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/StatusBadge";
import { Tag } from "@/components/Tag";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useKnowledgeStore } from "@/api/queries/knowledge";
import {
  statusToColor,
  isTransitionalStatus,
  statusLabel,
  PROVIDER_LABELS,
} from "@/components/knowledge/knowledge-utils";
import { knowledgePath, accountProfilePath } from "@/lib/routes";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { UserAvatar } from "@/components/UserAvatar";
import { cn } from "@/lib/utils";
import { Chip } from "./Chip";
import { OverviewTab } from "./OverviewTab";
import { LogsTab } from "./LogsTab";
import { SettingsPanel } from "./SettingsPanel";

export const meta: Route.MetaFunction = () => [{ title: "Knowledge Store | Astro" }];

type Tab = "overview" | "logs" | "settings";

function KnowledgeStoreDetailContent() {
  const { storeName } = useParams();
  const { isAuthenticated } = useAuth();
  const { activeAccount: account } = useActiveAccount();

  const { data: store, isLoading } = useKnowledgeStore(account, storeName ?? "", isAuthenticated && !!storeName);
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { key: Tab; label: string; hidden?: boolean; icon: React.ReactNode }[] = [
    { key: "overview", label: "Overview", icon: <Squares2X2Icon className="size-3.5 shrink-0" /> },
    { key: "logs", label: "Logs", hidden: store?.mode !== "managed", icon: <QueueListIcon className="size-3.5 shrink-0" /> },
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

  return (
    <div className="flex flex-1 min-h-0 overflow-hidden relative bg-background">
      <div className="flex flex-1 flex-col min-w-0 min-h-0">

        <PageBreadcrumb
          items={[
            { label: "Knowledge Stores", to: knowledgePath },
            { label: account, to: accountProfilePath(account) },
            { label: store.name },
          ]}
          mobileItems={[
            {
              label: (
                <span className="inline-flex items-center gap-2">
                  <UserAvatar handle={account} name={account} className="size-5" />
                  {account}
                </span>
              ),
              to: accountProfilePath(account),
            },
          ]}
        />

        <div className="bg-background border-b border-border shrink-0 pt-6">
          <div className="mb-4 px-8">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5 mb-2.5">
                <h1 className="text-heading-1 text-foreground w-full sm:w-auto">{store.name}</h1>
                <StatusBadge color={statusToColor(store.status)} spinning={isTransitionalStatus(store.status)}>
                  {statusLabel(store.status)}
                </StatusBadge>
                <Tag color={store.mode === "managed" ? "blue" : "default"}>
                  {store.mode === "managed" ? "Managed" : "External"}
                </Tag>
              </div>
              <div className="flex items-center gap-3 flex-wrap">
                <Chip>
                  <ProviderIcon provider={store.provider} className="size-3.5 shrink-0" />
                  {PROVIDER_LABELS[store.provider] ?? store.provider}
                </Chip>
                <Chip>
                  <Bot className="size-3.5 shrink-0" />
                  {store.bound_agents?.length ?? 0} bound agents
                </Chip>
                <Chip>
                  <Calendar className="size-3.5 shrink-0" />
                  Created {new Date(store.created_at).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })}
                </Chip>
                {(store.public_host || store.arn) && (
                  <Chip>
                    <span className="font-mono text-mono-sm">{store.public_host || store.arn}</span>
                    <CopyButton copyText={store.public_host || store.arn} className="size-4 p-0 shrink-0 border-0 bg-transparent shadow-none hover:bg-slate-200" iconClassName="size-3" />
                  </Chip>
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
          {tab === "overview" && <OverviewTab store={store} account={account} onViewLogs={() => setTab("logs")} />}
          {tab === "logs" && store.mode === "managed" && <LogsTab account={account} storeName={store.name} />}
          {tab === "settings" && <SettingsPanel store={store} account={account} />}
        </div>

      </div>
    </div>
  );
}

export default function KnowledgeStoreDetail() {
  return <KnowledgeStoreDetailContent />;
}
