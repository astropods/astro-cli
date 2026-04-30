import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { DeployedAgentsSection } from './DeployedAgentsSection';
import type { AgentDeployment } from '@/lib/api';

// The dashboard derives the "Update available" badge purely from
// `deployment.latest_build_id` vs `deployment.build_id`. Both values are
// computed authoritatively on the server (populateLatestBuildIDs in
// apps/astro-server/handlers/deploy.go); the client does not fan out
// per-account blueprint queries. These tests pin that contract so the
// fan-out path can't regress back in.

function renderSection(deployments: AgentDeployment[], account: string) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity, staleTime: 0 },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <DeployedAgentsSection deployments={deployments} account={account} isLoading={false} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function makeDeployment(overrides: Partial<AgentDeployment> & { id: string; name: string; build_id: string }): AgentDeployment {
  return {
    namespace: `ns-${overrides.id}`,
    status: 'Running',
    replicas: 1,
    ready: 1,
    created_at: '2026-01-01T00:00:00Z',
    components: [],
    ...overrides,
  };
}

const updateBadge = /update available/i;

describe('DeployedAgentsSection — upgrade badge from server-supplied latest_build_id', () => {
  beforeEach(() => cleanup());

  it('renders the badge when latest_build_id differs from current build_id', async () => {
    const deployments: AgentDeployment[] = [
      makeDeployment({
        id: 'dep-stale',
        name: 'critical-agent',
        build_id: 'old-build',
        latest_build_id: 'new-build',
        display_name: 'Stale Agent',
      }),
    ];

    renderSection(deployments, 'team');

    await waitFor(() => {
      expect(screen.getByText('Stale Agent')).toBeInTheDocument();
    });
    const card = screen.getByText('Stale Agent').closest('a, div');
    expect(card?.textContent ?? '').toMatch(updateBadge);
  });

  it('does not render the badge when latest_build_id equals current build_id', async () => {
    const deployments: AgentDeployment[] = [
      makeDeployment({
        id: 'dep-current',
        name: 'critical-agent',
        build_id: 'same-build',
        latest_build_id: 'same-build',
        display_name: 'Current Agent',
      }),
    ];

    renderSection(deployments, 'team');

    await waitFor(() => {
      expect(screen.getByText('Current Agent')).toBeInTheDocument();
    });
    const card = screen.getByText('Current Agent').closest('a, div');
    expect(card?.textContent ?? '').not.toMatch(updateBadge);
  });

  it('does not render the badge when latest_build_id is absent', async () => {
    // Mirrors the server behaviour when there are no published versions, the
    // batch lookup failed, or the response predates the field.
    const deployments: AgentDeployment[] = [
      makeDeployment({
        id: 'dep-unknown',
        name: 'critical-agent',
        build_id: 'some-build',
        display_name: 'Unknown Latest',
      }),
    ];

    renderSection(deployments, 'team');

    await waitFor(() => {
      expect(screen.getByText('Unknown Latest')).toBeInTheDocument();
    });
    const card = screen.getByText('Unknown Latest').closest('a, div');
    expect(card?.textContent ?? '').not.toMatch(updateBadge);
  });

  it('badges only the deployments whose latest_build_id has drifted', async () => {
    const deployments: AgentDeployment[] = [
      makeDeployment({
        id: 'dep-stale',
        name: 'agent-a',
        build_id: 'a-old',
        latest_build_id: 'a-new',
        display_name: 'Stale A',
      }),
      makeDeployment({
        id: 'dep-current',
        name: 'agent-b',
        build_id: 'b-current',
        latest_build_id: 'b-current',
        display_name: 'Current B',
      }),
      makeDeployment({
        id: 'dep-unknown',
        name: 'agent-c',
        build_id: 'c-current',
        display_name: 'Unknown C',
      }),
    ];

    renderSection(deployments, 'team');

    await waitFor(() => {
      expect(screen.getByText('Stale A')).toBeInTheDocument();
      expect(screen.getByText('Current B')).toBeInTheDocument();
      expect(screen.getByText('Unknown C')).toBeInTheDocument();
    });
    expect(screen.getAllByText(updateBadge)).toHaveLength(1);
    const stale = screen.getByText('Stale A').closest('a, div');
    expect(stale?.textContent ?? '').toMatch(updateBadge);
  });
});
