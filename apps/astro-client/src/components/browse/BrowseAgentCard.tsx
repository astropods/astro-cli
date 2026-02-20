import { Link } from "react-router";
import { Badge } from "@/components/Badge";

export interface BrowseAgentCardProps {
  slug: string;
  account: string;
  name: string;
  description: string;
  categories: string[];
}

export function BrowseAgentCard({
  slug,
  account,
  name,
  description,
  categories,
}: BrowseAgentCardProps) {
  return (
    <Link
      to={`/${slug}`}
      className="flex items-start gap-3 rounded-lg border border-border bg-card p-4 transition-colors hover:bg-stone-200"
    >
      <div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-semibold text-primary">
        {name.charAt(0)}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <h3 className="truncate text-sm font-semibold text-foreground">
          <span className="font-normal text-muted-foreground">{account}/</span>
          {name}
        </h3>
        <p className="line-clamp-2 text-xs text-muted-foreground">
          {description}
        </p>
        {categories.length > 0 && (
          <div className="mt-1 flex flex-wrap gap-1">
            {categories.slice(0, 2).map((category) => (
              <Badge key={category}>{category}</Badge>
            ))}
          </div>
        )}
      </div>
    </Link>
  );
}
