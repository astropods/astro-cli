import { Users, Bot } from "lucide-react";
import { cn } from "@/lib/utils";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

export type ActivityView = "agents" | "users";

interface ViewToggleProps {
  value: ActivityView;
  onChange: (v: ActivityView) => void;
  /** Total user count rendered next to the People label. */
  usersCount?: number;
  /** Total agent count rendered next to the Agents label. */
  agentsCount?: number;
  className?: string;
}

// Uses the shared `variant="word"` chrome (sliding indicator) but with a
// hollowed-out container — transparent background so the toggle blends
// into the table header bar instead of standing out as a filled chip.
// The sliding active indicator still renders, giving the active label a
// subtle outline-pill highlight.
export function ViewToggle({ value, onChange, usersCount, agentsCount, className }: ViewToggleProps) {
  return (
    <ToggleGroup
      type="single"
      variant="word"
      value={value}
      onValueChange={(v) => { if (v === "agents" || v === "users") onChange(v); }}
      className={cn("bg-transparent", className)}
    >
      <ToggleGroupItem value="users" aria-label="People" className="gap-2 text-body-sm">
        <Users className="size-3.5" aria-hidden />
        People
        {usersCount !== undefined && (
          <span className="text-faint-foreground tabular-nums">{usersCount}</span>
        )}
      </ToggleGroupItem>
      <ToggleGroupItem value="agents" aria-label="Agents" className="gap-2 text-body-sm">
        <Bot className="size-3.5" aria-hidden />
        Agents
        {agentsCount !== undefined && (
          <span className="text-faint-foreground tabular-nums">{agentsCount}</span>
        )}
      </ToggleGroupItem>
    </ToggleGroup>
  );
}

export function parseActivityView(raw: string | null): ActivityView {
  return raw === "users" ? "users" : "agents";
}
