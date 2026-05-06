import { useState, useEffect } from "react";
import type { MetaFunction } from "react-router";
import { useSearchParams, Link } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { TimezoneSelect } from "@/components/ui/timezone-select";
import { useAuth } from "@/lib/auth";
import { useSavedFlash } from "@/hooks/use-saved-flash";
import { useLogTimezone } from "@/lib/timezone";
import { SectionHeader, SavedIndicator } from "@/components/settings/SettingsShared";
import { ChangeUsernameDialog } from "@/components/settings/ChangeUsernameDialog";
import { DeleteAccountDialog } from "@/components/settings/DeleteAccountDialog";
import { DangerZoneItem } from "@/components/settings/DangerZoneItem";
import { useGitHubAccountStatus, useGitHubAccountDisconnect, useGitHubAccountConnect, useGitHubAccountConnections } from "@/api/queries/github";
import { useSlackAccountStatus, useSlackAccountDisconnect, useSlackAccountConnect } from "@/api/queries/slack";
import { githubKeys, slackKeys } from "@/api/queries/keys";
import { repoHref, repoLabel } from "@/lib/github-utils";
import { Button } from "@/components/ui/button";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";
import { GitHubIcon } from "@/components/ui/svgs/githubIcon";
import { Slack } from "@/components/ui/svgs/slack";
import { AstroIcon } from "@/components/ui/astro-icon";
import { ArrowRight, X } from "lucide-react";

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

function GitHubSection() {
  const { personalAccount } = useAuth();
  const account = personalAccount?.name ?? "";
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const fromOAuth = searchParams.get('github_connected') === 'true';
  const oauthLogin = searchParams.get('github_login') ?? '';

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

  useEffect(() => {
    if (!fromOAuth) return;
    setSearchParams((p) => { p.delete('github_connected'); p.delete('github_login'); return p; }, { replace: true });
  }, [fromOAuth, setSearchParams]);

  const handleConnect = () => {
    connect.mutate("/settings/account", {
      onSuccess: (data) => {
        if (data.redirect_url) {
          window.location.href = data.redirect_url;
        }
      },
    });
  };

  const handleDisconnect = () => {
    const key = githubKeys.accountStatus(account);
    const previous = queryClient.getQueryData(key);
    queryClient.setQueryData(key, { connected: false });
    setConfirmOpen(false);
    disconnect.mutate(undefined, {
      onError: () => {
        queryClient.setQueryData(key, previous);
      },
    });
  };

  return (
    <>
      <div className="border border-border rounded-md bg-card overflow-hidden">
        <div className="flex items-center justify-between gap-4 px-3 py-3">
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
          <div className="border-t border-border divide-y divide-border">
            {connections.map((c) => (
              <div key={`${c.agent_name}:${c.repo_full_name}`} className="flex items-center gap-2.5 px-3 py-2.5 text-[12px] bg-background">
                <a
                  href={repoHref(c.repo_full_name)}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-1.5 text-muted-foreground hover:text-foreground transition-colors"
                >
                  <GitHubIcon className="size-3.5 shrink-0" aria-hidden />
                  <span className="font-mono">{repoLabel(c.repo_full_name)}</span>
                </a>
                <ArrowRight className="size-3 shrink-0 text-muted-foreground" aria-hidden />
                <Link
                  to={`/${account}/${c.agent_name}`}
                  className="flex items-center gap-1.5 text-muted-foreground hover:text-foreground transition-colors"
                >
                  <AstroIcon className="size-3.5 shrink-0" />
                  <span>{c.agent_name}</span>
                </Link>
                <span className="ml-auto text-muted-foreground">
                  Connected on {new Date(c.created_at).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

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

function SlackSection() {
  const { personalAccount } = useAuth();
  const account = personalAccount?.name ?? "";
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();

  const justConnected = searchParams.get("slack_connected") === "true";
  const oauthError = searchParams.get("slack_error") ?? "";

  const { data: status, isLoading } = useSlackAccountStatus(account, {
    enabled: !!account,
  });
  const disconnect = useSlackAccountDisconnect(account);
  const connect = useSlackAccountConnect(account);

  const workspaces = status?.workspaces ?? [];

  // Strip the OAuth params after we've consumed them so a refresh doesn't
  // re-trigger the same UI state (and so a stale slack_error doesn't linger).
  // The status query refetches automatically and surfaces the new mapping.
  useEffect(() => {
    if (!justConnected && !oauthError) return;
    setSearchParams((p) => {
      p.delete("slack_connected");
      p.delete("slack_user");
      p.delete("slack_team");
      p.delete("slack_error");
      return p;
    }, { replace: true });
  }, [justConnected, oauthError, setSearchParams]);

  const handleConnect = () => {
    connect.mutate("/settings/account", {
      onSuccess: (data) => {
        if (data.redirect_url) {
          window.location.href = data.redirect_url;
        }
      },
    });
  };

  const handleDisconnectOne = (teamID: string) => {
    // Optimistic: drop the workspace from the cached list immediately. On
    // error we restore the previous list so the row reappears.
    const key = slackKeys.accountStatus(account);
    const previous = queryClient.getQueryData<typeof status>(key);
    if (previous) {
      queryClient.setQueryData(key, {
        ...previous,
        workspaces: previous.workspaces.filter((w) => w.team_id !== teamID),
      });
    }
    disconnect.mutate(teamID, {
      onError: () => {
        if (previous) queryClient.setQueryData(key, previous);
      },
    });
  };

  return (
    <>
      {oauthError && (
        <div className="mb-2 flex items-center justify-between gap-2 rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2">
          <p className="text-[12px] text-destructive">
            Couldn't link your Slack account ({oauthError}). Try again?
          </p>
        </div>
      )}

      <div className="border border-border rounded-md bg-card overflow-hidden">
        <div className="flex items-center justify-between gap-4 px-3 py-3">
          <div className="flex items-center gap-3 min-w-0">
            <Slack className="size-5 shrink-0" aria-hidden />
            <div className="min-w-0">
              {isLoading ? (
                <div className="h-4 w-32 rounded animate-pulse bg-muted" />
              ) : workspaces.length > 0 ? (
                <p className="text-[12px] text-muted-foreground">
                  {workspaces.length} workspace{workspaces.length !== 1 ? "s" : ""} linked
                </p>
              ) : (
                <span className="text-[13px] text-muted-foreground">Not connected</span>
              )}
            </div>
          </div>
          {!isLoading && (
            <Button variant="outline" size="sm" disabled={connect.isPending} onClick={handleConnect}>
              {connect.isPending ? "Opening Slack…" : workspaces.length > 0 ? "Add workspace" : "Connect Slack"}
            </Button>
          )}
        </div>

        {workspaces.length > 0 && (
          <ul className="border-t border-border divide-y divide-border">
            {workspaces.map((w) => (
              <li
                key={w.team_id}
                className="flex items-center gap-2.5 px-3 py-2.5 text-[12px] bg-background"
              >
                {w.icon ? (
                  <img
                    src={w.icon}
                    alt=""
                    className="size-4 shrink-0 rounded-sm object-cover"
                  />
                ) : (
                  <Slack className="size-3.5 shrink-0" aria-hidden />
                )}
                <span className="font-medium text-foreground">
                  {w.team || w.team_domain || w.team_id}
                </span>
                {w.slack_username && (
                  <>
                    <span className="text-muted-foreground">·</span>
                    <span className="text-muted-foreground">@{w.slack_username}</span>
                  </>
                )}
                <button
                  type="button"
                  aria-label={`Disconnect ${w.team || w.team_id}`}
                  onClick={() => handleDisconnectOne(w.team_id)}
                  disabled={disconnect.isPending}
                  className="ml-auto text-muted-foreground hover:text-destructive transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <X className="size-3.5" />
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
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
      <SectionHeader title="Account" subtitle="Email, username, and authentication" />
      <AccountSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="Preferences" subtitle="Display and localization" />
      <PreferencesSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="GitHub" subtitle="Connect your GitHub account to enable automatic builds from your repos." />
      <GitHubSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="Slack" subtitle="Link Slack workspaces to use deployed agents under your own identity." />
      <SlackSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="Danger Zone" subtitle="These actions are irreversible" />
      <DangerZone />
    </>
  );
}
