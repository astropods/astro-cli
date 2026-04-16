import { useState, useEffect, useRef } from "react";
import { useParams, useNavigate, Link } from "react-router";
import type { Route } from "./+types/KnowledgeStoreDetail";
import {
  ArrowLeftIcon,
  ClipboardIcon,
  EyeIcon,
  EyeSlashIcon,
  CheckIcon,
} from "@heroicons/react/24/outline";
import { ExclamationTriangleIcon, InformationCircleIcon } from "@heroicons/react/24/solid";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { StatusBadge } from "@/components/StatusBadge";
import { Spinner } from "@/components/ui/spinner";
import { useAuth } from "@/lib/auth";
import { useDefaultAccount } from "@/hooks/use-default-account";
import { useKnowledgeStore, useKnowledgeCredentials } from "@/api/queries/knowledge";
import { useApiClient } from "@/lib/api-context";
import { DeleteKnowledgeStoreDialog } from "@/components/knowledge/DeleteKnowledgeStoreDialog";
import {
  statusToColor,
  isTransitionalStatus,
  statusLabel,
  PROVIDER_LABELS,
} from "@/components/knowledge/knowledge-utils";
import { knowledgePath } from "@/lib/routes";
import type { KnowledgeStore, KnowledgeEvent } from "@/lib/api";
import { cn } from "@/lib/utils";

export const meta: Route.MetaFunction = () => [{ title: "Knowledge Store | Astro" }];

type Tab = "overview" | "credentials" | "logs" | "events";

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className="inline-flex items-center text-muted-foreground hover:text-foreground transition-colors"
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

function DetailRow({ label, value, copyable }: { label: string; value?: string | null; copyable?: boolean }) {
  if (!value) return null;
  return (
    <div className="flex items-start gap-4 py-2">
      <span className="w-36 shrink-0 font-mono text-mono-sm text-muted-foreground">{label}</span>
      <span className="text-body-sm text-foreground break-all">{value}</span>
      {copyable && <CopyButton text={value} />}
    </div>
  );
}

function OverviewTab({ store }: { store: KnowledgeStore }) {
  return (
    <div className="space-y-6">
      {store.status === "error" && store.error && (
        <div className="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700">
          <ExclamationTriangleIcon className="size-5 shrink-0 mt-0.5" />
          <div>{store.error}</div>
        </div>
      )}

      <div className="divide-y divide-border rounded-md border border-border bg-surface px-4">
        <DetailRow label="Name" value={store.name} />
        <DetailRow label="ARN" value={store.arn} copyable />
        <DetailRow label="Provider" value={PROVIDER_LABELS[store.provider] ?? store.provider} />
        <DetailRow label="Mode" value={store.mode === "managed" ? "Managed" : "External"} />
        <DetailRow label="Status" value={statusLabel(store.status)} />
        {store.storage && <DetailRow label="Storage" value={store.storage} />}
        {store.public_host && <DetailRow label="Public Host" value={store.public_host} copyable />}
        <DetailRow label="Created" value={new Date(store.created_at).toLocaleString()} />
      </div>

      {store.endpoint && (
        <div className="space-y-2">
          <h3 className="text-heading-4 text-foreground">PrivateLink</h3>
          <div className="divide-y divide-border rounded-md border border-border bg-surface px-4">
            <DetailRow label="Service" value={store.endpoint.endpoint_service} />
            <DetailRow label="Region" value={store.endpoint.region} />
            <DetailRow label="Endpoint ID" value={store.endpoint.endpoint_id} />
            <DetailRow label="DNS" value={store.endpoint.endpoint_dns} copyable />
            <DetailRow label="Status" value={store.endpoint.status} />
            {store.endpoint.error && <DetailRow label="Error" value={store.endpoint.error} />}
          </div>

          <PrivateLinkProgress store={store} />
        </div>
      )}
    </div>
  );
}

function PrivateLinkProgress({ store }: { store: KnowledgeStore }) {
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
    <div className="mt-4 space-y-3">
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
  );
}

function CredentialsTab({ account, storeName }: { account: string; storeName: string }) {
  const [enabled, setEnabled] = useState(false);
  const { data, isLoading, isError, error } = useKnowledgeCredentials(account, storeName, enabled);
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});

  if (!enabled) {
    return (
      <div className="flex flex-col items-center py-12">
        <p className="text-body-sm text-muted-foreground mb-4">Credentials are loaded on demand.</p>
        <Button variant="outline" onClick={() => setEnabled(true)}>Load credentials</Button>
      </div>
    );
  }

  if (isLoading) {
    return <div className="flex justify-center py-12"><Spinner size={24} /></div>;
  }

  const is404 = isError && (error as unknown as { status?: number })?.status === 404;
  if (is404) {
    return (
      <div className="rounded-md border border-yellow-200 bg-yellow-50 p-4 text-sm text-yellow-800">
        Credentials not available (KMS was not configured when this store was created).
      </div>
    );
  }

  if (isError) {
    return (
      <div className="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700">
        Failed to load credentials.
      </div>
    );
  }

  if (!data || Object.keys(data).length === 0) {
    return <p className="text-body-sm text-muted-foreground py-4">No credentials found.</p>;
  }

  return (
    <div className="divide-y divide-border rounded-md border border-border bg-surface">
      {Object.entries(data).map(([key, value]) => (
        <div key={key} className="flex items-center gap-4 px-4 py-3">
          <span className="w-36 shrink-0 font-mono text-mono-sm text-muted-foreground">{key}</span>
          <span className="flex-1 text-body-sm text-foreground font-mono break-all">
            {revealed[key] ? value : "••••••••"}
          </span>
          <div className="flex items-center gap-1">
            <button
              type="button"
              className="text-muted-foreground hover:text-foreground transition-colors"
              onClick={() => setRevealed((r) => ({ ...r, [key]: !r[key] }))}
            >
              {revealed[key] ? <EyeSlashIcon className="size-4" /> : <EyeIcon className="size-4" />}
            </button>
            <CopyButton text={value} />
          </div>
        </div>
      ))}
    </div>
  );
}

function LogsTab({ account, storeName }: { account: string; storeName: string }) {
  const api = useApiClient();
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(true);
  const containerRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    let cancelled = false;
    const url = api.getKnowledgeLogsStreamUrl(account, storeName);

    async function fetchLogs() {
      try {
        const res = await fetch(url, { credentials: "include" });
        if (!res.ok || !res.body) {
          setLoading(false);
          return;
        }
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        setLoading(false);

        while (!cancelled) {
          const { done, value } = await reader.read();
          if (done) break;
          const text = decoder.decode(value, { stream: true });
          setLogs((prev) => prev + text);
        }
      } catch {
        setLoading(false);
      }
    }
    fetchLogs();
    return () => { cancelled = true; };
  }, [api, account, storeName]);

  useEffect(() => {
    if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [logs]);

  if (loading) {
    return <div className="flex justify-center py-12"><Spinner size={24} /></div>;
  }

  return (
    <pre
      ref={containerRef}
      className="max-h-[500px] overflow-auto rounded-md border border-border bg-stone-950 p-4 font-mono text-mono-sm text-stone-200 whitespace-pre-wrap"
    >
      {logs || "No logs available."}
    </pre>
  );
}

function EventsTab({ store }: { store: KnowledgeStore }) {
  const events = store.events ?? [];

  if (events.length === 0) {
    return <p className="text-body-sm text-muted-foreground py-4">No events.</p>;
  }

  return (
    <div className="space-y-2">
      {events.map((event, i) => (
        <EventRow key={i} event={event} />
      ))}
    </div>
  );
}

function EventRow({ event }: { event: KnowledgeEvent }) {
  const isWarning = event.type === "Warning";
  return (
    <div className="flex items-start gap-3 rounded-md border border-border bg-surface p-3">
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
}

function KnowledgeStoreDetailContent() {
  const { storeName } = useParams();
  const navigate = useNavigate();
  const { personalAccount, isAuthenticated } = useAuth();
  const { validStoredDefault } = useDefaultAccount();
  const account = validStoredDefault || personalAccount?.name || "";

  const { data: store, isLoading } = useKnowledgeStore(account, storeName ?? "", isAuthenticated && !!storeName);
  const [tab, setTab] = useState<Tab>("overview");
  const [deleteOpen, setDeleteOpen] = useState(false);

  const tabs: { key: Tab; label: string; hidden?: boolean }[] = [
    { key: "overview", label: "Overview" },
    { key: "credentials", label: "Credentials" },
    { key: "logs", label: "Logs", hidden: store?.mode !== "managed" },
    { key: "events", label: "Events", hidden: store?.mode !== "managed" },
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
    <div className="flex-1 bg-muted">
      <div className="px-6 py-6">
        {/* Header */}
        <div className="mb-6 flex flex-col gap-3">
          <Link
            to={knowledgePath}
            className="inline-flex items-center gap-1 text-body-sm text-muted-foreground hover:text-foreground transition-colors w-fit"
          >
            <ArrowLeftIcon className="size-3.5" />
            Knowledge Stores
          </Link>

          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <h1 className="text-heading-1 text-foreground">{store.name}</h1>
              <StatusBadge
                color={statusToColor(store.status)}
                indicator
                spinning={isTransitionalStatus(store.status)}
              >
                {statusLabel(store.status)}
              </StatusBadge>
            </div>
            <Button variant="outline" size="sm" className="text-destructive" onClick={() => setDeleteOpen(true)}>
              Delete
            </Button>
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
        {tab === "credentials" && <CredentialsTab account={account} storeName={store.name} />}
        {tab === "logs" && store.mode === "managed" && <LogsTab account={account} storeName={store.name} />}
        {tab === "events" && store.mode === "managed" && <EventsTab store={store} />}
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

export default function KnowledgeStoreDetail() {
  return <KnowledgeStoreDetailContent />;
}
