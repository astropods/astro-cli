import { describe, it, expect } from 'vitest';
import {
  buildBlueprintListQuery,
  blueprintListParamsKey,
  hasBlueprintListFilters,
} from './blueprint-list-params';

describe('buildBlueprintListQuery', () => {
  it('encodes q param', () => {
    expect(buildBlueprintListQuery({ q: 'foo' })).toBe('q=foo');
  });

  it('omits empty filters', () => {
    expect(buildBlueprintListQuery({})).toBe('');
    expect(buildBlueprintListQuery({ q: '  ' })).toBe('');
  });

  it('encodes filters and pagination', () => {
    const qs = buildBlueprintListQuery({
      q: 'bot',
      tag: 'Data',
      visibility: 'public',
      sort: 'newest',
      limit: 50,
      offset: 10,
    });
    const params = new URLSearchParams(qs);
    expect(params.get('q')).toBe('bot');
    expect(params.get('tag')).toBe('Data');
    expect(params.get('visibility')).toBe('public');
    expect(params.get('sort')).toBe('newest');
    expect(params.get('limit')).toBe('50');
    expect(params.get('offset')).toBe('10');
  });
});

describe('blueprintListParamsKey', () => {
  it('trims and drops empty values', () => {
    expect(blueprintListParamsKey({ q: '  x  ', tag: '' })).toEqual({ q: 'x' });
  });
});

describe('hasBlueprintListFilters', () => {
  it('ignores pagination-only params', () => {
    expect(hasBlueprintListFilters({ limit: 50, offset: 0 })).toBe(false);
  });

  it('detects search', () => {
    expect(hasBlueprintListFilters({ q: 'a' })).toBe(true);
  });
});
