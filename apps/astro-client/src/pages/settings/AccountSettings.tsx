import { useState } from "react";
import type { MetaFunction } from "react-router";
import { TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
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
  const { showSaved: usernameSaved, flash: flashUsername } = useSavedFlash();
  const { showSaved: timezoneSaved, flash: flashTimezone } = useSavedFlash();
  const { timezone, setTimezone } = useLogTimezone();

  const handleUsernameSuccess = () => {
    refresh();
    setOpen(false);
    flashUsername();
  };

  return (
    <div className="flex flex-col gap-4">
      {user && (
        <div className="max-w-sm">
          <Label size="md">Email</Label>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <div>
                  <Input defaultValue={user.email} disabled />
                </div>
              </TooltipTrigger>
              <TooltipContent>Email is managed by your sign-in provider</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      )}
      {personalAccount && (
        <div>
          <Label size="md">Username</Label>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
            <span className="font-mono text-body text-foreground">
              @{personalAccount.name}
            </span>
            <Button
              variant="link"
              className="h-auto p-0 text-body-sm"
              onClick={() => setOpen(true)}
            >
              Change username
            </Button>
            <SavedIndicator visible={usernameSaved} />
          </div>
          <ChangeUsernameDialog
            currentName={personalAccount.name}
            open={open}
            onOpenChange={setOpen}
            onSuccess={handleUsernameSuccess}
          />
        </div>
      )}
      <div className="flex flex-col gap-2 max-w-sm">
        <Label size="md">Timezone</Label>
        <div className="flex items-center gap-3">
          <TimezoneSelect
            value={timezone}
            onValueChange={(tz) => { setTimezone(tz); flashTimezone(); }}
            className="flex-1"
          />
          <SavedIndicator visible={timezoneSaved} />
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
      <SectionHeader
        title="Account"
        subtitle="Manage your profile and preferences"
      />
      <AccountSection />
      <section className="pt-8">
        <h3 className="flex items-center gap-1.5 font-mono text-mono-sm uppercase tracking-wide text-faint-foreground">
          <TriangleAlert className="size-3.5 shrink-0" />
          Danger Zone
        </h3>
        <hr className="border-border mb-5 mt-2" />
        <DangerZone />
      </section>
    </>
  );
}
