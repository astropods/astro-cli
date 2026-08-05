import { describe, it, expect } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useBlueprintSearch } from './use-blueprint-search';

// The hook holds the settled term only; the search box debounces before it
// reports (see DebouncedFilterInput.test.tsx), so a term reaching this hook is
// already one the user stopped typing.
describe('useBlueprintSearch', () => {
  it('turns a settled term into list params', () => {
    const { result } = renderHook(() => useBlueprintSearch());
    expect(result.current.params).toEqual({});
    expect(result.current.hasActiveFilters).toBe(false);

    act(() => {
      result.current.setSearch('bot');
    });

    expect(result.current.params).toEqual({ q: 'bot' });
    expect(result.current.hasActiveFilters).toBe(true);
  });

  it('clears params when the term is emptied', () => {
    const { result } = renderHook(() => useBlueprintSearch());

    act(() => {
      result.current.setSearch('bot');
    });
    expect(result.current.params).toEqual({ q: 'bot' });

    act(() => {
      result.current.setSearch('');
    });
    expect(result.current.params).toEqual({});
    expect(result.current.hasActiveFilters).toBe(false);
  });

  it('ignores a whitespace-only term', () => {
    const { result } = renderHook(() => useBlueprintSearch());

    act(() => {
      result.current.setSearch('   ');
    });

    expect(result.current.params).toEqual({});
    expect(result.current.hasActiveFilters).toBe(false);
  });
});
