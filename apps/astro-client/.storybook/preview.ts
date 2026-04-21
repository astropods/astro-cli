import type { Preview } from '@storybook/react-vite'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'
import { MemoryRouter } from 'react-router'
import '../src/index.css'
import { AuthContext, type AuthContextType } from '../src/lib/auth-context'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
    },
  },
})

const mockAuth: AuthContextType = {
  user: { id: 'user-1', email: 'dev@example.com', first_name: 'Dev', last_name: 'User', email_verified: true, created_at: '', updated_at: '' },
  sessionId: 'session-1',
  organizationId: 'org-1',
  role: 'admin',
  permissions: [],
  expiresAt: new Date(Date.now() + 86400000),
  isLoading: false,
  isAuthenticated: true,
  error: null,
  accounts: [{ id: 'acct-1', name: 'devuser', type: 'personal' }],
  needsOnboarding: false,
  refreshVersion: 0,
  login: () => {},
  logout: () => {},
  refresh: async () => {},
  checkAuth: async () => {},
  switchOrg: async () => {},
}

const preview: Preview = {
  globalTypes: {
    theme: {
      description: 'Toggle light/dark mode',
      toolbar: {
        title: 'Theme',
        icon: 'circlehollow',
        items: [
          { value: 'light', icon: 'sun', title: 'Light' },
          { value: 'dark', icon: 'moon', title: 'Dark' },
        ],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: {
    theme: 'light',
  },
  decorators: [
    (Story, context) => {
      const theme = context.globals.theme;
      document.documentElement.classList.toggle('dark', theme === 'dark');
      return React.createElement(
        MemoryRouter,
        null,
        React.createElement(
          AuthContext.Provider,
          { value: mockAuth },
          React.createElement(QueryClientProvider, { client: queryClient }, Story())
        )
      );
    },
  ],
  parameters: {
    controls: {
      matchers: {
       color: /(background|color)$/i,
       date: /Date$/i,
      },
    },
  },
};

export default preview;
