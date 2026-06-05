import { Users, Bot } from "lucide-react";
import { PillToggle, type PillOption } from "./PillToggle";

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

export function ViewToggle({ value, onChange, usersCount, agentsCount, className }: ViewToggleProps) {
  const options: PillOption<ActivityView>[] = [
    { key: "users", label: "People", icon: <Users className="size-3.5" aria-hidden />, count: usersCount },
    { key: "agents", label: "Agents", icon: <Bot className="size-3.5" aria-hidden />, count: agentsCount },
  ];
  return (
    <PillToggle
      value={value}
      options={options}
      onChange={onChange}
      layoutId="insights-view-pill"
      size="md"
      className={className}
    />
  );
}

export function parseActivityView(raw: string | null): ActivityView {
  return raw === "users" ? "users" : "agents";
}
