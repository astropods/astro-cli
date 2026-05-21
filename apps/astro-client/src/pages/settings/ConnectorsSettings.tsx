import { useState, type ReactNode } from "react";
import type { MetaFunction } from "react-router";
import { useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth } from "@/lib/auth";
import { SectionHeader } from "@/components/settings/SettingsShared";
import { ConnectorCard, ConnectorCardRow } from "@/components/settings/ConnectorCard";
import { useGitHubAccountStatus, useGitHubAccountDisconnect, useGitHubAccountConnect, useGitHubAccountOrgs } from "@/api/queries/github";
import { useSlackAccountStatus, useSlackAccountDisconnect, useSlackAccountConnect } from "@/api/queries/slack";
import { githubKeys, slackKeys } from "@/api/queries/keys";
import { Button } from "@/components/ui/button";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";
import { GitHubIcon } from "@/components/ui/svgs/githubIcon";
import { Slack } from "@/components/ui/svgs/slack";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useCleanupOAuthParams } from "@/hooks/use-cleanup-oauth-params";
import { Square2StackIcon, CheckIcon } from "@heroicons/react/24/outline";
import { X } from "lucide-react";

export const meta: MetaFunction = () => [{ title: "Connectors - Settings | Astro" }];

const RETURN_PATH = "/settings/connectors";
const GITHUB_OAUTH_PARAMS = ["github_connected", "github_login"] as const;
const SLACK_OAUTH_PARAMS = ["slack_connected", "slack_user", "slack_team", "slack_error"] as const;

function CopyInline({ value, label }: { value: string; label: string }) {
  const { copy, copied } = useCopyToClipboard(1600);
  return (
    <button
      type="button"
      aria-label={label}
      title={copied ? "Copied!" : label}
      onClick={() => void copy(value)}
      className="shrink-0 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
    >
      {copied
        ? <CheckIcon className="size-3 text-success" />
        : <Square2StackIcon className="size-3" />}
    </button>
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

  const handleConnect = () => {
    connect.mutate(RETURN_PATH, {
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

  const statusLine: ReactNode = connected
    ? <>Connected as <span className="font-mono">@{login}</span></>
    : "Not connected";
  const metaLine = connected
    ? `${orgs.length} organization${orgs.length !== 1 ? "s" : ""} accessible`
    : undefined;

  const action = connected ? (
    <div className="flex items-center gap-2">
      <Button variant="outline" size="sm" disabled={connect.isPending} onClick={handleConnect}>
        {connect.isPending ? "Opening GitHub…" : "Reauthorize"}
      </Button>
      <Button
        variant="outline"
        size="sm"
        disabled={disconnect.isPending}
        onClick={() => setConfirmOpen(true)}
      >
        {disconnect.isPending ? "Disconnecting…" : "Disconnect"}
      </Button>
    </div>
  ) : (
    <Button variant="outline" size="sm" disabled={connect.isPending} onClick={handleConnect}>
      {connect.isPending ? "Connecting…" : "Connect GitHub"}
    </Button>
  );

  return (
    <>
      <ConnectorCard
        icon={<GitHubIcon className="size-5 text-foreground" aria-hidden />}
        isLoading={isLoading}
        status={statusLine}
        meta={metaLine}
        action={action}
      >
        {connected && orgsLoading && (
          <ConnectorCardRow>
            <div className="h-3.5 w-32 rounded animate-pulse bg-muted" />
          </ConnectorCardRow>
        )}
        {connected && !orgsLoading && orgs.length === 0 && (
          <ConnectorCardRow>
            <span className="text-muted-foreground italic">
              No organizations granted. Use Reauthorize to add access.
            </span>
          </ConnectorCardRow>
        )}
        {connected && !orgsLoading && orgs.map((o) => (
          <ConnectorCardRow key={o.login}>
            {o.avatar_url ? (
              <img
                src={o.avatar_url}
                alt=""
                className="size-3.5 shrink-0 rounded-sm object-cover"
              />
            ) : (
              <GitHubIcon className="size-3.5 shrink-0" aria-hidden />
            )}
            <span className="font-medium text-foreground truncate">{o.login}</span>
          </ConnectorCardRow>
        ))}
      </ConnectorCard>

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

  const handleConnect = () => {
    connect.mutate(RETURN_PATH, {
      onSuccess: (data) => {
        if (data.redirect_url) {
          window.location.href = data.redirect_url;
        }
      },
    });
  };

  const handleDisconnectOne = (teamID: string) => {
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

  const statusLine: ReactNode = connected ? "Connected" : "Not connected";
  const metaLine = connected
    ? `${workspaces.length} workspace${workspaces.length !== 1 ? "s" : ""} linked`
    : undefined;

  const action = (
    <Button variant="outline" size="sm" disabled={connect.isPending} onClick={handleConnect}>
      {connect.isPending ? "Opening Slack…" : connected ? "Add workspace" : "Connect Slack"}
    </Button>
  );

  return (
    <>
      {oauthError && (
        <div className="mb-2 flex items-center justify-between gap-2 rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2">
          <p className="text-[12px] text-destructive">
            Couldn't link your Slack account ({oauthError}). Try again?
          </p>
        </div>
      )}

      <ConnectorCard
        icon={<Slack className="size-5" aria-hidden />}
        isLoading={isLoading}
        status={statusLine}
        meta={metaLine}
        action={action}
      >
        {connected && workspaces.map((w) => (
          <ConnectorCardRow key={w.team_id}>
            {w.icon ? (
              <img
                src={w.icon}
                alt=""
                className="size-3.5 shrink-0 rounded-sm object-cover"
              />
            ) : (
              <Slack className="size-3.5 shrink-0" aria-hidden />
            )}
            <span className="font-medium text-foreground truncate">
              {w.team || w.team_domain || w.team_id}
            </span>
            <div className="ml-auto flex items-center gap-2 min-w-0">
              {w.slack_username && (
                <span className="font-mono text-muted-foreground truncate">@{w.slack_username}</span>
              )}
              <CopyInline value={w.slack_user_id} label="Copy user ID" />
              <button
                type="button"
                aria-label={`Disconnect ${w.team || w.team_id}`}
                onClick={() => handleDisconnectOne(w.team_id)}
                disabled={disconnect.isPending}
                className="shrink-0 text-muted-foreground hover:text-destructive transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <X className="size-3.5" />
              </button>
            </div>
          </ConnectorCardRow>
        ))}
      </ConnectorCard>
    </>
  );
}

export default function ConnectorsSettings() {
  return (
    <>
      <SectionHeader
        title="GitHub"
        subtitle="Link a GitHub account so Astro can clone, build, and watch repositories on your behalf. Reauthorize to grant access to additional organizations."
      />
      <GitHubSection />
      <hr className="my-2 border-border" />
      <SectionHeader
        title="Slack"
        subtitle="Map your Astro identity to your Slack identity in each workspace. Agents use this mapping to authorize requests that reach them from Slack."
      />
      <SlackSection />
    </>
  );
}
