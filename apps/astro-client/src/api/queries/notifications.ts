import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type UpdateNotificationPreferenceInput } from '@/lib/api';
import { notificationKeys } from './keys';

export function useNotificationPreferences(account: string) {
  return useQuery({
    queryKey: notificationKeys.preferences(account),
    queryFn: () => api.getNotificationPreferences(account),
    enabled: !!account,
  });
}

export function useUpdateNotificationPreference(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: UpdateNotificationPreferenceInput) =>
      api.updateNotificationPreference(account, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: notificationKeys.preferences(account) });
    },
  });
}

export function useSendTestNotification(account: string) {
  return useMutation({
    mutationFn: () => api.sendTestNotification(account),
  });
}

export function useNotificationInboxConfig(enabled: boolean) {
  return useQuery({
    queryKey: notificationKeys.inboxConfig(),
    queryFn: () => api.getNotificationInboxConfig(),
    enabled,
    staleTime: 5 * 60 * 1000, // config rarely changes; the HMAC is per-user-stable
  });
}
