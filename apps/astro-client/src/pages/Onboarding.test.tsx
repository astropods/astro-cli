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

const hasPersonalAuth: AuthContextType = {
  ...mockAuthContext,
  accounts: [{ id: 'acct-1', name: 'testuser', type: 'personal' }],
  needsOnboarding: false,
};

function renderGuarded(auth: AuthContextType, initialEntries = ['/']) {
  return renderRoute(
    [
      { path: '/', Component: () => <OnboardingGuard><div>Home Page</div></OnboardingGuard> },
      { path: '/onboarding', Component: () => <OnboardingGuard><Onboarding /></OnboardingGuard> },
    ],
    { initialEntries, auth },
  );
}

describe('Onboarding', () => {
  it('redirects org-only users to /onboarding', async () => {
    renderGuarded(orgOnlyAuth, ['/']);
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /set up your account/i })).toBeInTheDocument();
    });
  });

  it('redirects away from /onboarding when user has a personal account', async () => {
    renderGuarded(hasPersonalAuth, ['/onboarding']);
    await waitFor(() => {
      expect(screen.getByText('Home Page')).toBeInTheDocument();
    });
  });

  it('submits a personal account via the claim form', async () => {
    let capturedBody: unknown = null;

    server.use(
      http.get('/api/v1/accounts/check/:name', () => HttpResponse.json({ available: true })),
      http.post('/api/v1/accounts', async ({ request }) => {
        capturedBody = await request.json();
        return HttpResponse.json(
          { id: 'new-acct', name: 'mynewname', type: 'personal', created_at: '', updated_at: '' },
          { status: 201 },
        );
      }),
    );

    const user = userEvent.setup();
    renderGuarded({ ...mockAuthContext, accounts: [], needsOnboarding: true }, ['/onboarding']);

    await user.type(screen.getByPlaceholderText('username'), 'mynewname');
    await user.type(screen.getByPlaceholderText('Your name'), 'Test User');
    await user.click(screen.getByRole('checkbox'));
    await waitFor(() => expect(screen.getByText('Available')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: /get started/i }));
    await waitFor(() => expect(capturedBody).toEqual({ name: 'mynewname', type: 'personal', display_name: 'Test User' }));
  });

  it('disables submit when terms are not accepted', async () => {
    server.use(
      http.get('/api/v1/accounts/check/:name', () => HttpResponse.json({ available: true })),
    );

    const user = userEvent.setup();
    renderGuarded({ ...mockAuthContext, accounts: [], needsOnboarding: true }, ['/onboarding']);

    await user.type(screen.getByPlaceholderText('username'), 'mynewname');
    await user.type(screen.getByPlaceholderText('Your name'), 'Test User');
    await waitFor(() => expect(screen.getByText('Available')).toBeInTheDocument());

    expect(screen.getByRole('button', { name: /get started/i })).toBeDisabled();
  });

  it('disables submit when display name is empty', async () => {
    server.use(
      http.get('/api/v1/accounts/check/:name', () => HttpResponse.json({ available: true })),
    );

    const user = userEvent.setup();
    renderGuarded({ ...mockAuthContext, accounts: [], needsOnboarding: true }, ['/onboarding']);

    await user.type(screen.getByPlaceholderText('username'), 'mynewname');
    await user.click(screen.getByRole('checkbox'));
    await waitFor(() => expect(screen.getByText('Available')).toBeInTheDocument());

    expect(screen.getByRole('button', { name: /get started/i })).toBeDisabled();
  });
});
