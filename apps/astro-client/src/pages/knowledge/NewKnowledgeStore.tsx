import { useState, useMemo, useCallback, useEffect } from "react";
import { Link } from "react-router";
import type { Route } from "./+types/NewKnowledgeStore";
import {
  ArrowLeftIcon,
  GlobeAltIcon,
  CheckIcon,
  ClipboardIcon,
  ExclamationTriangleIcon,
  InformationCircleIcon,
} from "@heroicons/react/24/outline";
import { CircleStackIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
import { knowledgePath, knowledgeDetailPath } from "@/lib/routes";
import { getIntegrationIconUrl } from "@/lib/assets";
import type { KnowledgeProvider, KnowledgeStore, KnowledgeEvent } from "@/lib/api";
import { cn } from "@/lib/utils";

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

const STORAGE_OPTIONS = ["1Gi", "5Gi", "10Gi", "20Gi", "50Gi", "100Gi"];

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

function CopyButton({ text, className }: { text: string; className?: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [text]);

  return (
    <button
      type="button"
      onClick={handleCopy}
      className={cn("shrink-0 rounded p-1.5 text-muted-foreground hover:text-foreground hover:bg-muted transition-colors", className)}
    >
      {copied ? <CheckIcon className="size-4" /> : <ClipboardIcon className="size-4" />}
    </button>
  );
}

// --- Step 1: Provider selection ---

function ProviderList({ onSelect }: { onSelect: (p: KnowledgeProvider) => void }) {
  return (
    <div className="mx-auto max-w-2xl">
      <h2 className="text-heading-1 text-foreground">Choose a provider</h2>
      <p className="mt-1 text-body-sm text-muted-foreground">
        Start with a managed store, or connect an existing database.
      </p>

      <div className="mt-6 space-y-3">
        {ALL_PROVIDERS.map((p) => (
          <button
            key={p}
            type="button"
            onClick={() => onSelect(p)}
            className="flex w-full cursor-pointer items-center gap-4 rounded-lg border border-border bg-popover px-5 py-4 text-left transition-colors hover:bg-muted/40"
          >
            <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted">
              <ProviderIcon provider={p} className="size-6" />
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-medium text-foreground">{PROVIDER_LABELS[p]}</span>
                {MANAGED_SET.has(p) && <Tag color="teal">Managed available</Tag>}
              </div>
              <p className="text-body-sm text-muted-foreground">{PROVIDER_CATEGORIES[p]}</p>
            </div>
          </button>
        ))}
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
      {/* Header */}
      <div className="flex flex-col items-center text-center">
        <div className="flex size-12 items-center justify-center rounded-full border-2 border-teal-200 bg-teal-50">
          <CheckIcon className="size-6 text-teal-600" />
        </div>
        <h2 className="mt-4 text-heading-1 text-foreground">Store added</h2>
        <p className="mt-1 text-body-sm text-muted-foreground">
          {store.mode === "managed"
            ? "Your managed store is ready. Bind it to an agent to start using it."
            : "Your store is connected. Bind it to an agent to start using it."}
        </p>
      </div>

      {/* Store info card */}
      <div className="mt-8 space-y-4">
        <div className="rounded-lg border border-border bg-surface p-5">
          <div className="flex items-center gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted">
              <ProviderIcon provider={store.provider} className="size-6" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-medium text-foreground">{store.name}</span>
                <span className="inline-flex items-center gap-1 text-mono-sm text-teal-700">
                  <span className="size-1.5 rounded-full bg-teal-600" />
                  Online
                </span>
              </div>
              <p className="text-body-sm text-muted-foreground">
                {PROVIDER_LABELS[store.provider]} &middot; {modeLabel}
              </p>
            </div>
          </div>

          <div className="mt-4 border-t border-border pt-4">
            <p className="font-mono text-mono-sm uppercase tracking-wide text-muted-foreground">
              Astro Resource Name
            </p>
            <div className="mt-1 flex items-center justify-between gap-2">
              <code className="text-body-sm text-foreground">{store.arn}</code>
              <CopyButton text={store.arn} />
            </div>
          </div>
        </div>

        {/* YAML snippet */}
        <div className="rounded-lg border border-border bg-surface p-5">
          <div className="flex items-start justify-between gap-2">
            <div>
              <p className="font-medium text-foreground">Use in your agent</p>
              <p className="text-body-sm text-muted-foreground">
                Add this to your astropods.yml to give an agent access.
              </p>
            </div>
            <CopyButton text={yamlSnippet} />
          </div>
          <pre className="mt-3 whitespace-pre rounded-md bg-muted px-4 py-3 font-mono text-mono-sm text-foreground">
            {yamlSnippet}
          </pre>
        </div>

        {/* CLI shortcut */}
        <div className="rounded-lg border border-border bg-surface p-5">
          <div className="flex items-start justify-between gap-2">
            <div>
              <p className="font-medium text-foreground">CLI shortcut</p>
              <p className="text-body-sm text-muted-foreground">Use in local development</p>
            </div>
            <CopyButton text={cliCommand} />
          </div>
          <code className="mt-3 block font-mono text-body-sm text-foreground">{cliCommand}</code>
        </div>
      </div>

      {/* Actions */}
      <div className="mt-8">
        <Button size="lg" className="w-full" asChild>
          <Link to={knowledgeDetailPath(store.name)}>
            View store &rarr;
          </Link>
        </Button>
        <div className="mt-3 text-center">
          <Link
            to={knowledgePath}
            className="text-body-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            Back to Knowledge Stores
          </Link>
        </div>
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
        {/* Mode selector — only if provider supports managed */}
        {canManage && (
          <section className="mb-8">
            <h2 className="text-heading-4 text-foreground">Mode</h2>
            <p className="mt-1 text-body-sm text-muted-foreground">
              Choose how this {PROVIDER_LABELS[provider]} store is hosted.
            </p>
            <div className="mt-4 space-y-3">
              {(["managed", "external"] as const).map((m) => {
                const selected = mode === m;
                return (
                  <button
                    key={m}
                    type="button"
                    onClick={() => setMode(m)}
                    className={cn(
                      "flex w-full items-center gap-3 rounded border px-5 py-4 text-left transition-colors",
                      selected ? "border-teal-600 bg-muted" : "border-border bg-surface hover:border-muted-foreground/30",
                    )}
                  >
                    <div className={cn(
                      "flex size-5 shrink-0 items-center justify-center rounded-full border-2",
                      selected ? "border-teal-600" : "border-muted-foreground/30",
                    )}>
                      {selected && <div className="size-2.5 rounded-full bg-teal-600" />}
                    </div>
                    <div>
                      <p className="font-medium text-foreground">
                        {m === "managed" ? "Managed by Astro" : "Connect your own"}
                      </p>
                      <p className="text-body-sm text-muted-foreground">
                        {m === "managed"
                          ? "Astro provisions and operates this database. No credentials needed."
                          : "Register an existing instance. Astro stores your credentials securely."}
                      </p>
                    </div>
                  </button>
                );
              })}
            </div>
          </section>
        )}

        {/* Form section */}
        <section>
          <h2 className="text-heading-4 text-foreground">
            {mode === "managed" ? "Configuration" : "Connection"}
          </h2>
          <p className="mt-1 text-body-sm text-muted-foreground">
            {mode === "managed"
              ? "Provision a managed database for your agents."
              : "Connect your own database. Credentials are encrypted at rest."}
          </p>

          <div className="mt-4 border-t border-border pt-5 space-y-5">
            {/* Name */}
            <div className="space-y-1.5">
              <label htmlFor="ks-name" className="text-sm font-medium">Name</label>
              <Input
                id="ks-name"
                placeholder="my-store"
                value={name}
                onChange={(e) => { setName(e.target.value); mutation.reset(); }}
                autoComplete="off"
                autoFocus
              />
              {name && nameError && <p className="text-xs text-destructive">{nameError}</p>}
            </div>

            {mode === "managed" ? (
              <>
                {/* Storage */}
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">Storage</label>
                  <Select value={storage} onValueChange={setStorage}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select a storage size" />
                    </SelectTrigger>
                    <SelectContent>
                      {STORAGE_OPTIONS.map((s) => (
                        <SelectItem key={s} value={s}>{s}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {/* Make private */}
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-medium">Make private</p>
                    <p className="text-xs text-muted-foreground max-w-sm">
                      Removes the public DNS hostname. You'll need to run migrations and access the
                      store from inside your network — recommended only for advanced setups.
                    </p>
                  </div>
                  <Switch checked={!isPublic} onCheckedChange={(v) => setIsPublic(!v)} />
                </div>
              </>
            ) : (
              <>
                {/* PrivateLink card — host+port always inside */}
                <div className="overflow-hidden rounded border border-border">
                  {/* Toggle row */}
                  <div className="flex items-center gap-3 bg-muted px-5 py-4">
                    <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-surface">
                      <GlobeAltIcon className="size-5 text-muted-foreground" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium">PrivateLink</p>
                      <p className="text-xs text-muted-foreground">
                        Connect via AWS PrivateLink — traffic stays on your provider's backbone.
                      </p>
                    </div>
                    <Switch checked={privateLink} onCheckedChange={setPrivateLink} />
                  </div>

                  {/* Host + Port */}
                  <div className="grid grid-cols-[1fr_auto] gap-3 border-t border-border bg-surface px-5 py-4">
                    <div className="space-y-1.5">
                      <label htmlFor="ks-host" className="text-sm font-medium">{hostLabel}</label>
                      <Input
                        id="ks-host"
                        placeholder={hostPlaceholder}
                        value={host}
                        onChange={(e) => setHost(e.target.value)}
                        autoComplete="off"
                      />
                      {privateLink && host && hostError && <p className="text-xs text-destructive">{hostError}</p>}
                    </div>
                    {fields.includes("port") && (
                      <div className="space-y-1.5 w-24">
                        <label htmlFor="ks-port" className="text-sm font-medium">Port</label>
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
                </div>

                {/* Dynamic credential fields */}
                {fields.includes("database") && (
                  <div className="space-y-1.5">
                    <label htmlFor="ks-db" className="text-sm font-medium">Database</label>
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
                  <div className="space-y-1.5">
                    <label htmlFor="ks-user" className="text-sm font-medium">Username</label>
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
                  <div className="space-y-1.5">
                    <label htmlFor="ks-pass" className="text-sm font-medium">Password</label>
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
                  <div className="space-y-1.5">
                    <label htmlFor="ks-apikey" className="text-sm font-medium">API Key</label>
                    <Input
                      id="ks-apikey"
                      type="password"
                      value={apiKey}
                      onChange={(e) => setApiKey(e.target.value)}
                      autoComplete="new-password"
                    />
                  </div>
                )}

                {/* Skip health check — only when not using PrivateLink */}
                {!privateLink && (
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="text-sm font-medium">Skip health check</p>
                      <p className="text-xs text-muted-foreground">Connect without verifying reachability</p>
                    </div>
                    <Switch checked={skipHealthCheck} onCheckedChange={setSkipHealthCheck} />
                  </div>
                )}
              </>
            )}
          </div>
        </section>

        {/* Error */}
        {mutation.isError && (
          <p className="mt-4 text-xs text-destructive">
            {mutation.error instanceof Error ? mutation.error.message : "Failed to create store"}
          </p>
        )}

        {/* Submit */}
        <div className="mt-8 flex justify-end gap-2">
          <Button type="button" variant="outline" asChild>
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
    <div className="flex-1 bg-muted">
      {/* Breadcrumb bar */}
      <div className="border-b border-border bg-surface px-6 py-3">
        <div className="flex items-center gap-2 text-body-sm">
          <Link
            to={knowledgePath}
            className="flex items-center gap-1 text-muted-foreground hover:text-foreground transition-colors"
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
