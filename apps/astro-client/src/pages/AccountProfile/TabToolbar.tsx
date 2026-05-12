import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Search, X, ChevronDown, Check } from "lucide-react";
import { cn } from "@/lib/utils";

// ── TabButton ─────────────────────────────────────────────────────────────────

export function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "inline-flex items-center pb-3 text-body border-b-2 -mb-px transition-colors cursor-pointer",
        active
          ? "border-primary text-foreground font-semibold"
          : "border-transparent text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}

// ── TabSearchInput ────────────────────────────────────────────────────────────

interface TabSearchInputProps {
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  disabled?: boolean;
}

export function TabSearchInput({ value, onChange, placeholder, disabled }: TabSearchInputProps) {
  return (
    <div className="relative flex-1 min-w-48 max-w-72">
      <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground pointer-events-none" />
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        disabled={disabled}
        className="pl-8 h-8 text-body-sm"
      />
      {value && !disabled && (
        <button
          type="button"
          onClick={() => onChange("")}
          className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
        >
          <X className="size-3.5" />
        </button>
      )}
    </div>
  );
}

// ── TabFilterDropdown ─────────────────────────────────────────────────────────

interface TabFilterDropdownOption<T extends string> {
  value: T;
  label: string;
}

interface TabFilterDropdownProps<T extends string> {
  value: T;
  onChange: (v: T) => void;
  options: TabFilterDropdownOption<T>[];
  triggerLabel: string;
  minWidth?: string;
  disabled?: boolean;
}

export function TabFilterDropdown<T extends string>({
  value,
  onChange,
  options,
  triggerLabel,
  minWidth = "min-w-40",
  disabled,
}: TabFilterDropdownProps<T>) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild disabled={disabled}>
        <Button variant="outline" size="sm" disabled={disabled}>
          {triggerLabel}
          <ChevronDown className="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className={minWidth}>
        {options.map((opt) => (
          <DropdownMenuItem key={opt.value} onSelect={() => onChange(opt.value)} className="gap-2 py-1 text-body-sm">
            {value === opt.value ? <Check className="size-3.5" /> : <span className="size-3.5" />}
            {opt.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
