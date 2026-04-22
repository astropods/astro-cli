import { useState } from "react";
import type { MetaFunction } from "react-router";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { TimezoneSelect } from "@/components/ui/timezone-select";
import { useAuth } from "@/lib/auth";
import { useUpdateProfile } from "@/api/queries";
import { useSavedFlash } from "@/hooks/use-saved-flash";
import { useLogTimezone } from "@/lib/timezone";
import { SectionHeader, SavedIndicator } from "@/components/settings/SettingsShared";
import { ProfileEditor } from "@/components/settings/ProfileEditor";
import { ChangeUsernameDialog } from "@/components/settings/ChangeUsernameDialog";
import { DeleteAccountDialog } from "@/components/settings/DeleteAccountDialog";
import { DangerZoneItem } from "@/components/settings/DangerZoneItem";
import { useGitHubAccountStatus, useGitHubAccountDisconnect, useGitHubAccountConnect } from "@/api/queries/github";
import { useGitHubAccountConnections } from "@/api/queries/github";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";

export const meta: MetaFunction = () => [{ title: "Account - Settings | Astro" }];

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

function GitHubSection() {
  const { personalAccount } = useAuth();
  const account = personalAccount?.name ?? "";
  const { data: status, isLoading } = useGitHubAccountStatus(account, { enabled: !!account });
  const { data: connectionsData } = useGitHubAccountConnections(account, { enabled: !!account && !!status?.connected });
  const disconnect = useGitHubAccountDisconnect(account);
  const connect = useGitHubAccountConnect(account);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const connected = status?.connected ?? false;
  const connections = connectionsData?.connections ?? [];

  const handleToggle = (checked: boolean) => {
    if (!checked) {
      setConfirmOpen(true);
    } else {
      const redirectTo = `/${account}/settings/account`;
      connect.mutate(redirectTo, {
        onSuccess: (res) => {
          if (res.redirect_url) window.location.href = res.redirect_url;
        },
      });
    }
  };

  const handleDisconnect = () => {
    disconnect.mutate(undefined, { onSuccess: () => setConfirmOpen(false) });
  };

  return (
    <>
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <svg viewBox="0 0 24 24" className="size-5 shrink-0 text-foreground" fill="currentColor" aria-hidden>
            <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
          </svg>
          <div className="min-w-0">
            {isLoading ? (
              <div className="h-4 w-32 rounded animate-pulse bg-muted" />
            ) : connected ? (
              <>
                <span className="text-[13px] font-medium text-foreground">@{status?.github_login}</span>
                {connections.length > 0 && (
                  <p className="text-[12px] text-muted-foreground mt-0.5">
                    {connections.length} repo{connections.length !== 1 ? "s" : ""} connected
                  </p>
                )}
              </>
            ) : (
              <span className="text-[13px] text-muted-foreground">Not connected</span>
            )}
          </div>
        </div>
        <Switch
          checked={connected}
          disabled={isLoading || disconnect.isPending || connect.isPending}
          onCheckedChange={handleToggle}
        />
      </div>

      {connected && connections.length > 0 && (
        <ul className="mt-3 space-y-1.5">
          {connections.map((c) => (
            <li key={`${c.agent_name}:${c.repo_full_name}`} className="flex items-center gap-2 text-[12px] text-muted-foreground">
              <span className="font-mono">{c.repo_full_name}</span>
              <span className="text-border">·</span>
              <span>{c.agent_name}</span>
            </li>
          ))}
        </ul>
      )}

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Disconnect GitHub?</DialogTitle>
            <DialogDescription>
              This will remove all repo connections and stop automatic builds for every agent in this account. You'll need to reconnect and relink repos to resume builds.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)}>Cancel</Button>
            <Button variant="destructive" onClick={handleDisconnect} disabled={disconnect.isPending}>
              {disconnect.isPending ? "Disconnecting…" : "Disconnect"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
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
      <SectionHeader title="Preferences" subtitle="Display and localization" />
      <PreferencesSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="GitHub" subtitle="Connect your GitHub account to enable automatic builds from your repos." />
      <GitHubSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="Danger Zone" subtitle="These actions are irreversible" />
      <DangerZone />
    </>
  );
}
