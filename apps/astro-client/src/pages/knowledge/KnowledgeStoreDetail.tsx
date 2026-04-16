import { useState } from "react";
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
import { Spinner } from "@/components/ui/spinner";
import { MetricCard } from "@/components/MetricCard";
import { LogViewer, type LogTimeRange } from "@/components/LogViewer";
import { SidePanel } from "@/components/deployed-agent/detail/SidePanel";
import { useAuth } from "@/lib/auth";
import { useDefaultAccount } from "@/hooks/use-default-account";
import { useKnowledgeStore, useKnowledgeCredentials, useKnowledgeLogs } from "@/api/queries/knowledge";
import { DeleteKnowledgeStoreDialog } from "@/components/knowledge/DeleteKnowledgeStoreDialog";
import {
  statusToColor,
  isTransitionalStatus,
  statusLabel,
} from "@/components/knowledge/knowledge-utils";
import { knowledgePath } from "@/lib/routes";
import { getIntegrationIconUrl } from "@/lib/assets";
import type { KnowledgeStore, KnowledgeProvider } from "@/lib/api";
import { cn } from "@/lib/utils";

export const meta: Route.MetaFunction = () => [{ title: "Knowledge Store | Astro" }];

const PROVIDERS_WITH_ICON = new Set<KnowledgeProvider>(["postgres", "qdrant", "redis", "pinecone"]);
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

function formatEventTime(dateStr: string): string {
  const d = new Date(dateStr);
  return d.toLocaleDateString("en-US", { month: "short", day: "numeric" }) +
    " \u2022 " +
    d.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" });
}

// --- Overview tab ---

function OverviewTab({ store }: { store: KnowledgeStore }) {
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
        <MetricCard label="Requests" value="—" showTrend={false} />
        <MetricCard label="P95 Latency" value="—" showTrend={false} />
        <MetricCard label="Error Rate" value="—" showTrend={false} />
        <MetricCard
          label="Data Size"
          value={store.storage ?? "—"}
          showTrend={false}
        />
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

        {/* Event log */}
        <div className="rounded-lg border border-border bg-surface p-5">
          <h3 className="text-heading-4 text-foreground mb-4">Event log</h3>
          <EventTimeline store={store} />
        </div>
      </div>

      {/* PrivateLink progress (if applicable) */}
      {store.endpoint && <PrivateLinkSection store={store} />}
    </div>
  );
}

function EventTimeline({ store }: { store: KnowledgeStore }) {
  const events = [
    ...(store.endpoint?.status === "ready"
      ? [{ label: "PrivateLink established", time: store.updated_at }]
      : []),
    ...(store.endpoint
      ? [{ label: "Connection verified", time: store.updated_at }]
      : []),
    { label: "Store created", time: store.created_at },
  ];

  return (
    <div className="relative space-y-0">
      {/* Timeline line */}
      <div className="absolute left-[5px] top-2 bottom-2 w-px bg-border" />

      {events.map((event, i) => (
        <div key={i} className="relative flex items-start gap-4 py-2.5">
          <span className="relative z-10 mt-1.5 size-[11px] shrink-0 rounded-full border-2 border-teal-600 bg-surface" />
          <div className="min-w-0">
            <p className="text-body-sm text-foreground">{event.label}</p>
            <p className="text-mono-sm text-muted-foreground">{formatEventTime(event.time)}</p>
          </div>
        </div>
      ))}
    </div>
  );
}

function PrivateLinkSection({ store }: { store: KnowledgeStore }) {
  if (!store.endpoint) return null;
  const status = store.endpoint.status;

  const steps = [
    { key: "connecting", label: "Creating endpoint" },
    { key: "pending-acceptance", label: "Waiting for acceptance" },
    { key: "ready", label: "Ready" },
  ];

  const currentIdx = steps.findIndex((s) => s.key === status);
  const isError = status === "error";

  return (
    <div className="rounded-lg border border-border bg-surface p-5">
      <h3 className="text-heading-4 text-foreground mb-4">PrivateLink</h3>

      <div className="space-y-3">
        {steps.map((step, i) => {
          const isActive = step.key === status;
          const isDone = !isError && currentIdx > i;
          return (
            <div key={step.key} className="flex items-center gap-3">
              <div className={cn(
                "flex size-6 items-center justify-center rounded-full text-xs font-medium",
                isDone && "bg-teal-100 text-teal-700",
                isActive && !isError && "bg-yellow-100 text-yellow-700",
                !isDone && !isActive && "bg-muted text-muted-foreground",
              )}>
                {isDone ? <CheckIcon className="size-3.5" /> : isActive && !isError ? <Spinner size={14} /> : i + 1}
              </div>
              <span className={cn("text-body-sm", (isDone || isActive) ? "text-foreground" : "text-muted-foreground")}>
                {step.label}
              </span>
            </div>
          );
        })}

        {status === "pending-acceptance" && (
          <div className="flex items-start gap-3 rounded-md border border-yellow-200 bg-yellow-50 p-4 text-sm text-yellow-800">
            <ExclamationTriangleIcon className="size-5 shrink-0 mt-0.5 text-yellow-600" />
            <div>
              <p className="font-medium">Action required</p>
              <p>Accept the endpoint connection request in your AWS console.</p>
              {store.endpoint?.region && (
                <p className="mt-1 text-xs text-yellow-700">
                  Region: {store.endpoint.region}
                  {store.endpoint.endpoint_id && <> &middot; Endpoint: {store.endpoint.endpoint_id}</>}
                </p>
              )}
            </div>
          </div>
        )}

        {isError && store.endpoint?.error && (
          <div className="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700">
            <ExclamationTriangleIcon className="size-5 shrink-0 mt-0.5" />
            <div>{store.endpoint.error}</div>
          </div>
        )}
      </div>
    </div>
  );
}

// --- Logs tab ---

function LogsTab({ account, storeName }: { account: string; storeName: string }) {
  const [timeRange, setTimeRange] = useState<LogTimeRange>("1h");
  const { data: logs, isLoading, isError } = useKnowledgeLogs(account, storeName, timeRange);

  return (
    <div className="h-[600px]">
      <LogViewer
        logs={logs ?? []}
        isLoading={isLoading}
        timeRange={timeRange}
        onTimeRangeChange={setTimeRange}
        error={isError ? "Failed to load logs" : undefined}
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
                {store.arn && (
                  <div className="flex items-center gap-1.5 mt-0.5">
                    <span className="font-mono text-mono-sm text-muted-foreground">{store.arn}</span>
                    <CopyButton text={store.arn} />
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
          {tab === "overview" && <OverviewTab store={store} />}
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
