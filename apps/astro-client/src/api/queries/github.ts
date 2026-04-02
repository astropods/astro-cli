import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type GitHubLinkInput } from '../../lib/api';
import { githubKeys, blueprintKeys } from './keys';

export function useGitHubStatus(account: string, name: string, opts?: { enabled?: boolean }) {
  return useQuery({
    queryKey: githubKeys.status(account, name),
    queryFn: () => api.getGitHubStatus(account, name),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
  });
}

export function useGitHubRepos(account: string, name: string, opts?: { enabled?: boolean }) {
  return useQuery({
    queryKey: githubKeys.repos(account, name),
    queryFn: () => api.gitHubListRepos(account, name),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
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
    },
  });
}

export function useGitHubDisconnect(account: string, name: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.gitHubDisconnect(account, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: githubKeys.status(account, name) });
    },
  });
}
