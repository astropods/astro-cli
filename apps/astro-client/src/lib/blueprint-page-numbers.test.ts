import { describe, expect, it } from 'vitest';
import { blueprintGridSlotCount } from './blueprint-page-numbers';

describe('blueprintGridSlotCount', () => {
  const base = {
    pageSizeReady: true,
    showFilteredEmpty: false,
    pageSize: 10,
  };

  it('returns undefined when all items fit on one page', () => {
    expect(blueprintGridSlotCount({ ...base, totalCount: 5 })).toBeUndefined();
  });

  it('returns page size when the list spans multiple pages', () => {
    expect(blueprintGridSlotCount({ ...base, totalCount: 25 })).toBe(10);
  });
});
