import { useState, useMemo, useCallback, useReducer } from "react";
import { Link } from "react-router";
import { ExclamationTriangleIcon, GlobeAltIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Spinner } from "@/components/ui/spinner";
import { FormSection } from "@/components/deploy/FormSection";
import { ErrorPanel } from "@/components/ui/status-panel";
import { useConnectKnowledgeStore } from "@/api/queries/knowledge";
import {
  validateStoreName,
  PROVIDER_FIELDS,
  PROVIDER_PORTS,
} from "@/components/knowledge/knowledge-utils";
import { knowledgePath, knowledgeDetailPath } from "@/lib/routes";
import type { KnowledgeProvider, KnowledgeStore } from "@/lib/api";
import { cn } from "@/lib/utils";
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
                  <Switch checked={connectionFields.privateLink} onCheckedChange={(v) => setConnectionFields((f) => ({ ...f, privateLink: v }))} />
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
                        value={connectionFields.host}
                        onChange={(e) => setConnectionFields((f) => ({ ...f, host: e.target.value }))}
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
                          value={connectionFields.port}
                          onChange={(e) => setConnectionFields((f) => ({ ...f, port: e.target.value }))}
                          autoComplete="off"
                        />
                      </div>
                    )}
                  </div>

                  {!privateLink && (
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
                  )}
                </div>
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
