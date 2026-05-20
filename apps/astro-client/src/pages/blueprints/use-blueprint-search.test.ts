import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useBlueprintSearch } from './use-blueprint-search';

describe('useBlueprintSearch', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('debounces search into params', () => {
    const { result } = renderHook(() => useBlueprintSearch(300));

    act(() => {
      result.current.setSearch('bot');
    });
    expect(result.current.params).toEqual({});

    act(() => {
      vi.advanceTimersByTime(300);
    });
    expect(result.current.params).toEqual({ q: 'bot' });
    expect(result.current.hasActiveFilters).toBe(true);
  });

  it('clears params when search is emptied', async () => {
    const { result } = renderHook(() => useBlueprintSearch(300));

    act(() => {
      result.current.setSearch('bot');
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    expect(result.current.params).toEqual({ q: 'bot' });

    act(() => {
      result.current.setSearch('');
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    expect(result.current.params).toEqual({});
    expect(result.current.hasActiveFilters).toBe(false);
  });
});
