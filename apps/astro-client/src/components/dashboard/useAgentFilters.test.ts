import { describe, it, expect } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useAgentFilters } from './useAgentFilters';
import type { AgentDeploymentSummary } from '@/lib/api';

function makeDeployment(overrides: Partial<AgentDeploymentSummary> & { id: string; name: string }): AgentDeploymentSummary {
  return {
    build_id: 'b1',
    namespace: 'ns-1',
    status: 'Running',
    created_at: '2025-01-01T00:00:00Z',
    ...overrides,
  };
}

const alpha = makeDeployment({ id: 'a', name: 'alpha-agent', display_name: 'Alpha Bot', created_at: '2025-01-03T00:00:00Z' });
const beta = makeDeployment({ id: 'b', name: 'beta-agent', display_name: 'Beta Bot', created_at: '2025-01-02T00:00:00Z' });
const gamma = makeDeployment({ id: 'c', name: 'gamma-agent', display_name: 'Error Gamma', status: 'error', created_at: '2025-01-01T00:00:00Z' });

const deployments = [alpha, beta, gamma];

describe('useAgentFilters', () => {
  it('returns all deployments with no filters applied', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    expect(result.current.filtered).toHaveLength(3);
  });

  it('filters by name substring (case-insensitive)', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onFilterChange('alpha'));
    expect(result.current.filtered).toEqual([alpha]);
  });

  it('filters by display_name substring', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onFilterChange('error'));
    expect(result.current.filtered).toEqual([gamma]);
  });

  it('returns empty when filter matches nothing', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onFilterChange('zzz-no-match'));
    expect(result.current.filtered).toHaveLength(0);
  });

  it('sorts by name alphabetically', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onSortChange('name'));
    const names = result.current.filtered.map((d) => d.display_name);
    expect(names).toEqual(['Alpha Bot', 'Beta Bot', 'Error Gamma']);
  });

  it('sorts by recent (most recently updated/created first)', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onSortChange('recent'));
    expect(result.current.filtered[0]).toEqual(alpha);
    expect(result.current.filtered[2]).toEqual(gamma);
  });

  it('sorts by requests descending', () => {
    const counts = new Map([
      ['a', 10],
      ['b', 50],
      ['c', 5],
    ]);
    const { result } = renderHook(() => useAgentFilters(deployments, counts));
    act(() => result.current.toolbarProps.onSortChange('requests'));
    expect(result.current.filtered.map((d) => d.id)).toEqual(['b', 'a', 'c']);
  });

  it('applies text filter and sort together', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => {
      result.current.toolbarProps.onFilterChange('bot');
      result.current.toolbarProps.onSortChange('name');
    });
    expect(result.current.filtered).toEqual([alpha, beta]);
  });

  it('clears filter on empty string', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onFilterChange('alpha'));
    expect(result.current.filtered).toHaveLength(1);
    act(() => result.current.toolbarProps.onFilterChange(''));
    expect(result.current.filtered).toHaveLength(3);
  });

  it('filters by status: active matches the Running UI status', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onStatusChange('active'));
    expect(result.current.filtered).toEqual([alpha, beta]);
  });

  it('filters by status: error matches the error UI status', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onStatusChange('error'));
    expect(result.current.filtered).toEqual([gamma]);
  });

  it('filters by status: stopped matches the Stopped UI status', () => {
    const stopped = makeDeployment({ id: 'd', name: 'delta-agent', display_name: 'Delta Bot', status: 'Stopped' });
    const { result } = renderHook(() => useAgentFilters([...deployments, stopped]));
    act(() => result.current.toolbarProps.onStatusChange('stopped'));
    expect(result.current.filtered.map((d) => d.id)).toEqual(['d']);
  });

  it('combines the status filter with text search', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => {
      result.current.toolbarProps.onStatusChange('active');
      result.current.toolbarProps.onFilterChange('alpha');
    });
    expect(result.current.filtered).toEqual([alpha]);
  });

  it('clears the status filter with null', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onStatusChange('error'));
    expect(result.current.filtered).toHaveLength(1);
    act(() => result.current.toolbarProps.onStatusChange(null));
    expect(result.current.filtered).toHaveLength(3);
  });

  it('persists the status filter and restores it on a later mount', () => {
    const first = renderHook(() => useAgentFilters(deployments));
    act(() => first.result.current.toolbarProps.onStatusChange('error'));
    first.unmount();

    // A fresh mount (e.g. the user navigating back) restores the last filter.
    const second = renderHook(() => useAgentFilters(deployments));
    expect(second.result.current.toolbarProps.statusFilter).toBe('error');
    expect(second.result.current.filtered).toEqual([gamma]);
  });

  it('does not restore a cleared status filter', () => {
    const first = renderHook(() => useAgentFilters(deployments));
    act(() => first.result.current.toolbarProps.onStatusChange('error'));
    act(() => first.result.current.toolbarProps.onStatusChange(null));
    first.unmount();

    const second = renderHook(() => useAgentFilters(deployments));
    expect(second.result.current.toolbarProps.statusFilter).toBeNull();
  });
});
