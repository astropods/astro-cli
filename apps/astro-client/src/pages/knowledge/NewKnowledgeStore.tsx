import { useState, useMemo, useCallback, useEffect } from "react";
import { Link } from "react-router";
import type { Route } from "./+types/NewKnowledgeStore";
import {
  ArrowLeftIcon,
  GlobeAltIcon,
  CheckIcon,
  ExclamationTriangleIcon,
  InformationCircleIcon,
} from "@heroicons/react/24/outline";
import { CircleStackIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Spinner } from "@/components/ui/spinner";
import { Tag } from "@/components/Tag";
import { FormSection } from "@/components/deploy/FormSection";
import { useAuth } from "@/lib/auth";
import { useDefaultAccount } from "@/hooks/use-default-account";
import { useCreateKnowledgeStore, useConnectKnowledgeStore, useKnowledgeStore } from "@/api/queries/knowledge";
import {
  validateStoreName,
  MANAGED_PROVIDERS,
  PROVIDER_LABELS,
  PROVIDER_FIELDS,
  PROVIDER_PORTS,
} from "@/components/knowledge/knowledge-utils";
import { PrivateLinkSection } from "@/components/knowledge/PrivateLinkSection";
import { knowledgePath, knowledgeDetailPath } from "@/lib/routes";
import { getIntegrationIconUrl } from "@/lib/assets";
import type { KnowledgeProvider, KnowledgeStore, KnowledgeEvent } from "@/lib/api";
import { cn } from "@/lib/utils";
import { ErrorPanel } from "@/components/ui/status-panel";
import { CopyButton } from "@/components/ui/copy-button";
import { StatusBadge } from "@/components/StatusBadge";
import { LiveRevealConfetti } from "@/components/deployed-agent/detail/LiveRevealConfetti";

export const meta: Route.MetaFunction = () => [{ title: "Add Store | Knowledge Stores | Astro" }];

// --- Provider catalog ---

const PROVIDERS_WITH_ICON = new Set<KnowledgeProvider>(["postgres", "qdrant", "redis", "pinecone", "neo4j", "mysql"]);

const PROVIDER_CATEGORIES: Record<KnowledgeProvider, string> = {
  postgres: "Relational",
  qdrant: "Vector search",
  redis: "Key-value",
  neo4j: "Graph database",
  pinecone: "Vector search",
  mysql: "Relational",
};

const ALL_PROVIDERS: KnowledgeProvider[] = ["postgres", "qdrant", "redis", "neo4j", "mysql", "pinecone"];
const MANAGED_SET = new Set<KnowledgeProvider>(MANAGED_PROVIDERS);

const STORAGE_OPTIONS: { value: string; label: string }[] = [
  { value: "10Gi", label: "10 GB" },
  { value: "20Gi", label: "20 GB" },
  { value: "50Gi", label: "50 GB" },
  { value: "100Gi", label: "100 GB" },
  { value: "1Ti", label: "1 TB" },
];

// --- Helpers ---

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


// --- Step 1: Provider selection ---

function ProviderList({ onSelect }: { onSelect: (p: KnowledgeProvider) => void }) {
  return (
    <div className="mx-auto max-w-2xl">
      <h2 className="text-heading-1 text-foreground">Choose a provider</h2>
      <p className="mt-1 text-body-sm text-muted-foreground">
        Pick the database or vector store to back this knowledge store.
      </p>

      <div className="mt-6 space-y-3">
        {ALL_PROVIDERS.map((p) => (
          <button
            key={p}
            type="button"
            onClick={() => onSelect(p)}
            className="flex w-full cursor-pointer items-center gap-4 rounded-lg border border-border bg-white px-5 py-4 text-left transition-all hover:border-teal-500 hover:shadow-md dark:bg-popover dark:hover:border-teal-700"
          >
            <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted">
              <ProviderIcon provider={p} className="size-6" />
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-medium text-foreground">{PROVIDER_LABELS[p]}</span>
                {MANAGED_SET.has(p) && <Tag color="blue">Managed option</Tag>}
              </div>
              <p className="text-body-sm text-muted-foreground">{PROVIDER_CATEGORIES[p]}</p>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

const CLOUD_CONSOLE: Record<string, {
  stepTitle: string;
  label: string;
  description: string;
  url: (region: string, endpointId?: string) => string;
}> = {
  aws: {
    stepTitle: "Approve the VPC Endpoint request",
    label: "Open AWS Console ↗",
    description: "In AWS Console, go to VPC → Endpoints and approve the pending connection from Astro.",
    url: (region, endpointId) =>
      `https://console.aws.amazon.com/vpc/home?region=${region}#Endpoints:${endpointId ? `endpointId=${endpointId}` : ""}`,
  },
  gcp: {
    stepTitle: "Approve the Private Service Connect endpoint",
    label: "Open GCP Console ↗",
    description: "In GCP Console, go to Private Service Connect and approve the pending endpoint request from Astro.",
    url: () => "https://console.cloud.google.com/net-services/psc/list/endpoints",
  },
  azure: {
    stepTitle: "Approve the Private Endpoint connection",
    label: "Open Azure Portal ↗",
    description: "In Azure Portal, go to Private Link Center → Pending connections and approve the request from Astro.",
    url: () => "https://portal.azure.com/#view/HubsExtension/BrowseResource/resourceType/Microsoft.Network%2FprivateEndpoints",
  },
};

// --- Pending acceptance stage (PrivateLink) ---

function PendingAcceptanceStage({ store }: { store: KnowledgeStore }) {
  const cloud = store.endpoint ? (CLOUD_CONSOLE[store.endpoint.cloud_provider] ?? CLOUD_CONSOLE.aws) : null;
  const consoleUrl = cloud && store.endpoint ? cloud.url(store.endpoint.region, store.endpoint.endpoint_id) : "#";

  return (
    <div className="mx-auto max-w-lg flex flex-col gap-4">

      {/* Heading */}
      <div className="flex flex-col items-center text-center gap-1.5 mb-2">
        <h2 className="text-heading-1 text-foreground">Complete your PrivateLink setup</h2>
        <p className="text-body text-muted-foreground max-w-sm">
          Your store is registered. Follow these steps to finish connecting it.
        </p>
      </div>

      {/* Steps card — store header + steps */}
      <div className="rounded-lg overflow-hidden border border-border bg-white dark:bg-surface divide-y divide-border">

        {/* Store header */}
        <div className="flex items-center gap-3 px-4 py-3.5">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted">
            <ProviderIcon provider={store.provider} className="size-5" />
          </div>
          <div className="flex-1 min-w-0">
            <p className="font-medium leading-tight text-foreground">{store.name}</p>
            <p className="mt-0.5 text-body-sm text-muted-foreground">{PROVIDER_LABELS[store.provider]}</p>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Tag color={store.mode === "managed" ? "blue" : "default"}>
              {store.mode === "managed" ? "Managed" : "External"}
            </Tag>
            <StatusBadge color="warning" indicator spinning>Pending</StatusBadge>
          </div>
        </div>

        {/* Step 1 — Complete */}
        <div className="flex items-start gap-4 px-5 py-4">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-teal-600 mt-0.5">
            <CheckIcon className="size-3.5 text-white stroke-[2]" />
          </div>
          <div className="flex-1 min-w-0 pt-0.5">
            <p className="text-body font-medium text-foreground">Store registered in Astro</p>
            <p className="mt-0.5 text-body-sm text-muted-foreground">Your store and PrivateLink endpoint ID were saved.</p>
            {store.endpoint?.region && (
              <div className="mt-3 flex flex-wrap gap-x-6 gap-y-1">
                <div>
                  <p className="text-body-sm font-medium text-muted-foreground">Region</p>
                  <p className="font-mono text-mono-sm text-foreground">{store.endpoint.region}</p>
                </div>
                {store.endpoint.endpoint_id && (
                  <div>
                    <p className="text-body-sm font-medium text-muted-foreground">Endpoint ID</p>
                    <p className="font-mono text-mono-sm text-foreground">{store.endpoint.endpoint_id}</p>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Step 2 — Active */}
        <div className="flex items-start gap-4 px-5 py-4">
          <div
            className="flex size-8 shrink-0 items-center justify-center rounded-full border mt-0.5"
            style={{
              background: "color-mix(in oklch, var(--color-yellow-600) 12%, transparent)",
              borderColor: "color-mix(in oklch, var(--color-yellow-600) 28%, transparent)",
            }}
          >
            <span className="text-body-sm font-semibold text-yellow-700">2</span>
          </div>
          <div className="flex-1 min-w-0 pt-0.5">
            <p className="text-body font-semibold text-foreground">{cloud?.stepTitle ?? "Approve the endpoint request"}</p>
            <p className="mt-1 text-body-sm text-muted-foreground">{cloud?.description}</p>
            {cloud && (
              <Button variant="outline" size="sm" className="mt-3" onClick={() => window.open(consoleUrl, "_blank")}>
                {cloud.label}
              </Button>
            )}
          </div>
        </div>

        {/* Step 3 — Locked */}
        <div className="flex items-start gap-4 px-5 py-4">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-full border border-border mt-0.5">
            <span className="text-body-sm font-medium text-muted-foreground">3</span>
          </div>
          <div className="flex-1 min-w-0 pt-0.5">
            <p className="text-body font-medium text-faint-foreground">Astro verifies your connection</p>
            <p className="mt-0.5 text-body-sm text-faint-foreground">Happens automatically after you approve.</p>
          </div>
        </div>

      </div>

      {/* Live events */}
      {(store.events ?? []).length > 0 && (
        <div className="space-y-2">
          {(store.events ?? []).map((event, i) => {
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
      )}

      {/* Actions */}
      <div className="flex items-center justify-between">
        <Button variant="ghost" className="pl-0" asChild>
          <Link to={knowledgePath}>&larr; Back to stores</Link>
        </Button>
        <Button asChild>
          <Link to={knowledgeDetailPath(store.name)}>View store &rarr;</Link>
        </Button>
      </div>

    </div>
  );
}

// --- Step 3: Provisioning stage ---

function ProvisioningStage({
  account,
  storeName,
  provider,
  mode,
  onReady,
  onError,
}: {
  account: string;
  storeName: string;
  provider: KnowledgeProvider;
  mode: "managed" | "external";
  onReady: (store: KnowledgeStore) => void;
  onError: (error: string) => void;
}) {
  const { data: store } = useKnowledgeStore(account, storeName);

  // Transition on terminal status
  useEffect(() => {
    if (!store) return;
    if (store.status === "ready") {
      onReady(store);
    } else if (store.status === "error") {
      onError(store.error ?? "Provisioning failed");
    }
  }, [store, onReady, onError]);

  const events: KnowledgeEvent[] = store?.events ?? [];
  const heading = mode === "managed" ? "Provisioning your store" : "Connecting your store";
  const subtitle = mode === "managed"
    ? "Setting up infrastructure. This usually takes a moment."
    : "Verifying connectivity and saving credentials.";

  if (store?.status === "pending-acceptance") {
    return <PendingAcceptanceStage store={store} />;
  }

  return (
    <div className="mx-auto max-w-lg">
      <div className="flex flex-col items-center text-center">
        <Spinner size={40} className="text-teal-600" />
        <h2 className="mt-6 text-heading-4 text-foreground">{heading}</h2>
        <p className="mt-1 text-body-sm text-muted-foreground">{subtitle}</p>
      </div>

      {/* Store card */}
      <div className="mt-8 rounded-lg border border-border bg-surface p-5">
        <div className="flex items-center gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted">
            <ProviderIcon provider={provider} className="size-6" />
          </div>
          <div className="min-w-0">
            <span className="font-medium text-foreground">{storeName}</span>
            <p className="text-body-sm text-muted-foreground">
              {PROVIDER_LABELS[provider]} &middot; {mode === "managed" ? "Managed" : "External"}
            </p>
          </div>
        </div>
      </div>

      {/* Live events log */}
      {events.length > 0 && (
        <div className="mt-4 space-y-2">
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
      )}

      {/* PrivateLink progress */}
      {store?.endpoint && (
        <div className="mt-4">
          <PrivateLinkSection store={store} />
        </div>
      )}
    </div>
  );
}

// --- Step 4: Success stage ---

function SuccessStage({ store }: { store: KnowledgeStore }) {
  const modeLabel = store.mode === "managed" ? "Managed" : "External";
  const yamlSnippet = `knowledge:\n  - store: ${store.name}\n    as: ${store.name.split("-")[0]}`;
  const cliCommand = `ast dev --source ${store.name}`;

  return (
    <div className="mx-auto max-w-lg">
      <div className="fixed inset-0 pointer-events-none z-0">
        <LiveRevealConfetti />
      </div>
      {/* Header */}
      <div className="flex flex-col items-center text-center mb-9 gap-3.5">
        <div className="flex size-12 shrink-0 items-center justify-center rounded-full border-[1.5px] border-teal-600/25 bg-teal-600/10 [animation:ks-pop_0.5s_cubic-bezier(0.34,1.56,0.64,1)_both]">
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
            <path
              d="M4.5 10.5l4 4 7-8"
              strokeWidth="1.8"
              strokeLinecap="round"
              strokeLinejoin="round"
              pathLength="1"
              strokeDasharray="1"
              className="stroke-teal-700 [stroke-dashoffset:1] [animation:ks-check-draw_0.6s_ease-out_0.3s_both]"
            />
          </svg>
        </div>
        <div className="flex flex-col gap-1.5">
          <h2 className="text-heading-1 text-foreground">Store added</h2>
          <p className="text-body text-muted-foreground">
            {store.mode === "managed"
              ? "Your managed store is ready. Bind it to an agent to start using it."
              : "Your store is connected. Bind it to an agent to start using it."}
          </p>
        </div>
      </div>

      <div className="space-y-3 mb-7">
        {/* Store info card */}
        <div className="rounded-lg overflow-hidden border border-border bg-white dark:bg-surface">
          <div className="flex items-center gap-3 px-4 py-4">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted">
              <ProviderIcon provider={store.provider} className="size-5" />
            </div>
            <div className="flex-1 min-w-0">
              <span className="font-medium text-foreground">{store.name}</span>
              <p className="mt-0.5 text-body-sm text-muted-foreground">{PROVIDER_LABELS[store.provider]}</p>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              <Tag color={store.mode === "managed" ? "blue" : "default"}>{modeLabel}</Tag>
              <StatusBadge color="success" indicator>Ready</StatusBadge>
            </div>
          </div>
        </div>

        {/* YAML + CLI card */}
        <div className="rounded-lg overflow-hidden border border-border bg-white dark:bg-surface">
          {/* YAML section */}
          <div className="flex flex-col gap-2.5 px-5 pt-4 pb-3.5 border-b border-border/60">
            <p className="font-medium text-body text-foreground">Use in your agent</p>
            <div className="flex items-center justify-between gap-3 rounded-sm bg-stone-200 px-4 py-2 font-mono text-mono-sm text-foreground">
              <div className="flex-1 min-w-0 overflow-x-auto [scrollbar-width:thin] [scrollbar-color:theme(colors.stone.400)_transparent] [&::-webkit-scrollbar]:h-1 [&::-webkit-scrollbar-track]:bg-transparent [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-stone-400">
                <pre className="whitespace-pre leading-relaxed text-foreground">{yamlSnippet}</pre>
              </div>
              <CopyButton copyText={yamlSnippet} className="shrink-0 border-stone-200 bg-stone-200 hover:bg-stone-200 text-stone-500 hover:text-stone-700" />
            </div>
          </div>
          {/* CLI section */}
          <div className="flex flex-col gap-2.5 px-5 pt-4 pb-3.5">
            <p className="font-medium text-body text-foreground">CLI shortcut</p>
            <div className="flex items-center justify-between gap-3 rounded-sm bg-stone-200 px-4 py-2 font-mono text-mono-sm text-foreground">
              <div className="flex-1 min-w-0 overflow-x-auto [scrollbar-width:thin] [scrollbar-color:theme(colors.stone.400)_transparent] [&::-webkit-scrollbar]:h-1 [&::-webkit-scrollbar-track]:bg-transparent [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-stone-400">
                <code className="whitespace-nowrap text-foreground">
                  <span className="mr-2 text-muted-foreground">$</span>{cliCommand}
                </code>
              </div>
              <CopyButton copyText={cliCommand} className="shrink-0 border-stone-200 bg-stone-200 hover:bg-stone-200 text-stone-500 hover:text-stone-700" />
            </div>
          </div>
        </div>
      </div>

      {/* Actions */}
      <div className="flex items-center justify-between">
        <Button variant="ghost" className="pl-0" asChild>
          <Link to={knowledgePath}>&larr; Back to stores</Link>
        </Button>
        <Button asChild>
          <Link to={knowledgeDetailPath(store.name)}>View store &rarr;</Link>
        </Button>
      </div>
    </div>
  );
}

// --- Step 2: Configure form ---

type FormStep = "form" | "creating" | "provisioning" | "success" | "error";

function ConfigureForm({
  provider,
  account,
}: {
  provider: KnowledgeProvider;
  account: string;
}) {
  const canManage = MANAGED_SET.has(provider);
  const [mode, setMode] = useState<"managed" | "external">(canManage ? "managed" : "external");
  const [step, setStep] = useState<FormStep>("form");
  const [createdStore, setCreatedStore] = useState<KnowledgeStore | null>(null);
  const [submittedName, setSubmittedName] = useState("");
  const [provisionError, setProvisionError] = useState("");

  // Managed form state
  const [name, setName] = useState("");
  const [storage, setStorage] = useState("");
  const [isPublic, setIsPublic] = useState(true);

  // External form state
  const [privateLink, setPrivateLink] = useState(false);
  const [host, setHost] = useState("");
  const [port, setPort] = useState(() => {
    const defaultPort = PROVIDER_PORTS[provider];
    return defaultPort != null ? String(defaultPort) : "";
  });
  const [database, setDatabase] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [skipHealthCheck, setSkipHealthCheck] = useState(false);

  const create = useCreateKnowledgeStore(account);
  const connect = useConnectKnowledgeStore(account);
  const mutation = mode === "managed" ? create : connect;

  const nameError = validateStoreName(name);
  const fields = useMemo(() => PROVIDER_FIELDS[provider], [provider]);

  const hostLabel = privateLink ? "VPC Endpoint Service Name" : "Host";
  const hostPlaceholder = privateLink ? "com.amazonaws.vpce.us-east-1.vpce-svc-0a..." : "db.example.com";
  const hostError = useMemo(() => {
    if (!host || !privateLink) return null;
    if (!host.startsWith("com.amazonaws.vpce.")) return "Must start with com.amazonaws.vpce.";
    return null;
  }, [host, privateLink]);

  const needsPort = mode === "external" && fields.includes("port");
  const canSubmit =
    name &&
    !nameError &&
    !mutation.isPending &&
    (mode === "managed" || (host && !hostError && (!needsPort || port)));

  function onMutationSuccess(store: KnowledgeStore) {
    setSubmittedName(store.name);
    // If already ready (e.g. external connect), skip provisioning
    if (store.status === "ready") {
      setCreatedStore(store);
      setStep("success");
    } else {
      setStep("provisioning");
    }
  }

  const handleProvisionReady = useCallback((store: KnowledgeStore) => {
    setCreatedStore(store);
    setStep("success");
  }, []);

  const handleProvisionError = useCallback((error: string) => {
    setProvisionError(error);
    setStep("error");
  }, []);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;

    setStep("creating");

    if (mode === "managed") {
      create.mutate(
        { name, provider, storage: storage || undefined, public: isPublic },
        { onSuccess: onMutationSuccess, onError: () => setStep("form") },
      );
    } else {
      connect.mutate(
        {
          name,
          provider,
          host,
          port: port ? Number(port) : undefined,
          database: fields.includes("database") ? database || undefined : undefined,
          username: fields.includes("username") ? username || undefined : undefined,
          password: fields.includes("password") ? password || undefined : undefined,
          api_key: fields.includes("api_key") ? apiKey || undefined : undefined,
          private_link: privateLink || undefined,
          skip_health_check: (!privateLink && skipHealthCheck) || undefined,
        },
        { onSuccess: onMutationSuccess, onError: () => setStep("form") },
      );
    }
  }

  if (step === "creating") {
    return (
      <div className="mx-auto flex max-w-md flex-col items-center py-20 text-center">
        <Spinner size={40} className="text-teal-600" />
        <h2 className="mt-6 text-heading-4 text-foreground">
          {mode === "managed" ? "Creating your store..." : "Connecting your store..."}
        </h2>
      </div>
    );
  }

  if (step === "provisioning") {
    return (
      <ProvisioningStage
        account={account}
        storeName={submittedName}
        provider={provider}
        mode={mode}
        onReady={handleProvisionReady}
        onError={handleProvisionError}
      />
    );
  }

  if (step === "error") {
    return (
      <div className="mx-auto flex max-w-md flex-col items-center py-20 text-center">
        <div className="flex size-12 items-center justify-center rounded-full border-2 border-red-200 bg-red-50">
          <ExclamationTriangleIcon className="size-6 text-red-600" />
        </div>
        <h2 className="mt-4 text-heading-4 text-foreground">Provisioning failed</h2>
        <p className="mt-2 text-body-sm text-muted-foreground">{provisionError}</p>
        <div className="mt-6 flex gap-2">
          <Button variant="outline" asChild>
            <Link to={knowledgePath}>Back to Knowledge Stores</Link>
          </Button>
          {submittedName && (
            <Button asChild>
              <Link to={knowledgeDetailPath(submittedName)}>View store details</Link>
            </Button>
          )}
        </div>
      </div>
    );
  }

  if (step === "success" && createdStore) {
    return <SuccessStage store={createdStore} />;
  }

  return (
    <div className="mx-auto max-w-xl">
      <form onSubmit={handleSubmit}>
        <div className="space-y-12">
        {/* Mode selector — only if provider supports managed */}
        {canManage && (
          <FormSection
            title="Mode"
            description={`Choose how this ${PROVIDER_LABELS[provider]} store is hosted.`}
          >
            <div className="space-y-3">
              {(["managed", "external"] as const).map((m) => {
                const selected = mode === m;
                return (
                  <button
                    key={m}
                    type="button"
                    onClick={() => setMode(m)}
                    className={cn(
                      "flex w-full items-center gap-3 rounded-[6px] border px-5 py-4 text-left transition-[border-color,background-color]",
                      selected
                        ? "border-primary/40 bg-primary/5"
                        : "border-border bg-transparent hover:bg-stone-200/50",
                    )}
                  >
                    <div className={cn(
                      "flex size-5 shrink-0 items-center justify-center rounded-full border-2 transition-colors",
                      selected ? "border-primary" : "border-muted-foreground/30",
                    )}>
                      {selected && <div className="size-2.5 rounded-full bg-primary" />}
                    </div>
                    <div className="flex flex-col gap-0.5">
                      <span className="text-[13px] font-medium text-foreground">
                        {m === "managed" ? "Managed by Astro" : "Connect your own"}
                      </span>
                      <span className="text-[12px] text-muted-foreground">
                        {m === "managed"
                          ? "Astro provisions and operates this database. No credentials needed."
                          : "Register an existing instance. Astro stores your credentials securely."}
                      </span>
                    </div>
                  </button>
                );
              })}
            </div>
          </FormSection>
        )}

        {/* Form section */}
        <FormSection
          title={mode === "managed" ? "Configuration" : "Connection"}
          description={
            mode === "managed"
              ? "Provision a managed database for your agents."
              : "Connect your own database. Credentials are encrypted at rest."
          }
        >
          <div className="space-y-5">
            {/* Name */}
            <div>
              <Label htmlFor="ks-name" size="md">Name</Label>
              <Input
                id="ks-name"
                placeholder="my-store"
                value={name}
                onChange={(e) => { setName(e.target.value.replace(/ /g, "-")); mutation.reset(); }}
                autoComplete="off"
                autoFocus
              />
              {name && nameError && <p className="mt-1 text-xs text-destructive">{nameError}</p>}
            </div>

            {mode === "managed" ? (
              <>
                {/* Storage */}
                <div>
                  <Label size="md">Storage</Label>
                  <Select value={storage} onValueChange={setStorage}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select a storage size" />
                    </SelectTrigger>
                    <SelectContent>
                      {STORAGE_OPTIONS.map((s) => (
                        <SelectItem key={s.value} value={s.value}>{s.label}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {/* Make private */}
                <div className="flex items-center justify-between gap-3">
                  <div className="flex flex-col gap-0.5">
                    <span className="text-[13px] font-medium text-foreground">Make private</span>
                    <span className="text-[12px] text-muted-foreground max-w-sm">
                      Disables the public hostname. The store and its migrations will only be reachable
                      from inside your network. Recommended for advanced setups.
                    </span>
                  </div>
                  <Switch checked={!isPublic} onCheckedChange={(v) => setIsPublic(!v)} />
                </div>
              </>
            ) : (
              <>
                {/* PrivateLink card — host+port always inside */}
                <div
                  className={cn(
                    "overflow-hidden rounded-[6px] border transition-[border-color,background-color]",
                    privateLink ? "border-primary/40 bg-primary/5" : "border-border bg-transparent",
                  )}
                >
                  {/* Toggle row */}
                  <div className="flex items-center gap-4 px-5 py-4">
                    <div
                      className={cn(
                        "flex size-9 shrink-0 items-center justify-center rounded-sm transition-colors",
                        privateLink ? "bg-primary/10 text-primary" : "bg-stone-200 text-muted-foreground",
                      )}
                    >
                      <GlobeAltIcon className="size-5" />
                    </div>
                    <div className="flex flex-1 min-w-0 flex-col gap-0.5">
                      <span className="text-[13px] font-medium text-foreground">PrivateLink</span>
                      <span className="text-[12px] text-muted-foreground">
                        Connect via AWS PrivateLink. Traffic stays off the public internet.
                      </span>
                    </div>
                    <Switch checked={privateLink} onCheckedChange={setPrivateLink} />
                  </div>

                  {/* Host + Port + optional skip-test checkbox */}
                  <div
                    className={cn(
                      "border-t bg-surface px-5 py-4 transition-colors",
                      privateLink ? "border-primary/20" : "border-border",
                    )}
                  >
                    <div className="grid grid-cols-[1fr_auto] gap-3">
                      <div>
                        <Label htmlFor="ks-host" size="md">{hostLabel}</Label>
                        <Input
                          id="ks-host"
                          placeholder={hostPlaceholder}
                          value={host}
                          onChange={(e) => setHost(e.target.value)}
                          autoComplete="off"
                        />
                        {privateLink && host && hostError && <p className="mt-1 text-xs text-destructive">{hostError}</p>}
                      </div>
                      {fields.includes("port") && (
                        <div className="w-24">
                          <Label htmlFor="ks-port" size="md">Port</Label>
                          <Input
                            id="ks-port"
                            type="number"
                            min={1}
                            max={65535}
                            placeholder={String(PROVIDER_PORTS[provider] ?? 5432)}
                            value={port}
                            onChange={(e) => setPort(e.target.value)}
                            autoComplete="off"
                          />
                        </div>
                      )}
                    </div>

                    {!privateLink && (
                      <label className="mt-4 flex cursor-pointer items-start gap-2">
                        <input
                          type="checkbox"
                          checked={skipHealthCheck}
                          onChange={(e) => setSkipHealthCheck(e.target.checked)}
                          className="mt-0.5 size-4 shrink-0 accent-primary"
                        />
                        <div className="flex flex-col gap-0.5">
                          <span className="text-[13px] font-medium text-foreground select-none">
                            Skip connection test
                          </span>
                          <span className="text-[12px] text-muted-foreground">
                            Save credentials without verifying Astro can reach the database.
                          </span>
                        </div>
                      </label>
                    )}
                  </div>
                </div>

                {/* Dynamic credential fields */}
                {fields.includes("database") && (
                  <div>
                    <Label htmlFor="ks-db" size="md">Database</Label>
                    <Input
                      id="ks-db"
                      placeholder="mydb"
                      value={database}
                      onChange={(e) => setDatabase(e.target.value)}
                      autoComplete="off"
                    />
                  </div>
                )}
                {fields.includes("username") && (
                  <div>
                    <Label htmlFor="ks-user" size="md">Username</Label>
                    <Input
                      id="ks-user"
                      placeholder="admin"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      autoComplete="off"
                    />
                  </div>
                )}
                {fields.includes("password") && (
                  <div>
                    <Label htmlFor="ks-pass" size="md">Password</Label>
                    <Input
                      id="ks-pass"
                      type="password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      autoComplete="new-password"
                    />
                  </div>
                )}
                {fields.includes("api_key") && (
                  <div>
                    <Label htmlFor="ks-apikey" size="md">API Key</Label>
                    <Input
                      id="ks-apikey"
                      type="password"
                      value={apiKey}
                      onChange={(e) => setApiKey(e.target.value)}
                      autoComplete="new-password"
                    />
                  </div>
                )}
              </>
            )}
          </div>
        </FormSection>
        </div>

        {/* Error */}
        {mutation.isError && (
          <div className="mt-4">
            <ErrorPanel variant="inline">
              {mutation.error instanceof Error ? mutation.error.message : mode === "managed" ? "Failed to create store" : "Failed to connect store"}
            </ErrorPanel>
          </div>
        )}

        {/* Submit */}
        <hr className="border-border mt-12" />
        <div className="mt-12 flex justify-end gap-3">
          <Button type="button" variant="ghost" size="default" asChild>
            <Link to={knowledgePath}>Cancel</Link>
          </Button>
          <Button type="submit" disabled={!canSubmit}>
            {mode === "managed" ? "Create store" : "Connect store"}
          </Button>
        </div>
      </form>
    </div>
  );
}

// --- Page shell ---

function NewKnowledgeStoreContent() {
  const { personalAccount } = useAuth();
  const { validStoredDefault } = useDefaultAccount();
  const [provider, setProvider] = useState<KnowledgeProvider | null>(null);

  const account = validStoredDefault || personalAccount?.name || "";

  return (
    <div className="flex-1 bg-surface">
      {/* Breadcrumb bar */}
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
