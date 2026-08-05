import { useState } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, screen, waitFor, cleanup } from '@testing-library/react';
import { DeployedAgentsSection } from './DeployedAgentsSection';
import type { AgentDeploymentSummary } from '@/lib/api';
import { renderWithProviders } from '@/test/test-utils';

// An agent card is one of the most expensive components in the client: a
// 150-star SVG starfield plus two d3 spline sparklines, times a page of cards.
// The term the page filters by is debounced, so a half-typed term cannot have
// changed the results yet. Rendering the grid against it is pure waste, and at
// a page of cards it blocks the main thread on every keypress.
//
// The guarantee is structural: the in-flight text lives in the search box, so
// the page (and the grid under it) simply is not re-rendered while typing.
// These tests pin that, and would fail again if the text were lifted back up
// into page state.

const renders = vi.hoisted(() => ({ count: 0 }));

vi.mock('@/components/DeployedAgentCard', () => ({
  DeployedAgentCard: ({ displayName }: { displayName?: string }) => {
    renders.count++;
    return <div>{displayName}</div>;
  },
}));

const DEPLOYMENTS: AgentDeploymentSummary[] = Array.from({ length: 12 }, (_, i) => ({
  id: `dep-${i}`,
  name: `agent-${i}`,
  display_name: `Agent ${i}`,
  build_id: `build-${i}`,
  namespace: `ns-${i}`,
  created_at: '2026-01-01T00:00:00Z',
  status: 'Running',
}));

// Mirrors how AgentDashboard drives the section: the settled term lives above
// it and feeds the server query, so `deployments` is unchanged while typing.
function Harness() {
  const [search, setSearch] = useState('');
  return (
    <DeployedAgentsSection
      deployments={DEPLOYMENTS}
      account="team"
      isLoading={false}
      search={search}
      onSearchChange={setSearch}
      currentPage={1}
      totalPages={1}
      resultsTransitionKey="stable-scope"
    />
  );
}

function setup() {
  renderWithProviders(<Harness />);
  expect(screen.getByText('Agent 0')).toBeInTheDocument();
  expect(renders.count).toBe(DEPLOYMENTS.length);
  const input = screen.getByPlaceholderText('Search agents...');
  renders.count = 0;
  return input;
}

describe('DeployedAgentsSection: typing stays off the card render path', () => {
  beforeEach(() => {
    cleanup();
    renders.count = 0;
  });

  // fireEvent.change is synchronous, so these assertions run in the same tick
  // as the keystrokes. No debounce can have elapsed, so any card render seen
  // here is one caused by the keystroke itself.
  it('does not render any card while typing', () => {
    const input = setup();

    for (const value of ['a', 'ag', 'age', 'agen', 'agent']) {
      fireEvent.change(input, { target: { value } });
    }

    expect(input).toHaveValue('agent');
    expect(renders.count).toBe(0);
  });

  it('does not render any card while deleting', () => {
    const input = setup();
    fireEvent.change(input, { target: { value: 'agent' } });
    renders.count = 0;

    for (const value of ['agen', 'age', 'ag', 'a', '']) {
      fireEvent.change(input, { target: { value } });
    }

    expect(input).toHaveValue('');
    expect(renders.count).toBe(0);
  });

  it('renders the grid once the term settles', async () => {
    const input = setup();

    fireEvent.change(input, { target: { value: 'agent' } });

    // One pass over the grid for the settled term, not one per keystroke.
    await waitFor(() => expect(renders.count).toBe(DEPLOYMENTS.length));
    expect(renders.count).toBe(DEPLOYMENTS.length);
  });
});

// Because the text is local to the box, clearing filters has to reach it. An
// account filter matching nothing puts "Clear filters" on screen while the
// search term is still empty, so dropping the term is a no-op the box cannot
// observe on its own.
describe('DeployedAgentsSection: clearing filters drops in-flight search text', () => {
  beforeEach(cleanup);

  it('empties the box when no term has settled yet', async () => {
    const onSearchChange = vi.fn();
    const onClearAccountFilters = vi.fn();

    renderWithProviders(
      <DeployedAgentsSection
        deployments={[]}
        account="team"
        isLoading={false}
        hasExplicitAccountFilter
        onClearAccountFilters={onClearAccountFilters}
        search=""
        onSearchChange={onSearchChange}
      />,
    );

    const input = screen.getByPlaceholderText('Search agents...');
    fireEvent.change(input, { target: { value: 'z' } });
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }));

    // Past the default debounce: the discarded text must not surface late.
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(input).toHaveValue('');
    expect(onSearchChange).not.toHaveBeenCalledWith('z');
    expect(onClearAccountFilters).toHaveBeenCalled();
  });
});
