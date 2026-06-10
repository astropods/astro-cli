import { useState } from "react";
import { Check, Copy, Eye, EyeOff, LockKeyhole } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { FormSection } from "@/components/deploy/FormSection";
import { useKnowledgeCredentials } from "@/api/queries/knowledge";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type { KnowledgeMode, KnowledgeProvider } from "@/lib/api";

const CREDENTIAL_LABELS: Record<string, string> = {
  url: "URL",
  api_key: "API Key",
};

function credentialLabel(key: string): string {
  if (CREDENTIAL_LABELS[key]) return CREDENTIAL_LABELS[key];
  return key.split("_").map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(" ");
}

// Mirrors the server's credential shape per provider+mode so the locked-state
// placeholder renders the same row count as the real data, avoiding a layout
// jump on reveal. Managed: GenerateCredentials in
// internal/knowledgestore/credentials.go (only the providers in
// MANAGED_PROVIDERS are reachable). External: ExternalCredentialKeys in
// internal/knowledgestore/store.go.
const PLACEHOLDER_KEYS: Record<KnowledgeMode, Partial<Record<KnowledgeProvider, string[]>>> = {
  managed: {
    postgres: ["postgres_user", "postgres_password", "postgres_db"],
    redis: ["password"],
    neo4j: ["auth"],
  },
  external: {
    postgres: ["host", "port", "database", "username", "password"],
    mysql: ["host", "port", "database", "username", "password"],
    qdrant: ["host", "port", "api_key"],
    redis: ["host", "port", "password"],
    neo4j: ["host", "port", "username", "password"],
    pinecone: ["host", "api_key"],
  },
};

function CopyIconButton({ value }: { value: string }) {
  const { copy, copied } = useCopyToClipboard(1500);
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="size-7 text-muted-foreground hover:text-foreground"
      onClick={() => void copy(value)}
      aria-label="Copy value"
    >
      {copied ? <Check className="size-4 text-success" /> : <Copy className="size-4" />}
    </Button>
  );
}

export function CredentialsCard({ account, storeName, provider, mode }: { account: string; storeName: string; provider: KnowledgeProvider; mode: KnowledgeMode }) {
  const [enabled, setEnabled] = useState(false);
  const { data, isLoading, isError, error } = useKnowledgeCredentials(account, storeName, enabled);
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});

  const is404 = isError && (error as unknown as { status?: number })?.status === 404;
  const placeholderKeys = PLACEHOLDER_KEYS[mode][provider] ?? [];

  const renderBody = () => {
    if (!enabled) return (
      <div className="relative">
        <div className="rounded-md border border-border divide-y divide-border blur-[1.5px] select-none pointer-events-none opacity-70" aria-hidden>
          {placeholderKeys.map((key) => (
            <div key={key} className="flex items-center px-4 py-5 gap-3">
              <span className="w-28 shrink-0 text-body-sm text-muted-foreground">{credentialLabel(key)}</span>
              <span className="flex-1 min-w-0 truncate font-mono text-mono-sm text-foreground">
                ••••••••••••••••••••••••
              </span>
            </div>
          ))}
        </div>
        <div className="absolute inset-0 flex items-center justify-center">
          <Button variant="outline" size="sm" onClick={() => setEnabled(true)} className="bg-background shadow-sm">
            <LockKeyhole className="size-4" />
            Reveal credentials
          </Button>
        </div>
      </div>
    );
    if (isLoading) return (
      <div className="rounded-md border border-border divide-y divide-border">
        {placeholderKeys.map((key) => (
          <div key={key} className="flex items-center px-4 py-3 gap-3">
            <Skeleton className="h-4 w-20 rounded-sm shrink-0" />
            <Skeleton className="h-4 flex-1 rounded-sm" />
          </div>
        ))}
      </div>
    );
    if (is404) return <p className="text-body-sm text-muted-foreground">Not available. KMS was not configured when this store was created.</p>;
    if (isError || !data || Object.keys(data).length === 0) return <p className="text-body-sm text-muted-foreground">{isError ? "Failed to load credentials." : "No credentials found."}</p>;
    return (
      <div className="space-y-3">
        <div className="rounded-md border border-border divide-y divide-border">
          {Object.entries(data).map(([key, value]) => (
            <div key={key} className="flex items-center px-4 py-2.5 gap-3">
              <span className="w-28 shrink-0 text-body-sm text-muted-foreground">{credentialLabel(key)}</span>
              <span className="flex-1 min-w-0 truncate font-mono text-mono-sm text-foreground">
                {revealed[key] ? value : "••••••••••••••••"}
              </span>
              <div className="flex items-center gap-1 shrink-0">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-7 text-muted-foreground hover:text-foreground"
                  onClick={() => setRevealed((r) => ({ ...r, [key]: !r[key] }))}
                  aria-label={revealed[key] ? "Hide value" : "Show value"}
                >
                  {revealed[key] ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                </Button>
                <CopyIconButton value={value} />
              </div>
            </div>
          ))}
        </div>
        <div className="flex justify-end">
          <Button variant="ghost" size="sm" onClick={() => { setEnabled(false); setRevealed({}); }}>
            <EyeOff className="size-4" />
            Hide credentials
          </Button>
        </div>
      </div>
    );
  };

  return (
    <FormSection
      title="Credentials"
      description="Fetched securely on demand and not stored in your browser."
    >
      {renderBody()}
    </FormSection>
  );
}
