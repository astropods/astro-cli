import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { BLUEPRINT_PAGE_SIZE_COOKIE } from '@/lib/blueprint-page-size-preference';
import { BlueprintPageSizeControl } from './BlueprintPageSizeControl';

function clearCookie(name: string) {
  document.cookie = `${name}=;path=/;max-age=0`;
}

describe('BlueprintPageSizeControl', () => {
  beforeEach(() => {
    localStorage.clear();
    clearCookie(BLUEPRINT_PAGE_SIZE_COOKIE);
  });

  afterEach(() => {
    cleanup();
    localStorage.clear();
    clearCookie(BLUEPRINT_PAGE_SIZE_COOKIE);
  });

  it('renders the current value', () => {
    render(<BlueprintPageSizeControl value={20} onChange={() => {}} />);
    expect(screen.getByLabelText('20 per page')).toHaveAttribute('aria-pressed', 'true');
  });

  it('persists and reports changes', () => {
    const onChange = vi.fn();
    render(<BlueprintPageSizeControl value={50} onChange={onChange} />);

    fireEvent.click(screen.getByLabelText('10 per page'));

    expect(onChange).toHaveBeenCalledWith(10);
    expect(localStorage.getItem('astro:blueprints:page-size')).toBe('10');
    expect(document.cookie).toContain(`${BLUEPRINT_PAGE_SIZE_COOKIE}=10`);
  });
});
