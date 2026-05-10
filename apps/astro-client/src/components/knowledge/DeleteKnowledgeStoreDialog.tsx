import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useDeleteKnowledgeStore } from "@/api/queries/knowledge";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";
import type { BoundAgent } from "@/lib/api";

interface DeleteKnowledgeStoreDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  storeName: string;
  account: string;
  boundAgents?: BoundAgent[];
  onDeleted?: () => void;
}

export function DeleteKnowledgeStoreDialog({
  open,
  onOpenChange,
  storeName,
  account,
  boundAgents,
  onDeleted,
}: DeleteKnowledgeStoreDialogProps) {
  const [confirmation, setConfirmation] = useState("");
  const remove = useDeleteKnowledgeStore(account);

  const blockingAgents = boundAgents ?? [];
  if (blockingAgents.length > 0) {
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Can&rsquo;t delete &ldquo;{storeName}&rdquo;</DialogTitle>
            <DialogDescription>
              This store is bound to {blockingAgents.length === 1 ? "an active agent" : `${blockingAgents.length} active agents`}.
              Remove the bindings before deleting.
            </DialogDescription>
          </DialogHeader>
          <ul className="max-h-48 overflow-y-auto rounded-md border border-border bg-muted/30 px-3 py-2 text-body-sm">
            {blockingAgents.map((a) => (
              <li key={`${a.deployment_id}:${a.knowledge_name}`} className="flex items-baseline justify-between gap-3 py-0.5">
                <span className="truncate text-foreground">{a.display_name || a.agent_name}</span>
                <span className="shrink-0 font-mono text-mono-sm text-muted-foreground">{a.knowledge_name}</span>
              </li>
            ))}
          </ul>
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="outline">Close</Button>
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }

  const is409 = remove.isError && (remove.error as unknown as { status?: number })?.status === 409;

  return (
    <ConfirmationDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Delete knowledge store "${storeName}"?`}
      description={
        <>
          This will permanently delete the store and all its data.{" "}
          <span className="font-semibold text-destructive">This action cannot be undone.</span>
        </>
      }
      checkboxLabel={
        <>
          I understand that deleting this store is{" "}
          <strong>irreversible</strong> and all data will be destroyed.
        </>
      }
      actionLabel="Delete store"
      pendingLabel="Deleting..."
      error={remove.isError ? (remove.error as Error) : null}
      defaultErrorMessage={
        is409
          ? "This store is bound to active agents. Remove the bindings first."
          : "Failed to delete knowledge store."
      }
      isPending={remove.isPending}
      canConfirm={confirmation === storeName}
      onConfirm={() => {
        remove.mutate(
          { name: storeName },
          { onSuccess: () => { onOpenChange(false); onDeleted?.(); } },
        );
      }}
      onReset={() => {
        setConfirmation("");
        remove.reset();
      }}
    >
      <div>
        <Label size="md">
          Type <span className="font-semibold">&ldquo;{storeName}&rdquo;</span> to confirm
        </Label>
        <Input
          value={confirmation}
          onChange={(e) => setConfirmation(e.target.value)}
          placeholder={storeName}
          autoComplete="off"
        />
      </div>
    </ConfirmationDialog>
  );
}
