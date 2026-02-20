import { cn } from "@/lib/utils";

export interface CategorySidebarProps {
  categories: string[];
  selected: string;
  onSelect: (category: string) => void;
}

export function CategorySidebar({
  categories,
  selected,
  onSelect,
}: CategorySidebarProps) {
  return (
    <nav className="flex w-full gap-1 overflow-x-auto md:w-36 md:shrink-0 md:flex-col md:overflow-x-visible">
      {categories.map((category) => (
        <button
          key={category}
          type="button"
          onClick={() => onSelect(category)}
          className={cn(
            "whitespace-nowrap rounded-md px-3 py-1.5 text-left text-sm font-medium transition-colors cursor-pointer",
            selected === category
              ? "bg-muted text-foreground"
              : "text-muted-foreground hover:bg-muted/50 hover:text-foreground",
          )}
        >
          {category}
        </button>
      ))}
    </nav>
  );
}
