import { describe, it, expect, afterEach } from 'vitest';
import { screen, cleanup, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderWithProviders } from '@/test/test-utils';
import { useState } from 'react';
import { InviteInput, type InviteEntry } from './InviteInput';

afterEach(cleanup);

// MSW handler for account search — returns empty by default
server.use(
  http.get('/api/v1/accounts/search', () =>
    HttpResponse.json({ results: [] }),
  ),
);

/** Controlled wrapper so chips persist across re-renders. */
function Harness({
  initial = [],
  exclude,
}: {
  initial?: InviteEntry[];
  exclude?: Set<string>;
}) {
  const [entries, setEntries] = useState(initial);
  return (
    <InviteInput
      entries={entries}
      onChange={setEntries}
      exclude={exclude}
    />
  );
}

function renderHarness(props?: Parameters<typeof Harness>[0]) {
  return renderWithProviders(<Harness {...props} />);
}

function getInput() {
  return screen.getByRole('combobox');
}

describe('InviteInput', () => {
  describe('rendering', () => {
    it('shows placeholder when empty', () => {
      renderHarness();
      expect(getInput()).toHaveAttribute(
        'placeholder',
        'Search by username or enter an email',
      );
    });

    it('renders pre-existing entries as chips', () => {
      renderHarness({
        initial: [
          { value: 'alice', kind: 'account', valid: true },
          { value: 'bob@test.com', kind: 'email', valid: true },
        ],
      });
      expect(screen.getByText('alice')).toBeInTheDocument();
      expect(screen.getByText('bob@test.com')).toBeInTheDocument();
    });

    it('hides placeholder when entries exist', () => {
      renderHarness({
        initial: [{ value: 'alice', kind: 'account', valid: true }],
      });
      expect(getInput()).not.toHaveAttribute('placeholder');
    });

    it('applies strikethrough and reduced opacity for invalid entries', () => {
      renderHarness({
        initial: [{ value: 'bad', kind: 'email', valid: false }],
      });
      const text = screen.getByText('bad');
      expect(text).toHaveClass('line-through');
      // The chip (parent span with tabIndex) should have opacity-60
      expect(text.closest('[tabindex="-1"]')).toHaveClass('opacity-60');
    });
  });

  describe('adding entries', () => {
    it('adds an email entry via dropdown on Enter', async () => {
      const user = userEvent.setup();
      renderHarness();

      await user.type(getInput(), 'test@example.com');
      await user.keyboard('{Enter}');

      expect(screen.getByText('test@example.com')).toBeInTheDocument();
      expect(getInput()).toHaveValue('');
    });

    it('adds an email entry by clicking the dropdown option', async () => {
      const user = userEvent.setup();
      renderHarness();

      await user.type(getInput(), 'hi@test.com');

      const option = await screen.findByText((_, el) =>
        el?.textContent?.includes('hi@test.com') && el?.tagName === 'LI' ? true : false,
      );
      await user.click(option);

      expect(screen.getByText('hi@test.com')).toBeInTheDocument();
      expect(getInput()).toHaveValue('');
    });

    it('does not add duplicate entries', async () => {
      const user = userEvent.setup();
      renderHarness({
        initial: [{ value: 'a@b.com', kind: 'email', valid: true }],
      });

      await user.type(getInput(), 'a@b.com');
      // Dropdown should not show an option for the already-added email
      expect(screen.queryByRole('option')).not.toBeInTheDocument();
    });

    it('adds account entries from search results', async () => {
      server.use(
        http.get('/api/v1/accounts/search', () =>
          HttpResponse.json({
            results: [{ id: '1', name: 'charlie', type: 'personal' }],
          }),
        ),
      );
      const user = userEvent.setup();
      renderHarness();

      await user.type(getInput(), 'charlie');

      const option = await screen.findByText('charlie');
      await user.click(option);

      // Should now be a chip
      await waitFor(() => {
        expect(screen.getByText('charlie')).toBeInTheDocument();
      });
      expect(getInput()).toHaveValue('');
    });
  });

  describe('removing entries', () => {
    it('removes a chip when clicking its X button', async () => {
      const user = userEvent.setup();
      renderHarness({
        initial: [
          { value: 'alice', kind: 'account', valid: true },
          { value: 'bob', kind: 'account', valid: true },
        ],
      });

      // Click the X on alice's chip
      const aliceChip = screen.getByText('alice').closest('[tabindex="-1"]')!;
      const xButton = aliceChip.querySelector('span.cursor-pointer')!;
      await user.click(xButton);

      expect(screen.queryByText('alice')).not.toBeInTheDocument();
      expect(screen.getByText('bob')).toBeInTheDocument();
    });

    it('removes the last chip on Backspace when input is empty', async () => {
      const user = userEvent.setup();
      renderHarness({
        initial: [
          { value: 'alice', kind: 'account', valid: true },
          { value: 'bob', kind: 'account', valid: true },
        ],
      });

      getInput().focus();
      await user.keyboard('{Backspace}');

      expect(screen.queryByText('bob')).not.toBeInTheDocument();
      expect(screen.getByText('alice')).toBeInTheDocument();
    });

    it('removes a focused chip on Backspace', async () => {
      const user = userEvent.setup();
      renderHarness({
        initial: [
          { value: 'alice', kind: 'account', valid: true },
          { value: 'bob', kind: 'account', valid: true },
        ],
      });

      const aliceChip = screen.getByText('alice').closest('[tabindex="-1"]') as HTMLElement;
      aliceChip.focus();
      await user.keyboard('{Backspace}');

      expect(screen.queryByText('alice')).not.toBeInTheDocument();
      expect(screen.getByText('bob')).toBeInTheDocument();
    });
  });

  describe('chip keyboard navigation', () => {
    it('moves focus from input to last chip on ArrowLeft', async () => {
      const user = userEvent.setup();
      renderHarness({
        initial: [
          { value: 'alice', kind: 'account', valid: true },
          { value: 'bob', kind: 'account', valid: true },
        ],
      });

      getInput().focus();
      await user.keyboard('{ArrowLeft}');

      const bobChip = screen.getByText('bob').closest('[tabindex="-1"]');
      expect(document.activeElement).toBe(bobChip);
    });

    it('navigates between chips with ArrowLeft/ArrowRight', async () => {
      const user = userEvent.setup();
      renderHarness({
        initial: [
          { value: 'alice', kind: 'account', valid: true },
          { value: 'bob', kind: 'account', valid: true },
        ],
      });

      // Focus bob's chip
      const bobChip = screen.getByText('bob').closest('[tabindex="-1"]') as HTMLElement;
      bobChip.focus();

      // ArrowLeft → alice
      await user.keyboard('{ArrowLeft}');
      const aliceChip = screen.getByText('alice').closest('[tabindex="-1"]');
      expect(document.activeElement).toBe(aliceChip);

      // ArrowRight → bob
      await user.keyboard('{ArrowRight}');
      expect(document.activeElement).toBe(bobChip);

      // ArrowRight → input
      await user.keyboard('{ArrowRight}');
      expect(document.activeElement).toBe(getInput());
    });
  });

  describe('paste handling', () => {
    it('adds multiple emails from a comma-separated paste', async () => {
      const user = userEvent.setup();
      renderHarness();

      getInput().focus();
      await user.paste('a@b.com, c@d.com, e@f.com');

      expect(screen.getByText('a@b.com')).toBeInTheDocument();
      expect(screen.getByText('c@d.com')).toBeInTheDocument();
      expect(screen.getByText('e@f.com')).toBeInTheDocument();
      expect(getInput()).toHaveValue('');
    });

    it('deduplicates pasted values against existing entries', async () => {
      const user = userEvent.setup();
      renderHarness({
        initial: [{ value: 'a@b.com', kind: 'email', valid: true }],
      });

      getInput().focus();
      await user.paste('a@b.com, new@test.com');

      // Only one a@b.com chip should exist (the original)
      expect(screen.getAllByText('a@b.com')).toHaveLength(1);
      expect(screen.getByText('new@test.com')).toBeInTheDocument();
    });
  });

  describe('exclude prop', () => {
    it('excludes accounts from search results', async () => {
      server.use(
        http.get('/api/v1/accounts/search', () =>
          HttpResponse.json({
            results: [
              { id: '1', name: 'alice', type: 'personal' },
              { id: '2', name: 'bob', type: 'personal' },
            ],
          }),
        ),
      );
      const user = userEvent.setup();
      renderHarness({ exclude: new Set(['alice']) });

      await user.type(getInput(), 'alice');

      // Wait for results, but alice should be excluded
      await waitFor(() => {
        expect(screen.queryByRole('option', { name: /alice/ })).not.toBeInTheDocument();
      });
    });
  });
});
