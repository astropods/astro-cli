import { useState, useEffect } from "react";
import type { MetaFunction } from "react-router";
import { useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
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
import { useGitHubAccountStatus, useGitHubAccountDisconnect, useGitHubAccountConnect, useGitHubAccountConnections } from "@/api/queries/github";
import { githubKeys } from "@/api/queries/keys";
import { Button } from "@/components/ui/button";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";
import { GitHubIcon } from "@/components/ui/svgs/githubIcon";

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
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const fromOAuth = searchParams.get('github_connected') === 'true';
  const oauthLogin = searchParams.get('github_login') ?? '';

  // Seed the cache from the OAuth callback params so the toggle is visible
  // immediately. initialDataUpdatedAt: 0 marks it stale → background refetch follows.
  const { data: status, isLoading } = useGitHubAccountStatus(account, {
    enabled: !!account,
    initialData: fromOAuth ? { connected: true, github_login: oauthLogin } : undefined,
  });
  const { data: connectionsData } = useGitHubAccountConnections(account, { enabled: !!account && !!status?.connected });
  const disconnect = useGitHubAccountDisconnect(account);
  const connect = useGitHubAccountConnect(account);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const confirmPhrase = `disconnect ${account}`;

  const connected = status?.connected ?? false;
  const connections = connectionsData?.connections ?? [];

  // Clean up OAuth callback params after redirect.
  useEffect(() => {
    if (!fromOAuth) return;
    setSearchParams((p) => { p.delete('github_connected'); p.delete('github_login'); return p; }, { replace: true });
  }, [fromOAuth]);

  const handleConnect = () => {
    const redirectTo = `/settings/account`;
    connect.mutate(redirectTo, {
      onSuccess: (data) => {
        if (data.redirect_url) {
          window.location.href = data.redirect_url;
        }
      },
    });
  };

  const handleDisconnect = () => {
    const previous = queryClient.getQueryData(githubKeys.accountStatus(account));
    queryClient.setQueryData(githubKeys.accountStatus(account), { connected: false });
    setConfirmOpen(false);
    disconnect.mutate(undefined, {
      onError: () => {
        queryClient.setQueryData(githubKeys.accountStatus(account), previous);
      },
    });
  };

  return (
    <>
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <GitHubIcon className="size-5 shrink-0 text-foreground" aria-hidden />
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
        {isLoading ? null : connected ? (
          <Switch
            checked={true}
            disabled={disconnect.isPending}
            onCheckedChange={() => setConfirmOpen(true)}
          />
        ) : (
          <Button variant="outline" size="sm" disabled={connect.isPending} onClick={handleConnect}>
            {connect.isPending ? "Connecting…" : "Connect GitHub"}
          </Button>
        )}
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

      <ConfirmationDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Disconnect GitHub?"
        description="This will remove all repo connections and stop automatic builds for every agent in this account. You'll need to reconnect and relink repos to resume builds."
        checkboxLabel="I understand that disconnecting GitHub will remove all repo connections and stop automatic builds."
        actionLabel="Disconnect"
        pendingLabel="Disconnecting…"
        error={disconnect.isError ? (disconnect.error as Error) : null}
        defaultErrorMessage="Failed to disconnect GitHub. Please try again."
        isPending={disconnect.isPending}
        canConfirm={confirmation === confirmPhrase}
        onConfirm={handleDisconnect}
        onReset={() => { setConfirmation(""); disconnect.reset(); }}
      >
        <div>
          <Label size="md">
            Type <span className="font-semibold">&ldquo;{confirmPhrase}&rdquo;</span> to confirm
          </Label>
          <Input
            value={confirmation}
            onChange={(e) => setConfirmation(e.target.value)}
            placeholder={confirmPhrase}
            autoComplete="off"
          />
        </div>
      </ConfirmationDialog>
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
