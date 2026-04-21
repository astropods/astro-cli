import type { Preview } from '@storybook/react-vite'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'
import { AuthContext } from '../src/lib/auth-context'
import { mockAuthContext } from '../src/test/test-utils'
import '../src/index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
    },
  },
})

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
        AuthContext.Provider,
        { value: mockAuthContext },
        React.createElement(
          QueryClientProvider,
          { client: queryClient },
          Story()
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
