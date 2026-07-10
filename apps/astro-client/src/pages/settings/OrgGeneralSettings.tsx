import { useState } from "react";
import { useParams, useNavigate, type MetaFunction } from "react-router";
import { TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAuth } from "@/lib/auth";
import { useUpdateAccountDisplayName } from "@/api/queries";
import { SectionHeader } from "@/components/settings/SettingsShared";
import { ChangeUsernameDialog } from "@/components/settings/ChangeUsernameDialog";
import { ProfileEditor } from "@/components/settings/ProfileEditor";
import { DangerZoneItem } from "@/components/settings/DangerZoneItem";
import { LeaveOrganizationDialog } from "@/components/settings/LeaveOrganizationDialog";
import { DeleteOrganizationDialog } from "@/components/settings/DeleteOrganizationDialog";

export const meta: MetaFunction = () => [{ title: "General - Organization Settings | Astro" }];

function ProfileSection({ readOnly }: { readOnly?: boolean }) {
  const { orgSlug = "" } = useParams();
  const { accounts, patchAccount, refreshUserData } = useAuth();
  const org = accounts.find((a) => a.name === orgSlug);
  const updateDisplayName = useUpdateAccountDisplayName();

  if (!org) return null;

  return (
    <ProfileEditor
      accountName={org.name}
      currentDisplayName={org.display_name ?? ""}
      avatarDialogTitle="Upload organization image"
      currentAvatarUrl={org.avatar_url}
      onSave={async (displayName) => {
        await updateDisplayName.mutateAsync({ account: orgSlug, displayName });
        patchAccount(orgSlug, { display_name: displayName });
        await refreshUserData();
      }}
      isSaving={updateDisplayName.isPending}
      readOnly={readOnly}
      displayNameKind="organization"
    />
  );
}

function AccountSection({ readOnly }: { readOnly?: boolean }) {
  const { orgSlug = "" } = useParams();
  const { refresh } = useAuth();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);

  const handleSuccess = (newName: string) => {
    setOpen(false);
    navigate(`/settings/org/${newName}/general`, { replace: true });
    refresh();
  };

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Label size="md">Username</Label>
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5">
          <span
            className="min-w-0 max-w-full truncate font-mono text-body text-foreground"
            title={`@${orgSlug}`}
          >
            {`@${orgSlug}`}
          </span>
          {readOnly ? (
            <TooltipProvider delayDuration={300}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span>
                    <Button variant="link" className="h-auto p-0 text-body-sm" disabled>
                      Change username
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  You need admin or owner access to edit this
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          ) : (
            <Button
              variant="link"
              className="h-auto p-0 text-body-sm"
              onClick={() => setOpen(true)}
            >
              Change username
            </Button>
          )}
        </div>
        <ChangeUsernameDialog
          currentName={orgSlug}
          open={open}
          onOpenChange={setOpen}
          onSuccess={handleSuccess}
          variant="organization"
        />
      </div>
    </div>
  );
}

function DangerZone({ isAdmin }: { isAdmin: boolean }) {
  const { orgSlug = "" } = useParams();
  const [leaveOpen, setLeaveOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <div className="flex flex-col gap-3">
      <DangerZoneItem
        title="Leave organization"
        description="You will lose access to all organization agents and resources."
        actionLabel="Leave"
        onAction={() => setLeaveOpen(true)}
      />
      <LeaveOrganizationDialog
        orgSlug={orgSlug}
        open={leaveOpen}
        onOpenChange={setLeaveOpen}
      />
      <DangerZoneItem
        title="Delete organization"
        description="Permanently delete this organization and all associated data. This cannot be undone."
        actionLabel="Delete"
        onAction={() => setDeleteOpen(true)}
        disabled={!isAdmin}
        disabledReason="You need admin or owner access to delete this organization"
      />
      <DeleteOrganizationDialog
        orgSlug={orgSlug}
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
      />
    </div>
  );
}

export default function OrgGeneralSettings() {
  const { role } = useAuth();
  const isAdmin = role === "admin" || role === "owner";

  return (
    <>
      <SectionHeader
        title="Account"
        subtitle="Manage your organization's profile and identity"
      />
      <div className="flex flex-col gap-5">
        <ProfileSection readOnly={!isAdmin} />
        <AccountSection readOnly={!isAdmin} />
      </div>
      <section className="pt-8">
        <h3 className="flex items-center gap-1.5 font-mono text-mono-sm uppercase tracking-wide text-faint-foreground">
          <TriangleAlert className="size-3.5 shrink-0" />
          Danger Zone
        </h3>
        <hr className="border-border mb-5 mt-2" />
        <DangerZone isAdmin={isAdmin} />
      </section>
    </>
  );
}
