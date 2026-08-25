import { useDeletePaymentMethod } from "@/api/queries";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";

interface RemovePaymentMethodDialogProps {
  account: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Opens the add-card flow, which replaces the saved card in one step. */
  onUpdateCard: () => void;
}

export function RemovePaymentMethodDialog({
  account,
  open,
  onOpenChange,
  onUpdateCard,
}: RemovePaymentMethodDialogProps) {
  const removeCard = useDeletePaymentMethod(account);

  return (
    <ConfirmationDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Remove payment method"
      description={
        <>
          Your account returns to the free tier and{" "}
          <span className="font-semibold text-destructive">
            running agents stop
          </span>
          . To change cards, use{" "}
          <button
            type="button"
            className="text-foreground-accent underline decoration-foreground-accent/40 underline-offset-4 hover:decoration-foreground-accent"
            onClick={() => {
              onOpenChange(false);
              onUpdateCard();
            }}
          >
            Update card
          </button>{" "}
          instead: the new card replaces this one without leaving the account
          unpayable.
        </>
      }
      checkboxLabel={
        <>
          I understand my running agents will <strong>stop</strong> and this
          account returns to the free tier.
        </>
      }
      actionLabel="Remove payment method"
      pendingLabel="Removing…"
      error={removeCard.isError ? (removeCard.error as Error) : null}
      defaultErrorMessage="Failed to remove payment method."
      isPending={removeCard.isPending}
      canConfirm
      onConfirm={() => {
        removeCard.mutate(undefined, {
          onSuccess: () => onOpenChange(false),
        });
      }}
      onReset={() => removeCard.reset()}
    />
  );
}
