import { useState } from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor, cleanup, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DebouncedFilterInput } from './DebouncedFilterInput';

describe('DebouncedFilterInput', () => {
  it('reports the term once the user stops typing, not per keystroke', async () => {
    const user = userEvent.setup();
    const onDebouncedChange = vi.fn();
    render(
      <DebouncedFilterInput
        placeholder="Search"
        value=""
        onDebouncedChange={onDebouncedChange}
        debounceMs={50}
      />,
    );

    const input = screen.getByPlaceholderText('Search');
    await user.type(input, 'agent');

    // The box shows every keystroke immediately; the owner hears about it once.
    expect(input).toHaveValue('agent');
    await waitFor(() => expect(onDebouncedChange).toHaveBeenCalledWith('agent'));
    expect(onDebouncedChange).toHaveBeenCalledTimes(1);
    cleanup();
  });

  it('reports an emptied box so the owner can drop the filter', async () => {
    const user = userEvent.setup();
    const onDebouncedChange = vi.fn();
    render(
      <DebouncedFilterInput
        placeholder="Search"
        value=""
        onDebouncedChange={onDebouncedChange}
        debounceMs={50}
      />,
    );

    const input = screen.getByPlaceholderText('Search');
    await user.type(input, 'agent');
    await waitFor(() => expect(onDebouncedChange).toHaveBeenCalledWith('agent'));

    await user.clear(input);

    await waitFor(() => expect(onDebouncedChange).toHaveBeenLastCalledWith(''));
    cleanup();
  });

  it('does not report anything on mount, including with a term already set', async () => {
    const onDebouncedChange = vi.fn();
    render(
      <DebouncedFilterInput
        placeholder="Search"
        value="preset"
        onDebouncedChange={onDebouncedChange}
        debounceMs={10}
      />,
    );

    expect(screen.getByPlaceholderText('Search')).toHaveValue('preset');
    await new Promise((resolve) => setTimeout(resolve, 40));
    expect(onDebouncedChange).not.toHaveBeenCalled();
    cleanup();
  });

  it('adopts an external reset without echoing it back', async () => {
    // Mirrors a "Clear filters" button: the owner drops the term, and the box
    // has to follow without reporting the reset as a fresh user edit.
    const onDebouncedChange = vi.fn();

    function Harness() {
      const [term, setTerm] = useState('');
      return (
        <>
          <button onClick={() => setTerm('')}>clear</button>
          <DebouncedFilterInput
            placeholder="Search"
            value={term}
            onDebouncedChange={(next) => {
              onDebouncedChange(next);
              setTerm(next);
            }}
            debounceMs={50}
          />
        </>
      );
    }

    const user = userEvent.setup();
    render(<Harness />);
    const input = screen.getByPlaceholderText('Search');

    await user.type(input, 'agent');
    await waitFor(() => expect(onDebouncedChange).toHaveBeenCalledWith('agent'));
    onDebouncedChange.mockClear();

    await user.click(screen.getByRole('button', { name: 'clear' }));

    await waitFor(() => expect(input).toHaveValue(''));
    await new Promise((resolve) => setTimeout(resolve, 120));
    expect(onDebouncedChange).not.toHaveBeenCalled();
    cleanup();
  });

  it('adopts a reset that leaves the term unchanged', async () => {
    // "Clear filters" is reachable with an empty term: another filter can be
    // active and matching nothing. Dropping the term is then a no-op, so the
    // reset is invisible to `value` and has to be signalled separately.
    const onDebouncedChange = vi.fn();

    function Harness() {
      const [term, setTerm] = useState('');
      const [resetKey, setResetKey] = useState(0);
      return (
        <>
          <button
            onClick={() => {
              setTerm('');
              setResetKey((key) => key + 1);
            }}
          >
            clear
          </button>
          <DebouncedFilterInput
            placeholder="Search"
            value={term}
            resetKey={resetKey}
            onDebouncedChange={(next) => {
              onDebouncedChange(next);
              setTerm(next);
            }}
            debounceMs={50}
          />
        </>
      );
    }

    render(<Harness />);
    const input = screen.getByPlaceholderText('Search');

    // Both events are synchronous, so the clear lands inside the debounce
    // window, while the box holds text its owner has never heard about.
    fireEvent.change(input, { target: { value: 'z' } });
    fireEvent.click(screen.getByRole('button', { name: 'clear' }));

    await new Promise((resolve) => setTimeout(resolve, 150));
    expect(input).toHaveValue('');
    expect(onDebouncedChange).not.toHaveBeenCalled();
    cleanup();
  });
});
