import { useState } from "react";
import { useParams, useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAuth } from "@/lib/auth";
import { useUpdateAccountDisplayName, useRenameAccount } from "@/api/queries";
import { useAccountNameValidation } from "@/hooks/use-account-name";
import { useSavedFlash } from "@/hooks/use-saved-flash";
import { AccountNameInput } from "@/components/AccountNameInput";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";
import { SectionHeader, SavedIndicator } from "@/components/settings/SettingsShared";
import { ProfileEditor } from "@/components/settings/ProfileEditor";
import { DangerZoneItem } from "@/components/settings/DangerZoneItem";
import { LeaveOrganizationDialog } from "@/components/settings/LeaveOrganizationDialog";
import { DeleteOrganizationDialog } from "@/components/settings/DeleteOrganizationDialog";

function ProfileSection({ readOnly }: { readOnly?: boolean }) {
  const { orgSlug = "" } = useParams();
  const { accounts, refresh } = useAuth();
  const org = accounts.find((a) => a.name === orgSlug);
  const updateDisplayName = useUpdateAccountDisplayName();

  if (!org) return null;

  return (
    <ProfileEditor
      accountName={org.name}
      currentDisplayName={org.display_name ?? ""}
      avatarDialogTitle="Upload organization image"
      onSave={async (displayName) => {
        await updateDisplayName.mutateAsync({ account: orgSlug, displayName });
        refresh();
      }}
      isSaving={updateDisplayName.isPending}
      readOnly={readOnly}
    />
  );
}

function AccountSection({ readOnly }: { readOnly?: boolean }) {
  const { orgSlug = "" } = useParams();
  const { refresh } = useAuth();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const renameAccount = useRenameAccount();
  const { isChecking, isAvailable, displayError } = useAccountNameValidation(
    open ? newUsername : "",
  );
  const { showSaved, flash } = useSavedFlash();

  const handleSuccess = (newName: string) => {
    setOpen(false);
    navigate(`/settings/org/${newName}/general`, { replace: true });
    refresh();
    flash();
  };

  return (
    <div className="flex flex-col gap-5">
      <div>
        <Label size="md">Username</Label>
        <div className="flex items-center gap-2">
          <span className="font-mono text-[13px] text-foreground">
            @{orgSlug}
          </span>
          <ConfirmationDialog
            open={open}
            onOpenChange={setOpen}
            title="Change organization username"
            description={
              <>
                Changing the username will break any existing links or CLI
                configurations that reference the current name.
              </>
            }
            checkboxLabel={
              <>
                I understand that changing the username is a destructive action
                and any existing links to this organization on Astro will no
                longer function.
              </>
            }
            actionLabel="Change username"
            pendingLabel="Changing..."
            error={
              renameAccount.isError ? (renameAccount.error as Error) : null
            }
            defaultErrorMessage="Failed to rename organization."
            isPending={renameAccount.isPending}
            canConfirm={isAvailable}
            onConfirm={() => {
              const trimmed = newUsername.trim();
              renameAccount.mutate(
                { account: orgSlug, newName: trimmed },
                { onSuccess: () => handleSuccess(trimmed) },
              );
            }}
            onReset={() => {
              setNewUsername("");
              renameAccount.reset();
            }}
            trigger={
              readOnly ? (
                <TooltipProvider delayDuration={300}>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span>
                        <Button variant="link" className="h-auto p-0 text-[13px]" disabled>
                          Change username
                        </Button>
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>You do not have permission to edit this</TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              ) : (
                <Button variant="link" className="h-auto p-0 text-[13px]">
                  Change username
                </Button>
              )
            }
          >
            <div>
              <Label size="md">New username</Label>
              <AccountNameInput
                value={newUsername}
                onChange={setNewUsername}
                placeholder={orgSlug}
                isChecking={isChecking}
                isAvailable={isAvailable}
                displayError={displayError}
              />
            </div>
          </ConfirmationDialog>
          <SavedIndicator visible={showSaved} />
        </div>
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
        disabledReason="You do not have permission to delete this organization"
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
        title="Profile"
        subtitle="Your organization's public identity on Astro"
      />
      <ProfileSection readOnly={!isAdmin} />
      <hr className="my-2 border-border" />
      <SectionHeader
        title="Account"
        subtitle="Organization username and identity"
      />
      <AccountSection readOnly={!isAdmin} />
      <hr className="my-2 border-border" />
      <SectionHeader
        title="Danger Zone"
        subtitle="These actions may be irreversible"
      />
      <DangerZone isAdmin={isAdmin} />
    </>
  );
}
