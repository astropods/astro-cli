import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { DashboardAgentsEmptyState } from './DashboardAgentsEmptyState';
import { blueprintKeys } from '@/api/queries/keys';
import type { BlueprintsListResponse } from '@/lib/api';

// The empty-state CTA branches on whether the account already has any
// blueprints — "Deploy a blueprint" when it does, "Create blueprint" when
// it doesn't. The signal comes from `useAccountBlueprints`, which keys on
// `blueprintKeys.byAccount(account)`. Seeding that cache row directly
// avoids needing to mock the fetch layer or wait for a real query.

function renderEmptyState(account: string, data: BlueprintsListResponse) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity, staleTime: 0 },
      mutations: { retry: false },
    },
  });
  // Pre-populate so `useAccountBlueprints(account)` resolves synchronously
  // on first render — no waitFor needed for the CTA branch to settle.
  queryClient.setQueryData(blueprintKeys.byAccount(account), data);
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <DashboardAgentsEmptyState account={account} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('DashboardAgentsEmptyState — CTA branches on account blueprints', () => {
  beforeEach(() => cleanup());

  it('renders "Deploy a blueprint" linking to the account blueprints list when blueprints exist', () => {
    renderEmptyState('test-account', {
      // Blueprint has many required fields, but the component only reads
      // `agents.length`; cast keeps the test focused on the contract.
      agents: [{ name: 'my-blueprint' } as BlueprintsListResponse['agents'][number]],
      count: 1,
    });

    const deployLink = screen.getByRole('link', { name: /deploy a blueprint/i });
    expect(deployLink).toBeInTheDocument();
    expect(deployLink.getAttribute('href')).toContain('/blueprints?account=test-account');

    // Conversely, the "Create blueprint" affordance is absent on this path.
    expect(screen.queryByRole('link', { name: /create blueprint/i })).not.toBeInTheDocument();
  });

  it('renders "Create blueprint" linking to /getting-started when the account has no blueprints', () => {
    renderEmptyState('test-account', { agents: [], count: 0 });

    const createLink = screen.getByRole('link', { name: /create blueprint/i });
    expect(createLink).toBeInTheDocument();
    expect(createLink.getAttribute('href')).toContain('/getting-started');

    // The Deploy CTA must not appear when there's nothing to deploy.
    expect(screen.queryByRole('link', { name: /deploy a blueprint/i })).not.toBeInTheDocument();
  });
});
