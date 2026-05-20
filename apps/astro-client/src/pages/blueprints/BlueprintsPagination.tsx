import { ChevronLeft, ChevronRight } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import {
  buildBlueprintPageItems,
  totalBlueprintPages,
  type BlueprintPageItem,
} from '@/lib/blueprint-page-numbers';

export function BlueprintsPagination({
  currentPage,
  totalCount,
  pageSize,
  onPageChange,
  disabled,
}: {
  currentPage: number;
  totalCount: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  disabled?: boolean;
}) {
  const totalPages = totalBlueprintPages(totalCount, pageSize);
  const showControls = totalPages > 1;
  const items = showControls ? buildBlueprintPageItems(currentPage, totalPages) : [];

  return (
    <nav
      className={cn(
        'mt-6 flex h-8 items-center justify-center gap-1',
        !showControls && 'invisible pointer-events-none',
        disabled && 'pointer-events-none',
      )}
      aria-label="Blueprint list pagination"
      aria-hidden={!showControls}
      aria-busy={disabled}
    >
      <Button
        type="button"
        variant="outline"
        size="icon-sm"
        disabled={currentPage <= 1}
        aria-label="Previous page"
        onClick={() => onPageChange(currentPage - 1)}
      >
        <ChevronLeft className="size-4" />
      </Button>

      {items.map((item, index) => (
        <BlueprintPageControl
          key={pageItemKey(item, index)}
          item={item}
          currentPage={currentPage}
          onPageChange={onPageChange}
        />
      ))}

      <Button
        type="button"
        variant="outline"
        size="icon-sm"
        disabled={currentPage >= totalPages}
        aria-label="Next page"
        onClick={() => onPageChange(currentPage + 1)}
      >
        <ChevronRight className="size-4" />
      </Button>
    </nav>
  );
}

function BlueprintPageControl({
  item,
  currentPage,
  onPageChange,
}: {
  item: BlueprintPageItem;
  currentPage: number;
  onPageChange: (page: number) => void;
}) {
  if (item === 'ellipsis') {
    return (
      <span className="px-1 text-body-sm text-muted-foreground select-none" aria-hidden>
        …
      </span>
    );
  }

  const isActive = item === currentPage;

  return (
    <Button
      type="button"
      variant={isActive ? 'default' : 'outline'}
      size="sm"
      aria-label={`Page ${item}`}
      aria-current={isActive ? 'page' : undefined}
      className={cn('min-w-8 px-2', isActive && 'pointer-events-none')}
      onClick={() => onPageChange(item)}
    >
      {item}
    </Button>
  );
}

function pageItemKey(item: BlueprintPageItem, index: number): string {
  if (item === 'ellipsis') return `ellipsis-${index}`;
  return `page-${item}`;
}
