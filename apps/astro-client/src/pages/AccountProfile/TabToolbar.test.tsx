import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, cleanup, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/test-utils';
import { TabSearchInput, TabFilterDropdown } from './TabToolbar';

afterEach(cleanup);

// ── TabSearchInput ─────────────────────────────────────────────────────────────

describe('TabSearchInput', () => {
  it('renders with the given placeholder', () => {
    renderWithProviders(
      <TabSearchInput value="" onChange={vi.fn()} placeholder="Search things…" />,
    );
    expect(screen.getByPlaceholderText('Search things…')).toBeInTheDocument();
  });

  it('hides the clear button when value is empty', () => {
    renderWithProviders(
      <TabSearchInput value="" onChange={vi.fn()} placeholder="Search…" />,
    );
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('shows the clear button when value is non-empty', () => {
    renderWithProviders(
      <TabSearchInput value="hello" onChange={vi.fn()} placeholder="Search…" />,
    );
    expect(screen.getByRole('button')).toBeInTheDocument();
  });

  it('calls onChange with empty string when the clear button is clicked', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithProviders(
      <TabSearchInput value="hello" onChange={onChange} placeholder="Search…" />,
    );
    await user.click(screen.getByRole('button'));
    expect(onChange).toHaveBeenCalledWith('');
  });

  it('calls onChange with the new value when typing', () => {
    const onChange = vi.fn();
    renderWithProviders(
      <TabSearchInput value="" onChange={onChange} placeholder="Search…" />,
    );
    fireEvent.change(screen.getByPlaceholderText('Search…'), {
      target: { value: 'hello' },
    });
    expect(onChange).toHaveBeenCalledWith('hello');
  });
});

// ── TabFilterDropdown ─────────────────────────────────────────────────────────

const sortOptions = [
  { value: 'newest', label: 'Newest' },
  { value: 'name', label: 'Name A–Z' },
];

describe('TabFilterDropdown', () => {
  it('renders the trigger with the provided label', () => {
    renderWithProviders(
      <TabFilterDropdown
        value="modified"
        onChange={vi.fn()}
        options={sortOptions}
        triggerLabel="Last modified"
      />,
    );
    expect(screen.getByRole('button', { name: /last modified/i })).toBeInTheDocument();
  });

  it('shows all options when the trigger is clicked', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <TabFilterDropdown
        value="modified"
        onChange={vi.fn()}
        options={sortOptions}
        triggerLabel="Last modified"
      />,
    );
    await user.click(screen.getByRole('button', { name: /last modified/i }));
    expect(screen.getByRole('menuitem', { name: /newest/i })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: /name a/i })).toBeInTheDocument();
  });

  it('calls onChange with the correct value when an option is selected', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithProviders(
      <TabFilterDropdown
        value="modified"
        onChange={onChange}
        options={sortOptions}
        triggerLabel="Last modified"
      />,
    );
    await user.click(screen.getByRole('button', { name: /last modified/i }));
    await user.click(screen.getByRole('menuitem', { name: /name a/i }));
    expect(onChange).toHaveBeenCalledWith('name');
  });

  it('calls onChange even when re-selecting the already-selected option', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithProviders(
      <TabFilterDropdown
        value="newest"
        onChange={onChange}
        options={sortOptions}
        triggerLabel="Newest"
      />,
    );
    await user.click(screen.getByRole('button', { name: /newest/i }));
    await user.click(screen.getByRole('menuitem', { name: /newest/i }));
    // Radix DropdownMenuItem fires onSelect regardless of current value
    expect(onChange).toHaveBeenCalledWith('newest');
  });
});
