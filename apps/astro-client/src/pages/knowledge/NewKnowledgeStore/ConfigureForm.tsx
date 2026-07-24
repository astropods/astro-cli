import { useState, useMemo, useCallback, useReducer, type ReactNode } from "react";
import { Link, useNavigate } from "react-router";
import { CheckIcon, ExclamationTriangleIcon, GlobeAltIcon, InformationCircleIcon, LockClosedIcon, Square2StackIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";
import { FormSection } from "@/components/deploy/FormSection";
import { ErrorPanel } from "@/components/ui/status-panel";
import { useConnectKnowledgeStore } from "@/api/queries/knowledge";
import { useActiveAccount } from "@/hooks/use-active-account";
import {
  validateStoreName,
  PROVIDER_FIELDS,
  PROVIDER_PORTS,
  PROVIDER_LABELS,
} from "@/components/knowledge/knowledge-utils";
import { knowledgePath, knowledgeDetailPath } from "@/lib/routes";
import type { KnowledgeProvider, KnowledgeStore } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Tag } from "@/components/Tag";
import { StatusBadge } from "@/components/StatusBadge";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import { ProvisioningStage } from "./ProvisioningStage";
import { LiveRevealConfetti } from "@/components/ui/LiveRevealConfetti";

type FormState =
  | { step: "form" }
  | { step: "creating" }
  | { step: "provisioning"; submittedName: string }
  | { step: "success"; submittedName: string }
  | { step: "error"; submittedName: string; error: string };

export function ConfigureForm({
  provider,
  account,
}: {
  provider: KnowledgeProvider;
  account: string;
}) {
  const navigate = useNavigate();
  const { setCreateDefault } = useActiveAccount();
  const [formState, dispatch] = useReducer((_: FormState, next: FormState) => next, { step: "form" });

  const [name, setName] = useState("");
  const initialConnectionFields = {
    privateLink: false,
    host: "",
    port: String(PROVIDER_PORTS[provider] ?? ""),
    database: "",
    username: "",
    password: "",
    apiKey: "",
    skipHealthCheck: false,
  };
  const [connectionFields, setConnectionFields] = useState(initialConnectionFields);

  const connect = useConnectKnowledgeStore(account);

  const nameError = validateStoreName(name);
  const fields = useMemo(() => PROVIDER_FIELDS[provider], [provider]);

  const { privateLink, host, port } = connectionFields;
  const hostLabel = privateLink ? "VPC Endpoint Service Name" : "Host";
  const hostPlaceholder = privateLink ? "com.amazonaws.vpce.us-east-1.vpce-svc-0a..." : "db.example.com";
  const hostError = useMemo(() => {
    if (!host || !privateLink) return null;
    if (!host.startsWith("com.amazonaws.vpce.")) return "Must start with com.amazonaws.vpce.";
    return null;
  }, [host, privateLink]);

  const needsPort = fields.includes("port");
  const canSubmit =
    name &&
    !nameError &&
    !connect.isPending &&
    host &&
    !hostError &&
    (!needsPort || port);

  function onMutationSuccess(store: KnowledgeStore) {
    setCreateDefault(account);
    if (store.status === "ready") {
      navigate(knowledgeDetailPath(store.name, account), { replace: true });
    } else {
      dispatch({ step: "provisioning", submittedName: store.name });
    }
  }

  const handleProvisionReady = useCallback((store: KnowledgeStore) => {
    dispatch({ step: "success", submittedName: store.name });
  }, []);

  const handleProvisionError = useCallback((error: string, submittedName: string) => {
    dispatch({ step: "error", submittedName, error });
  }, []);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;

    dispatch({ step: "creating" });

    const { privateLink: pl, host: h, port: p, database, username, password, apiKey, skipHealthCheck } = connectionFields;
    connect.mutate(
      {
        name,
        provider,
        host: h,
        port: p ? Number(p) : undefined,
        database: fields.includes("database") ? database || undefined : undefined,
        username: fields.includes("username") ? username || undefined : undefined,
        password: fields.includes("password") ? password || undefined : undefined,
        api_key: fields.includes("api_key") ? apiKey || undefined : undefined,
        private_link: pl || undefined,
        skip_health_check: (!pl && skipHealthCheck) || undefined,
      },
      { onSuccess: onMutationSuccess, onError: () => dispatch({ step: "form" }) },
    );
  }

  if (formState.step === "creating") {
    return (
      <div className="mx-auto flex max-w-md flex-col items-center py-20 text-center">
        <Spinner size={40} className="text-teal-600" />
        <h2 className="mt-6 text-heading-4 text-foreground">
          Connecting your store...
        </h2>
      </div>
    );
  }

  if (formState.step === "provisioning") {
    return (
      <ProvisioningStage
        account={account}
        storeName={formState.submittedName}
        provider={provider}
        onReady={handleProvisionReady}
        onError={(error) => handleProvisionError(error, formState.submittedName)}
      />
    );
  }

  if (formState.step === "success") {
    return (
      <div className="mx-auto max-w-lg">
        <div className="fixed inset-0 pointer-events-none z-0">
          <LiveRevealConfetti />
        </div>

        <div className="relative z-10 flex flex-col items-center text-center mb-9 gap-3.5">
          <div className="flex size-12 shrink-0 items-center justify-center rounded-full border-[1.5px] border-success/25 bg-success/10 [animation:ks-pop_0.5s_cubic-bezier(0.34,1.56,0.64,1)_both]">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
              <path
                d="M4.5 10.5l4 4 7-8"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
                pathLength="1"
                strokeDasharray="1"
                className="text-success stroke-current [stroke-dashoffset:1] [animation:ks-check-draw_0.6s_ease-out_0.3s_both]"
              />
            </svg>
          </div>
          <div className="flex flex-col gap-1.5">
            <h2 className="text-heading-1 text-foreground">Store connected</h2>
            <p className="text-body text-muted-foreground">
              <span className="font-medium text-foreground">{formState.submittedName}</span> is ready to use.
            </p>
          </div>
        </div>

        <div className="relative z-10 rounded-lg overflow-hidden border border-border bg-card dark:bg-surface mb-7">
          <div className="flex flex-col gap-3 px-4 py-4 sm:flex-row sm:items-center">
            <div className="flex items-center gap-3 flex-1 min-w-0">
              <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted">
                <ProviderIcon provider={provider} className="size-5" />
              </div>
              <div className="min-w-0">
                <span className="font-medium text-foreground">{formState.submittedName}</span>
                <p className="mt-0.5 text-body-sm text-muted-foreground">{PROVIDER_LABELS[provider]}</p>
              </div>
            </div>
            <div className="flex items-center gap-2 sm:shrink-0">
              <Tag color="default">External</Tag>
              <StatusBadge color="success" indicator>Ready</StatusBadge>
            </div>
          </div>
        </div>

        <div className="relative z-10 flex flex-col-reverse gap-2">
          <Button variant="ghost" asChild>
            <Link to={knowledgePath}>Back to stores</Link>
          </Button>
          <Button asChild>
            <Link to={knowledgeDetailPath(formState.submittedName, account)}>View store →</Link>
          </Button>
        </div>
      </div>
    );
  }

  if (formState.step === "error") {
    return (
      <div className="mx-auto flex max-w-md flex-col items-center py-20 text-center">
        <div className="flex size-12 items-center justify-center rounded-full border-2 border-red-200 bg-red-50">
          <ExclamationTriangleIcon className="size-6 text-red-600" />
        </div>
        <h2 className="mt-4 text-heading-4 text-foreground">Provisioning failed</h2>
        <p className="mt-2 text-body-sm text-muted-foreground">{formState.error}</p>
        <div className="mt-6 flex gap-2">
          <Button variant="outline" asChild>
            <Link to={knowledgePath}>Back to Knowledge Stores</Link>
          </Button>
          <Button asChild>
            <Link to={knowledgeDetailPath(formState.submittedName, account)}>View store details</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-xl">
      <form onSubmit={handleSubmit}>
        <div className="space-y-12">
          <FormSection
            title="Connection"
            description="Connect your own database. Credentials are encrypted at rest."
          >
            <div className="space-y-5">
              <div>
                <Label htmlFor="ks-name" size="md">Name</Label>
                <Input
                  id="ks-name"
                  placeholder="my-store"
                  value={name}
                  onChange={(e) => { setName(e.target.value); connect.reset(); }}
                  autoComplete="off"
                  autoFocus
                />
                {name && nameError && <p className="mt-1 text-xs text-destructive">{nameError}</p>}
              </div>

              <div className="space-y-2" role="radiogroup" aria-label="Connection path">
                <KnowledgeFormRadioRow
                  selected={!privateLink}
                  onSelect={() => setConnectionFields((f) => ({ ...f, privateLink: false }))}
                  icon={<GlobeAltIcon className="h-5 w-5" />}
                  title="Public"
                  description="Connect using a hostname or IP reachable over the public internet (or your usual network path)."
                >
                  <div className="grid grid-cols-[1fr_auto] gap-3">
                    <div>
                      <Label htmlFor="ks-host" size="md">{hostLabel}</Label>
                      <Input
                        id="ks-host"
                        placeholder={hostPlaceholder}
                        value={connectionFields.host}
                        onChange={(e) => setConnectionFields((f) => ({ ...f, host: e.target.value }))}
                        autoComplete="off"
                      />
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
                          value={connectionFields.port}
                          onChange={(e) => setConnectionFields((f) => ({ ...f, port: e.target.value }))}
                          autoComplete="off"
                        />
                      </div>
                    )}
                  </div>
                  <NatGatewayHint />
                  <label className="mt-4 flex cursor-pointer items-start gap-2">
                    <input
                      type="checkbox"
                      checked={connectionFields.skipHealthCheck}
                      onChange={(e) => setConnectionFields((f) => ({ ...f, skipHealthCheck: e.target.checked }))}
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
                </KnowledgeFormRadioRow>
                <KnowledgeFormRadioRow
                  selected={privateLink}
                  onSelect={() => setConnectionFields((f) => ({ ...f, privateLink: true }))}
                  icon={<LockClosedIcon className="h-5 w-5" />}
                  title="AWS PrivateLink"
                  description="Connect through AWS PrivateLink with your VPC endpoint service name. Traffic stays off the public internet."
                >
                  <div className="grid grid-cols-[1fr_auto] gap-3">
                    <div>
                      <Label htmlFor="ks-host" size="md">{hostLabel}</Label>
                      <Input
                        id="ks-host"
                        placeholder={hostPlaceholder}
                        value={connectionFields.host}
                        onChange={(e) => setConnectionFields((f) => ({ ...f, host: e.target.value }))}
                        autoComplete="off"
                      />
                      {host && hostError && <p className="mt-1 text-xs text-destructive">{hostError}</p>}
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
                          value={connectionFields.port}
                          onChange={(e) => setConnectionFields((f) => ({ ...f, port: e.target.value }))}
                          autoComplete="off"
                        />
                      </div>
                    )}
                  </div>
                </KnowledgeFormRadioRow>
              </div>

              {fields.includes("database") && (
                <div>
                  <Label htmlFor="ks-db" size="md">Database</Label>
                  <Input
                    id="ks-db"
                    placeholder="mydb"
                    value={connectionFields.database}
                    onChange={(e) => setConnectionFields((f) => ({ ...f, database: e.target.value }))}
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
                    value={connectionFields.username}
                    onChange={(e) => setConnectionFields((f) => ({ ...f, username: e.target.value }))}
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
                    value={connectionFields.password}
                    onChange={(e) => setConnectionFields((f) => ({ ...f, password: e.target.value }))}
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
                    value={connectionFields.apiKey}
                    onChange={(e) => setConnectionFields((f) => ({ ...f, apiKey: e.target.value }))}
                    autoComplete="new-password"
                  />
                </div>
              )}
            </div>
          </FormSection>
        </div>

        {connect.isError && (
          <div className="mt-4">
            <ErrorPanel variant="inline">
              {connect.error instanceof Error ? connect.error.message : "Failed to connect store"}
            </ErrorPanel>
          </div>
        )}

        <hr className="border-border mt-12" />
        <div className="mt-12 flex justify-end gap-3">
          <Button type="button" variant="ghost" size="default" asChild>
            <Link to={knowledgePath}>Cancel</Link>
          </Button>
          <Button type="submit" disabled={!canSubmit}>
            Connect store
          </Button>
        </div>
      </form>
    </div>
  );
}

const NAT_GATEWAY_IPS = ["3.213.168.251/32", "13.222.89.6/32"];

function NatGatewayHint() {
  return (
    <div className="mt-3 flex items-start gap-2 text-[12px] text-muted-foreground">
      <InformationCircleIcon className="mt-0.5 size-3.5 shrink-0" aria-hidden />
      <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1">
        <span>If your database restricts inbound traffic, allowlist</span>
        {NAT_GATEWAY_IPS.map((ip, i) => (
          <span key={ip} className="flex items-center gap-1">
            <CopyableIp ip={ip} />
            {i < NAT_GATEWAY_IPS.length - 1 && <span>and</span>}
          </span>
        ))}
        <span>.</span>
      </div>
    </div>
  );
}

function CopyableIp({ ip }: { ip: string }) {
  const { copy, copied } = useCopyToClipboard();
  return (
    <button
      type="button"
      onClick={() => void copy(ip)}
      title={copied ? "Copied!" : "Copy"}
      className={cn(
        "inline-flex items-center gap-1 rounded-[4px] border px-1.5 py-0.5 text-mono-sm transition-colors cursor-pointer",
        copied
          ? "border-success/40 bg-success/10 text-success"
          : "border-border bg-card text-foreground hover:bg-muted-foreground/10",
      )}
    >
      <code>{ip}</code>
      {copied ? <CheckIcon className="size-3" /> : <Square2StackIcon className="size-3 opacity-60" />}
    </button>
  );
}

function KnowledgeFormRadioRow({
  selected,
  onSelect,
  icon,
  title,
  description,
  children,
}: {
  selected: boolean;
  onSelect: () => void;
  icon?: ReactNode;
  title: string;
  description: string;
  children?: ReactNode;
}) {
  const hasNested = selected && children !== undefined;
  return (
    <div
      className={cn(
        hasNested && "rounded-[6px] border transition-[border-color,background-color]",
        hasNested && "border-primary/40 bg-primary/5",
      )}
    >
      <button
        type="button"
        role="radio"
        aria-checked={selected}
        onClick={onSelect}
        className={cn(
          "flex w-full items-center gap-4 rounded-[6px] border px-3 py-3 text-left cursor-pointer transition-[border-color,background-color]",
          hasNested && "border-none bg-transparent hover:bg-transparent",
          !hasNested &&
            (selected
              ? "border-primary/40 bg-primary/5"
              : "border-border bg-transparent hover:bg-muted dark:hover:bg-muted/40"),
        )}
      >
        {icon && (
          <div
            className={cn(
              "flex h-9 w-9 shrink-0 items-center justify-center rounded-sm transition-colors",
              selected ? "bg-primary/10 text-foreground-accent" : "bg-muted text-muted-foreground",
            )}
            aria-hidden
          >
            {icon}
          </div>
        )}
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-[13px] font-medium text-foreground">{title}</span>
          <span className="text-[11px] text-muted-foreground">{description}</span>
        </div>
        <div
          className={cn(
            "flex h-5 w-5 shrink-0 items-center justify-center rounded-full border-2 transition-colors",
            selected ? "border-primary bg-background" : "border-input bg-background",
          )}
          aria-hidden
        >
          {selected && <div className="h-2.5 w-2.5 rounded-full bg-primary" />}
        </div>
      </button>
      {hasNested && (
        <div className="rounded-b-[6px] border-t border-primary/20 bg-surface px-6 py-4">
          {children}
        </div>
      )}
    </div>
  );
}
