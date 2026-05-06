import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type SlackStatusResponse } from '../../lib/api';
import { slackKeys } from './keys';

/** Returns every Slack workspace the current user has linked, and a
 *  derived `connected` flag (true when at least one active mapping
 *  exists). Multi-workspace by design — the panel shows a row per row. */
export function useSlackAccountStatus(account: string, opts?: { enabled?: boolean; initialData?: SlackStatusResponse }) {
  return useQuery({
    queryKey: slackKeys.accountStatus(account),
    queryFn: () => api.slackAccountStatus(account),
    enabled: (opts?.enabled ?? true) && !!account,
    initialData: opts?.initialData,
    // Seeded data is shown immediately, then a background refetch confirms.
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

/** Disconnect: omit teamID for "disconnect every workspace"; pass it for
 *  per-row "disconnect this workspace only" in a multi-workspace user. */
export function useSlackAccountDisconnect(account: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (teamID?: string) => api.slackAccountDisconnect(account, teamID),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: slackKeys.accountStatus(account) });
      // In-flight refetches race with server propagation and can briefly
      // resurrect a just-disconnected workspace; cancel them.
      queryClient.cancelQueries({ queryKey: slackKeys.accountStatus(account) });
    },
  });
}

/** Initiates a fresh Pipes OAuth round-trip every time — no
 *  already-connected short-circuit. The handler returns a redirect URL
 *  the page navigates to; on return the callback handler will redirect
 *  back with ?slack_connected=true (or ?slack_error=…) appended. */
export function useSlackAccountConnect(account: string) {
  return useMutation({
    mutationFn: (redirectTo: string) => api.slackConnectAccount(account, redirectTo),
  });
}
