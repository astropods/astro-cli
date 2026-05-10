import { createContext } from 'react';
import type { User, Account } from './api';

export interface AuthState {
  user: User | null;
  sessionId: string | null;
  organizationId: string | null;
  role: string | null;
  permissions: string[];
  expiresAt: Date | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  error: string | null;
  accounts: Account[];
  needsOnboarding: boolean;
  refreshVersion: number;
}

export interface AuthContextType extends AuthState {
  login: () => void;
  logout: () => void;
  refresh: () => Promise<void>;
  /** Refresh user/account data (display name, avatar, etc.) without triggering a blanket query invalidation. Use after profile edits instead of refresh(). */
  refreshUserData: () => Promise<void>;
  checkAuth: () => Promise<void>;
  switchOrg: (organizationId: string) => Promise<void>;
  /** Seed auth state from a server-side loader response. Skips the client-side check. */
  hydrateAuth: (response: import('./api').AuthResponse) => void;
}

export const initialAuthState: AuthState = {
  user: null,
  sessionId: null,
  organizationId: null,
  role: null,
  permissions: [],
  expiresAt: null,
  isLoading: true,
  isAuthenticated: false,
  error: null,
  accounts: [],
  needsOnboarding: false,
  refreshVersion: 0,
};

export const AuthContext = createContext<AuthContextType | null>(null);
