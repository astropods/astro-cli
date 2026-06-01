import { useState } from "react";
import type { MetaFunction } from "react-router";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TimezoneSelect } from "@/components/ui/timezone-select";
import { useAuth } from "@/lib/auth";
import { useSavedFlash } from "@/hooks/use-saved-flash";
import { useLogTimezone } from "@/lib/timezone";
import { SectionHeader, SavedIndicator } from "@/components/settings/SettingsShared";
import { ChangeUsernameDialog } from "@/components/settings/ChangeUsernameDialog";
import { DeleteAccountDialog } from "@/components/settings/DeleteAccountDialog";
import { DangerZoneItem } from "@/components/settings/DangerZoneItem";

export const meta: MetaFunction = () => [{ title: "Account - Settings | Astro" }];

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

function PreferencesSection() {
  const { timezone, setTimezone } = useLogTimezone();
  const { showSaved, flash } = useSavedFlash();

  return (
    <div className="flex flex-col gap-2 max-w-sm">
      <Label size="md">Timezone</Label>
      <div className="flex items-center gap-3">
        <TimezoneSelect
          value={timezone}
          onValueChange={(tz) => { setTimezone(tz); flash(); }}
          className="flex-1"
        />
        <SavedIndicator visible={showSaved} />
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
      <SectionHeader title="Account" subtitle="Manage your profile and account settings" />
      <AccountSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="Preferences" subtitle="Display and localization" />
      <PreferencesSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="Danger Zone" subtitle="These actions are irreversible" />
      <DangerZone />
    </>
  );
}
