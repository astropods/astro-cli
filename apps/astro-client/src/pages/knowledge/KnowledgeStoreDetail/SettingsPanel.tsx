import { useState } from "react";
import { useNavigate } from "react-router";
import { TriangleAlert } from "lucide-react";
import { DangerZoneItem } from "@/components/settings/DangerZoneItem";
import { DeleteKnowledgeStoreDialog } from "@/components/knowledge/DeleteKnowledgeStoreDialog";
import { EditCredentialsDialog } from "@/components/knowledge/EditCredentialsDialog";
import { knowledgePath } from "@/lib/routes";
import type { KnowledgeStore } from "@/lib/api";
import { CredentialsCard } from "./CredentialsCard";

export function SettingsPanel({ store, account }: { store: KnowledgeStore; account: string }) {
  const navigate = useNavigate();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const isExternal = store.mode === "external";

  return (
    <div className="max-w-2xl mx-auto space-y-12">
      <CredentialsCard account={account} storeName={store.name} provider={store.provider} mode={store.mode} />

      <section className="pt-2">
        <h2 className="flex items-center gap-1.5 font-mono text-mono-sm uppercase tracking-wide text-faint-foreground">
          <TriangleAlert className="size-3.5 shrink-0" />
          Danger Zone
        </h2>
        <hr className="border-border mb-5 mt-2" />
        <div className="space-y-3">
          {isExternal && (
            <DangerZoneItem
              title="Update connection"
              description="Change the host, credentials, or other connection details. New details are verified before they're saved."
              actionLabel="Update connection"
              onAction={() => setEditOpen(true)}
            />
          )}
          <DangerZoneItem
            title="Delete store"
            description="Permanently removes this store and all agent bindings."
            actionLabel="Delete store"
            onAction={() => setDeleteOpen(true)}
          />
        </div>
      </section>

      {isExternal && (
        <EditCredentialsDialog
          open={editOpen}
          onOpenChange={setEditOpen}
          store={store}
          account={account}
        />
      )}

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
