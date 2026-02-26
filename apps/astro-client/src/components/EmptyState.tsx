import { Link } from "react-router";
import { Button } from "@/components/ui/button";

export interface EmptyStateProps {
  title: string;
  description: string;
  actionLabel: string;
  actionTo: string;
}

export function EmptyState({ title, description, actionLabel, actionTo }: EmptyStateProps) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
      <p className="text-lg font-medium">{title}</p>
      <p className="text-sm text-muted-foreground">{description}</p>
      <Button asChild className="mt-2">
        <Link to={actionTo}>{actionLabel}</Link>
      </Button>
    </div>
  );
}
