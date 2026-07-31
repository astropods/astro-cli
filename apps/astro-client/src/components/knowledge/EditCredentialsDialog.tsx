import { useEffect, useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { ErrorPanel } from "@/components/ui/status-panel";
import {
  useKnowledgeCredentials,
  useUpdateKnowledgeCredentials,
} from "@/api/queries/knowledge";
import { PROVIDER_FIELDS } from "./knowledge-utils";
import type { KnowledgeStore, UpdateKnowledgeCredentialsInput } from "@/lib/api";

const FIELD_LABELS: Record<string, string> = {
  host: "Host",
  port: "Port",
  database: "Database",
  username: "Username",
  password: "Password",
  api_key: "API Key",
};

const SECRET_FIELDS = new Set(["password", "api_key"]);
// Fields Supabase manages server-side (resolved from the session pooler); the
// server rejects edits to these, so we lock them in the form.
const SUPABASE_LOCKED = new Set(["host", "port", "username"]);

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  store: KnowledgeStore;
  account: string;
}

export function EditCredentialsDialog({ open, onOpenChange, store, account }: Props) {
  const isSupabase = store.annotations?.source === "supabase";
  const fields = useMemo(() => PROVIDER_FIELDS[store.provider] ?? [], [store.provider]);
  const update = useUpdateKnowledgeCredentials(account, store.name);

  // Fetch current credentials (only while open) to prefill the non-secret
  // connection fields. Secrets are never prefilled — they stay blank and are
  // only sent when the user types a new value.
  const { data: current } = useKnowledgeCredentials(account, store.name, open);
  const currentValue = useMemo(() => {
    return (key: string) => current?.[key.toUpperCase()] ?? current?.[key] ?? "";
  }, [current]);

  const [values, setValues] = useState<Record<string, string>>({});

  // Seed non-secret fields once credentials arrive (or reset when reopened).
  useEffect(() => {
    if (!open) return;
    const seed: Record<string, string> = {};
    for (const f of fields) {
      // Locked Supabase fields are re-resolved from the pooler on save, so don't
      // seed them with the (possibly stale) stored value.
      if (isSupabase && SUPABASE_LOCKED.has(f)) seed[f] = "";
      else seed[f] = SECRET_FIELDS.has(f) ? "" : currentValue(f);
    }
    setValues(seed);
    update.reset();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, current]);

  const payload = useMemo<UpdateKnowledgeCredentialsInput>(() => {
    const out: UpdateKnowledgeCredentialsInput = {};
    for (const f of fields) {
      if (isSupabase && SUPABASE_LOCKED.has(f)) continue;
      const val = values[f] ?? "";
      if (SECRET_FIELDS.has(f)) {
        if (val !== "") out[f as keyof UpdateKnowledgeCredentialsInput] = val as never;
      } else if (val !== currentValue(f)) {
        if (f === "port") out.port = Number(val);
        else out[f as keyof UpdateKnowledgeCredentialsInput] = val as never;
      }
    }
    return out;
  }, [fields, values, isSupabase, currentValue]);

  const hasChanges = Object.keys(payload).length > 0;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!hasChanges) return;
    update.mutate(payload, { onSuccess: () => onOpenChange(false) });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Update connection</DialogTitle>
          <DialogDescription>
            Change the connection details for &ldquo;{store.name}&rdquo;. New details are verified before they&apos;re saved.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4 py-1">
          {fields.map((f) => {
            const locked = isSupabase && SUPABASE_LOCKED.has(f);
            const isSecret = SECRET_FIELDS.has(f);
            return (
              <div key={f} className="space-y-1.5">
                <label htmlFor={`ks-cred-${f}`} className="text-sm font-medium">
                  {FIELD_LABELS[f] ?? f}
                </label>
                <Input
                  id={`ks-cred-${f}`}
                  type={isSecret ? "password" : f === "port" ? "number" : "text"}
                  min={f === "port" ? 1 : undefined}
                  max={f === "port" ? 65535 : undefined}
                  value={values[f] ?? ""}
                  disabled={locked}
                  autoComplete={isSecret ? "new-password" : "off"}
                  placeholder={
                    locked ? "Set automatically from Supabase" : isSecret ? "Leave blank to keep current" : undefined
                  }
                  onChange={(e) => setValues((v) => ({ ...v, [f]: e.target.value }))}
                />
                {locked && (
                  <p className="text-xs text-muted-foreground">
                    Managed by Supabase — resolved from the session pooler on save.
                  </p>
                )}
              </div>
            );
          })}

          {store.bound_agents && store.bound_agents.length > 0 && (
            <p className="text-xs text-muted-foreground">
              Bound agents pick up new credentials on their next deployment.
            </p>
          )}

          {update.isError && (
            <ErrorPanel variant="inline">
              {update.error instanceof Error ? update.error.message : "Failed to update credentials"}
            </ErrorPanel>
          )}

          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!hasChanges || update.isPending}>
              {update.isPending && <Spinner size={14} className="mr-2" />}
              Save connection
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
