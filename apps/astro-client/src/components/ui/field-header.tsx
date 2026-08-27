import type { HTMLAttributes, ReactNode } from "react";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

interface FieldHeaderProps extends HTMLAttributes<HTMLDivElement> {
  label: ReactNode;
  description: ReactNode;
  htmlFor?: string;
  labelId?: string;
  descriptionId?: string;
  adornment?: ReactNode;
  action?: ReactNode;
}

function FieldHeader({
  label,
  description,
  htmlFor,
  labelId,
  descriptionId,
  adornment,
  action,
  className,
  ...props
}: FieldHeaderProps) {
  return (
    <div className={cn("mb-3 flex items-start justify-between gap-3", className)} {...props}>
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          <Label id={labelId} htmlFor={htmlFor} size="md" className="mb-0">
            {label}
          </Label>
          {adornment}
        </div>
        <p id={descriptionId} className="mt-0.5 text-body-sm text-muted-foreground">
          {description}
        </p>
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}

export { FieldHeader };
export type { FieldHeaderProps };
