import { useMemo, useState } from "react";
import { Link } from "react-router";
import { ArrowUp, ChevronDown, ChevronUp, Info } from "lucide-react";
import { useDeployments } from "@/api/queries/deployments";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { useAuth } from "@/lib/auth";
import { formatRelativeTime, shortBuildId } from "@/lib/deployment-utils";
import { deploymentConfigurePath, deploymentPath } from "@/lib/routes";
import type { AgentDeploymentSummary } from "@/lib/api";
import { SidebarSection } from "./SidebarSection";

const VISIBLE_COUNT = 4;

export interface SidebarDeployedAgentsProps {
  account: string;
  blueprintName: string;
  buildIds: string[];
}

export function SidebarDeployedAgents({
  account,
  blueprintName,
  buildIds,
}: SidebarDeployedAgentsProps) {
  const { isAuthenticated, accounts } = useAuth();
  const isMember = isAuthenticated && accounts.some((a) => a.name === account);
  const enabled = isMember && buildIds.length > 0;

  const { data, isLoading } = useDeployments(account, enabled);
  const [open, setOpen] = useState(false);

  const matches = useMemo(() => {
    const set = new Set(buildIds);
    return (data?.deployments ?? [])
      .filter((d) => set.has(d.build_id))
      .sort((a, b) => b.created_at.localeCompare(a.created_at));
  }, [data, buildIds]);

  if (!isMember) return null;
  if (isLoading) return null;
  if (matches.length === 0) return null;

  const visible = matches.slice(0, VISIBLE_COUNT);
  const hidden = matches.slice(VISIBLE_COUNT);

  return (
    <SidebarSection
      title="Deployed agents"
      badge={<span className="text-muted-foreground"><Info className="h-3 w-3" /></span>}
      badgeTooltip="Deployments of this blueprint in accounts you belong to. Instances running an older build can be upgraded to the latest."
      trailing={
        <span className="text-mono-sm font-mono text-faint-foreground">{matches.length}</span>
      }
      bodyClassName="px-2 py-2"
    >
      <ul className="flex flex-col">
        {visible.map((deployment) => (
          <DeployedAgentRow
            key={deployment.id}
            account={account}
            blueprintName={blueprintName}
            deployment={deployment}
          />
        ))}
      </ul>

      {hidden.length > 0 && (
        <Collapsible open={open} onOpenChange={setOpen}>
          <CollapsibleContent>
            <ul className="flex flex-col">
              {hidden.map((deployment) => (
                <DeployedAgentRow
                  key={deployment.id}
                  account={account}
                  blueprintName={blueprintName}
                  deployment={deployment}
                />
              ))}
            </ul>
          </CollapsibleContent>
          <CollapsibleTrigger asChild>
            <button
              type="button"
              className="mt-1 flex w-full items-center justify-center gap-1 px-2 py-1.5 text-body-sm text-muted-foreground hover:text-foreground transition-colors"
            >
              {open ? (
                <>Show less <ChevronUp className="h-3 w-3" /></>
              ) : (
                <>Show {hidden.length} more <ChevronDown className="h-3 w-3" /></>
              )}
            </button>
          </CollapsibleTrigger>
        </Collapsible>
      )}
    </SidebarSection>
  );
}

function DeployedAgentRow({
  account,
  blueprintName,
  deployment,
}: {
  account: string;
  blueprintName: string;
  deployment: AgentDeploymentSummary;
}) {
  const shortBuild = shortBuildId(deployment.build_id);
  const latest = deployment.latest_build_id;
  const isBehind = !!latest && deployment.build_id !== latest;
  const isCurrent = !!latest && deployment.build_id === latest;
  return (
    <li className="flex items-center gap-1 rounded-[3px] pr-1.5">
      <Link
        to={deploymentPath(account, deployment.id)}
        className="group flex min-w-0 flex-1 items-center gap-2.5 rounded-[3px] px-2 py-1.5 focus-visible:bg-card focus-visible:outline-none"
      >
        <BlueprintIdentity
          account={account}
          name={blueprintName}
          size={16}
          className="h-4 w-4 shrink-0 rounded-[3px]"
        />
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="truncate text-body-sm font-medium text-foreground group-hover:underline">
            {deployment.display_name || deployment.name}
          </span>
          <span className="truncate text-mono-sm text-faint-foreground">
            <span className="font-mono">{shortBuild}</span> · {formatRelativeTime(deployment.created_at)}
          </span>
        </span>
      </Link>
      {isBehind ? (
        <Button asChild size="xs" variant="outline" className="shrink-0">
          <Link
            to={`${deploymentConfigurePath(account, deployment.id)}?build=${encodeURIComponent(latest!)}`}
          >
            <ArrowUp className="size-3" />
            Upgrade
          </Link>
        </Button>
      ) : isCurrent ? (
        <span className="shrink-0 pr-1 text-body-sm text-muted-foreground">Latest</span>
      ) : null}
    </li>
  );
}
