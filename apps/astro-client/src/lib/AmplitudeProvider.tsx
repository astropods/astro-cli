import { useEffect, type ReactNode } from 'react';
import { useAuth } from './auth';
import { initAmplitude, identifyUser, resetUser } from './amplitude';

const AMPLITUDE_API_KEY = import.meta.env.VITE_AMPLITUDE_API_KEY ?? '';

/**
 * Initializes Amplitude and syncs user identity on auth changes.
 * Page views, sessions, element interactions, and form tracking
 * are all handled automatically by the SDK's autocapture.
 * No-ops if VITE_AMPLITUDE_API_KEY is not set.
 */
export function AmplitudeProvider({ children }: { children: ReactNode }) {
  const { user, isAuthenticated, organizationId } = useAuth();

  // Initialize once
  useEffect(() => {
    initAmplitude(AMPLITUDE_API_KEY);
  }, []);

  // Sync user identity
  useEffect(() => {
    if (!AMPLITUDE_API_KEY) return;
    if (isAuthenticated && user) {
      identifyUser(user.id, user.email, organizationId ?? undefined);
    } else {
      resetUser();
    }
  }, [isAuthenticated, user, organizationId]);

  return <>{children}</>;
}
