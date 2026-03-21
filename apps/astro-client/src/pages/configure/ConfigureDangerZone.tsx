import { useState } from "react";
import { useOutletContext } from "react-router";
import { Button } from "@/components/ui/button";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";
import type { ConfigureContext } from "./types";

export default function ConfigureDangerZone() {
  const { account, deployment } = useOutletContext<ConfigureContext>();
  const [deleteOpen, setDeleteOpen] = useState(false);

  const displayName = deployment.display_name || deployment.name;

  return (
    <>
      <div className="space-y-1 mb-4">
        <h2 className="text-base font-bold text-foreground">Danger Zone</h2>
        <p className="text-[13px] text-faint-foreground">These actions are irreversible</p>
      </div>
      <div className="flex items-center justify-between gap-4 rounded-lg border border-destructive/30 bg-destructive/5 px-5 py-4">
        <div>
          <div className="text-[13px] font-semibold text-foreground">Delete deployment</div>
          <p className="text-[12px] text-muted-foreground">
            Permanently delete {displayName}, tear down all running resources, and remove associated data. This cannot be undone.
          </p>
        </div>
        <Button
          variant="outline"
          className="shrink-0 border-destructive/30 bg-surface text-destructive hover:bg-destructive/[0.08] hover:text-destructive active:bg-destructive/15 active:text-destructive"
          onClick={() => setDeleteOpen(true)}
        >
          Delete deployment
        </Button>
      </div>
      <DeleteDeploymentDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        deploymentId={deployment.id}
        deploymentName={deployment.name}
        displayName={displayName}
        account={account}
      />
    </>
  );
}
