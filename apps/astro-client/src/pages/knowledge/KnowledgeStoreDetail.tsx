import { useState, useCallback, useEffect, useRef } from "react";
import { useParams, useNavigate, Link } from "react-router";
import type { Route } from "./+types/KnowledgeStoreDetail";
import {
  ArrowLeftIcon,
  ClipboardIcon,
  CheckIcon,
  Cog6ToothIcon,
  XMarkIcon,
  CircleStackIcon,
  EyeIcon,
  EyeSlashIcon,
} from "@heroicons/react/24/outline";
import { ExclamationTriangleIcon } from "@heroicons/react/24/solid";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { StatusBadge } from "@/components/StatusBadge";
import { MetricCard } from "@/components/MetricCard";
import { LogViewer, type LogTimeRange } from "@/components/LogViewer";
import { SidePanel } from "@/components/deployed-agent/detail/SidePanel";
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
} from "@/components/knowledge/knowledge-utils";
import { knowledgePath } from "@/lib/routes";
import { getIntegrationIconUrl } from "@/lib/assets";
import { InformationCircleIcon } from "@heroicons/react/24/outline";
import type { KnowledgeStore, KnowledgeEvent, KnowledgeProvider } from "@/lib/api";
import { cn } from "@/lib/utils";

export const meta: Route.MetaFunction = () => [{ title: "Knowledge Store | Astro" }];

const PROVIDERS_WITH_ICON = new Set<KnowledgeProvider>(["postgres", "qdrant", "redis", "pinecone", "neo4j", "mysql"]);
type Tab = "overview" | "logs";

// --- Helpers ---

function CopyButton({ text, className }: { text: string; className?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className={cn("inline-flex items-center text-muted-foreground hover:text-foreground transition-colors", className)}
      onClick={() => {
        navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      }}
    >
      {copied ? <CheckIcon className="size-3.5" /> : <ClipboardIcon className="size-3.5" />}
    </button>
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
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}

function formatCPU(cores: number): string {
  if (cores < 0.01) return `${(cores * 1000).toFixed(0)}m`;
  return `${cores.toFixed(2)}`;
}

function OverviewTab({ store, account }: { store: KnowledgeStore; account: string }) {
  const isReady = store.status === "ready";
  const { data: metrics, isLoading: metricsLoading } = useKnowledgeMetrics(account, store.name, isReady);

  const cpuValue = metrics?.cpu_cores != null ? formatCPU(metrics.cpu_cores) : "—";
  const memValue = metrics?.memory_bytes != null ? formatBytes(metrics.memory_bytes) : "—";
  const storageValue = metrics?.storage_used != null
    ? `${formatBytes(metrics.storage_used)}${metrics.storage_total != null ? ` / ${formatBytes(metrics.storage_total)}` : ""}`
    : (store.storage ?? "—");
  const uptimeValue = metrics ? formatUptime(metrics.uptime_seconds) : "—";

  return (
    <div className="space-y-6">
      {store.status === "error" && store.error && (
        <div className="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700">
          <ExclamationTriangleIcon className="size-5 shrink-0 mt-0.5" />
          <div>{store.error}</div>
        </div>
      )}

      {/* Metrics row */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <MetricCard label="CPU" value={cpuValue} showTrend={false} loading={metricsLoading} />
        <MetricCard label="Memory" value={memValue} showTrend={false} loading={metricsLoading} />
        <MetricCard label="Storage" value={storageValue} showTrend={false} loading={metricsLoading} />
        <MetricCard label="Uptime" value={uptimeValue} showTrend={false} loading={metricsLoading} />
      </div>

      {/* Two-column: Agent bindings + Event log */}
      <div className="grid gap-6 lg:grid-cols-2">
        {/* Agent bindings */}
        <div className="rounded-lg border border-border bg-surface p-5">
          <div className="flex items-center gap-2 mb-4">
            <h3 className="text-heading-4 text-foreground">Agent bindings</h3>
            <span className="inline-flex items-center justify-center rounded-full bg-muted px-2 py-0.5 font-mono text-mono-sm text-muted-foreground">
              0
            </span>
          </div>
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <CircleStackIcon className="size-10 text-muted-foreground/30" />
            <p className="mt-3 text-body-sm text-muted-foreground">
              No agents are bound to this store yet.
            </p>
            <p className="mt-1 text-body-sm text-muted-foreground">
              Add a <code className="font-mono text-mono-sm">knowledge</code> block in your astropods.yml to bind an agent.
            </p>
          </div>
        </div>

        {/* Right column: Event log (managed only) + PrivateLink */}
        <div className="space-y-6">
          {store.mode === "managed" && (
            <div className="rounded-lg border border-border bg-surface p-5">
              <h3 className="text-heading-4 text-foreground mb-4">Event log</h3>
              <EventTimeline store={store} />
            </div>
          )}

          {store.endpoint && <PrivateLinkSection store={store} />}
        </div>
      </div>
    </div>
  );
}

function EventTimeline({ store }: { store: KnowledgeStore }) {
  const events: KnowledgeEvent[] = store.events ?? [];

  if (events.length === 0) {
    return <p className="text-body-sm text-muted-foreground">No events</p>;
  }

  return (
    <div className="space-y-2">
      {events.map((event, i) => {
        const isWarning = event.type === "Warning";
        return (
          <div key={i} className="flex items-start gap-3 rounded-md border border-border bg-surface px-4 py-3">
            {isWarning ? (
              <ExclamationTriangleIcon className="size-4 shrink-0 mt-0.5 text-yellow-600" />
            ) : (
              <InformationCircleIcon className="size-4 shrink-0 mt-0.5 text-blue-600" />
            )}
            <div className="flex-1 min-w-0">
              <span className="font-medium text-body-sm text-foreground">{event.reason}</span>
              <span className="text-body-sm text-muted-foreground">: {event.message}</span>
            </div>
            {event.count > 1 && (
              <span className="shrink-0 rounded-full border border-border px-1.5 py-0.5 font-mono text-mono-sm text-muted-foreground">
                x{event.count}
              </span>
            )}
          </div>
        );
      })}
    </div>
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

function SettingsPanel({
  store,
  account,
  onClose,
}: {
  store: KnowledgeStore;
  account: string;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <div className="flex h-full w-full flex-col">
      {/* Header */}
      <div className="flex h-[63px] shrink-0 items-center gap-2 border-b border-border px-5">
        <Cog6ToothIcon className="size-3.5 text-primary shrink-0" />
        <span className="flex-1 min-w-0 text-heading-4 font-semibold text-foreground">Settings</span>
        <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
          <XMarkIcon className="size-4" />
        </Button>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto px-5 py-5 space-y-6">
        {/* Name */}
        <div className="space-y-1.5">
          <label className="text-sm font-medium">Name</label>
          <Input value={store.name} disabled />
        </div>

        {/* Storage */}
        {store.mode === "managed" && (
          <div className="space-y-1.5">
            <label className="text-sm font-medium">Storage</label>
            <Input value={store.storage ?? "—"} disabled />
            <p className="text-xs text-muted-foreground">
              Resizing requires a backup and restore by the Astro team &mdash;{" "}
              <a href="#" className="text-teal-700 hover:underline">contact support</a> to change it.
            </p>
          </div>
        )}

        {/* Public access */}
        {store.mode === "managed" && (
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">Public access</p>
              <p className="text-xs text-muted-foreground">Expose with a DNS hostname</p>
            </div>
            <Switch checked={store.public ?? false} disabled />
          </div>
        )}

        {/* Credentials */}
        <CredentialsSection account={account} storeName={store.name} />

        {/* Danger zone */}
        <div className="rounded-lg border border-red-200 bg-red-50/50 p-4 space-y-2">
          <p className="text-sm font-medium text-foreground">Danger zone</p>
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">
              Permanently removes this store and all bindings.
            </p>
            <Button
              variant="outline"
              size="sm"
              className="shrink-0 text-destructive border-red-200 hover:bg-red-100"
              onClick={() => setDeleteOpen(true)}
            >
              Delete
            </Button>
          </div>
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

function CredentialsSection({ account, storeName }: { account: string; storeName: string }) {
  const [enabled, setEnabled] = useState(false);
  const { data, isLoading, isError, error } = useKnowledgeCredentials(account, storeName, enabled);
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});

  if (!enabled) {
    return (
      <div className="space-y-1.5">
        <label className="text-sm font-medium">Credentials</label>
        <p className="text-xs text-muted-foreground mb-2">Loaded on demand for security.</p>
        <Button variant="outline" size="sm" onClick={() => setEnabled(true)}>Load credentials</Button>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="space-y-1.5">
        <label className="text-sm font-medium">Credentials</label>
        <Skeleton className="h-24 w-full rounded-md" />
      </div>
    );
  }

  const is404 = isError && (error as unknown as { status?: number })?.status === 404;
  if (is404) {
    return (
      <div className="space-y-1.5">
        <label className="text-sm font-medium">Credentials</label>
        <p className="text-xs text-muted-foreground">Not available (KMS was not configured when this store was created).</p>
      </div>
    );
  }

  if (isError || !data || Object.keys(data).length === 0) {
    return (
      <div className="space-y-1.5">
        <label className="text-sm font-medium">Credentials</label>
        <p className="text-xs text-muted-foreground">{isError ? "Failed to load credentials." : "No credentials found."}</p>
      </div>
    );
  }

  return (
    <div className="space-y-1.5">
      <label className="text-sm font-medium">Credentials</label>
      <div className="divide-y divide-border rounded-md border border-border bg-muted/30">
        {Object.entries(data).map(([key, value]) => (
          <div key={key} className="flex items-center gap-3 px-3 py-2">
            <span className="w-24 shrink-0 font-mono text-mono-sm text-muted-foreground truncate">{key}</span>
            <span className="flex-1 text-mono-sm text-foreground font-mono truncate">
              {revealed[key] ? value : "••••••••"}
            </span>
            <div className="flex items-center gap-1">
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground transition-colors"
                onClick={() => setRevealed((r) => ({ ...r, [key]: !r[key] }))}
              >
                {revealed[key] ? <EyeSlashIcon className="size-3.5" /> : <EyeIcon className="size-3.5" />}
              </button>
              <CopyButton text={value} />
            </div>
          </div>
        ))}
      </div>
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
  const [settingsOpen, setSettingsOpen] = useState(false);

  const tabs: { key: Tab; label: string; hidden?: boolean }[] = [
    { key: "overview", label: "Overview" },
    { key: "logs", label: "Logs", hidden: store?.mode !== "managed" },
  ];

  if (isLoading) {
    return (
      <div className="flex-1 bg-muted">
        <div className="px-6 py-6 space-y-4">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-64 w-full rounded-md" />
        </div>
      </div>
    );
  }

  if (!store) {
    return (
      <div className="flex-1 bg-muted">
        <div className="px-6 py-6">
          <p className="text-body-sm text-muted-foreground">Knowledge store not found.</p>
          <Button asChild variant="outline" className="mt-4">
            <Link to={knowledgePath}>Back to stores</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* Main content */}
      <div className="flex-1 overflow-y-auto bg-muted">
        <div className="px-6 py-6">
          {/* Header */}
          <div className="mb-6">
            {/* Top row: back + settings */}
            <div className="flex items-center justify-between mb-3">
              <Link
                to={knowledgePath}
                className="inline-flex items-center gap-1.5 text-body-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                <ArrowLeftIcon className="size-3.5" />
                Knowledge Stores
              </Link>

              <Button
                variant="outline"
                size="sm"
                onClick={() => setSettingsOpen(!settingsOpen)}
                className={cn(settingsOpen && "bg-muted")}
              >
                <Cog6ToothIcon className="size-4" />
                Settings
              </Button>
            </div>

            {/* Name + status + provider icon + ARN */}
            <div className="flex items-center gap-3">
              <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-surface border border-border">
                <ProviderIcon provider={store.provider} className="size-5" />
              </div>
              <div className="min-w-0">
                <div className="flex items-center gap-2.5">
                  <h1 className="text-heading-1 text-foreground">{store.name}</h1>
                  <StatusBadge
                    color={statusToColor(store.status)}
                    indicator
                    spinning={isTransitionalStatus(store.status)}
                  >
                    {statusLabel(store.status)}
                  </StatusBadge>
                </div>
                {(store.public_host || store.arn) && (
                  <div className="flex items-center gap-1.5 mt-0.5">
                    <span className="font-mono text-mono-sm text-muted-foreground">
                      {store.public_host || store.arn}
                    </span>
                    <CopyButton text={store.public_host || store.arn} />
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Tabs */}
          <div className="mb-6 flex gap-4 border-b border-border">
            {tabs.filter((t) => !t.hidden).map((t) => (
              <button
                key={t.key}
                type="button"
                className={cn(
                  "pb-2 text-body-sm font-medium transition-colors border-b-2",
                  tab === t.key
                    ? "border-teal-700 text-foreground"
                    : "border-transparent text-muted-foreground hover:text-foreground",
                )}
                onClick={() => setTab(t.key)}
              >
                {t.label}
              </button>
            ))}
          </div>

          {/* Tab content */}
          {tab === "overview" && <OverviewTab store={store} account={account} />}
          {tab === "logs" && store.mode === "managed" && <LogsTab account={account} storeName={store.name} />}
        </div>
      </div>

      {/* Settings side panel */}
      <SidePanel open={settingsOpen}>
        {settingsOpen && (
          <SettingsPanel
            store={store}
            account={account}
            onClose={() => setSettingsOpen(false)}
          />
        )}
      </SidePanel>
    </div>
  );
}

export default function KnowledgeStoreDetail() {
  return <KnowledgeStoreDetailContent />;
}
