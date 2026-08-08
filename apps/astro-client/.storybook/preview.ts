import type { Preview } from '@storybook/react-vite'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import React from 'react'
import { initialize, mswLoader } from 'msw-storybook-addon'
import { AuthContext } from '../src/lib/auth-context'
import { mockAuthContext } from '../src/test/test-utils'
import { handlers } from '../src/test/msw/handlers'
import { setTheme } from '../src/lib/theme'
import '../src/index.css'

initialize(
  {
    onUnhandledRequest: ({ method, url }) => {
      // eslint-disable-next-line no-console
      console.warn(`[MSW] Unhandled ${method} ${url}`)
    },
  },
  handlers,
)

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
    },
  },
})

interface ThemePaneProps {
  label: string;
  dark: boolean;
  children: React.ReactNode;
}

const ThemePane = ({ label, dark, children }: ThemePaneProps) =>
  React.createElement(
    'div',
    { className: `${dark ? 'dark ' : ''}bg-surface text-foreground rounded-md border border-border p-4` },
    React.createElement(
      'div',
      { className: 'mb-3 font-mono text-label uppercase tracking-[0.07em] text-faint-foreground' },
      label,
    ),
    children,
  )

/** Drives the real theme store, on commit — setTheme notifies subscribers. */
const ThemeSync = ({ theme }: { theme: 'light' | 'dark' | 'split' }) => {
  React.useLayoutEffect(() => {
    setTheme(theme === 'split' ? 'light' : theme)
  }, [theme])
  return null
}

const preview: Preview = {
  globalTypes: {
    theme: {
      description: 'Toggle light/dark mode (single-pane only)',
      toolbar: {
        title: 'Theme',
        icon: 'circlehollow',
        items: [
          { value: 'light', icon: 'sun', title: 'Light' },
          { value: 'dark', icon: 'moon', title: 'Dark' },
          { value: 'split', icon: 'mirror', title: 'Light + Dark (split)' },
        ],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: {
    theme: 'light',
  },
  loaders: [mswLoader],
  decorators: [
    (Story, context) => {
      const theme = context.globals.theme as 'light' | 'dark' | 'split';

      const renderStory = () =>
        React.createElement(
          AuthContext.Provider,
          { value: mockAuthContext },
          React.createElement(
            QueryClientProvider,
            { client: queryClient },
            React.createElement(MemoryRouter, null, Story()),
          ),
        )

      const content =
        theme === 'split'
          ? React.createElement(
              'div',
              { className: 'grid grid-cols-2 gap-4 p-4 items-stretch' },
              React.createElement(ThemePane, { label: 'Light', dark: false }, renderStory()),
              React.createElement(ThemePane, { label: 'Dark', dark: true }, renderStory()),
            )
          : renderStory()

      return React.createElement(
        React.Fragment,
        null,
        React.createElement(ThemeSync, { theme }),
        content,
      )
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
