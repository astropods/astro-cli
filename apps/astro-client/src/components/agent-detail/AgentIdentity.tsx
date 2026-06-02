import { useMemo, useState } from "react";
import { Link, useNavigate, useParams, useLocation } from "react-router";
import { ChevronDown, BookOpen, RotateCw, Share2, Trash2 } from "lucide-react";
import type { AgentDeployment } from "@/lib/api";
import type { CardData } from "astro-trading-card";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import { useRestartDeployment, useDeploymentsSummary } from "@/api/queries/deployments";
import { useBlueprint } from "@/api/queries/blueprints";
import { getBlueprintIntegrations } from "@/lib/blueprint-utils";
import { formatDate } from "@/lib/deployment-utils";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";
import { TradingCardModal } from "@/components/trading-card/TradingCardModal";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuLabel,
  DropdownMenuGroup,
} from "@/components/ui/dropdown-menu";

interface AgentIdentityProps {
  account: string;
  deployment: AgentDeployment;
}

export function AgentIdentity({ account, deployment }: AgentIdentityProps) {
  const avatarBust = useDeploymentAvatarBust(deployment.id);
  const avatarUrl = avatarBust ?? getDeploymentAvatarUrl(deployment.id);
  const displayName = deployment.display_name || deployment.name;
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [restartOpen, setRestartOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const restartMutation = useRestartDeployment(account);
  const navigate = useNavigate();
  const { deploymentId } = useParams<{ deploymentId: string }>();
  const location = useLocation();
  const activeTab = location.pathname.split("/").pop() ?? "monitor";

  // Fetch the blueprint only when the badge modal is opened — keeps the
  // detail page from paying for the query on every mount.
  const { data: blueprint } = useBlueprint(account, deployment.name, { enabled: shareOpen });
  const integrations = blueprint ? getBlueprintIntegrations(blueprint) : [];
  const cardData = useMemo<CardData>(() => {
    const origin = typeof window !== "undefined" ? window.location.origin : "";
    return {
      name: deployment.name,
      displayName: deployment.display_name,
      account,
      avatar: { url: avatarUrl },
      stats: [
        { label: "Deployed", value: formatDate(deployment.created_at) },
        { label: "From", value: `${account}/${deployment.name}` },
      ],
      barcodeId: deployment.id,
      qrUrl: `${origin}/${account}/${deployment.name}`,
    };
  }, [account, avatarUrl, deployment.id, deployment.name, deployment.display_name, deployment.created_at]);

  const { data: summaryData } = useDeploymentsSummary();
  const accounts = (summaryData?.accounts ?? [])
    .map((a) => ({ ...a, deployments: a.deployments.filter((d) => d.id !== deploymentId) }))
    .filter((a) => a.deployments.length > 0);

  return (
    <div className="absolute top-4 left-0 z-20 pl-5">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button aria-label="Agent menu" className="flex cursor-pointer items-center gap-3 rounded-[8px] bg-background p-1 pl-1 pr-2.5 transition-colors hover:bg-background/90 dark:-ml-2 dark:-mt-1.5 dark:rounded-md dark:bg-transparent dark:p-1.5 dark:pl-2 dark:pr-3 dark:hover:bg-white/5">
            <BlueprintIdentity
              account={account}
              name={deployment.name}
              size={32}
              url={avatarUrl}
              className="rounded-sm"
            />
            <span
              className="max-w-[10rem] overflow-hidden whitespace-nowrap text-base font-medium tracking-wide text-foreground [--fade-start:8rem] [--fade-end:10rem] @max-[500px]:hidden min-[1100px]:max-w-[18rem] min-[1100px]:[--fade-start:16rem] min-[1100px]:[--fade-end:18rem]"
              style={{
                maskImage: "linear-gradient(to right, black var(--fade-start), transparent var(--fade-end))",
                WebkitMaskImage: "linear-gradient(to right, black var(--fade-start), transparent var(--fade-end))",
              }}
            >
              {displayName}
            </span>
            <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="flex w-[260px] flex-col">
          {/* Actions for current agent — always visible */}
          <DropdownMenuItem asChild>
            <Link to={`/${account}/${deployment.name}`}>
              <BookOpen className="size-4" />
              View blueprint
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setShareOpen(true)}>
            <Share2 className="size-4" />
            Share agent badge
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onClick={() => setRestartOpen(true)}>
            <RotateCw className="size-4" />
            Restart deployment
          </DropdownMenuItem>
          <DropdownMenuItem variant="destructive" onClick={() => setDeleteOpen(true)}>
            <Trash2 className="size-4" />
            Delete agent
          </DropdownMenuItem>

          {/* Agent quick-switch list — scrollable */}
          {accounts.length > 0 && <DropdownMenuSeparator />}
          <div className="max-h-[300px] overflow-y-auto">
            {accounts.map((acct) => (
              <DropdownMenuGroup key={acct.id}>
                {accounts.length > 1 && (
                  <DropdownMenuLabel className="text-faint-foreground">
                    {acct.display_name || acct.name}
                  </DropdownMenuLabel>
                )}
                {acct.deployments.map((dep) => (
                  <DropdownMenuItem
                    key={dep.id}
                    asChild
                  >
                    <Link to={`/${acct.name}/agents/${dep.id}/${activeTab}`}>
                      <BlueprintIdentity
                        account={acct.name}
                        name={dep.name}
                        size={20}
                        url={dep.avatar_url}
                        className="size-5 shrink-0 rounded-sm"
                      />
                      <span className="truncate">
                        {dep.display_name || dep.name}
                      </span>
                    </Link>
                  </DropdownMenuItem>
                ))}
              </DropdownMenuGroup>
            ))}
          </div>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={restartOpen} onOpenChange={setRestartOpen}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Are you sure?</DialogTitle>
            <DialogDescription>All running containers will be restarted. There may be a brief interruption.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestartOpen(false)}>Cancel</Button>
            <Button variant="destructive" onClick={() => { setRestartOpen(false); restartMutation.mutate({ deploymentId: deployment.id }); }}>Restart</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <DeleteDeploymentDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        deploymentId={deployment.id}
        deploymentName={deployment.name}
        displayName={deployment.display_name}
        account={account}
        onDeleted={() => navigate("/agents")}
      />

      <TradingCardModal
        open={shareOpen}
        onOpenChange={setShareOpen}
        data={cardData}
        avatarColors={deployment.avatar_colors}
        integrations={integrations}
      />
    </div>
  );
}
