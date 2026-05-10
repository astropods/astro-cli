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
  // Reorder defaults
  reorderMode: 'idle' as const,
  hasCustomOrder: false,
  onEnterReorder: vi.fn(),
  onSaveReorder: vi.fn(),
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

// ── Customize order button ────────────────────────────────────────────────────

describe('BlueprintsTab customize order', () => {
  it('shows "Customize order" button for owner in internal view', () => {
    renderWithProviders(
      <BlueprintsTab {...defaultProps} blueprints={[]} isOwner isInternalView />,
    );
    expect(screen.getByRole('button', { name: /customize order/i })).toBeInTheDocument();
  });

  it('hides "Customize order" button for visitor', () => {
    renderWithProviders(
      <BlueprintsTab {...defaultProps} blueprints={[]} isOwner={false} />,
    );
    expect(screen.queryByRole('button', { name: /customize order/i })).not.toBeInTheDocument();
  });

  it('hides "Customize order" button in external view', () => {
    renderWithProviders(
      <BlueprintsTab {...defaultProps} blueprints={[]} isOwner isInternalView={false} />,
    );
    expect(screen.queryByRole('button', { name: /customize order/i })).not.toBeInTheDocument();
  });

  it('still shows "Customize order" when a custom order is already saved', () => {
    renderWithProviders(
      <BlueprintsTab {...defaultProps} blueprints={[]} hasCustomOrder />,
    );
    expect(screen.getByRole('button', { name: /customize order/i })).toBeInTheDocument();
  });

  it('calls onEnterReorder when "Customize order" is clicked', async () => {
    const user = userEvent.setup();
    const onEnterReorder = vi.fn();
    renderWithProviders(
      <BlueprintsTab {...defaultProps} blueprints={[]} onEnterReorder={onEnterReorder} />,
    );
    await user.click(screen.getByRole('button', { name: /customize order/i }));
    expect(onEnterReorder).toHaveBeenCalledOnce();
  });
});

// ── Editing mode ──────────────────────────────────────────────────────────────

const blueprintsForReorder = [
  makeBlueprint('alpha'),
  makeBlueprint('beta'),
  makeBlueprint('gamma'),
];

describe('BlueprintsTab editing mode', () => {
  it('shows "Save changes" button when reorderMode is editing', () => {
    renderWithProviders(
      <BlueprintsTab
        {...defaultProps}
        blueprints={blueprintsForReorder}
        reorderMode="editing"
      />,
    );
    expect(screen.getByRole('button', { name: /save changes/i })).toBeInTheDocument();
    // No cancel button — Taylor's UX has no cancel
    expect(screen.queryByRole('button', { name: /cancel/i })).not.toBeInTheDocument();
  });

  it('disables search/sort controls in editing mode', () => {
    renderWithProviders(
      <BlueprintsTab
        {...defaultProps}
        blueprints={blueprintsForReorder}
        reorderMode="editing"
      />,
    );
    expect(screen.getByPlaceholderText(/search blueprints/i)).toBeDisabled();
    expect(screen.getByRole('button', { name: /newest/i })).toBeDisabled();
  });

  it('renders all blueprints as sortable cards in editing mode', () => {
    renderWithProviders(
      <BlueprintsTab
        {...defaultProps}
        blueprints={blueprintsForReorder}
        reorderMode="editing"
      />,
    );
    expect(screen.getByRole('heading', { name: 'alpha' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'beta' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'gamma' })).toBeInTheDocument();
  });

  it('calls onSaveReorder with blueprint names when "Save changes" is clicked', async () => {
    const user = userEvent.setup();
    const onSaveReorder = vi.fn();
    renderWithProviders(
      <BlueprintsTab
        {...defaultProps}
        blueprints={blueprintsForReorder}
        reorderMode="editing"
        onSaveReorder={onSaveReorder}
      />,
    );
    await user.click(screen.getByRole('button', { name: /save changes/i }));
    expect(onSaveReorder).toHaveBeenCalledOnce();
    expect(onSaveReorder).toHaveBeenCalledWith(['alpha', 'beta', 'gamma']);
  });

  it('shows "Saved" confirmation when reorderMode is saved', () => {
    renderWithProviders(
      <BlueprintsTab
        {...defaultProps}
        blueprints={blueprintsForReorder}
        reorderMode="saved"
      />,
    );
    expect(screen.getByRole('button', { name: /saved/i })).toBeInTheDocument();
    // No cancel button in saved state either
    expect(screen.queryByRole('button', { name: /cancel/i })).not.toBeInTheDocument();
  });
});

// ── Grip handles ──────────────────────────────────────────────────────────────
// Grips only appear in editing mode (SortableBlueprintCard); idle mode shows no grip.

describe('BlueprintsTab grip handles', () => {
  it('does not show grip hints in idle mode', () => {
    renderWithProviders(
      <BlueprintsTab
        {...defaultProps}
        blueprints={[makeBlueprint('alpha'), makeBlueprint('beta')]}
        isOwner
        isInternalView
      />,
    );
    expect(screen.queryByTestId('grip-hint')).not.toBeInTheDocument();
  });
});

// ── localStorage persistence (via parent IndividualProfile) ──────────────────
// These tests verify that IndividualProfile correctly loads/saves custom order.
// The actual persistence is tested at the IndividualProfile level.

describe('BlueprintsTab onEnterReorder resets filters', () => {
  it('clicking "Customize order" calls onEnterReorder which resets filters in parent', async () => {
    const user = userEvent.setup();
    const onEnterReorder = vi.fn();
    renderWithProviders(
      <BlueprintsTab
        {...defaultProps}
        blueprints={[makeBlueprint('alpha')]}
        search="foo"
        sort="name"
        onEnterReorder={onEnterReorder}
      />,
    );
    await user.click(screen.getByRole('button', { name: /customize order/i }));
    // The actual filter reset is done by the parent — we just verify the callback fires
    expect(onEnterReorder).toHaveBeenCalledOnce();
  });
});
