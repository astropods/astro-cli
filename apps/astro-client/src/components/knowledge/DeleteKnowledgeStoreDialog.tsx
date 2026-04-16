import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useDeleteKnowledgeStore } from "@/api/queries/knowledge";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";

interface DeleteKnowledgeStoreDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  storeName: string;
  account: string;
  onDeleted?: () => void;
}

export function DeleteKnowledgeStoreDialog({
  open,
  onOpenChange,
  storeName,
  account,
  onDeleted,
}: DeleteKnowledgeStoreDialogProps) {
  const [confirmation, setConfirmation] = useState("");
  const remove = useDeleteKnowledgeStore(account);

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
          {" "}If this store has active agent bindings, deletion will be blocked.
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
