import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Navigate, useLocation } from 'react-router';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import { useAuth } from '@/lib/auth';
import type { AuthContextType } from '@/lib/auth-context';
import Onboarding from './Onboarding';

afterEach(cleanup);

/*
 * The OnboardingGuard lives in root.tsx and isn't exported, so we recreate it
 * here to test the redirect behavior in isolation with controlled auth state.
 */
const OnboardingGuard = ({ children }: { children: React.ReactNode }) => {
  const { isAuthenticated, isLoading, needsOnboarding } = useAuth();
  const location = useLocation();

  if (isLoading) return <>{children}</>;

  if (isAuthenticated && needsOnboarding && location.pathname !== '/onboarding') {
    return <Navigate to="/onboarding" replace />;
  }

  if (isAuthenticated && !needsOnboarding && location.pathname === '/onboarding') {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
};

const orgOnlyAuth: AuthContextType = {
  ...mockAuthContext,
  accounts: [{ id: 'org-1', name: 'my-org', type: 'organization' }],
  needsOnboarding: true,
};

const noAccountsAuth: AuthContextType = {
  ...mockAuthContext,
  accounts: [],
  needsOnboarding: true,
};

const hasPersonalAuth: AuthContextType = {
  ...mockAuthContext,
  accounts: [{ id: 'acct-1', name: 'testuser', type: 'personal' }],
  needsOnboarding: false,
};

const HomePage = () => <div>Home Page</div>;

function renderGuarded(auth: AuthContextType, initialEntries = ['/']) {
  return renderRoute(
    [
      {
        path: '/',
        Component: () => (
          <OnboardingGuard>
            <HomePage />
          </OnboardingGuard>
        ),
      },
      {
        path: '/onboarding',
        Component: () => (
          <OnboardingGuard>
            <Onboarding />
          </OnboardingGuard>
        ),
      },
    ],
    { initialEntries, auth },
  );
}

function renderOnboarding(auth: AuthContextType) {
  return renderRoute(
    [
      {
        path: '/onboarding',
        // @ts-expect-error: matches won't align between test code and app code
        Component: Onboarding,
      },
      {
        path: '/',
        Component: HomePage,
      },
    ],
    { initialEntries: ['/onboarding'], auth },
  );
}

describe('Onboarding', () => {
  describe('OnboardingGuard redirects', () => {
    it('redirects to /onboarding when user has only org accounts', async () => {
      renderGuarded(orgOnlyAuth, ['/']);

      await waitFor(() => {
        expect(
          screen.getByRole('heading', { name: /choose your username/i }),
        ).toBeInTheDocument();
      });
    });

    it('redirects to /onboarding when user has no accounts', async () => {
      renderGuarded(noAccountsAuth, ['/']);

      await waitFor(() => {
        expect(
          screen.getByRole('heading', { name: /choose your username/i }),
        ).toBeInTheDocument();
      });
    });

    it('redirects away from /onboarding when user has a personal account', async () => {
      renderGuarded(hasPersonalAuth, ['/onboarding']);

      await waitFor(() => {
        expect(screen.getByText('Home Page')).toBeInTheDocument();
      });
    });
  });

  describe('page rendering', () => {
    it('shows "Choose your username" for new users', () => {
      renderOnboarding(noAccountsAuth);

      expect(
        screen.getByRole('heading', { name: /choose your username/i }),
      ).toBeInTheDocument();
    });

    it('shows "Choose your username" for users with org accounts', () => {
      renderOnboarding(orgOnlyAuth);

      expect(
        screen.getByRole('heading', { name: /choose your username/i }),
      ).toBeInTheDocument();
    });
  });

  describe('form submission', () => {
    it('checks name availability and submits a personal account', async () => {
      let capturedBody: unknown = null;

      server.use(
        http.get('/api/v1/accounts/check/:name', () => {
          return HttpResponse.json({ available: true });
        }),
        http.post('/api/v1/accounts', async ({ request }) => {
          capturedBody = await request.json();
          return HttpResponse.json(
            {
              id: 'new-acct',
              name: 'mynewname',
              type: 'personal',
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            },
            { status: 201 },
          );
        }),
      );

      const user = userEvent.setup();
      renderOnboarding(noAccountsAuth);

      const input = screen.getByPlaceholderText('username');
      await user.type(input, 'mynewname');

      await waitFor(() => {
        expect(screen.getByText('Available')).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: /claim username/i }));

      await waitFor(() => {
        expect(capturedBody).toEqual({ name: 'mynewname', type: 'personal' });
      });
    });
  });
});
