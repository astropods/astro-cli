/** Page items for a compact pagination control (1 … 4 5 6 … 12). */
export type BlueprintPageItem = number | 'ellipsis';

export function totalBlueprintPages(totalCount: number, pageSize: number): number {
  if (totalCount <= 0 || pageSize <= 0) return 0;
  return Math.ceil(totalCount / pageSize);
}

/** Pad the grid to pageSize when paginated so page controls stay put; skip when everything fits one page. */
export function blueprintGridSlotCount(opts: {
  showFilteredEmpty: boolean;
  totalCount: number;
  pageSize: number;
}): number | undefined {
  if (opts.showFilteredEmpty) {
    return undefined;
  }
  if (totalBlueprintPages(opts.totalCount, opts.pageSize) <= 1) {
    return undefined;
  }
  return opts.pageSize;
}

export function buildBlueprintPageItems(currentPage: number, totalPages: number): BlueprintPageItem[] {
  if (totalPages <= 0) return [];
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, i) => i + 1);
  }

  const items: BlueprintPageItem[] = [];
  const push = (item: BlueprintPageItem) => {
    if (items.length > 0 && items[items.length - 1] === item) return;
    items.push(item);
  };

  push(1);

  const windowStart = Math.max(2, currentPage - 1);
  const windowEnd = Math.min(totalPages - 1, currentPage + 1);

  if (windowStart > 2) {
    push('ellipsis');
  }

  for (let page = windowStart; page <= windowEnd; page += 1) {
    push(page);
  }

  if (windowEnd < totalPages - 1) {
    push('ellipsis');
  }

  push(totalPages);
  return items;
}
