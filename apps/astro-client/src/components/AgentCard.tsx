import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Badge } from "./Badge";
import { IntegrationIconStack } from "./IntegrationIconStack";
import { getIntegrationItems } from "@/lib/integrationIcons";

export interface AgentCardProps {
  slug: string;
  account: string;
  name: string;
  description: string;
  integrations: string[];
  categories: string[];
  onInstall?: (slug: string) => void;
}

export function AgentCard({
  slug,
  account,
  name,
  description,
  integrations,
  categories,
  onInstall,
}: AgentCardProps) {
  const integrationItems = getIntegrationItems(integrations);

  return (
    <div className="@container flex flex-col gap-3 rounded-lg border border-border bg-card p-5">
      {/* Header row: avatar placeholder + integration icons */}
      <div className="flex items-start justify-between">
        <div className="flex size-14 items-center justify-center rounded bg-primary/10 text-lg font-semibold text-primary">
          {name.charAt(0)}
        </div>
        <IntegrationIconStack integrations={integrationItems} max={3} />
      </div>

      {/* Name */}
      <h3 className="text-lg font-semibold"><span className="font-normal text-muted-foreground">{account}/</span>{name}</h3>

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
        <Button className="flex-1" onClick={() => onInstall?.(slug)}>
          Install Agent
        </Button>
        <Button variant="outline" className="flex-1" asChild>
          <Link to={`/hire/${slug}`}>View details</Link>
        </Button>
      </div>
    </div>
  );
}
