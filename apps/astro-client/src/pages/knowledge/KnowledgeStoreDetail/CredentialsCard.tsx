import { useState } from "react";
import { Eye, EyeOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { CopyButton } from "@/components/ui/copy-button";
import { useKnowledgeCredentials } from "@/api/queries/knowledge";

import { SettingRow } from "./SettingRow";

const CREDENTIAL_DESCRIPTIONS: Record<string, string> = {
  host: "The hostname or IP address of the store.",
  port: "The port number to connect on.",
  database: "The database name within the store.",
  api_key: "Secret key for authenticating API requests.",
  url: "The full connection URL including credentials.",
};

export function CredentialsCard({ account, storeName }: { account: string; storeName: string }) {
  const [enabled, setEnabled] = useState(false);
  const { data, isLoading, isError, error } = useKnowledgeCredentials(account, storeName, enabled);
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});

  const is404 = isError && (error as unknown as { status?: number })?.status === 404;

  const renderBody = () => {
    if (!enabled) return null;
    if (isLoading) return <div className="px-5 py-4"><Skeleton className="h-24 w-full rounded-sm" /></div>;
    if (is404) return <div className="px-5 py-4"><p className="text-body-sm text-muted-foreground">Not available — KMS was not configured when this store was created.</p></div>;
    if (isError || !data || Object.keys(data).length === 0) return <div className="px-5 py-4"><p className="text-body-sm text-muted-foreground">{isError ? "Failed to load credentials." : "No credentials found."}</p></div>;
    return Object.entries(data).map(([key, value]) => (
      <SettingRow key={key} label={key.split("_").map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(" ")} description={CREDENTIAL_DESCRIPTIONS[key]}>
        <div className="relative flex items-center">
          <Input value={revealed[key] ? value : "••••••••••••"} readOnly className="pr-16 font-mono cursor-default bg-stone-100 focus-visible:ring-0 focus-visible:border-border" />
          <div className="absolute right-2 flex items-center gap-1">
            <Button type="button" variant="ghost" size="icon" className="size-6 text-muted-foreground hover:text-foreground" onClick={() => setRevealed((r) => ({ ...r, [key]: !r[key] }))}>
              {revealed[key] ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
            </Button>
            <CopyButton copyText={value} className="size-6 p-0 shrink-0" iconClassName="size-3.5" />
          </div>
        </div>
      </SettingRow>
    ));
  };

  return (
    <div className="rounded-md border border-border overflow-hidden divide-y divide-border">
      <div className="flex items-center justify-between px-5 py-3 bg-stone-200 dark:bg-muted">
        <div>
          <h3 className="text-heading-4 text-foreground">Credentials</h3>
          <p className="mt-0.5 text-body-sm text-muted-foreground">Fetched securely on demand and not stored in your browser.</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => { setEnabled((v) => !v); setRevealed({}); }}>
          {enabled ? "Hide credentials" : "View credentials"}
        </Button>
      </div>
      {renderBody()}
    </div>
  );
}
