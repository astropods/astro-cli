import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, cleanup, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/test-utils';
import { HeartsTab } from './HeartsTab';
import type { HeartSort } from './HeartsTab';
import type { HeartedAgent } from '@/lib/api';
import { useHeartedBlueprints, useHeartToggleInList } from '@/api/queries/hearts';

vi.mock('@/api/queries/hearts', () => ({
  useHeartedBlueprints: vi.fn(),
  useHeartToggleInList: vi.fn(),
}));

afterEach(cleanup);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const makeItem = (name: string, account = 'someorg', heartCount = 1): HeartedAgent => ({
  account,
  name,
  visibility: 'public',
  heart_count: heartCount,
  deploy_count: 2,
  hearted_at: '2025-01-01T00:00:00Z',
  description: `${name} description`,
});

const mockMutate = vi.fn();

function setupMocks(
  items: HeartedAgent[] = [],
  opts: { isLoading?: boolean; nextCursor?: string } = {},
) {
  vi.mocked(useHeartedBlueprints).mockReturnValue({
    data: { items, next_cursor: opts.nextCursor },
    isLoading: opts.isLoading ?? false,
  } as unknown as ReturnType<typeof useHeartedBlueprints>);
  vi.mocked(useHeartToggleInList).mockReturnValue({ mutate: mockMutate } as unknown as ReturnType<typeof useHeartToggleInList>);
  mockMutate.mockClear();
}

const defaultProps = {
  accountName: 'testuser',
  isOwner: true,
  search: '',
  onSearchChange: vi.fn(),
  sort: 'newest' as HeartSort,
  onSortChange: vi.fn(),
};

// ── Loading ───────────────────────────────────────────────────────────────────

describe('HeartsTab loading', () => {
  it('shows loading text while fetching', () => {
    vi.mocked(useHeartedBlueprints).mockReturnValue({ data: undefined, isLoading: true } as unknown as ReturnType<typeof useHeartedBlueprints>);
    vi.mocked(useHeartToggleInList).mockReturnValue({ mutate: mockMutate } as unknown as ReturnType<typeof useHeartToggleInList>);
    renderWithProviders(<HeartsTab {...defaultProps} />);
    expect(screen.getByText('Loading…')).toBeInTheDocument();
  });
});

// ── Rendering ─────────────────────────────────────────────────────────────────

describe('HeartsTab rendering', () => {
  it('shows empty state when there are no items and no filters', () => {
    setupMocks([]);
    renderWithProviders(<HeartsTab {...defaultProps} />);
    expect(screen.getByText('No hearted blueprints yet.')).toBeInTheDocument();
  });

  it('shows filtered empty state when search is active but there are no matches', () => {
    setupMocks([]);
    renderWithProviders(<HeartsTab {...defaultProps} search="nomatch" />);
    expect(screen.getByText('No hearts match your search.')).toBeInTheDocument();
  });

  it('shows filtered empty state when sort is non-default and list is empty', () => {
    setupMocks([]);
    renderWithProviders(<HeartsTab {...defaultProps} sort="name" />);
    expect(screen.getByText('No hearts match your search.')).toBeInTheDocument();
  });

  it('renders a card for each item', () => {
    setupMocks([makeItem('alpha'), makeItem('beta')]);
    renderWithProviders(<HeartsTab {...defaultProps} />);
    expect(screen.getByRole('heading', { name: 'alpha' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'beta' })).toBeInTheDocument();
  });
});

// ── Search ────────────────────────────────────────────────────────────────────

describe('HeartsTab search', () => {
  it('renders the search input', () => {
    setupMocks();
    renderWithProviders(<HeartsTab {...defaultProps} />);
    expect(screen.getByPlaceholderText('Filter this page…')).toBeInTheDocument();
  });

  it('calls onSearchChange when typing in the search input', () => {
    setupMocks();
    const onSearchChange = vi.fn();
    renderWithProviders(<HeartsTab {...defaultProps} onSearchChange={onSearchChange} />);
    fireEvent.change(screen.getByPlaceholderText('Filter this page…'), { target: { value: 'foo' } });
    expect(onSearchChange).toHaveBeenCalledWith('foo');
  });

  it('filters cards by the search prop', () => {
    setupMocks([makeItem('alpha'), makeItem('zebra')]);
    renderWithProviders(<HeartsTab {...defaultProps} search="alpha" />);
    expect(screen.getByRole('heading', { name: 'alpha' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'zebra' })).not.toBeInTheDocument();
  });
});

// ── Sort dropdown ─────────────────────────────────────────────────────────────

describe('HeartsTab sort', () => {
  it('shows the default sort label', () => {
    setupMocks();
    renderWithProviders(<HeartsTab {...defaultProps} />);
    expect(screen.getByRole('button', { name: /date hearted/i })).toBeInTheDocument();
  });

  it('calls onSortChange when a sort option is selected', async () => {
    setupMocks();
    const user = userEvent.setup();
    const onSortChange = vi.fn();
    renderWithProviders(<HeartsTab {...defaultProps} onSortChange={onSortChange} />);
    await user.click(screen.getByRole('button', { name: /date hearted/i }));
    await user.click(screen.getByRole('menuitem', { name: /name a/i }));
    expect(onSortChange).toHaveBeenCalledWith('name');
  });

  it('sorts cards alphabetically when sort is "name"', () => {
    setupMocks([makeItem('zebra'), makeItem('apple')]);
    renderWithProviders(<HeartsTab {...defaultProps} sort="name" />);
    const headings = screen.getAllByRole('heading').map((h) => h.textContent);
    expect(headings.indexOf('apple')).toBeLessThan(headings.indexOf('zebra'));
  });

  it('sorts cards by heart count descending when sort is "popular"', () => {
    setupMocks([makeItem('less-popular', 'org', 2), makeItem('more-popular', 'org', 10)]);
    renderWithProviders(<HeartsTab {...defaultProps} sort="popular" />);
    const headings = screen.getAllByRole('heading').map((h) => h.textContent);
    expect(headings.indexOf('more-popular')).toBeLessThan(headings.indexOf('less-popular'));
  });
});

// ── Heart toggle ──────────────────────────────────────────────────────────────

describe('HeartsTab heart toggle', () => {
  it('shows the solid heart button for the owner', () => {
    setupMocks([makeItem('cool-agent')]);
    renderWithProviders(<HeartsTab {...defaultProps} isOwner />);
    expect(screen.getByRole('button', { name: /remove from hearts/i })).toBeInTheDocument();
  });

  it('hides the heart button for a non-owner', () => {
    setupMocks([makeItem('cool-agent')]);
    renderWithProviders(<HeartsTab {...defaultProps} isOwner={false} />);
    expect(
      screen.queryByRole('button', { name: /remove from hearts|add to hearts/i }),
    ).not.toBeInTheDocument();
  });

  it('calls mutate with account and name when the heart button is clicked', async () => {
    setupMocks([makeItem('cool-agent', 'someorg')]);
    const user = userEvent.setup();
    renderWithProviders(<HeartsTab {...defaultProps} isOwner />);
    await user.click(screen.getByRole('button', { name: /remove from hearts/i }));
    expect(mockMutate).toHaveBeenCalledWith(
      { account: 'someorg', name: 'cool-agent' },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });

  it('reverts optimistic toggle when the mutation errors', async () => {
    let capturedOnError: (() => void) | undefined;
    vi.mocked(useHeartedBlueprints).mockReturnValue({
      data: { items: [makeItem('cool-agent')], next_cursor: undefined },
      isLoading: false,
    } as unknown as ReturnType<typeof useHeartedBlueprints>);
    vi.mocked(useHeartToggleInList).mockReturnValue({
      mutate: vi.fn((_vars, opts?: { onError?: () => void }) => {
        capturedOnError = opts?.onError;
      }),
    } as unknown as ReturnType<typeof useHeartToggleInList>);
    const user = userEvent.setup();
    renderWithProviders(<HeartsTab {...defaultProps} isOwner />);
    await user.click(screen.getByRole('button', { name: /remove from hearts/i }));
    // After click, optimistic state shows outline
    expect(screen.getByRole('button', { name: /add to hearts/i })).toBeInTheDocument();
    // Simulate API failure — onError should revert to solid
    capturedOnError?.();
    expect(await screen.findByRole('button', { name: /remove from hearts/i })).toBeInTheDocument();
  });

  it('switches from solid to outline heart after click', async () => {
    setupMocks([makeItem('cool-agent')]);
    const user = userEvent.setup();
    renderWithProviders(<HeartsTab {...defaultProps} isOwner />);
    await user.click(screen.getByRole('button', { name: /remove from hearts/i }));
    expect(screen.getByRole('button', { name: /add to hearts/i })).toBeInTheDocument();
  });

  it('toggles back to solid heart when clicked again', async () => {
    setupMocks([makeItem('cool-agent')]);
    const user = userEvent.setup();
    renderWithProviders(<HeartsTab {...defaultProps} isOwner />);
    await user.click(screen.getByRole('button', { name: /remove from hearts/i }));
    await user.click(screen.getByRole('button', { name: /add to hearts/i }));
    expect(screen.getByRole('button', { name: /remove from hearts/i })).toBeInTheDocument();
  });

  it('toggles each card independently', async () => {
    setupMocks([makeItem('alpha'), makeItem('beta')]);
    const user = userEvent.setup();
    renderWithProviders(<HeartsTab {...defaultProps} isOwner />);
    const [alphaBtn] = screen.getAllByRole('button', { name: /remove from hearts/i });
    await user.click(alphaBtn);
    // alpha is now outline; beta is still solid
    expect(screen.getAllByRole('button', { name: /remove from hearts/i })).toHaveLength(1);
    expect(screen.getByRole('button', { name: /add to hearts/i })).toBeInTheDocument();
  });
});

// ── Pagination ────────────────────────────────────────────────────────────────

describe('HeartsTab pagination', () => {
  it('hides pagination when there is only one page', () => {
    setupMocks([makeItem('alpha')]);
    renderWithProviders(<HeartsTab {...defaultProps} />);
    expect(screen.queryByRole('button', { name: /previous/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /next/i })).not.toBeInTheDocument();
  });

  it('shows Next (enabled) and Previous (disabled) on the first page when more pages exist', () => {
    setupMocks([makeItem('alpha')], { nextCursor: 'cursor-abc' });
    renderWithProviders(<HeartsTab {...defaultProps} />);
    expect(screen.getByRole('button', { name: /next/i })).toBeEnabled();
    expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled();
  });
});
