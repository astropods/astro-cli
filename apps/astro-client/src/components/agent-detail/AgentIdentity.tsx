import { useMemo, useState } from "react";
import { Link, useNavigate, useLocation } from "react-router";
import { BookOpen, RotateCw, Share2, Trash2 } from "lucide-react";
import type { AgentDeployment } from "@/lib/api";
import type { CardData } from "astro-trading-card";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import { useRestartDeployment } from "@/api/queries/deployments";
import { AgentDeploymentMenu } from "@/components/agent-detail/AgentDeploymentMenu";
import { useBlueprint } from "@/api/queries/blueprints";
import { getBlueprintIntegrations } from "@/lib/blueprint-utils";
import { formatDate } from "@/lib/deployment-utils";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";
import { TradingCardModal } from "@/components/trading-card/TradingCardModal";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";

interface AgentIdentityProps {
  account: string;
  deployment: AgentDeployment;
}

export function AgentIdentity({ account, deployment }: AgentIdentityProps) {
  const avatarBust = useDeploymentAvatarBust(deployment.id);
  const avatarUrl = avatarBust ?? getDeploymentAvatarUrl(deployment.id);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [restartOpen, setRestartOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const restartMutation = useRestartDeployment(account);
  const navigate = useNavigate();
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

  return (
    <div className="absolute top-4 left-0 z-20 pl-5">
      <AgentDeploymentMenu
        account={account}
        deployment={{ ...deployment, avatar_url: avatarUrl }}
        variant="detail"
        getDeploymentPath={(acct, dep) =>
          `/${acct}/agents/${dep.id}/${activeTab}`
        }
        menuPrefix={
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
        }
      />

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
