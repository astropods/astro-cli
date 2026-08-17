import { Users, Bot, Boxes } from "lucide-react";
import { PillToggle, type PillOption } from "./PillToggle";

export type ActivityView = "agents" | "users" | "models";

interface ViewToggleProps {
  value: ActivityView;
  onChange: (v: ActivityView) => void;
  /** Total user count rendered next to the Users label. */
  usersCount?: number;
  /** Total agent count rendered next to the Agents label. */
  agentsCount?: number;
  /** Total model count rendered next to the Models label. */
  modelsCount?: number;
  className?: string;
}

export function ViewToggle({ value, onChange, usersCount, agentsCount, modelsCount, className }: ViewToggleProps) {
  const options: PillOption<ActivityView>[] = [
    { key: "users", label: "Users", icon: <Users className="size-3.5" aria-hidden />, count: usersCount },
    { key: "agents", label: "Agents", icon: <Bot className="size-3.5" aria-hidden />, count: agentsCount },
    { key: "models", label: "Models", icon: <Boxes className="size-3.5" aria-hidden />, count: modelsCount },
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
  if (raw === "users") return "users";
  if (raw === "models") return "models";
  return "agents";
}
