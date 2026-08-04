import { useState, useMemo } from "react";
import { useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
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
import { useConnectKnowledgeStore } from "@/api/queries/knowledge";
import { knowledgeDetailPath } from "@/lib/routes";
import {
  validateStoreName,
  EXTERNAL_PROVIDERS,
  PROVIDER_LABELS,
  PROVIDER_FIELDS,
  PROVIDER_PORTS,
} from "./knowledge-utils";
import type { KnowledgeProvider } from "@/lib/api";
import { ErrorPanel } from "@/components/ui/status-panel";

interface ConnectKnowledgeStoreDialogProps {
  account: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ConnectKnowledgeStoreDialog({ account, open, onOpenChange }: ConnectKnowledgeStoreDialogProps) {
  const [name, setName] = useState("");
  const [provider, setProvider] = useState<KnowledgeProvider | "">("");
  const [privateLink, setPrivateLink] = useState(false);
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [database, setDatabase] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [skipHealthCheck, setSkipHealthCheck] = useState(false);
  const navigate = useNavigate();
  const connect = useConnectKnowledgeStore(account);

  const nameError = validateStoreName(name);
  const fields = useMemo(() => (provider ? PROVIDER_FIELDS[provider as KnowledgeProvider] : []), [provider]);

  const hostLabel = privateLink ? "VPC Endpoint Service Name" : "Host";
  const hostPlaceholder = privateLink ? "com.amazonaws.vpce.us-east-1.vpce-svc-..." : "db.example.com";

  const hostError = useMemo(() => {
    if (!host || !privateLink) return null;
    if (!host.startsWith("com.amazonaws.vpce.")) return "Must start with com.amazonaws.vpce.";
    return null;
  }, [host, privateLink]);

  function handleProviderChange(v: string) {
    setProvider(v as KnowledgeProvider);
    const defaultPort = PROVIDER_PORTS[v as KnowledgeProvider];
    setPort(defaultPort != null ? String(defaultPort) : "");
    // Reset credential fields
    setDatabase("");
    setUsername("");
    setPassword("");
    setApiKey("");
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (nameError || !name || !provider || !host || hostError) return;
    connect.mutate(
      {
        name,
        provider: provider as KnowledgeProvider,
        host,
        port: port ? Number(port) : undefined,
        database: fields.includes("database") ? database || undefined : undefined,
        username: fields.includes("username") ? username || undefined : undefined,
        password: fields.includes("password") ? password || undefined : undefined,
        api_key: fields.includes("api_key") ? apiKey || undefined : undefined,
        private_link: privateLink || undefined,
        skip_health_check: (!privateLink && skipHealthCheck) || undefined,
      },
      {
        onSuccess: (store) => {
          onOpenChange(false);
          resetForm();
          navigate(knowledgeDetailPath(store.name, account));
        },
      }
    );
  }

  function resetForm() {
    setName("");
    setProvider("");
    setPrivateLink(false);
    setHost("");
    setPort("");
    setDatabase("");
    setUsername("");
    setPassword("");
    setApiKey("");
    setSkipHealthCheck(false);
    connect.reset();
  }

  function handleOpenChange(next: boolean) {
    if (!next) resetForm();
    onOpenChange(next);
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Connect external store</DialogTitle>
          <DialogDescription>
            Connect your own database. Credentials are encrypted at rest.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4 py-1">
          <div className="space-y-1.5">
            <label htmlFor="ks-c-name" className="text-sm font-medium">Name</label>
            <Input
              id="ks-c-name"
              placeholder="my-store"
              value={name}
              onChange={(e) => { setName(e.target.value); connect.reset(); }}
              autoComplete="off"
              autoFocus
            />
            {name && nameError && <p className="text-xs text-destructive">{nameError}</p>}
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium">Provider</label>
            <Select value={provider} onValueChange={handleProviderChange}>
              <SelectTrigger>
                <SelectValue placeholder="Select a provider" />
              </SelectTrigger>
              <SelectContent>
                {EXTERNAL_PROVIDERS.map((p) => (
                  <SelectItem key={p} value={p}>{PROVIDER_LABELS[p]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">PrivateLink</p>
              <p className="text-xs text-muted-foreground">Connect via AWS PrivateLink</p>
            </div>
            <Switch checked={privateLink} onCheckedChange={setPrivateLink} />
          </div>

          {provider && (
            <>
              <div className="space-y-1.5">
                <label htmlFor="ks-c-host" className="text-sm font-medium">{hostLabel}</label>
                <Input
                  id="ks-c-host"
                  placeholder={hostPlaceholder}
                  value={host}
                  onChange={(e) => setHost(e.target.value)}
                  autoComplete="off"
                />
                {host && hostError && <p className="text-xs text-destructive">{hostError}</p>}
              </div>

              {fields.includes("port") && (
                <div className="space-y-1.5">
                  <label htmlFor="ks-c-port" className="text-sm font-medium">Port</label>
                  <Input
                    id="ks-c-port"
                    type="number"
                    min={1}
                    max={65535}
                    placeholder="5432"
                    value={port}
                    onChange={(e) => setPort(e.target.value)}
                    autoComplete="off"
                  />
                </div>
              )}

              {fields.includes("database") && (
                <div className="space-y-1.5">
                  <label htmlFor="ks-c-db" className="text-sm font-medium">Database</label>
                  <Input
                    id="ks-c-db"
                    placeholder="mydb"
                    value={database}
                    onChange={(e) => setDatabase(e.target.value)}
                    autoComplete="off"
                  />
                </div>
              )}

              {fields.includes("username") && (
                <div className="space-y-1.5">
                  <label htmlFor="ks-c-user" className="text-sm font-medium">Username</label>
                  <Input
                    id="ks-c-user"
                    placeholder="admin"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    autoComplete="off"
                  />
                </div>
              )}

              {fields.includes("password") && (
                <div className="space-y-1.5">
                  <label htmlFor="ks-c-pass" className="text-sm font-medium">Password</label>
                  <Input
                    id="ks-c-pass"
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="new-password"
                  />
                </div>
              )}

              {fields.includes("api_key") && (
                <div className="space-y-1.5">
                  <label htmlFor="ks-c-apikey" className="text-sm font-medium">API Key</label>
                  <Input
                    id="ks-c-apikey"
                    type="password"
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    autoComplete="new-password"
                  />
                </div>
              )}

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

          {connect.isError && (
            <ErrorPanel variant="inline">
              {connect.error instanceof Error ? connect.error.message : "Failed to connect store"}
            </ErrorPanel>
          )}

          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name || !provider || !host || !!nameError || !!hostError || connect.isPending}>
              {connect.isPending && <Spinner size={14} className="mr-2" />}
              Connect
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
