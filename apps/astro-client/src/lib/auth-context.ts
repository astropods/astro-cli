import { createContext } from 'react';
import type { User } from './api';

export interface AuthState {
  user: User | null;
  sessionId: string | null;
  organizationId: string | null;
  role: string | null;
  expiresAt: Date | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  error: string | null;
}

export interface AuthContextType extends AuthState {
  login: () => void;
  logout: () => void;
  refresh: () => Promise<void>;
  checkAuth: () => Promise<void>;
}

export const initialAuthState: AuthState = {
  user: null,
  sessionId: null,
  organizationId: null,
  role: null,
  expiresAt: null,
  isLoading: true,
  isAuthenticated: false,
  error: null,
};

export const AuthContext = createContext<AuthContextType | null>(null);
