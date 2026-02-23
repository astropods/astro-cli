import { Link } from "react-router";
import { Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "./Badge";
import { AgentIdentity } from "./AgentIdentity";
import { IntegrationIconStack } from "./IntegrationIconStack";
import { getIntegrationItems } from "@/lib/integrationIcons";

export interface AgentCardProps {
  slug: string;
  account: string;
  name: string;
  description: string;
  integrations: string[];
  categories: string[];
  ownerPictureUrl?: string;
}

export function AgentCard({
  slug,
  account,
  name,
  description,
  integrations,
  categories,
  ownerPictureUrl,
}: AgentCardProps) {
  const integrationItems = getIntegrationItems(integrations);

  return (
    <div className="@container flex flex-col rounded-sm border border-border bg-card">
      <div className="flex flex-col gap-3 p-3.5">
        {/* Header row: avatar placeholder + integration icons */}
        <div className="flex items-start justify-between">
          <AgentIdentity account={account} name={name} size={56} className="size-14 rounded-sm overflow-hidden" />
          <IntegrationIconStack integrations={integrationItems} max={3} />
        </div>

        {/* Name */}
        <h3 className="text-base font-semibold text-foreground">{name}</h3>

        {/* Description */}
        <p className="text-sm text-muted-foreground line-clamp-3">
          {description}
        </p>

        {/* Category badges */}
        {categories.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {categories.map((category) => (
              <Badge key={category}>{category}</Badge>
            ))}
          </div>
        )}

        {/* Actions */}
        <div className="mt-auto flex flex-col @[200px]:flex-row gap-2 pt-1">
          <Button className="flex-1" asChild>
            <Link to={`/deploy/${slug}`}>Install Agent</Link>
          </Button>
          <Button variant="outline" className="flex-1" asChild>
            <Link to={`/${slug}`}>View details</Link>
          </Button>
        </div>
      </div>

      {/* Footer */}
      <div className="flex items-center gap-2 border-t border-border px-3.5 py-2">
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
    </div>
  );
}
