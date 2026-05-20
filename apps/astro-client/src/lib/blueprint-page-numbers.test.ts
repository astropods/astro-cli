import { describe, it, expect } from 'vitest';
import { buildBlueprintPageItems, totalBlueprintPages } from '@/lib/blueprint-page-numbers';

describe('totalBlueprintPages', () => {
  it('returns 0 when there are no items', () => {
    expect(totalBlueprintPages(0, 10)).toBe(0);
  });

  it('ceil-divides total count by page size', () => {
    expect(totalBlueprintPages(25, 10)).toBe(3);
    expect(totalBlueprintPages(20, 10)).toBe(2);
  });
});

describe('buildBlueprintPageItems', () => {
  it('returns all pages when there are seven or fewer', () => {
    expect(buildBlueprintPageItems(1, 5)).toEqual([1, 2, 3, 4, 5]);
  });

  it('collapses distant pages with ellipses', () => {
    expect(buildBlueprintPageItems(5, 12)).toEqual([1, 'ellipsis', 4, 5, 6, 'ellipsis', 12]);
  });
});
