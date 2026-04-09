import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth } from "@/lib/auth";
import { useUpdateProfile } from "@/api/queries";
import { useSavedFlash } from "@/hooks/use-saved-flash";
import { SectionHeader, SavedIndicator } from "@/components/settings/SettingsShared";
import { ProfileEditor } from "@/components/settings/ProfileEditor";
import { ChangeUsernameDialog } from "@/components/settings/ChangeUsernameDialog";
import { DeleteAccountDialog } from "@/components/settings/DeleteAccountDialog";
import { DangerZoneItem } from "@/components/settings/DangerZoneItem";

function ProfileSection() {
  const { personalAccount, refresh } = useAuth();
  const updateProfile = useUpdateProfile();

  if (!personalAccount) return null;

  return (
    <ProfileEditor
      accountName={personalAccount.name}
      currentDisplayName={personalAccount.display_name ?? ""}
      avatarDialogTitle="Upload profile image"
      onSave={async (displayName) => {
        await updateProfile.mutateAsync({ display_name: displayName });
        refresh();
      }}
      isSaving={updateProfile.isPending}
    />
  );
}

function AccountSection() {
  const { user, personalAccount, refresh } = useAuth();
  const [open, setOpen] = useState(false);
  const { showSaved, flash } = useSavedFlash();

  const handleSuccess = () => {
    refresh();
    setOpen(false);
    flash();
  };

  return (
    <div className="flex flex-col gap-5">
      {user && (
        <div className="max-w-sm">
          <Label size="md">Email</Label>
          <Input defaultValue={user.email} disabled />
        </div>
      )}
      <div>
        <Label size="md">Username</Label>
        <div className="flex items-center gap-2">
          <span className="font-mono text-[13px] text-foreground">
            @{personalAccount?.name}
          </span>
          {personalAccount && (
            <ChangeUsernameDialog
              currentName={personalAccount.name}
              open={open}
              onOpenChange={setOpen}
              onSuccess={handleSuccess}
            />
          )}
          <SavedIndicator visible={showSaved} />
        </div>
      </div>
    </div>
  );
}

function DangerZone() {
  const [open, setOpen] = useState(false);

  return (
    <>
      <DangerZoneItem
        title="Delete account"
        description="Permanently delete your account and all associated data. This cannot be undone."
        actionLabel="Delete account"
        onAction={() => setOpen(true)}
      />
      <DeleteAccountDialog open={open} onOpenChange={setOpen} />
    </>
  );
}

export default function AccountSettings() {
  return (
    <>
      <SectionHeader title="Profile" subtitle="Your public identity on Astro" />
      <ProfileSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="Account" subtitle="Email, password, and authentication" />
      <AccountSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="Danger Zone" subtitle="These actions are irreversible" />
      <DangerZone />
    </>
  );
}
