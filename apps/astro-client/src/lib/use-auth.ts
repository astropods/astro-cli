import { useContext, useCallback, useMemo } from 'react';
import { AuthContext, type AuthContextType } from './auth-context';
import type { Account } from './api';

export function useAuth(): AuthContextType & {
  hasPermission: (permission: string) => boolean;
  personalAccount: Account | undefined;
} {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  const hasPermission = useCallback(
    (permission: string) => context.permissions.includes(permission),
    [context.permissions],
  );
  const personalAccount = useMemo(
    () => context.accounts.find((a) => a.type === 'personal'),
    [context.accounts],
  );
  return { ...context, hasPermission, personalAccount };
}
