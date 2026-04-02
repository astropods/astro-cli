import { describe, it, expect } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useAgentFilters } from './useAgentFilters';
import type { AgentDeployment } from '@/lib/api';

function makeDeployment(overrides: Partial<AgentDeployment> & { id: string; name: string }): AgentDeployment {
  return {
    build_id: 'b1',
    namespace: 'ns-1',
    status: 'Running',
    replicas: 1,
    ready: 1,
    created_at: '2025-01-01T00:00:00Z',
    components: [],
    ...overrides,
  };
}

const alpha = makeDeployment({ id: 'a', name: 'alpha-agent', display_name: 'Alpha Bot', created_at: '2025-01-03T00:00:00Z' });
const beta = makeDeployment({ id: 'b', name: 'beta-agent', display_name: 'Beta Bot', created_at: '2025-01-02T00:00:00Z' });
const gamma = makeDeployment({ id: 'c', name: 'gamma-agent', display_name: 'Error Gamma', status: 'Error', ready: 0, created_at: '2025-01-01T00:00:00Z' });

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

  it('filters by status', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onStatusFilterChange(['error']));
    expect(result.current.filtered).toEqual([gamma]);
  });

  it('returns only active deployments when status filter is active', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onStatusFilterChange(['active']));
    expect(result.current.filtered).toHaveLength(2);
    expect(result.current.filtered).not.toContain(gamma);
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

  it('applies text filter and status filter together', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => {
      result.current.toolbarProps.onFilterChange('bot');
      result.current.toolbarProps.onStatusFilterChange(['active']);
    });
    expect(result.current.filtered).toHaveLength(2);
    expect(result.current.filtered).not.toContain(gamma);
  });

  it('clears filter on empty string', () => {
    const { result } = renderHook(() => useAgentFilters(deployments));
    act(() => result.current.toolbarProps.onFilterChange('alpha'));
    expect(result.current.filtered).toHaveLength(1);
    act(() => result.current.toolbarProps.onFilterChange(''));
    expect(result.current.filtered).toHaveLength(3);
  });
});
