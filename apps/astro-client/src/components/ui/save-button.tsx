import type { ComponentProps } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";

interface SaveButtonProps extends ComponentProps<typeof Button> {
  isSaving?: boolean;
}

export function SaveButton({
  isSaving = false,
  children = "Save",
  disabled,
  ...props
}: SaveButtonProps) {
  return (
    <Button size="sm" disabled={disabled || isSaving} {...props}>
      {isSaving && <Loader2 className="size-3.5 animate-spin" />}
      {children}
    </Button>
  );
}
