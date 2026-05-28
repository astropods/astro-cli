import { cn } from "@/lib/utils";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

export type ActivityView = "agents" | "users";

interface ViewToggleProps {
  value: ActivityView;
  onChange: (v: ActivityView) => void;
  className?: string;
}

export function ViewToggle({ value, onChange, className }: ViewToggleProps) {
  return (
    <ToggleGroup
      type="single"
      variant="word"
      value={value}
      onValueChange={(v) => { if (v === "agents" || v === "users") onChange(v); }}
      // Override the ToggleGroup root chrome: same border + rounded-sm
      // shape as the filter bar. Background is `bg-card` rather than the
      // primitive's translucent default so the PageStarField behind the
      // Insights page doesn't bleed through the inactive item.
      className={cn(
        "h-7 rounded-sm border-input bg-card",
        className,
      )}
    >
      <ToggleGroupItem value="agents" aria-label="By agent" className="py-1 font-mono text-body-sm">
        By Agent
      </ToggleGroupItem>
      <ToggleGroupItem value="users" aria-label="By user" className="py-1 font-mono text-body-sm">
        By User
      </ToggleGroupItem>
    </ToggleGroup>
  );
}

export function parseActivityView(raw: string | null): ActivityView {
  return raw === "users" ? "users" : "agents";
}
