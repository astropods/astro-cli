import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/test-utils';
import { BlueprintsTab } from './BlueprintsTab';
import type { VisibilityFilter, BlueprintSort } from './BlueprintsTab';
import type { Blueprint } from '@/lib/api';

afterEach(cleanup);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const makeBlueprint = (name: string, visibility: 'public' | 'private' = 'public'): Blueprint => ({
  name,
  account: 'testuser',
  registry: 'reg.example.com',
  visibility,
  versions: [],
});

const defaultProps = {
  accountName: 'testuser',
  isOwner: true,
  isInternalView: true,
  search: '',
  onSearchChange: vi.fn(),
  visibility: 'all' as VisibilityFilter,
  onVisibilityChange: vi.fn(),
  sort: 'newest' as BlueprintSort,
  onSortChange: vi.fn(),
};

// ── Rendering ─────────────────────────────────────────────────────────────────

describe('BlueprintsTab rendering', () => {
  it('renders a card for each blueprint', () => {
    renderWithProviders(
      <BlueprintsTab
        {...defaultProps}
        blueprints={[makeBlueprint('alpha'), makeBlueprint('beta')]}
      />,
    );
    expect(screen.getByRole('heading', { name: 'alpha' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'beta' })).toBeInTheDocument();
  });

  it('shows the empty state when there are no blueprints and no active filters', () => {
    renderWithProviders(<BlueprintsTab {...defaultProps} blueprints={[]} />);
    expect(screen.getByText('No blueprints published yet.')).toBeInTheDocument();
  });

  it('shows the filtered empty state when search is active but there are no matches', () => {
    renderWithProviders(
      <BlueprintsTab {...defaultProps} blueprints={[]} search="nomatch" />,
    );
    expect(screen.getByText('No blueprints match your filters.')).toBeInTheDocument();
  });

  it('shows the filtered empty state when sort is non-default', () => {
    renderWithProviders(
      <BlueprintsTab {...defaultProps} blueprints={[]} sort="name" />,
    );
    expect(screen.getByText('No blueprints match your filters.')).toBeInTheDocument();
  });
});

// ── Visibility dropdown ───────────────────────────────────────────────────────

describe('BlueprintsTab visibility filter', () => {
  it('shows the visibility dropdown in internal view', () => {
    renderWithProviders(
      <BlueprintsTab {...defaultProps} blueprints={[]} isInternalView />,
    );
    // Trigger label when "all" is selected
    expect(screen.getByRole('button', { name: /visibility/i })).toBeInTheDocument();
  });

  it('hides the visibility dropdown in external view', () => {
    renderWithProviders(
      <BlueprintsTab {...defaultProps} blueprints={[]} isInternalView={false} />,
    );
    expect(screen.queryByRole('button', { name: /visibility/i })).not.toBeInTheDocument();
  });

  it('calls onVisibilityChange when a visibility option is selected', async () => {
    const user = userEvent.setup();
    const onVisibilityChange = vi.fn();
    renderWithProviders(
      <BlueprintsTab
        {...defaultProps}
        blueprints={[]}
        isInternalView
        onVisibilityChange={onVisibilityChange}
      />,
    );
    await user.click(screen.getByRole('button', { name: /visibility/i }));
    await user.click(screen.getByRole('menuitem', { name: /private/i }));
    expect(onVisibilityChange).toHaveBeenCalledWith('private');
  });
});

// ── Sort dropdown ─────────────────────────────────────────────────────────────

describe('BlueprintsTab sort', () => {
  it('shows the sort dropdown with the default label', () => {
    renderWithProviders(<BlueprintsTab {...defaultProps} blueprints={[]} />);
    expect(screen.getByRole('button', { name: /newest/i })).toBeInTheDocument();
  });

  it('calls onSortChange when a sort option is selected', async () => {
    const user = userEvent.setup();
    const onSortChange = vi.fn();
    renderWithProviders(
      <BlueprintsTab {...defaultProps} blueprints={[]} onSortChange={onSortChange} />,
    );
    await user.click(screen.getByRole('button', { name: /newest/i }));
    await user.click(screen.getByRole('menuitem', { name: /name a/i }));
    expect(onSortChange).toHaveBeenCalledWith('name');
  });
});
