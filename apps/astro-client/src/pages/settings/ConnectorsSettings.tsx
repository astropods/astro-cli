import { useState, type ReactNode } from "react";
import type { MetaFunction } from "react-router";
import { useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth } from "@/lib/auth";
import { SectionHeader } from "@/components/settings/SettingsShared";
import { ConnectorRow, ConnectorRowList, ConnectorRowItem } from "@/components/settings/ConnectorRow";
import { useGitHubAccountStatus, useGitHubAccountDisconnect, useGitHubAccountConnect, useGitHubAccountOrgs } from "@/api/queries/github";
import { useSlackAccountStatus, useSlackAccountDisconnect, useSlackAccountConnect } from "@/api/queries/slack";
import { useSupabaseStatus, useSupabaseConnect, useSupabaseDisconnect } from "@/api/queries/supabase";
import type { GitHubConnectResponse } from "@/lib/api";
import { githubKeys, slackKeys, supabaseKeys } from "@/api/queries/keys";
import { Button } from "@/components/ui/button";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";
import { GitHubIcon } from "@/components/ui/svgs/githubIcon";
import { Slack } from "@/components/ui/svgs/slack";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import { useCleanupOAuthParams } from "@/hooks/use-cleanup-oauth-params";
import { ArrowUpRight, Building2, Check, MoreHorizontal } from "lucide-react";
import { ErrorPanel } from "@/components/ui/status-panel";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";

export const meta: MetaFunction = () => [{ title: "Connectors - Settings | Astro" }];

const RETURN_PATH = "/settings/connectors";
const GITHUB_OAUTH_PARAMS = ["github_connected", "github_login"] as const;
const SLACK_OAUTH_PARAMS = ["slack_connected", "slack_user", "slack_team", "slack_error"] as const;
const SUPABASE_OAUTH_PARAMS = ["supabase_connected", "supabase_error"] as const;

function slackErrorMessage(code: string): string {
  if (code === "access_denied") return "Slack didn't authorize the connection. Try again, or contact your workspace admin if this keeps happening.";
  return "Couldn't connect to Slack. Please try again.";
}
const GITHUB_APP_SETTINGS_URL = "https://github.com/settings/connections/applications";

const GITHUB_DESCRIPTION = "Build and deploy agents directly from your repositories.";
const SLACK_DESCRIPTION = "Message agents directly from any connected Slack workspace.";
const SUPABASE_DESCRIPTION = "Import your Supabase Postgres projects when creating a knowledge store.";

function ConnectedSummary({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        className="inline-flex size-3 shrink-0 items-center justify-center rounded-full bg-success"
        aria-hidden
      >
        <Check className="size-2.5 stroke-white stroke-[3] dark:stroke-green-950" />
      </span>
      <span>{children}</span>
    </span>
  );
}

function RequestAccessLink() {
  return (
    <a
      href={GITHUB_APP_SETTINGS_URL}
      target="_blank"
      rel="noreferrer"
      className="inline-flex items-center gap-1 text-foreground-accent underline-offset-2 hover:underline"
    >
      Request access on GitHub
      <ArrowUpRight className="size-3.5 shrink-0" aria-hidden />
    </a>
  );
}

function GitHubSection() {
  const { personalAccount } = useAuth();
  const account = personalAccount?.name ?? "";
  const queryClient = useQueryClient();
  const [searchParams] = useSearchParams();
  const fromOAuth = searchParams.get('github_connected') === 'true';
  const oauthLogin = searchParams.get('github_login') ?? '';
  useCleanupOAuthParams(GITHUB_OAUTH_PARAMS);

  const { data: status, isLoading } = useGitHubAccountStatus(account, {
    enabled: !!account,
    initialData: fromOAuth ? { connected: true, github_login: oauthLogin } : undefined,
  });
  const disconnect = useGitHubAccountDisconnect(account);
  const connect = useGitHubAccountConnect(account);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const confirmPhrase = `disconnect ${account}`;

  const connected = status?.connected ?? false;
  const login = status?.github_login ?? "";

  const { data: orgsData, isLoading: orgsLoading } = useGitHubAccountOrgs(account, {
    enabled: !!account && connected,
  });
  const orgs = orgsData?.orgs ?? [];

  const onConnectSuccess = (data: GitHubConnectResponse) => {
    if (data.redirect_url) window.location.href = data.redirect_url;
  };

  const handleConnect = () => {
    connect.mutate({ redirectTo: RETURN_PATH }, { onSuccess: onConnectSuccess });
  };

  const handleReauthorize = () => {
    connect.mutate({ redirectTo: RETURN_PATH, force: true }, { onSuccess: onConnectSuccess });
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

  const action = connected ? (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="GitHub options">
          <MoreHorizontal className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          disabled={connect.isPending}
          onClick={handleReauthorize}
        >
          {connect.isPending ? "Opening GitHub…" : "Reauthorize"}
        </DropdownMenuItem>
        <DropdownMenuItem
          variant="destructive"
          disabled={disconnect.isPending}
          onClick={() => setConfirmOpen(true)}
        >
          {disconnect.isPending ? "Disconnecting…" : "Disconnect"}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  ) : (
    <Button variant="outline" size="sm" aria-label="Connect GitHub" disabled={connect.isPending} onClick={handleConnect}>
      {connect.isPending ? "Connecting…" : "Connect"}
    </Button>
  );

  return (
    <>
      <ConnectorRow
        icon={<GitHubIcon className="size-6 text-foreground" aria-hidden />}
        name="GitHub"
        description={connected ? (
          <ConnectedSummary>
            Connected as{" "}
            <span className="font-semibold text-foreground">@{login}</span>
          </ConnectedSummary>
        ) : GITHUB_DESCRIPTION}
        action={action}
        isLoading={isLoading}
        stackActionOnMobile={!connected}
      >
        {connected && (
          <>
            {!orgsLoading && orgs.length === 0 ? (
              <div className="flex flex-wrap items-center gap-1.5 px-4 pb-4 text-body-sm sm:px-5">
                <span className="text-muted-foreground">
                  No organizations have approved Astro yet.
                </span>
                <RequestAccessLink />
              </div>
            ) : (
              <>
                <ConnectorRowList className={!orgsLoading ? "mb-2" : undefined}>
                  {orgsLoading && (
                    <ConnectorRowItem>
                      <div className="h-3.5 w-32 rounded animate-pulse bg-muted" />
                    </ConnectorRowItem>
                  )}
                  {!orgsLoading && orgs.map((o) => (
                    <ConnectorRowItem key={o.login}>
                      {o.avatar_url ? (
                        <img
                          src={o.avatar_url}
                          alt=""
                          className="size-5 shrink-0 rounded object-cover"
                        />
                      ) : (
                        <Building2 className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                      )}
                      <span className="font-medium text-foreground truncate">{o.login}</span>
                    </ConnectorRowItem>
                  ))}
                </ConnectorRowList>
                {!orgsLoading && orgs.length > 0 && (
                  <div className="flex flex-wrap items-center gap-1.5 px-4 pb-4 text-body-sm sm:px-5">
                    <span className="text-muted-foreground">Missing an organization?</span>
                    <RequestAccessLink />
                  </div>
                )}
              </>
            )}
          </>
        )}
      </ConnectorRow>

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
  const [searchParams] = useSearchParams();

  const oauthError = searchParams.get("slack_error") ?? "";
  useCleanupOAuthParams(SLACK_OAUTH_PARAMS);

  const { data: status, isLoading } = useSlackAccountStatus(account, {
    enabled: !!account,
  });
  const disconnect = useSlackAccountDisconnect(account);
  const connect = useSlackAccountConnect(account);

  const workspaces = status?.workspaces ?? [];
  const connected = workspaces.length > 0;
  const [pendingRemove, setPendingRemove] = useState<{ teamId: string; team: string } | null>(null);
  const [pendingDisconnectAll, setPendingDisconnectAll] = useState(false);

  const handleConnect = () => {
    connect.mutate(RETURN_PATH, {
      onSuccess: (data) => {
        if (data.redirect_url) {
          window.location.href = data.redirect_url;
        }
      },
    });
  };

  const handleRemoveOne = (teamID: string) => {
    const key = slackKeys.accountStatus(account);
    const previous = queryClient.getQueryData<typeof status>(key);
    if (previous) {
      queryClient.setQueryData(key, {
        ...previous,
        workspaces: previous.workspaces.filter((w) => w.team_id !== teamID),
      });
    }
    setPendingRemove(null);
    disconnect.mutate(teamID, {
      onError: () => {
        if (previous) queryClient.setQueryData(key, previous);
      },
    });
  };

  const handleDisconnectAll = () => {
    const key = slackKeys.accountStatus(account);
    const previous = queryClient.getQueryData<typeof status>(key);
    if (previous) {
      queryClient.setQueryData(key, { ...previous, workspaces: [] });
    }
    setPendingDisconnectAll(false);
    disconnect.mutate(undefined, {
      onError: () => {
        if (previous) queryClient.setQueryData(key, previous);
      },
    });
  };

  const action = connected ? (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Slack options">
          <MoreHorizontal className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          variant="destructive"
          disabled={disconnect.isPending}
          onClick={() => setPendingDisconnectAll(true)}
        >
          {disconnect.isPending ? "Disconnecting…" : "Disconnect"}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  ) : (
    <Button variant="outline" size="sm" aria-label="Connect Slack" disabled={connect.isPending} onClick={handleConnect}>
      {connect.isPending ? "Opening Slack…" : "Connect"}
    </Button>
  );

  return (
    <>
      {oauthError && (
        <div className="mb-3">
          <ErrorPanel variant="inline">{slackErrorMessage(oauthError)}</ErrorPanel>
        </div>
      )}

      <ConnectorRow
        icon={<Slack className="size-6" aria-hidden />}
        name="Slack"
        description={
          connected
            ? (
              <ConnectedSummary>
                Connected to {workspaces.length} workspace{workspaces.length === 1 ? "" : "s"}
              </ConnectedSummary>
            )
            : SLACK_DESCRIPTION
        }
        action={action}
        isLoading={isLoading}
        stackActionOnMobile={!connected}
      >
        {connected && (
          <>
            <ConnectorRowList className="mb-2">
              {workspaces.map((w) => (
                <ConnectorRowItem
                  key={w.team_id}
                  className="flex-col items-stretch sm:flex-row sm:items-center"
                >
                  <div className="flex min-w-0 items-center gap-2.5">
                    {w.icon ? (
                      <img
                        src={w.icon}
                        alt=""
                        className="size-5 shrink-0 rounded-sm object-cover"
                      />
                    ) : (
                      <Building2 className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                    )}
                    <div className="flex min-w-0 flex-wrap items-baseline gap-x-1.5">
                      <div className="max-w-full truncate font-medium text-foreground">
                        {w.team || w.team_domain || w.team_id}
                      </div>
                      {w.slack_username && (
                        <div className="inline-flex shrink-0 items-baseline gap-1 text-muted-foreground">
                          <span aria-hidden>·</span>
                          <span className="text-muted-foreground">@{w.slack_username}</span>
                        </div>
                      )}
                    </div>
                  </div>
                  <div className="flex min-w-0 items-center justify-between gap-2 sm:ml-auto sm:justify-end">
                    <Button
                      variant="outline"
                      size="xs"
                      aria-label={`Disconnect ${w.team || w.team_domain || w.team_id}`}
                      className="!border-destructive/40 text-destructive hover:bg-destructive/[0.08] hover:text-destructive active:bg-destructive/15 active:text-destructive"
                      disabled={disconnect.isPending}
                      onClick={() => setPendingRemove({ teamId: w.team_id, team: w.team || w.team_domain || w.team_id })}
                    >
                      Disconnect
                    </Button>
                  </div>
                </ConnectorRowItem>
              ))}
            </ConnectorRowList>
            <div className="px-4 pb-4 text-body-sm sm:px-5">
              <Button
                variant="link"
                size="xs"
                className="-ml-2 text-foreground-accent no-underline underline-offset-2 hover:text-foreground-accent hover:underline"
                disabled={connect.isPending}
                onClick={handleConnect}
              >
                {connect.isPending ? "Opening Slack…" : "Add workspace"}
                <ArrowUpRight className="size-3.5" aria-hidden />
              </Button>
            </div>
          </>
        )}
      </ConnectorRow>

      <ConfirmationDialog
        open={pendingRemove !== null}
        onOpenChange={(o) => { if (!o) setPendingRemove(null); }}
        title="Remove Slack workspace?"
        description={
          <>
            Astro will no longer be able to act on messages from <span className="font-semibold text-foreground">{pendingRemove?.team}</span>. You can reconnect anytime.
          </>
        }
        checkboxLabel={<>I understand that removing <span className="font-semibold">{pendingRemove?.team}</span> will stop Astro from acting in that workspace.</>}
        actionLabel="Remove"
        pendingLabel="Removing…"
        error={disconnect.isError ? (disconnect.error as Error) : null}
        defaultErrorMessage="Failed to remove the workspace. Please try again."
        isPending={disconnect.isPending}
        canConfirm
        onConfirm={() => pendingRemove && handleRemoveOne(pendingRemove.teamId)}
        onReset={() => disconnect.reset()}
      />

      <ConfirmationDialog
        open={pendingDisconnectAll}
        onOpenChange={setPendingDisconnectAll}
        title="Disconnect Slack?"
        description={`This will remove all ${workspaces.length} workspace${workspaces.length !== 1 ? "s" : ""} and stop Astro from acting on any Slack messages. You can reconnect anytime.`}
        checkboxLabel="I understand that disconnecting Slack will remove every workspace."
        actionLabel="Disconnect Slack"
        pendingLabel="Disconnecting…"
        error={disconnect.isError ? (disconnect.error as Error) : null}
        defaultErrorMessage="Failed to disconnect Slack. Please try again."
        isPending={disconnect.isPending}
        canConfirm
        onConfirm={handleDisconnectAll}
        onReset={() => disconnect.reset()}
      />
    </>
  );
}

function SupabaseSection() {
  const { personalAccount } = useAuth();
  const account = personalAccount?.name ?? "";
  const queryClient = useQueryClient();
  const [searchParams] = useSearchParams();

  // Capture once — useCleanupOAuthParams strips the param on mount, so reading
  // it live would flash the error banner away before it can be read.
  const [oauthError] = useState(() => searchParams.get("supabase_error") ?? "");
  useCleanupOAuthParams(SUPABASE_OAUTH_PARAMS);

  const { data: status, isLoading } = useSupabaseStatus(account, { enabled: !!account });
  const connect = useSupabaseConnect(account);
  const disconnect = useSupabaseDisconnect(account);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const connected = status?.connected ?? false;

  const handleConnect = () => {
    connect.mutate(RETURN_PATH, {
      onSuccess: (data) => {
        if (data.redirect_url) window.location.href = data.redirect_url;
      },
    });
  };

  const handleDisconnect = () => {
    const key = supabaseKeys.status(account);
    const previous = queryClient.getQueryData(key);
    queryClient.setQueryData(key, { connected: false });
    setConfirmOpen(false);
    disconnect.mutate(undefined, {
      onError: () => { queryClient.setQueryData(key, previous); },
    });
  };

  const action = connected ? (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Supabase options">
          <MoreHorizontal className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          variant="destructive"
          disabled={disconnect.isPending}
          onClick={() => setConfirmOpen(true)}
        >
          {disconnect.isPending ? "Disconnecting…" : "Disconnect"}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  ) : (
    <Button variant="outline" size="sm" aria-label="Connect Supabase" disabled={connect.isPending} onClick={handleConnect}>
      {connect.isPending ? "Connecting…" : "Connect"}
    </Button>
  );

  return (
    <>
      {oauthError && (
        <div className="mb-3">
          <ErrorPanel variant="inline">Couldn't connect to Supabase. Please try again.</ErrorPanel>
        </div>
      )}

      <ConnectorRow
        icon={<ProviderIcon provider="supabase" className="size-6" />}
        name="Supabase"
        description={connected ? <ConnectedSummary>Connected</ConnectedSummary> : SUPABASE_DESCRIPTION}
        action={action}
        isLoading={isLoading}
      />

      <ConfirmationDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Disconnect Supabase?"
        description="Existing knowledge stores keep working, but you won't be able to import new Supabase projects until you reconnect."
        checkboxLabel="I understand that disconnecting Supabase will remove my OAuth connection."
        actionLabel="Disconnect"
        pendingLabel="Disconnecting…"
        error={disconnect.isError ? (disconnect.error as Error) : null}
        defaultErrorMessage="Failed to disconnect Supabase. Please try again."
        isPending={disconnect.isPending}
        canConfirm
        onConfirm={handleDisconnect}
        onReset={() => disconnect.reset()}
      />
    </>
  );
}

export default function ConnectorsSettings() {
  return (
    <>
      <SectionHeader title="Connectors" subtitle="Connect external services to use them across Astro" />
      <div className="overflow-hidden rounded-lg border border-border bg-surface">
        <GitHubSection />
        <SlackSection />
        <SupabaseSection />
      </div>
    </>
  );
}
