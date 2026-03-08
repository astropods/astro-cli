import { SidebarNav, SidebarNavItem } from "@/components/ui/sidebar-layout";

export interface CategorySidebarProps {
  categories: string[];
  selected: string;
  onSelect: (category: string) => void;
  className?: string;
}

export function CategorySidebar({
  categories,
  selected,
  onSelect,
  className,
}: CategorySidebarProps) {
  return (
    <SidebarNav label="Category" className={className}>
      {categories.map((category) => (
        <SidebarNavItem
          key={category}
          active={selected === category}
          onClick={() => onSelect(category)}
        >
          {category}
        </SidebarNavItem>
      ))}
    </SidebarNav>
  );
}
