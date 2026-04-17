import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { api, type GitHubLinkInput, type GitHubStatusResponse } from '../../lib/api';
import { githubKeys, blueprintKeys } from './keys';

export function useGitHubStatus(account: string, name: string, opts?: { enabled?: boolean; refetchInterval?: number | false; initialData?: GitHubStatusResponse }) {
  const result = useQuery({
    queryKey: githubKeys.status(account, name),
    queryFn: () => api.getGitHubStatus(account, name),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
    refetchInterval: opts?.refetchInterval,
    initialData: opts?.initialData,
    // Keep previous data during refetches so callers never see a brief undefined
    // window (e.g. after a rebuild trigger invalidates the query).
    placeholderData: keepPreviousData,
  });

  // Poll every 5 seconds only while the most recent build is still in-flight.
  // Older stuck builds (e.g. from a server restart) don't keep the poll alive.
  const latestBuild = result.data?.builds[0];
  const hasActiveBuilds = latestBuild?.status === 'pending' || latestBuild?.status === 'building';
  useQuery({
    queryKey: githubKeys.status(account, name),
    queryFn: () => api.getGitHubStatus(account, name),
    enabled: !!hasActiveBuilds,
    refetchInterval: 5000,
  });

  return result;
}

export function useGitHubRepos(account: string, name: string, opts?: { enabled?: boolean }) {
  return useQuery({
    queryKey: githubKeys.repos(account, name),
    queryFn: () => api.gitHubListRepos(account, name),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
  });
}

export function useGitHubRebuild(account: string, name: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.gitHubRebuild(account, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: githubKeys.status(account, name) });
    },
  });
}

export function useGitHubBuildLogs(account: string, name: string, buildId: string, opts?: { enabled?: boolean; refetchInterval?: number | false }) {
  return useQuery({
    queryKey: ['github', account, name, 'builds', buildId, 'logs'],
    queryFn: () => api.getGitHubBuildLogs(account, name, buildId),
    enabled: (opts?.enabled ?? false) && !!buildId,
    refetchInterval: opts?.refetchInterval,
    retry: false,
  });
}

export function useGitHubConnect(account: string, name: string) {
  return useMutation({
    mutationFn: () => api.gitHubConnect(account, name),
  });
}

export function useGitHubLink(account: string, name: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: GitHubLinkInput) => api.gitHubLink(account, name, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: githubKeys.status(account, name) });
      queryClient.invalidateQueries({ queryKey: blueprintKeys.detail(account, name) });
      queryClient.invalidateQueries({ queryKey: githubKeys.accountConnections(account) });
    },
  });
}

export function useGitHubDisconnect(account: string, name: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.gitHubDisconnect(account, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: githubKeys.status(account, name) });
      queryClient.invalidateQueries({ queryKey: githubKeys.accountConnections(account) });
    },
  });
}

export function useGitHubAccountConnect(account: string) {
  return useMutation({
    mutationFn: (redirectTo: string) => api.gitHubConnectAccount(account, redirectTo),
  });
}

export function useGitHubAccountRepos(account: string, opts?: { enabled?: boolean }) {
  return useQuery({
    queryKey: githubKeys.accountRepos(account),
    queryFn: () => api.gitHubListAccountRepos(account),
    enabled: (opts?.enabled ?? true) && !!account,
  });
}

export function useGitHubAccountScan(account: string) {
  return useMutation({
    mutationFn: ({ repo, branch, agentName }: { repo: string; branch: string; agentName?: string }) =>
      api.gitHubAccountScan(account, repo, branch, agentName),
  });
}

export function useGitHubAccountConnections(account: string, opts?: { enabled?: boolean }) {
  return useQuery({
    queryKey: githubKeys.accountConnections(account),
    queryFn: () => api.gitHubListAccountConnections(account),
    enabled: (opts?.enabled ?? true) && !!account,
  });
}
