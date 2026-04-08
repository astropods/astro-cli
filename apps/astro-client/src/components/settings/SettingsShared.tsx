import { CheckIcon } from "@heroicons/react/24/outline";

const headingClass = {
  h1: "text-heading-1",
  h2: "text-heading-2",
} as const;

export function SectionHeader({
  as: Heading = "h2",
  title,
  subtitle,
}: {
  as?: "h1" | "h2";
  title: string;
  subtitle: string;
}) {
  return (
    <div className="space-y-1">
      <Heading className={`${headingClass[Heading]} text-foreground`}>{title}</Heading>
      <p className="text-[13px] text-muted-foreground">{subtitle}</p>
    </div>
  );
}

export function SavedIndicator({ visible }: { visible: boolean }) {
  if (!visible) return null;
  return (
    <span className="flex items-center gap-1 text-[13px] text-muted-foreground">
      <CheckIcon className="size-3.5" />
      Saved
    </span>
  );
}
