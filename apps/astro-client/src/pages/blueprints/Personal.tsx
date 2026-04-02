import { useState } from "react";
import { Navigate } from "react-router";
import { Plus } from "lucide-react";
import type { Route } from "./+types/Personal";
import { AccountBlueprintsList } from "@/components/browse/AccountBlueprintsList";
import { CreateBlueprintDialog } from "@/components/blueprint/CreateBlueprintDialog";
import { Button } from "@/components/ui/button";
import { createServerApi } from "@/lib/api.server";
import { useAuth } from "@/lib/auth";
import { blueprintsPaths } from "@/lib/routes";

export async function loader({ request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const profile = await api.getProfile().catch(() => null);
  const personalAccount = profile?.accounts?.find((a) => a.type === "personal");
  if (!personalAccount) return { accountName: null, blueprintsData: null };
  const blueprintsData = await api.listAccountBlueprints(personalAccount.name).catch(() => null);
  return { accountName: personalAccount.name, blueprintsData };
}

export default function Personal({ loaderData }: Route.ComponentProps) {
  const { isAuthenticated } = useAuth();
  const accountName = loaderData?.accountName;
  const [createOpen, setCreateOpen] = useState(false);

  if (!isAuthenticated || !accountName) {
    return <Navigate to={blueprintsPaths.discover} replace />;
  }

  return (
    <>
      <div className="flex items-center justify-between">
        <h1 className="text-heading-1 text-foreground">Personal blueprints</h1>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="h-4 w-4 mr-1.5" />
          New blueprint
        </Button>
      </div>

      <AccountBlueprintsList
        account={accountName}
        initialData={loaderData?.blueprintsData ?? undefined}
      />

      <CreateBlueprintDialog
        account={accountName}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />
    </>
  );
}
