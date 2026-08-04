import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { buildBlueprintPageItems, type BlueprintPageItem } from "@/lib/blueprint-page-numbers";
import { cn } from "@/lib/utils";

export function ListPagination({
  currentPage,
  totalPages,
  onPageChange,
  disabled = false,
  ariaLabel,
}: {
  currentPage: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  disabled?: boolean;
  ariaLabel: string;
}) {
  const showControls = totalPages > 1;
  const items = showControls ? buildBlueprintPageItems(currentPage, totalPages) : [];

  return (
    <nav
      className={cn(
        "mt-6 flex h-8 items-center justify-center gap-1",
        !showControls && "invisible pointer-events-none",
        disabled && "pointer-events-none",
      )}
      aria-label={ariaLabel}
      aria-hidden={!showControls}
      aria-busy={disabled}
    >
      <Button
        type="button"
        variant="outline"
        size="icon-sm"
        disabled={disabled || currentPage <= 1}
        aria-label="Previous page"
        onClick={() => onPageChange(currentPage - 1)}
      >
        <ChevronLeft className="size-4" />
      </Button>

      {items.map((item, index) => (
        <PageControl
          key={pageItemKey(item, index)}
          item={item}
          currentPage={currentPage}
          disabled={disabled}
          onPageChange={onPageChange}
        />
      ))}

      <Button
        type="button"
        variant="outline"
        size="icon-sm"
        disabled={disabled || currentPage >= totalPages}
        aria-label="Next page"
        onClick={() => onPageChange(currentPage + 1)}
      >
        <ChevronRight className="size-4" />
      </Button>
    </nav>
  );
}

function PageControl({
  item,
  currentPage,
  disabled,
  onPageChange,
}: {
  item: BlueprintPageItem;
  currentPage: number;
  disabled: boolean;
  onPageChange: (page: number) => void;
}) {
  if (item === "ellipsis") {
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
      variant={isActive ? "default" : "outline"}
      size="sm"
      disabled={disabled}
      aria-label={`Page ${item}`}
      aria-current={isActive ? "page" : undefined}
      className={cn("min-w-8 px-2", isActive && "pointer-events-none")}
      onClick={() => onPageChange(item)}
    >
      {item}
    </Button>
  );
}

function pageItemKey(item: BlueprintPageItem, index: number): string {
  return item === "ellipsis" ? `ellipsis-${index}` : `page-${item}`;
}
