import { useContext, useCallback } from 'react';
import { AuthContext, type AuthContextType } from './auth-context';

export function useAuth(): AuthContextType & { hasPermission: (permission: string) => boolean } {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  const hasPermission = useCallback(
    (permission: string) => context.permissions.includes(permission),
    [context.permissions],
  );
  return { ...context, hasPermission };
}
