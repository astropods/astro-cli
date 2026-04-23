import { useState, useMemo, useCallback, useReducer } from "react";
import { Link } from "react-router";
import { ExclamationTriangleIcon, GlobeAltIcon } from "@heroicons/react/24/outline";
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
import { FormSection } from "@/components/deploy/FormSection";
import { ErrorPanel } from "@/components/ui/status-panel";
import { useCreateKnowledgeStore, useConnectKnowledgeStore } from "@/api/queries/knowledge";
import {
  validateStoreName,
  PROVIDER_LABELS,
  PROVIDER_FIELDS,
  PROVIDER_PORTS,
} from "@/components/knowledge/knowledge-utils";
import { knowledgePath, knowledgeDetailPath } from "@/lib/routes";
import type { KnowledgeProvider, KnowledgeStore } from "@/lib/api";
import { cn } from "@/lib/utils";
import { MANAGED_SET, STORAGE_OPTIONS } from "@/components/knowledge/knowledge-utils";
import { ProvisioningStage } from "./ProvisioningStage";
import { SuccessStage } from "./SuccessStage";

type FormState =
  | { step: "form" }
  | { step: "creating" }
  | { step: "provisioning"; submittedName: string }
  | { step: "success"; store: KnowledgeStore }
  | { step: "error"; submittedName: string; error: string };

export function ConfigureForm({
  provider,
  account,
}: {
  provider: KnowledgeProvider;
  account: string;
}) {
  const canManage = MANAGED_SET.has(provider);
  const [mode, setMode] = useState<"managed" | "external">(canManage ? "managed" : "external");
  const [formState, dispatch] = useReducer((_: FormState, next: FormState) => next, { step: "form" });

  const [name, setName] = useState("");
  const [managedFields, setManagedFields] = useState({ storage: "", isPublic: true });
  const initialExternalFields = { privateLink: false, host: "", port: String(PROVIDER_PORTS[provider] ?? ""), database: "", username: "", password: "", apiKey: "", skipHealthCheck: false };
  const [externalFields, setExternalFields] = useState(initialExternalFields);

  const create = useCreateKnowledgeStore(account);
  const connect = useConnectKnowledgeStore(account);
  const mutation = mode === "managed" ? create : connect;

  const nameError = validateStoreName(name);
  const fields = useMemo(() => PROVIDER_FIELDS[provider], [provider]);

  const { privateLink, host, port } = externalFields;
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

  function handleModeChange(next: "managed" | "external") {
    setMode(next);
    if (next === "managed") setExternalFields(initialExternalFields);
    else setManagedFields({ storage: "", isPublic: true });
  }

  function onMutationSuccess(store: KnowledgeStore) {
    if (store.status === "ready") {
      dispatch({ step: "success", store });
    } else {
      dispatch({ step: "provisioning", submittedName: store.name });
    }
  }

  const handleProvisionReady = useCallback((store: KnowledgeStore) => {
    dispatch({ step: "success", store });
  }, []);

  const handleProvisionError = useCallback((error: string, submittedName: string) => {
    dispatch({ step: "error", submittedName, error });
  }, []);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;

    dispatch({ step: "creating" });

    if (mode === "managed") {
      create.mutate(
        { name, provider, storage: managedFields.storage || undefined, public: managedFields.isPublic },
        { onSuccess: onMutationSuccess, onError: () => dispatch({ step: "form" }) },
      );
    } else {
      const { privateLink, host, port, database, username, password, apiKey, skipHealthCheck } = externalFields;
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
        { onSuccess: onMutationSuccess, onError: () => dispatch({ step: "form" }) },
      );
    }
  }

  if (formState.step === "creating") {
    return (
      <div className="mx-auto flex max-w-md flex-col items-center py-20 text-center">
        <Spinner size={40} className="text-teal-600" />
        <h2 className="mt-6 text-heading-4 text-foreground">
          {mode === "managed" ? "Creating your store..." : "Connecting your store..."}
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
        mode={mode}
        onReady={handleProvisionReady}
        onError={(error) => handleProvisionError(error, formState.submittedName)}
      />
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
            <Link to={knowledgeDetailPath(formState.submittedName)}>View store details</Link>
          </Button>
        </div>
      </div>
    );
  }

  if (formState.step === "success") {
    return <SuccessStage store={formState.store} />;
  }

  return (
    <div className="mx-auto max-w-xl">
      <form onSubmit={handleSubmit}>
        <div className="space-y-12">
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
                      onClick={() => handleModeChange(m)}
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

          <FormSection
            title={mode === "managed" ? "Configuration" : "Connection"}
            description={
              mode === "managed"
                ? "Provision a managed database for your agents."
                : "Connect your own database. Credentials are encrypted at rest."
            }
          >
            <div className="space-y-5">
              <div>
                <Label htmlFor="ks-name" size="md">Name</Label>
                <Input
                  id="ks-name"
                  placeholder="my-store"
                  value={name}
                  onChange={(e) => { setName(e.target.value); mutation.reset(); }}
                  autoComplete="off"
                  autoFocus
                />
                {name && nameError && <p className="mt-1 text-xs text-destructive">{nameError}</p>}
              </div>

              {mode === "managed" ? (
                <>
                  <div>
                    <Label size="md">Storage</Label>
                    <Select value={managedFields.storage} onValueChange={(v) => setManagedFields((f) => ({ ...f, storage: v }))}>
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

                  <div className="flex items-center justify-between gap-3">
                    <div className="flex flex-col gap-0.5">
                      <span className="text-[13px] font-medium text-foreground">Make private</span>
                      <span className="text-[12px] text-muted-foreground max-w-sm">
                        Disables the public hostname. The store and its migrations will only be reachable
                        from inside your network. Recommended for advanced setups.
                      </span>
                    </div>
                    <Switch checked={!managedFields.isPublic} onCheckedChange={(v) => setManagedFields((f) => ({ ...f, isPublic: !v }))} />
                  </div>
                </>
              ) : (
                <>
                  <div
                    className={cn(
                      "overflow-hidden rounded-[6px] border transition-[border-color,background-color]",
                      privateLink ? "border-primary/40 bg-primary/5" : "border-border bg-transparent",
                    )}
                  >
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
                      <Switch checked={externalFields.privateLink} onCheckedChange={(v) => setExternalFields((f) => ({ ...f, privateLink: v }))} />
                    </div>

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
                            value={externalFields.host}
                            onChange={(e) => setExternalFields((f) => ({ ...f, host: e.target.value }))}
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
                              value={externalFields.port}
                              onChange={(e) => setExternalFields((f) => ({ ...f, port: e.target.value }))}
                              autoComplete="off"
                            />
                          </div>
                        )}
                      </div>

                      {!privateLink && (
                        <label className="mt-4 flex cursor-pointer items-start gap-2">
                          <input
                            type="checkbox"
                            checked={externalFields.skipHealthCheck}
                            onChange={(e) => setExternalFields((f) => ({ ...f, skipHealthCheck: e.target.checked }))}
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

                  {fields.includes("database") && (
                    <div>
                      <Label htmlFor="ks-db" size="md">Database</Label>
                      <Input
                        id="ks-db"
                        placeholder="mydb"
                        value={externalFields.database}
                        onChange={(e) => setExternalFields((f) => ({ ...f, database: e.target.value }))}
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
                        value={externalFields.username}
                        onChange={(e) => setExternalFields((f) => ({ ...f, username: e.target.value }))}
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
                        value={externalFields.password}
                        onChange={(e) => setExternalFields((f) => ({ ...f, password: e.target.value }))}
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
                        value={externalFields.apiKey}
                        onChange={(e) => setExternalFields((f) => ({ ...f, apiKey: e.target.value }))}
                        autoComplete="new-password"
                      />
                    </div>
                  )}
                </>
              )}
            </div>
          </FormSection>
        </div>

        {mutation.isError && (
          <div className="mt-4">
            <ErrorPanel variant="inline">
              {mutation.error instanceof Error ? mutation.error.message : mode === "managed" ? "Failed to create store" : "Failed to connect store"}
            </ErrorPanel>
          </div>
        )}

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
