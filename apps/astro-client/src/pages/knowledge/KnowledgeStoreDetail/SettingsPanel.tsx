import { useState } from "react";
import { useNavigate } from "react-router";
import { Switch } from "@/components/ui/switch";
import { DangerZoneItem } from "@/components/settings/DangerZoneItem";
import { DeleteKnowledgeStoreDialog } from "@/components/knowledge/DeleteKnowledgeStoreDialog";
import { knowledgePath } from "@/lib/routes";
import type { KnowledgeStore } from "@/lib/api";
import { CredentialsCard } from "./CredentialsCard";
import { SettingRow } from "./SettingRow";

export function SettingsPanel({ store, account }: { store: KnowledgeStore; account: string }) {
  const navigate = useNavigate();
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <div className="max-w-2xl space-y-6">
      {store.mode === "managed" && (
        <div className="rounded-md border border-border overflow-hidden divide-y divide-border">
          <div className="px-5 py-3 bg-surface">
            <h3 className="text-heading-4 text-foreground">Configuration</h3>
            <p className="mt-0.5 text-body-sm text-muted-foreground">These settings can't be changed after creation. <a href="mailto:support@astropods.com" className="text-primary dark:text-indigo-300 underline">Contact us</a> if you need to make changes.</p>
          </div>
          <SettingRow label="Storage">
            <span className="font-mono text-mono-sm text-foreground">{store.storage ?? "—"}</span>
          </SettingRow>
          <SettingRow label="Public access" description="Exposes the store on a public DNS hostname for external connections.">
            <div className="flex items-center gap-2">
              <Switch checked={!!(store.public && store.public_host)} disabled />
              {store.public && store.public_host ? (
                <span className="font-mono text-mono-sm text-muted-foreground truncate">{store.public_host}</span>
              ) : (
                <span className="text-body-sm text-muted-foreground">Not enabled</span>
              )}
            </div>
          </SettingRow>
        </div>
      )}

      <CredentialsCard account={account} storeName={store.name} />

      <div className="rounded-md border border-destructive/25 overflow-hidden divide-y divide-destructive/10">
        <div className="px-5 py-3 bg-destructive/5">
          <h3 className="text-heading-4 text-destructive">Danger Zone</h3>
        </div>
        <DangerZoneItem
          variant="inline"
          title="Delete store"
          description="Permanently removes this store and all agent bindings."
          actionLabel="Delete store"
          onAction={() => setDeleteOpen(true)}
        />
      </div>

      <DeleteKnowledgeStoreDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        storeName={store.name}
        account={account}
        boundAgents={store.bound_agents}
        onDeleted={() => navigate(knowledgePath)}
      />
    </div>
  );
}
