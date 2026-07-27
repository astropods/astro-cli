import { useMemo, useState } from "react";
import { Link, useNavigate, useLocation } from "react-router";
import { BookOpen, MoreHorizontal, RotateCw, Share2, Trash2 } from "lucide-react";
import type { AgentDeployment } from "@/lib/api";
import type { CardData } from "astro-trading-card";
import { useDeploymentAvatarUrl } from "@/lib/avatar-bust";
import { useRestartDeployment } from "@/api/queries/deployments";
import { AgentDeploymentMenu } from "@/components/agent-detail/AgentDeploymentMenu";
import { useBlueprint } from "@/api/queries/blueprints";
import { getBlueprintIntegrations } from "@/lib/blueprint-utils";
import { formatDate } from "@/lib/deployment-utils";
import { useMediaBreakpoint } from "@/hooks/use-compact-layout";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";
import { TradingCardModal } from "@/components/trading-card/TradingCardModal";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

interface AgentIdentityProps {
  account: string;
  deployment: AgentDeployment;
}

// Below this width the header crowds, so the actions fold into the selector
// instead of a standalone kebab.
const KEBAB_BREAKPOINT = 1024;

export function AgentIdentity({ account, deployment }: AgentIdentityProps) {
  const avatarUrl = useDeploymentAvatarUrl(deployment);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [restartOpen, setRestartOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const restartMutation = useRestartDeployment(account);
  const navigate = useNavigate();
  const location = useLocation();
  const activeTab = location.pathname.split("/").pop() ?? "monitor";
  const compact = useMediaBreakpoint(KEBAB_BREAKPOINT);

  // Fetch the blueprint only when the badge modal is open.
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
        { label: "From", value: `${account}/${deployment.name}`, wrap: true },
      ],
      barcodeId: deployment.id,
      qrUrl: `${origin}/${account}/${deployment.name}`,
    };
  }, [account, avatarUrl, deployment.id, deployment.name, deployment.display_name, deployment.created_at]);

  const desktopActionItems = (
    <>
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
    </>
  );
  const compactActionItems = (
    <>
      <Button
        asChild
        variant="ghost"
        size="sm"
        className="w-full justify-start font-normal"
      >
        <Link to={`/${account}/${deployment.name}`}>
          <BookOpen className="size-4" />
          View blueprint
        </Link>
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className="w-full justify-start font-normal"
        onClick={() => setShareOpen(true)}
      >
        <Share2 className="size-4" />
        Share agent badge
      </Button>
      <div role="separator" className="my-1 h-px bg-border" />
      <Button
        variant="ghost"
        size="sm"
        className="w-full justify-start font-normal text-destructive hover:bg-destructive/10 hover:text-destructive dark:text-red-400 dark:hover:bg-destructive/20 dark:hover:text-red-400"
        onClick={() => setRestartOpen(true)}
      >
        <RotateCw className="size-4" />
        Restart deployment
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className="w-full justify-start font-normal text-destructive hover:bg-destructive/10 hover:text-destructive dark:text-red-400 dark:hover:bg-destructive/20 dark:hover:text-red-400"
        onClick={() => setDeleteOpen(true)}
      >
        <Trash2 className="size-4" />
        Delete agent
      </Button>
    </>
  );

  return (
    <>
      <div className="pointer-events-auto flex min-w-0 max-w-full items-center gap-1 dark:-ml-2 dark:-mt-1.5">
        <AgentDeploymentMenu
          deployment={deployment}
          showFullName
          triggerClassName="dark:mt-0 dark:ml-0"
          getDeploymentPath={(acct, dep) =>
            `/${acct}/agents/${dep.id}/${activeTab}`
          }
          menuPrefix={compact ? compactActionItems : undefined}
        />
        {!compact && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Agent actions"
                className="text-muted-foreground hover:text-foreground"
              >
                <MoreHorizontal className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-52">
              {desktopActionItems}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>

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
    </>
  );
}
