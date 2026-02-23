import { Link } from "react-router";
import { Download } from "lucide-react";
import { Badge } from "@/components/Badge";
import { AgentIdentity } from "@/components/AgentIdentity";

export interface BrowseAgentCardProps {
  slug: string;
  account: string;
  name: string;
  description: string;
  categories: string[];
  ownerPictureUrl?: string;
}

export function BrowseAgentCard({
  slug,
  account,
  name,
  description,
  categories,
  ownerPictureUrl,
}: BrowseAgentCardProps) {
  return (
    <Link
      to={`/${slug}`}
      className="group flex flex-col rounded-sm border border-border bg-white transition-colors hover:bg-stone-50"
    >
      <div className="flex items-start gap-3 p-3">
        <AgentIdentity account={account} name={name} size={40} className="size-10 shrink-0 rounded-sm overflow-hidden" />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h3 className="truncate text-base font-semibold text-foreground transition-colors group-hover:text-primary">
            {name}
          </h3>
          <p className="line-clamp-2 text-sm text-muted-foreground">
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
      </div>
      <div className="flex items-center gap-2 border-t border-border px-3 py-2">
        <Download size={14} className="text-muted-foreground" />
        <span className="text-xs font-mono text-muted-foreground">1.2k</span>
        <div className="flex-1" />
        {ownerPictureUrl ? (
          <img
            src={ownerPictureUrl}
            alt={account}
            className="size-5 rounded-full object-cover"
          />
        ) : (
          <div className="flex size-5 items-center justify-center rounded-full bg-stone-200 text-[10px] font-semibold text-muted-foreground">
            {account.charAt(0).toUpperCase()}
          </div>
        )}
        <span className="text-xs font-mono text-muted-foreground">{account}</span>
      </div>
    </Link>
  );
}
