import { useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { DestructiveConfirmCheckbox } from "@/components/ui/destructive-confirm-checkbox";

export interface ConfirmationDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: ReactNode;
  checkboxLabel: ReactNode;
  actionLabel: string;
  pendingLabel: string;
  error?: Error | null;
  defaultErrorMessage: string;
  isPending: boolean;
  canConfirm: boolean;
  onConfirm: () => void;
  onReset?: () => void;
  trigger?: ReactNode;
  children?: ReactNode;
}

export function ConfirmationDialog({
  open,
  onOpenChange,
  title,
  description,
  checkboxLabel,
  actionLabel,
  pendingLabel,
  error,
  defaultErrorMessage,
  isPending,
  canConfirm,
  onConfirm,
  onReset,
  trigger,
  children,
}: ConfirmationDialogProps) {
  const [confirmed, setConfirmed] = useState(false);

  const handleOpenChange = (o: boolean) => {
    onOpenChange(o);
    if (!o) {
      setConfirmed(false);
      onReset?.();
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {children}
        <DestructiveConfirmCheckbox checked={confirmed} onChange={setConfirmed}>
          {checkboxLabel}
        </DestructiveConfirmCheckbox>
        {error && (
          <p className="text-[13px] text-destructive">
            {error.message || defaultErrorMessage}
          </p>
        )}
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">Cancel</Button>
          </DialogClose>
          <Button
            variant="destructive"
            disabled={!canConfirm || !confirmed || isPending}
            onClick={onConfirm}
          >
            {isPending ? pendingLabel : actionLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
