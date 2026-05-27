import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, cleanup, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/test-utils';
import { AgentsTab } from './AgentsTab';
import type { AgentSort } from './AgentsTab';
import type { AgentDeployment } from '@/lib/api';

afterEach(cleanup);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const makeDeployment = (id: string, name: string, displayName?: string): AgentDeployment => ({
  id,
  name,
  display_name: displayName,
  status: 'Running',
  replicas: 1,
  ready: 1,
  build_id: 'b1',
  namespace: 'ns1',
  created_at: '2025-01-01T00:00:00Z',
  components: [],
});

const defaultProps = {
  accountName: 'testuser',
  search: '',
  onSearchChange: vi.fn(),
  sort: 'modified' as AgentSort,
  onSortChange: vi.fn(),
};

// ── Rendering ─────────────────────────────────────────────────────────────────

describe('AgentsTab rendering', () => {
  it('renders a card for each deployment', () => {
    renderWithProviders(
      <AgentsTab
        {...defaultProps}
        deployments={[
          makeDeployment('d1', 'bot-one', 'Bot One'),
          makeDeployment('d2', 'bot-two', 'Bot Two'),
        ]}
      />,
    );
    expect(screen.getByText('Bot One')).toBeInTheDocument();
    expect(screen.getByText('Bot Two')).toBeInTheDocument();
  });

  it('falls back to name when displayName is not provided', () => {
    renderWithProviders(
      <AgentsTab
        {...defaultProps}
        deployments={[makeDeployment('d1', 'my-agent')]}
      />,
    );
    expect(screen.getByText('my-agent')).toBeInTheDocument();
  });

  it('shows the empty state when there are no deployments and no active filters', () => {
    renderWithProviders(<AgentsTab {...defaultProps} deployments={[]} />);
    expect(screen.getByText('No agents deployed yet.')).toBeInTheDocument();
  });

  it('shows the filtered empty state when search is active but there are no matches', () => {
    renderWithProviders(
      <AgentsTab {...defaultProps} deployments={[]} search="nomatch" />,
    );
    expect(screen.getByText('No agents match your search.')).toBeInTheDocument();
  });
});

// ── Search ────────────────────────────────────────────────────────────────────

describe('AgentsTab search', () => {
  it('renders the search input', () => {
    renderWithProviders(<AgentsTab {...defaultProps} deployments={[makeDeployment('d1', 'agent')]} />);
    expect(screen.getByPlaceholderText('Search agents')).toBeInTheDocument();
  });

  it('calls onSearchChange when typing in the search input', () => {
    const onSearchChange = vi.fn();
    renderWithProviders(
      <AgentsTab {...defaultProps} deployments={[makeDeployment('d1', 'agent')]} onSearchChange={onSearchChange} />,
    );
    const input = screen.getByPlaceholderText('Search agents');
    fireEvent.change(input, { target: { value: 'my-bot' } });
    expect(onSearchChange).toHaveBeenCalledWith('my-bot');
  });
});

// ── Sort dropdown ─────────────────────────────────────────────────────────────

describe('AgentsTab sort', () => {
  it('shows the sort dropdown with the default label', () => {
    renderWithProviders(<AgentsTab {...defaultProps} deployments={[makeDeployment('d1', 'agent')]} />);
    expect(screen.getByRole('button', { name: /Last modified/i })).toBeInTheDocument();
  });

  it('calls onSortChange when a sort option is selected', async () => {
    const user = userEvent.setup();
    const onSortChange = vi.fn();
    renderWithProviders(
      <AgentsTab {...defaultProps} deployments={[makeDeployment('d1', 'agent')]} onSortChange={onSortChange} />,
    );
    await user.click(screen.getByRole('button', { name: /Last modified/i }));
    await user.click(screen.getByRole('menuitem', { name: /name a/i }));
    expect(onSortChange).toHaveBeenCalledWith('name');
  });
});
