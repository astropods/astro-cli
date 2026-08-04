import { useState } from 'react';
import { describe, it, expect, beforeEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DeployedAgentsSection } from './DeployedAgentsSection';
import type { AgentDeploymentSummary } from '@/lib/api';
import { renderWithProviders } from '@/test/test-utils';

// The dashboard derives the "Update available" badge purely from
// `deployment.latest_build_id` vs `deployment.build_id`. Both values are
// computed authoritatively on the server (populateLatestBuildIDs in
// apps/astro-server/handlers/deploy.go); the client does not fan out
// per-account blueprint queries. These tests pin that contract so the
// fan-out path can't regress back in.

function renderSection(deployments: AgentDeploymentSummary[], account: string) {
  return renderWithProviders(
    <DeployedAgentsSection deployments={deployments} account={account} isLoading={false} />,
  );
}

function makeDeployment(overrides: Partial<AgentDeploymentSummary> & { id: string; name: string; build_id: string }): AgentDeploymentSummary {
  return {
    namespace: `ns-${overrides.id}`,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

const updateBadge = /update available/i;
const errorBadge = /error/i;

describe('DeployedAgentsSection — upgrade badge from server-supplied latest_build_id', () => {
  beforeEach(() => cleanup());

  it('renders the badge when latest_build_id differs from current build_id', async () => {
    const deployments: AgentDeploymentSummary[] = [
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
    const deployments: AgentDeploymentSummary[] = [
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
    const deployments: AgentDeploymentSummary[] = [
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

  it('renders the error badge when status is "error"', async () => {
    // The server maps StatusFailed (DB) → "error" in agentDeploymentFromDB.
    // The card checks deployment.status === "error" exactly — pin that contract.
    renderSection(
      [makeDeployment({ id: 'dep-err', name: 'broken', build_id: 'b1', display_name: 'Broken Agent', status: 'error' })],
      'team',
    );
    await waitFor(() => expect(screen.getByText('Broken Agent')).toBeInTheDocument());
    const card = screen.getByText('Broken Agent').closest('a, div');
    expect(card?.textContent ?? '').toMatch(errorBadge);
  });

  it('does not render the error badge when status is absent or non-error', async () => {
    renderSection(
      [makeDeployment({ id: 'dep-ok', name: 'healthy', build_id: 'b1', display_name: 'Healthy Agent', status: 'Running' })],
      'team',
    );
    await waitFor(() => expect(screen.getByText('Healthy Agent')).toBeInTheDocument());
    const card = screen.getByText('Healthy Agent').closest('a, div');
    expect(card?.textContent ?? '').not.toMatch(errorBadge);
  });

  it('badges only the deployments whose latest_build_id has drifted', async () => {
    const deployments: AgentDeploymentSummary[] = [
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

  it('keeps pagination focus when the animated results remount', async () => {
    const user = userEvent.setup();

    function Harness() {
      const [page, setPage] = useState(1);
      return (
        <DeployedAgentsSection
          deployments={[
            makeDeployment({
              id: 'dep-page',
              name: 'paged-agent',
              build_id: 'build-1',
              display_name: 'Paged Agent',
            }),
          ]}
          account="team"
          isLoading={false}
          currentPage={page}
          totalPages={3}
          onPageChange={setPage}
          resultsTransitionKey={`page-${page}`}
        />
      );
    }

    renderWithProviders(<Harness />);
    const nextPage = await screen.findByRole('button', { name: 'Next page' });

    await user.click(nextPage);

    expect(screen.getByRole('button', { name: 'Next page' })).toBe(nextPage);
    expect(nextPage).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');
  });
});
