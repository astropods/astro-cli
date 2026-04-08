import { useState } from "react";
import { useOutletContext } from "react-router";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";
import { DangerZoneItem } from "@/components/settings/DangerZoneItem";
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
      <DangerZoneItem
        title="Delete deployment"
        description={`Permanently delete ${displayName}, tear down all running resources, and remove associated data. This cannot be undone.`}
        actionLabel="Delete deployment"
        onAction={() => setDeleteOpen(true)}
      />
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
