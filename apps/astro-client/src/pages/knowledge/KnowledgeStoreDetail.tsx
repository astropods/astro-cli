import React, { useState, useCallback, useEffect, useRef } from "react";
import { useParams, useNavigate, Link } from "react-router";
import { Eye, EyeOff, Bot, Calendar, ChevronRight } from "lucide-react";
import type { Route } from "./+types/KnowledgeStoreDetail";
import {
  CircleStackIcon,
  Squares2X2Icon,
  QueueListIcon,
  Cog6ToothIcon,
} from "@heroicons/react/24/outline";
import { ExclamationTriangleIcon, CheckCircleIcon } from "@heroicons/react/24/solid";
import { CopyButton as SharedCopyButton } from "@/components/ui/copy-button";
import { ErrorPanel } from "@/components/ui/status-panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import { DangerZoneItem } from "@/components/settings/DangerZoneItem";
import { StatusBadge } from "@/components/StatusBadge";
import { MetricCard } from "@/components/MetricCard";
import { LogViewer, type LogTimeRange } from "@/components/LogViewer";
import { useAuth } from "@/lib/auth";
import { useDefaultAccount } from "@/hooks/use-default-account";
import { useKnowledgeStore, useKnowledgeCredentials, useKnowledgeLogs, useKnowledgeMetrics } from "@/api/queries/knowledge";
import { useApiClient } from "@/lib/api-context";
import type { LogEntry } from "@/lib/log-utils";
import { DeleteKnowledgeStoreDialog } from "@/components/knowledge/DeleteKnowledgeStoreDialog";
import { PrivateLinkSection } from "@/components/knowledge/PrivateLinkSection";
import {
  statusToColor,
  isTransitionalStatus,
  statusLabel,
  PROVIDER_LABELS,
} from "@/components/knowledge/knowledge-utils";
import { knowledgePath } from "@/lib/routes";
import { Tag } from "@/components/Tag";
import { getIntegrationIconUrl } from "@/lib/assets";
import type { KnowledgeStore, KnowledgeEvent, KnowledgeProvider } from "@/lib/api";
import { cn } from "@/lib/utils";

export const meta: Route.MetaFunction = () => [{ title: "Knowledge Store | Astro" }];

const PROVIDERS_WITH_ICON = new Set<KnowledgeProvider>(["postgres", "qdrant", "redis", "pinecone", "neo4j", "mysql"]);
type Tab = "overview" | "logs" | "settings";

// --- Helpers ---

function Chip({ children }: { children: React.ReactNode }) {
  return (
    <div className="inline-flex items-center gap-1.5 h-7 rounded-sm border border-border bg-white px-2 text-body-sm text-muted-foreground">
      {children}
    </div>
  );
}

function ProviderIcon({ provider, className }: { provider: KnowledgeProvider; className?: string }) {
  if (PROVIDERS_WITH_ICON.has(provider)) {
    return (
      <img
        src={getIntegrationIconUrl(provider, "light")}
        alt=""
        className={cn("object-contain", className)}
        loading="lazy"
      />
    );
  }
  return <CircleStackIcon className={cn("text-muted-foreground/60", className)} />;
}

// --- Overview tab ---

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function formatUptime(seconds: number): string {
  if (seconds < 60) { const v = seconds; return `${v} ${v === 1 ? "second" : "seconds"}`; }
  if (seconds < 3600) { const v = Math.floor(seconds / 60); return `${v} ${v === 1 ? "minute" : "minutes"}`; }
  if (seconds < 86400) { const v = Math.floor(seconds / 3600); return `${v} ${v === 1 ? "hour" : "hours"}`; }
  const v = Math.floor(seconds / 86400);
  return `${v} ${v === 1 ? "day" : "days"}`;
}

function formatCPU(cores: number): string {
  if (cores < 0.01) return `${(cores * 1000).toFixed(0)}m`;
  return `${cores.toFixed(2)}`;
}

function OverviewTab({ store, account, onViewLogs }: { store: KnowledgeStore; account: string; onViewLogs: () => void }) {
  const isReady = store.status === "ready";
  const { data: metrics, isLoading: metricsLoading } = useKnowledgeMetrics(account, store.name, isReady);

  const cpuValue = metrics?.cpu_cores != null ? formatCPU(metrics.cpu_cores) : "—";
  const memValue = metrics?.memory_bytes != null ? formatBytes(metrics.memory_bytes) : "—";
  const storageUsed = metrics?.storage_used != null ? formatBytes(metrics.storage_used) : (store.storage ?? "—");
  const storageSuffix = metrics?.storage_total != null ? `/ ${formatBytes(metrics.storage_total)}` : undefined;
  const uptimeValue = metrics?.uptime_seconds != null ? formatUptime(metrics.uptime_seconds) : "—";

  return (
    <div className="space-y-6">
      {store.status === "error" && store.error && (
        <ErrorPanel>{store.error}</ErrorPanel>
      )}

      {store.endpoint && <PrivateLinkSection store={store} />}

      {store.status !== "pending-acceptance" && <>
      {/* Metrics row */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <MetricCard label="CPU" value={cpuValue} showTrend={false} loading={metricsLoading} sparkline={metrics?.cpu_cores != null ? [0.08,0.11,0.09,0.14,0.12,0.10,0.13,0.12,0.15,0.12] : undefined} />
        <MetricCard label="Memory" value={memValue} showTrend={false} loading={metricsLoading} sparkline={metrics?.memory_bytes != null ? [110,118,125,122,130,128,134,132,134,134] : undefined} />
        <MetricCard label="Storage" value={storageUsed} valueSuffix={storageSuffix} showTrend={false} loading={metricsLoading} sparkline={metrics?.storage_used != null ? [30,35,38,40,44,46,48,50,52,52] : undefined} />
        <MetricCard label="Uptime" value={uptimeValue} showTrend={false} loading={metricsLoading}
          description={metrics?.uptime_seconds != null ? <span className="flex items-center gap-1.5 text-body-sm text-muted-foreground"><CheckCircleIcon className="size-3.5 shrink-0 text-teal-600" />No restarts detected</span> : undefined}
        />
      </div>

      {/* Two-column: Agent bindings + Event log sidebar (managed only) */}
      <div className={cn("grid gap-8", store.mode === "managed" && "lg:grid-cols-[1fr_420px]")}>
        {/* Agent bindings */}
        <div className="rounded-lg border border-border bg-white p-5">
          <div className="flex items-center gap-2 mb-4">
            <h3 className="text-heading-4 text-foreground">Agent bindings</h3>
            <Tag>{store.bound_agents?.length ?? 0}</Tag>
          </div>
          {store.bound_agents && store.bound_agents.length > 0 ? (
            <div className="divide-y divide-border">
              {store.bound_agents.map((agent) => (
                <div key={agent} className="flex items-center gap-2 py-2.5">
                  <span className="font-mono text-mono-sm text-foreground">{agent}</span>
                </div>
              ))}
            </div>
          ) : (
            <div className="flex flex-col items-center justify-center py-12 text-center">
              <Bot className="size-10 text-muted-foreground" />
              <p className="mt-3 text-body-sm text-foreground font-medium">No agents are bound to this store yet.</p>
              <p className="mt-1 text-body-sm text-muted-foreground">
                Add a <code className="font-mono text-mono-sm">knowledge</code> block in your astropods.yml to bind an agent.
              </p>
            </div>
          )}
        </div>

        {/* Event log sidebar */}
        {store.mode === "managed" && (
          <div className="flex flex-col">
            <div className="rounded-lg border border-border bg-surface overflow-hidden">
              <div className="flex items-center justify-between px-5 py-3 border-b border-border bg-white">
                <h3 className="text-heading-4 text-foreground">Event log</h3>
                <Button variant="outline" size="sm" onClick={onViewLogs}>View logs →</Button>
              </div>
              <EventTimeline store={store} />
            </div>
          </div>
        )}
      </div>
      </>}

    </div>
  );
}

function EventTimeline({ store }: { store: KnowledgeStore }) {
  const events: KnowledgeEvent[] = store.events ?? [];

  if (events.length === 0) {
    return <p className="px-5 py-6 text-center text-body-sm text-muted-foreground">No events recorded</p>;
  }

  // Group events by date
  const groups: { date: string; events: KnowledgeEvent[] }[] = [];
  for (const event of events) {
    const date = event.timestamp
      ? new Date(event.timestamp).toLocaleDateString("en-US", { month: "long", day: "numeric", year: "numeric" })
      : "Unknown date";
    const last = groups[groups.length - 1];
    if (last && last.date === date) {
      last.events.push(event);
    } else {
      groups.push({ date, events: [event] });
    }
  }

  return (
    <>
      {groups.map((group, gi) => (
        <div key={group.date}>
          <div className={cn("px-5 py-1 border-b border-border", gi > 0 && "border-t")}>
            <span className="font-mono text-label uppercase tracking-[0.07em] text-faint-foreground">{group.date}</span>
          </div>
          {group.events.map((event, i) => (
            <div key={i} className="flex items-center gap-3 px-5 py-3 bg-white border-b border-border last:border-0">
              {event.timestamp && (
                <span className="w-14 shrink-0 whitespace-nowrap font-mono text-mono-sm tabular-nums text-muted-foreground">
                  {new Date(event.timestamp).toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit", hour12: true })}
                </span>
              )}
              <span className="flex-1 text-body-sm font-medium text-foreground">
                {event.reason}
                {event.count > 1 && <span className="ml-1 font-normal text-muted-foreground">×{event.count}</span>}
              </span>
              {event.type === "Warning" ? (
                <ExclamationTriangleIcon className="size-4 shrink-0 text-red-500" />
              ) : (
                <CheckCircleIcon className="size-4 shrink-0 text-teal-600" />
              )}
            </div>
          ))}
        </div>
      ))}
    </>
  );
}

// --- Logs tab ---

const MAX_TAIL_LINES = 5000;

function useKnowledgeLogStream(account: string, storeName: string) {
  const api = useApiClient();
  const [lines, setLines] = useState<LogEntry[]>([]);
  const [status, setStatus] = useState<"idle" | "connecting" | "tailing" | "reconnecting">("idle");
  const [error, setError] = useState<string>();
  const esRef = useRef<EventSource | null>(null);

  const stop = useCallback(() => {
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }
    setStatus("idle");
    setLines([]);
    setError(undefined);
  }, []);

  const start = useCallback(() => {
    stop();
    setStatus("connecting");

    const url = api.getKnowledgeLogsStreamUrl(account, storeName);
    const es = new EventSource(url);
    esRef.current = es;
    let hasBeenLive = false;

    es.onmessage = (e: MessageEvent) => {
      try {
        const parsed = JSON.parse(e.data) as { timestamp: string; level: string; message: string };
        setLines((prev) => {
          const next = [...prev, { timestamp: parsed.timestamp, level: parsed.level || null, message: parsed.message }];
          return next.length > MAX_TAIL_LINES ? next.slice(-MAX_TAIL_LINES) : next;
        });
      } catch { /* ignore */ }
    };

    es.addEventListener("ready", () => {
      hasBeenLive = true;
      setStatus("tailing");
    });

    es.addEventListener("error", (e: Event) => {
      if ("data" in e) {
        try {
          const parsed = JSON.parse((e as MessageEvent).data) as { message?: string };
          setError(parsed.message ?? "Stream error");
        } catch { /* ignore */ }
      }
    });

    es.onerror = () => {
      if (es.readyState === EventSource.CONNECTING) {
        if (hasBeenLive) setStatus("reconnecting");
        else setError("Failed to connect to log stream");
      } else if (es.readyState === EventSource.CLOSED) {
        setStatus("idle");
      }
    };
  }, [api, account, storeName, stop]);

  useEffect(() => {
    return () => { esRef.current?.close(); esRef.current = null; };
  }, []);

  return { lines, status, error, start, stop };
}

function LogsTab({ account, storeName }: { account: string; storeName: string }) {
  const [timeRange, setTimeRange] = useState<LogTimeRange>("1h");
  const [tailing, setTailing] = useState(false);
  const { data: historyLogs, isLoading, isError } = useKnowledgeLogs(account, storeName, timeRange, { enabled: !tailing });
  const stream = useKnowledgeLogStream(account, storeName);

  const handleTailToggle = useCallback(() => {
    if (tailing) {
      stream.stop();
      setTailing(false);
    } else {
      stream.start();
      setTailing(true);
    }
  }, [tailing, stream]);

  const logs = tailing ? stream.lines : (historyLogs ?? []);
  const loading = tailing ? stream.status === "connecting" : isLoading;
  const errorMsg = tailing ? stream.error : (isError ? "Failed to load logs" : undefined);

  return (
    <div className="h-[600px]">
      <LogViewer
        logs={logs}
        isLoading={loading}
        timeRange={timeRange}
        onTimeRangeChange={setTimeRange}
        error={errorMsg}
        isTailing={tailing && stream.status === "tailing"}
        isReconnecting={stream.status === "reconnecting"}
        onTailToggle={handleTailToggle}
      />
    </div>
  );
}

// --- Settings panel ---

function SettingRow({ label, description, children }: { label: string; description?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-2 px-5 py-4 sm:grid-cols-[220px_1fr] sm:gap-8 sm:items-center">
      <div>
        <p className="text-[13px] font-semibold text-foreground">{label}</p>
        {description && <p className="mt-0.5 text-body-sm text-muted-foreground">{description}</p>}
      </div>
      <div>{children}</div>
    </div>
  );
}

function SettingsPanel({ store, account }: { store: KnowledgeStore; account: string }) {
  const navigate = useNavigate();
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <div className="max-w-2xl space-y-6">
      {/* Configuration card — managed only */}
      {store.mode === "managed" && <div className="rounded-lg border border-border bg-white overflow-hidden">
        <div className="px-5 py-4 border-b border-border">
          <h3 className="text-heading-4 text-foreground">Configuration</h3>
          <p className="mt-0.5 text-body-sm text-muted-foreground">These settings can't be changed after creation. <a href="mailto:support@astropods.com" className="text-teal-700 underline">Contact us</a> if you need to make changes.</p>
        </div>
        <SettingRow label="Storage">
          <span className="font-mono text-mono-sm text-foreground">{store.storage ?? "—"}</span>
        </SettingRow>
        <SettingRow label="Public access" description="Exposes the store on a public DNS hostname for external connections.">
          <div className="flex items-center gap-2">
            <Switch checked={!!(store.public && store.public_host)} disabled />
            {store.public && store.public_host ? (
              <span className="font-mono text-mono-sm text-muted-foreground truncate">{store.public_host}</span>
            ) : (
              <span className="text-body-sm text-muted-foreground">Not enabled</span>
            )}
          </div>
        </SettingRow>
      </div>}

      {/* Credentials card */}
      <CredentialsCard account={account} storeName={store.name} />

      {/* Danger zone */}
      <div className="rounded-lg border border-border bg-white overflow-hidden">
        <div className="px-5 py-4 border-b border-border">
          <h3 className="text-heading-4 text-foreground">Danger Zone</h3>
          <p className="mt-0.5 text-body-sm text-muted-foreground">These actions are irreversible.</p>
        </div>
        <div className="p-5">
          <DangerZoneItem
            title="Delete store"
            description="Permanently removes this store and all agent bindings."
            actionLabel="Delete store"
            onAction={() => setDeleteOpen(true)}
          />
        </div>
      </div>

      <DeleteKnowledgeStoreDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        storeName={store.name}
        account={account}
        onDeleted={() => navigate(knowledgePath)}
      />
    </div>
  );
}

function CredentialsCard({ account, storeName }: { account: string; storeName: string }) {
  const [enabled, setEnabled] = useState(false);
  const { data, isLoading, isError, error } = useKnowledgeCredentials(account, storeName, enabled);
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});

  const is404 = isError && (error as unknown as { status?: number })?.status === 404;

  const renderBody = () => {
    if (!enabled) return null;
    if (isLoading) return <div className="px-5 py-4"><Skeleton className="h-24 w-full rounded-sm" /></div>;
    if (is404) return <div className="px-5 py-4"><p className="text-body-sm text-muted-foreground">Not available — KMS was not configured when this store was created.</p></div>;
    if (isError || !data || Object.keys(data).length === 0) return <div className="px-5 py-4"><p className="text-body-sm text-muted-foreground">{isError ? "Failed to load credentials." : "No credentials found."}</p></div>;
    const CREDENTIAL_DESCRIPTIONS: Record<string, string> = {
      host: "The hostname or IP address of the store.",
      port: "The port number to connect on.",
      database: "The database name within the store.",
      api_key: "Secret key for authenticating API requests.",
      url: "The full connection URL including credentials.",
    };
    return Object.entries(data).map(([key, value]) => (
      <SettingRow key={key} label={key.split("_").map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(" ")} description={CREDENTIAL_DESCRIPTIONS[key]}>
        <div className="relative flex items-center">
          <Input value={revealed[key] ? value : "••••••••••••"} readOnly className="pr-16 font-mono cursor-default bg-stone-100 focus-visible:ring-0 focus-visible:border-border" />
          <div className="absolute right-2 flex items-center gap-1">
            <button type="button" className="cursor-pointer text-muted-foreground hover:text-foreground transition-colors p-1" onClick={() => setRevealed((r) => ({ ...r, [key]: !r[key] }))}>
              {revealed[key] ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
            </button>
            <SharedCopyButton copyText={value} className="size-6 p-0 shrink-0" iconClassName="size-3.5" />
          </div>
        </div>
      </SettingRow>
    ));
  };

  return (
    <div className="rounded-lg border border-border bg-white overflow-hidden">
      <div className={cn("flex items-center justify-between px-5 py-4", enabled && "border-b border-border")}>
        <div>
          <h3 className="text-heading-4 text-foreground">Credentials</h3>
          <p className="mt-0.5 text-body-sm text-muted-foreground">Fetched securely on demand and not stored in your browser.</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => { setEnabled((v) => !v); setRevealed({}); }}>
          {enabled ? "Hide credentials" : "View credentials"}
        </Button>
      </div>
      {renderBody()}
    </div>
  );
}

// --- Page shell ---

function KnowledgeStoreDetailContent() {
  const { storeName } = useParams();
  const { personalAccount, isAuthenticated } = useAuth();
  const { validStoredDefault } = useDefaultAccount();
  const account = validStoredDefault || personalAccount?.name || "";

  const { data: store, isLoading } = useKnowledgeStore(account, storeName ?? "", isAuthenticated && !!storeName);
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { key: Tab; label: string; hidden?: boolean; icon: React.ReactNode }[] = [
    {
      key: "overview",
      label: "Overview",
      icon: <Squares2X2Icon className="size-3.5 shrink-0" />,
    },
    {
      key: "logs",
      label: "Logs",
      hidden: store?.mode !== "managed",
      icon: <QueueListIcon className="size-3.5 shrink-0" />,
    },
    {
      key: "settings",
      label: "Settings",
      icon: <Cog6ToothIcon className="size-3.5 shrink-0" />,
    },
  ];

  if (isLoading) {
    return (
      <div className="flex-1 bg-surface">
        <div className="px-8 py-6 space-y-4">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-64 w-full rounded-md" />
        </div>
      </div>
    );
  }

  if (!store) {
    return (
      <div className="flex-1 bg-surface">
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
    <div className="flex flex-1 min-h-0 overflow-hidden relative bg-surface">
      <div className="flex flex-1 flex-col min-w-0 min-h-0">

        {/* Breadcrumb */}
        <div className="flex h-[52px] items-center border-b border-stone-300 bg-surface px-8 shrink-0">
          <div className="flex items-center gap-2 font-mono text-mono-sm text-muted-foreground">
            <Link to={knowledgePath} className="hover:text-foreground transition-colors">Knowledge stores</Link>
            <ChevronRight className="size-3 text-faint-foreground" />
            <Link to={`/${account}`} className="hover:text-foreground transition-colors font-medium text-foreground">{account}</Link>
          </div>
        </div>

        {/* Sticky header + tab bar */}
        <div className="bg-surface border-b border-border shrink-0 pt-6">

          {/* Main header */}
          <div className="mb-4 px-8">
            <div className="min-w-0">
              <div className="flex items-center gap-2.5 mb-2.5">
                <h1 className="text-heading-1 text-foreground">{store.name}</h1>
                <StatusBadge
                  color={statusToColor(store.status)}
                  spinning={isTransitionalStatus(store.status)}
                >
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
                {(store.public_host || store.arn) && (
                  <Chip>
                    <span className="font-mono text-mono-sm">{store.public_host || store.arn}</span>
                    <SharedCopyButton copyText={store.public_host || store.arn} className="size-4 p-0 shrink-0 border-0 bg-transparent shadow-none hover:bg-stone-200" iconClassName="size-3" />
                  </Chip>
                )}
                <Chip>
                  <Bot className="size-3.5 shrink-0" />
                  {store.bound_agents?.length ?? 0} bound agents
                </Chip>
                <Chip>
                  <Calendar className="size-3.5 shrink-0" />
                  Created {new Date(store.created_at).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })}
                </Chip>
              </div>
            </div>
          </div>

          {/* Tab bar */}
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
                    ? "cursor-pointer font-medium text-foreground border-b-[var(--color-teal-600)]"
                    : "cursor-pointer font-normal text-faint-foreground border-b-transparent",
                )}
              >
                {t.icon}
                {t.label}
              </button>
            ))}
          </div>
        </div>

        {/* Scrollable tab content */}
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
